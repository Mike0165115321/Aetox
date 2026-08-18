package main

// The Windows half of the native browser: every tab is a WebView2 embedded in
// a Win32 child window over the dock's browser pane.
//
// Threading model: WebView2 is COM/STA — every webview lives on ONE dedicated
// OS thread that runs a Windows message pump. All operations are marshalled
// onto that thread via a command queue + PostThreadMessage(WM_APP) wake-up.
// This is the one platform that gets a thread of its own; GTK and Cocoa
// require their webviews on the app's main thread instead, which is why
// hostBackend.do is specified as asynchronous rather than "posts to the
// browser thread".

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/wailsapp/go-webview2/pkg/edge"
)

// bridgePost is how a page hands a message back to Go on this engine. See
// metaScript in browser.go.
const bridgePost = "window.chrome.webview.postMessage"

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostThreadMsgW   = user32.NewProc("PostThreadMessageW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")

	procGetWindowDpiAwarenessCtx = user32.NewProc("GetWindowDpiAwarenessContext")
	procSetThreadDpiAwarenessCtx = user32.NewProc("SetThreadDpiAwarenessContext")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")

	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")

	ole32              = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx = ole32.NewProc("CoInitializeEx")
)

const (
	wmApp      = 0x8000
	wsChild    = 0x40000000
	wsVisible  = 0x10000000
	wsClipSibl = 0x04000000
	wsPopup    = 0x80000000
	swHide     = 0

	// A window that is visible to the compositor and to nobody else: no taskbar
	// button, never activated, never stealing focus. Paired with a position
	// outside every monitor, this is the deck export's window (deck_render.go).
	wsExToolWindow = 0x00000080
	wsExNoActivate = 0x08000000

	coinitApartmentThreaded = 0x2

	// hwndTop + these SWP flags force the tab's WebView2 child window to the
	// top of the Z order: two separate WebView2 controllers in the same
	// top-level window each composite independently, so plain ShowWindow/
	// MoveWindow (no Z-order change) can leave the tab rendered behind the
	// app's own webview — invisible, even though it's really navigated and
	// painting.
	hwndTop = 0
	// The other end of the same fact: a tab sent to the BOTTOM of the Z order
	// is still visible to Windows and still painting, and simply has the app's
	// own webview drawn over it. That is what an export needs — composited, so
	// a capture has frames to capture, and unseen, so nothing flashes. See
	// sendBehind.
	hwndBottom    = 1
	swpNoMove     = 0x0002
	swpNoSize     = 0x0001
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040
)

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

// win32Tab is one WebView2 in its own child window.
type win32Tab struct {
	hwnd     uintptr
	chromium *edge.Chromium
	// hostThread and reportErr are the tripwire, not the plumbing: see
	// requireHostThread.
	hostThread uint32
	reportErr  func(error)
}

// requireHostThread is CEF's CEF_REQUIRE_UI_THREAD, which is DCHECK(CefCurrentlyOn(TID_UI))
// at the top of every function that has a thread it must be on. Same idea, one
// difference that matters here.
//
// browserHost.onTab already makes the accident impossible at compile time —
// there is no view to reach without going through it. What a compiler cannot
// see is a tabView copied out of an onTab callback and used later, and nothing
// in Go will ever see that. CEF's answer covers exactly that gap, so the two
// together cover more than either alone: a wall for the reach, a tripwire for
// the stash.
//
// It does not panic. A crash in a shipping desktop app over a browser tab is
// not a trade worth making, and a log line is what §127.8 already proved nobody
// reads. So it reports through onEngineError — the channel built for precisely
// this in §128.4 — which means a wrong-thread call now names itself, in our own
// words, in the tool result, before WebView2's more cryptic refusal arrives.
//
// The call is then made anyway. The engine will refuse it; the point is that by
// then somebody has already been told why.
func (t *win32Tab) requireHostThread(what string) {
	if t.hostThread == 0 {
		return // a tab built by a test, with no host thread to be off
	}
	cur, _, _ := procGetCurrentThreadID.Call()
	if uint32(cur) == t.hostThread {
		return
	}
	err := fmt.Errorf("browser.%s was called from thread %d, not the webview's thread %d — WebView2 will refuse it", what, cur, t.hostThread)
	debuglog.Msg("%v", err)
	if t.reportErr != nil {
		t.reportErr(err)
	}
}

func (t *win32Tab) navigate(url string) {
	t.requireHostThread("navigate")
	t.chromium.Navigate(url)
}

func (t *win32Tab) eval(js string) {
	t.requireHostThread("eval")
	t.chromium.Eval(js)
}

func (t *win32Tab) setZoom(f float64) {
	t.requireHostThread("setZoom")
	t.chromium.PutZoomFactor(f)
}

func (t *win32Tab) openDevTools() {
	t.requireHostThread("openDevTools")
	t.chromium.OpenDevToolsWindow()
}

// capture adapts the vendored patch's result type to the portable one, so
// browser.go's tabView never names an engine. See third_party/go-webview2's
// AETOX-PATCH.md.
func (t *win32Tab) capture() <-chan shotResult {
	out := make(chan shotResult, 1)
	t.requireHostThread("capture")
	src := t.chromium.CapturePreview()
	go func() {
		r := <-src
		out <- shotResult{PNG: r.PNG, Err: r.Err}
	}()
	return out
}

// callEngine runs a Chrome DevTools Protocol method on this tab.
//
// The DevTools door rather than ICoreWebView2_7::PrintToPdf and friends: that
// route is four COM interfaces for one call (the webview's, the environment's,
// the settings', and a handler), where this is one method and one handler —
// both already patched into the vendored fork, and reusable for every later CDP
// question. See third_party/go-webview2/AETOX-PATCH.md.
//
// Nothing is decoded here. What "data" means differs per method, and the layer
// that asked is the layer that knows.
func (t *win32Tab) callEngine(method, paramsJSON string) <-chan engineReply {
	out := make(chan engineReply, 1)
	t.requireHostThread("callEngine")
	src := t.chromium.CallDevToolsProtocolMethod(method, paramsJSON)
	go func() {
		r := <-src
		out <- engineReply{JSON: r.JSON, Err: r.Err}
	}()
	return out
}

// sendBehind is what the deck export used to ask for and no longer can.
//
// It set the tab to the bottom of the Z order on the reasoning that a tab left
// under the app's own webview is "invisible, even though it's really navigated
// and painting" — a sentence this file records from experience. The reasoning
// was right about siblings and wrong about this: the app's webview is the
// PARENT's content, and a child window paints over its parent's client area
// whatever the Z order says. The export tab sat on top of the whole application
// for the length of every export.
//
// The export now gets its own top-level window instead (see openTab), so this
// has no caller. It stays because tabView declares it and because a future
// engine may have a Z order that does mean something here.
func (t *win32Tab) sendBehind() {
	t.requireHostThread("sendBehind")
	procSetWindowPos.Call(t.hwnd, hwndBottom, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

func (t *win32Tab) setBounds(x, y, w, h int) {
	t.requireHostThread("setBounds")
	procSetWindowPos.Call(t.hwnd, hwndTop, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpShowWindow|swpNoActivate)
	t.chromium.Resize()
}

func (t *win32Tab) setVisible(visible bool) {
	t.requireHostThread("setVisible")
	if visible {
		procSetWindowPos.Call(t.hwnd, hwndTop, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow|swpNoActivate)
		return
	}
	procShowWindow.Call(t.hwnd, uintptr(swHide))
}

// ponytail: DestroyWindow only — the WebView2 controller isn't explicitly
// Closed (the wrapper doesn't expose it); its process is reclaimed when the
// app exits. The WebKit hosts can do better, since both toolkits expose a real
// teardown.
func (t *win32Tab) destroy() { procDestroyWindow.Call(t.hwnd) }

type win32Host struct {
	mu       sync.Mutex
	cmds     []func()
	threadID uint32
	parent   uintptr
	ready    chan struct{}
	started  bool
	class    *uint16
}

func newHostBackend() hostBackend { return &win32Host{ready: make(chan struct{})} }

// start spins up the dedicated STA browser thread (idempotent).
func (h *win32Host) start() error {
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		<-h.ready
		return nil
	}
	h.started = true
	h.mu.Unlock()

	parent := findOwnMainWindow()
	if parent == 0 {
		debuglog.Msg("browser.start: main window not found")
		// Release the claim. Without this every later call parks forever on
		// h.ready, which is never closed on this path — and browserHostLazy
		// calls start() on every single binding, so one early failure would
		// hang the whole browser surface rather than failing it.
		h.mu.Lock()
		h.started = false
		h.mu.Unlock()
		return fmt.Errorf("main window not found")
	}
	debuglog.Msg("browser.start: parent hwnd=%#x (pid=%d)", parent, os.Getpid())
	h.parent = parent

	go h.run()
	<-h.ready
	debuglog.Msg("browser.start: host thread ready (tid=%d)", h.threadID)
	return nil
}

// findOwnMainWindow returns this process's visible top-level window (the wails
// main window). Never look it up by TITLE: FindWindowW("Aetox Desktop") matches
// any window that happens to carry that text — a browser tab showing the dev
// URL, explorer's taskbar thumbnail host, another instance — and a parent from
// a foreign process makes every CreateWindowExW child fail with "Access is
// denied", silently killing all browser tabs.
//
// ponytail: enumerating at all is what the port blueprint's rule 1 says not to
// do ("hold a direct handle from the toolkit"). Wails v2.13 exports no such
// handle, so retiring this needs a patch to the vendored Wails — planned with
// phase 3a, where GTK forces the question anyway. ARCHITECTURE.md §48.
func findOwnMainWindow() uintptr {
	self := uint32(os.Getpid())
	var found uintptr
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != self {
			return 1 // keep enumerating
		}
		if vis, _, _ := procIsWindowVisible.Call(hwnd); vis == 0 {
			return 1
		}
		found = hwnd
		return 0 // stop
	})
	procEnumWindows.Call(cb, 0)
	return found
}

func (h *win32Host) run() {
	runtime.LockOSThread()
	procCoInitializeEx.Call(0, coinitApartmentThreaded)

	// Match the main window's DPI awareness context. Windows refuses to
	// create a child window whose thread runs under a different DPI context
	// than the parent — CreateWindowExW fails with ERROR_ACCESS_DENIED. A raw
	// goroutine thread starts on the process default, which does not
	// necessarily match the wails main window's per-monitor context.
	if ctx, _, _ := procGetWindowDpiAwarenessCtx.Call(h.parent); ctx != 0 {
		prev, _, _ := procSetThreadDpiAwarenessCtx.Call(ctx)
		debuglog.Msg("browser.run: thread DPI ctx set to parent's (prev=%#x)", prev)
	}

	tid, _, _ := procGetCurrentThreadID.Call()
	h.threadID = uint32(tid)

	// Child window class; all messages go to DefWindowProc — sizing is driven
	// explicitly from BrowserSetBounds.
	wndProc := syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
		r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
		return r
	})
	className, _ := syscall.UTF16PtrFromString("AetoxBrowserHost")
	h.class = className
	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		WndProc:   wndProc,
		ClassName: className,
	}
	atom, _, regErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	debuglog.Msg("browser.run: RegisterClassExW atom=%d err=%v", atom, regErr)

	close(h.ready)

	var msg winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 {
			return
		}
		h.drain()
		if msg.Message != wmApp {
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

// do queues fn onto the browser thread and wakes its pump. Asynchronous, as
// hostBackend.do requires.
func (h *win32Host) do(fn func()) {
	h.mu.Lock()
	h.cmds = append(h.cmds, fn)
	h.mu.Unlock()
	procPostThreadMsgW.Call(uintptr(h.threadID), wmApp, 0, 0)
}

func (h *win32Host) drain() {
	for {
		h.mu.Lock()
		if len(h.cmds) == 0 {
			h.mu.Unlock()
			return
		}
		fn := h.cmds[0]
		h.cmds = h.cmds[1:]
		h.mu.Unlock()
		fn()
	}
}

// openTab creates the child window + WebView2 for a tab. Already on the
// browser thread — browserHost.open calls this from inside do.
// offscreenTabID names the one tab that gets a window of its own rather than a
// child of the app's. Kept here rather than passed in because the reason is a
// Win32 fact, not a caller's preference — see below.
const offscreenTabID = exportTabID

func (h *win32Host) openTab(id, url string, x, y, w, hgt int, cb tabCallbacks) tabView {
	// A child window ALWAYS paints over its parent's client area. Z-order only
	// orders siblings, and the app's own webview is not a sibling — it is the
	// parent's content. So there is no arrangement of a WS_CHILD window that is
	// composited and unseen, which is what the deck export needs: frames to
	// capture, and nothing on screen while it works. `sendBehind` was written
	// on the opposite assumption and could never have worked.
	//
	// Its own top-level window can be both. WS_POPUP at -32000 is visible to
	// the compositor — DWM keeps a redirection surface for it, so it paints —
	// and outside every monitor, so nobody sees it. WS_EX_TOOLWINDOW keeps it
	// off the taskbar and WS_EX_NOACTIVATE keeps it from stealing focus.
	style, exStyle, parent := uintptr(wsChild|wsVisible|wsClipSibl), uintptr(0), h.parent
	if id == offscreenTabID {
		style, exStyle, parent = wsPopup|wsVisible, wsExToolWindow|wsExNoActivate, 0
		x, y = -32000, -32000
	}
	hwnd, _, lastErr := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(h.class)),
		0,
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(hgt),
		parent, 0, 0, 0,
	)
	if hwnd == 0 {
		debuglog.Msg("browser.open(%s): CreateWindowExW FAILED: %v", id, lastErr)
		return nil
	}

	chromium := edge.NewChromium()
	chromium.DataPath = webviewUserDataDir("browser")
	if chromium.DataPath == "" {
		chromium.DataPath = filepath.Join(os.Getenv("AppData"), "aetox-browser")
	}
	chromium.SetErrorCallback(func(err error) {
		// default handler calls os.Exit(1) — never acceptable for a tab
		fmt.Fprintln(os.Stderr, "browser tab error:", err)
		debuglog.Msg("browser tab %s error: %v", id, err)
		// And out to the portable half, which is the part that was missing.
		// For a week this function was the entire fate of every engine
		// complaint: two lines nobody reads while the tool above answered
		// "page did not finish loading" and the agent guessed at the network.
		if cb.onEngineError != nil {
			cb.onEngineError(err)
		}
	})

	view := &win32Tab{hwnd: hwnd, chromium: chromium, hostThread: h.threadID, reportErr: cb.onEngineError}

	chromium.MessageCallback = func(message string, _ *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		source, _ := args.GetSource()
		cb.onMessage(message, source)
	}
	chromium.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		// An unreadable flag counts as success: the tab is usable either way,
		// and inventing a failure is worse than missing one.
		ok := true
		if args != nil {
			if success, err := args.GetIsSuccess(); err == nil {
				ok = success
			} else {
				debuglog.Msg("browser tab %s: navigation status unavailable: %v", id, err)
			}
		}
		cb.onNavDone(view, ok)
	}

	debuglog.Msg("browser.open(%s): embedding webview (dataPath=%s)", id, chromium.DataPath)
	if !chromium.Embed(hwnd) {
		debuglog.Msg("browser.open(%s): Embed FAILED", id)
		procDestroyWindow.Call(hwnd)
		return nil
	}
	debuglog.Msg("browser.open(%s): embed ok, navigating", id)
	chromium.Resize()
	view.setVisible(true)

	// Registered here and not a line earlier: Init reaches through to the
	// controller, which does not exist until Embed has returned true — Embed's
	// own last act is an Init call for exactly that reason. It still lands
	// before any page script, because it applies to documents created from now
	// on and the first Navigate is below. A page whose first statement is
	// confirm() would otherwise get the real blocking dialog and stop the tab
	// dead with nobody able to answer it. See dialogScript.
	chromium.Init(dialogScript())

	if url != "" {
		chromium.Navigate(url)
	}
	return view
}
