package main

// Native in-app browser: each browser tab is a real WebView2 (same engine the
// app itself runs in) embedded as a Win32 child window positioned over the
// dock's browser pane. This exists because iframes can't render sites that
// send X-Frame-Options/CSP deny (YouTube, Google, anything with bot checks),
// and because the AI needs to read real page content (BrowserGetText).
//
// Threading model: WebView2 is COM/STA — every webview lives on ONE dedicated
// OS thread that runs a Windows message pump. All operations are marshalled
// onto that thread via a command queue + PostThreadMessage(WM_APP) wake-up.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procMoveWindow       = user32.NewProc("MoveWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostThreadMsgW   = user32.NewProc("PostThreadMessageW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")

	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")

	ole32             = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx = ole32.NewProc("CoInitializeEx")
)

const (
	wmApp       = 0x8000
	wsChild     = 0x40000000
	wsVisible   = 0x10000000
	wsClipSibl  = 0x04000000
	swHide      = 0
	swShowNoAct = 8

	coinitApartmentThreaded = 0x2
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

// aetoxMsg is the JSON envelope pages post back to Go via
// window.chrome.webview.postMessage (see metaScript / textScript).
type aetoxMsg struct {
	Aetox string `json:"__aetox"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
	Text  string `json:"text,omitempty"`
}

const metaScript = `window.chrome.webview.postMessage(JSON.stringify({__aetox:"meta",title:document.title,url:location.href}))`
const textScript = `window.chrome.webview.postMessage(JSON.stringify({__aetox:"text",title:document.title,url:location.href,text:(document.body&&document.body.innerText||"").slice(0,200000)}))`

type browserTab struct {
	hwnd     uintptr
	chromium *edge.Chromium

	navDone chan struct{} // closed after the first completed navigation
	navOnce sync.Once

	metaMu sync.Mutex
	title  string
	url    string

	textMu sync.Mutex
	textCh chan string
}

func (t *browserTab) meta() (title, url string) {
	t.metaMu.Lock()
	defer t.metaMu.Unlock()
	return t.title, t.url
}

type browserHost struct {
	app *App

	mu       sync.Mutex
	cmds     []func()
	tabs     map[string]*browserTab
	lastID   string // most recently opened/shown tab — what browser_read targets
	threadID uint32
	parent   uintptr
	ready    chan struct{}
	started  bool
	class    *uint16
}

func newBrowserHost(app *App) *browserHost {
	return &browserHost{app: app, tabs: map[string]*browserTab{}, ready: make(chan struct{})}
}

// start spins up the dedicated STA browser thread (idempotent).
func (h *browserHost) start() error {
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		<-h.ready
		return nil
	}
	h.started = true
	h.mu.Unlock()

	title, _ := syscall.UTF16PtrFromString("Aetox Desktop")
	parent, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if parent == 0 {
		return fmt.Errorf("main window not found")
	}
	h.parent = parent

	go h.run()
	<-h.ready
	return nil
}

func (h *browserHost) run() {
	runtime.LockOSThread()
	procCoInitializeEx.Call(0, coinitApartmentThreaded)

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
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

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

// do queues fn onto the browser thread and wakes its pump.
func (h *browserHost) do(fn func()) {
	h.mu.Lock()
	h.cmds = append(h.cmds, fn)
	h.mu.Unlock()
	procPostThreadMsgW.Call(uintptr(h.threadID), wmApp, 0, 0)
}

func (h *browserHost) drain() {
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

func (h *browserHost) tab(id string) *browserTab {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tabs[id]
}

// open creates the child window + webview for a tab (on the browser thread).
func (h *browserHost) open(id, url string, x, y, w, hgt int) {
	h.do(func() {
		if _, exists := h.tabs[id]; exists {
			return
		}
		hwnd, _, _ := procCreateWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(h.class)),
			0,
			wsChild|wsVisible|wsClipSibl,
			uintptr(x), uintptr(y), uintptr(w), uintptr(hgt),
			h.parent, 0, 0, 0,
		)
		if hwnd == 0 {
			return
		}

		chromium := edge.NewChromium()
		chromium.DataPath = filepath.Join(os.Getenv("AppData"), "aetox-browser")
		chromium.SetErrorCallback(func(err error) {
			// default handler calls os.Exit(1) — never acceptable for a tab
			fmt.Fprintln(os.Stderr, "browser tab error:", err)
		})
		tab := &browserTab{hwnd: hwnd, chromium: chromium, navDone: make(chan struct{})}

		chromium.MessageCallback = func(message string, _ *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
			h.onMessage(id, tab, message)
		}
		chromium.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, _ *edge.ICoreWebView2NavigationCompletedEventArgs) {
			tab.navOnce.Do(func() { close(tab.navDone) })
			chromium.Eval(metaScript)
		}

		if !chromium.Embed(hwnd) {
			procDestroyWindow.Call(hwnd)
			return
		}
		chromium.Resize()

		h.mu.Lock()
		h.tabs[id] = tab
		h.lastID = id
		h.mu.Unlock()

		if url != "" {
			chromium.Navigate(url)
		}
	})
}

func (h *browserHost) onMessage(id string, tab *browserTab, raw string) {
	var m aetoxMsg
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m.Aetox == "" {
		return
	}
	switch m.Aetox {
	case "meta":
		tab.metaMu.Lock()
		tab.title, tab.url = m.Title, m.URL
		tab.metaMu.Unlock()
		if h.app.ctx != nil {
			wailsruntime.EventsEmit(h.app.ctx, "browser:meta:"+id, map[string]string{"title": m.Title, "url": m.URL})
		}
	case "text":
		tab.textMu.Lock()
		ch := tab.textCh
		tab.textCh = nil
		tab.textMu.Unlock()
		if ch != nil {
			ch <- m.Text
		}
	}
}

// ---------------------------------------------------------------------------
// Wails bindings
// ---------------------------------------------------------------------------

func (a *App) browserHostLazy() (*browserHost, error) {
	a.terminalsMu.Lock()
	if a.browsers == nil {
		a.browsers = newBrowserHost(a)
	}
	h := a.browsers
	a.terminalsMu.Unlock()
	return h, h.start()
}

// BrowserOpen creates a native browser tab at the given physical-pixel bounds.
func (a *App) BrowserOpen(id, url string, x, y, w, h int) error {
	host, err := a.browserHostLazy()
	if err != nil {
		return err
	}
	host.open(id, url, x, y, w, h)
	return nil
}

// BrowserNavigate loads a URL in an existing tab.
func (a *App) BrowserNavigate(id, url string) {
	if host, err := a.browserHostLazy(); err == nil {
		if t := host.tab(id); t != nil {
			host.do(func() { t.chromium.Navigate(url) })
		}
	}
}

// BrowserSetBounds moves/resizes a tab's window (physical pixels, relative to
// the main window client area).
func (a *App) BrowserSetBounds(id string, x, y, w, h int) {
	if host, err := a.browserHostLazy(); err == nil {
		if t := host.tab(id); t != nil {
			host.do(func() {
				procMoveWindow.Call(t.hwnd, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
				t.chromium.Resize()
			})
		}
	}
}

// BrowserSetVisible shows/hides a tab (hidden when its dock tab is inactive or
// the settings overlay is open — a native window always floats above the UI).
func (a *App) BrowserSetVisible(id string, visible bool) {
	if host, err := a.browserHostLazy(); err == nil {
		if t := host.tab(id); t != nil {
			if visible {
				host.mu.Lock()
				host.lastID = id
				host.mu.Unlock()
			}
			host.do(func() {
				cmd := uintptr(swHide)
				if visible {
					cmd = swShowNoAct
				}
				procShowWindow.Call(t.hwnd, cmd)
			})
		}
	}
}

// BrowserBack / BrowserForward / BrowserReload drive history via script — the
// vtable procs for GoBack/GoForward aren't exposed by the edge wrapper.
func (a *App) BrowserBack(id string)    { a.browserEval(id, "history.back()") }
func (a *App) BrowserForward(id string) { a.browserEval(id, "history.forward()") }
func (a *App) BrowserReload(id string)  { a.browserEval(id, "location.reload()") }

func (a *App) browserEval(id, js string) {
	if host, err := a.browserHostLazy(); err == nil {
		if t := host.tab(id); t != nil {
			host.do(func() { t.chromium.Eval(js) })
		}
	}
}

// BrowserClose destroys a tab's native window.
func (a *App) BrowserClose(id string) {
	if host, err := a.browserHostLazy(); err == nil {
		if t := host.tab(id); t != nil {
			host.mu.Lock()
			delete(host.tabs, id)
			host.mu.Unlock()
			// ponytail: DestroyWindow only — the WebView2 controller isn't
			// explicitly Closed (wrapper doesn't expose it); its process is
			// reclaimed when the app exits.
			host.do(func() { procDestroyWindow.Call(t.hwnd) })
		}
	}
}

// BrowserGetText returns the visible text content of a tab's current page —
// this is the read-path the AI agent uses to work with the browser.
func (a *App) BrowserGetText(id string) (string, error) {
	host, err := a.browserHostLazy()
	if err != nil {
		return "", err
	}
	t := host.tab(id)
	if t == nil {
		return "", fmt.Errorf("no browser tab %q", id)
	}

	ch := make(chan string, 1)
	t.textMu.Lock()
	t.textCh = ch
	t.textMu.Unlock()

	host.do(func() { t.chromium.Eval(textScript) })

	select {
	case text := <-ch:
		return text, nil
	case <-time.After(5 * time.Second):
		t.textMu.Lock()
		t.textCh = nil
		t.textMu.Unlock()
		return "", fmt.Errorf("page did not respond (still loading?)")
	}
}
