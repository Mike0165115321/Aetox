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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/wailsapp/go-webview2/pkg/edge"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procFindWindowW      = user32.NewProc("FindWindowW")
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

	procGetWindowDpiAwarenessCtx  = user32.NewProc("GetWindowDpiAwarenessContext")
	procSetThreadDpiAwarenessCtx  = user32.NewProc("SetThreadDpiAwarenessContext")
	procGetWindowThreadProcessID  = user32.NewProc("GetWindowThreadProcessId")
	procEnumWindows               = user32.NewProc("EnumWindows")
	procIsWindowVisible           = user32.NewProc("IsWindowVisible")

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
	swHide = 0

	coinitApartmentThreaded = 0x2

	// hwndTop + these SWP flags force the tab's WebView2 child window to the
	// top of the Z order: two separate WebView2 controllers in the same
	// top-level window each composite independently, so plain ShowWindow/
	// MoveWindow (no Z-order change) can leave the tab rendered behind the
	// app's own webview — invisible, even though it's really navigated and
	// painting.
	hwndTop        = 0
	swpNoMove      = 0x0002
	swpNoSize      = 0x0001
	swpNoActivate  = 0x0010
	swpShowWindow  = 0x0040
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
//
// SECURITY: any page loaded in the tab can call window.chrome.webview.
// postMessage itself, at any time, with an arbitrary __aetox envelope — this
// bridge is not exclusive to our own injected scripts. Two checks guard
// against that (see onMessage): the "meta" case cross-checks the claimed URL
// against args.GetSource() (the frame's real origin, reported by the WebView2
// runtime itself — a page cannot forge this), so a page can't make the
// address bar show a URL it isn't actually at (phishing-enabling spoof). The
// "text" case additionally requires a per-request Token minted by
// BrowserGetText, so a page can't preempt/replay a fake page-content response
// into the AI agent's read path. Neither check stops a page from lying within
// its own real DOM/title — that's inherent to any "agent reads a live page"
// feature and is a prompt-injection risk to be handled by treating fetched
// page text as untrusted data, not by this transport.
type aetoxMsg struct {
	Aetox    string           `json:"__aetox"`
	Title    string           `json:"title,omitempty"`
	URL      string           `json:"url,omitempty"`
	Text     string           `json:"text,omitempty"`
	Token    string           `json:"token,omitempty"`
	Elements []browserElement `json:"elements,omitempty"`
}

// browserElement is one clickable/typeable element found on the page, tagged
// with a data-aetox-ref attribute so a later browser_click/browser_type call
// can find the same node again by ref.
type browserElement struct {
	Ref  int    `json:"ref"`
	Tag  string `json:"tag"`
	Role string `json:"role,omitempty"`
	Text string `json:"text"`
}

// browserSnapshot is the result of one textScript round trip: page text plus
// the interactive elements found on it.
type browserSnapshot struct {
	Text     string
	Elements []browserElement
}

const metaScript = `window.chrome.webview.postMessage(JSON.stringify({__aetox:"meta",title:document.title,url:location.href}))`

// textScript reads page text and, in the same pass, tags every visible
// interactive element with a data-aetox-ref so browser_click/browser_type can
// target it later. Refs are reassigned fresh each call.
func textScript(token string) string {
	return fmt.Sprintf(`(function(){
  var out=[];
  var sel='a[href],button,input,select,textarea,[role="button"],[role="link"],[contenteditable="true"]';
  var els=document.querySelectorAll(sel);
  for(var i=0;i<els.length&&out.length<150;i++){
    var el=els[i];
    var r=el.getBoundingClientRect();
    if(r.width<=0||r.height<=0)continue;
    var ref=out.length+1;
    el.setAttribute('data-aetox-ref',String(ref));
    var txt=(el.innerText||el.value||el.getAttribute('aria-label')||el.getAttribute('placeholder')||'').trim().replace(/\s+/g,' ').slice(0,80);
    out.push({ref:ref,tag:el.tagName.toLowerCase(),role:el.getAttribute('role')||'',text:txt});
  }
  window.chrome.webview.postMessage(JSON.stringify({__aetox:"text",token:%q,title:document.title,url:location.href,text:(document.body&&document.body.innerText||"").slice(0,200000),elements:out}));
})()`, token)
}

// clickScript clicks the element tagged with the given ref (see textScript).
func clickScript(ref int) string {
	return fmt.Sprintf(`(function(){
  var el=document.querySelector('[data-aetox-ref="%d"]');
  if(!el)return;
  el.scrollIntoView({block:"center"});
  el.click();
})()`, ref)
}

// typeScript sets an input/textarea/contenteditable's value via the native
// setter (so React/Vue-controlled inputs pick it up) and fires input+change.
func typeScript(ref int, text string) string {
	encoded, _ := json.Marshal(text)
	return fmt.Sprintf(`(function(){
  var el=document.querySelector('[data-aetox-ref="%d"]');
  if(!el)return;
  el.focus();
  if(el.tagName==="INPUT"||el.tagName==="TEXTAREA"){
    var proto=el.tagName==="TEXTAREA"?window.HTMLTextAreaElement.prototype:window.HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto,"value").set.call(el,%s);
  } else {
    el.textContent=%s;
  }
  el.dispatchEvent(new Event("input",{bubbles:true}));
  el.dispatchEvent(new Event("change",{bubbles:true}));
})()`, ref, encoded, encoded)
}

// sameOrigin reports whether a and b share a scheme+host — used to check a
// page's claimed URL against its real origin as reported by WebView2.
func sameOrigin(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	if err1 != nil || err2 != nil || ua.Scheme == "" || ua.Scheme != ub.Scheme {
		return false
	}
	// file: URLs have no host, so the host check below would reject every
	// local page. The check's purpose is stopping a page from spoofing the
	// address bar as a trusted SITE — a file page claiming some other local
	// path can't do that, so scheme match is enough for file↔file.
	if ua.Scheme == "file" {
		return true
	}
	return ua.Host != "" && ua.Host == ub.Host
}

// newMessageToken mints a per-request nonce for BrowserGetText, so a stray or
// forged "text" message can't be mistaken for the response to a specific call.
func newMessageToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type browserTab struct {
	hwnd     uintptr
	chromium *edge.Chromium

	navDone chan struct{} // closed after the first completed navigation
	navOnce sync.Once

	metaMu sync.Mutex
	title  string
	url    string

	visMu  sync.Mutex
	hidden bool // BrowserSetVisible(false); nav-completed re-glue must not surface hidden tabs

	textMu    sync.Mutex
	textCh    chan browserSnapshot
	textToken string // token BrowserGetText is currently waiting on; empty = none pending
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

	parent := findOwnMainWindow()
	if parent == 0 {
		debuglog.Msg("browser.start: main window not found")
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

func (h *browserHost) run() {
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
	debuglog.Msg("browser.open(%s): queueing (url=%s)", id, url)
	h.do(func() {
		debuglog.Msg("browser.open(%s): running on browser thread", id)
		if _, exists := h.tabs[id]; exists {
			return
		}
		hwnd, _, lastErr := procCreateWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(h.class)),
			0,
			wsChild|wsVisible|wsClipSibl,
			uintptr(x), uintptr(y), uintptr(w), uintptr(hgt),
			h.parent, 0, 0, 0,
		)
		if hwnd == 0 {
			debuglog.Msg("browser.open(%s): CreateWindowExW FAILED: %v", id, lastErr)
			return
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
		})
		tab := &browserTab{hwnd: hwnd, chromium: chromium, navDone: make(chan struct{})}

		chromium.MessageCallback = func(message string, _ *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
			source, _ := args.GetSource()
			h.onMessage(id, tab, message, source)
		}
		chromium.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, _ *edge.ICoreWebView2NavigationCompletedEventArgs) {
			tab.navOnce.Do(func() { close(tab.navDone) })
			// Force this tab's window to the top of the Z order now that the
			// page has rendered. The frontend's browser:meta handler used to be
			// the only thing doing this, which made visibility depend on page
			// JS delivering a message that passes the origin check — never true
			// for file:// before the sameOrigin fix, and fragile in general:
			// the page stayed loaded but composited invisibly behind the app's
			// own webview. Runs on the browser STA thread (WebView2 callback).
			tab.visMu.Lock()
			hidden := tab.hidden
			tab.visMu.Unlock()
			if !hidden {
				procSetWindowPos.Call(hwnd, hwndTop, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow|swpNoActivate)
			}
			chromium.Eval(metaScript)
		}

		debuglog.Msg("browser.open(%s): embedding webview (dataPath=%s)", id, chromium.DataPath)
		if !chromium.Embed(hwnd) {
			debuglog.Msg("browser.open(%s): Embed FAILED", id)
			procDestroyWindow.Call(hwnd)
			return
		}
		debuglog.Msg("browser.open(%s): embed ok, navigating", id)
		chromium.Resize()
		procSetWindowPos.Call(hwnd, hwndTop, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow|swpNoActivate)

		h.mu.Lock()
		h.tabs[id] = tab
		h.lastID = id
		h.mu.Unlock()

		if url != "" {
			chromium.Navigate(url)
		}
	})
}

// onMessage handles one postMessage envelope from a tab's page. source is the
// sending frame's real origin per WebView2 (args.GetSource()) — trustworthy,
// unlike anything else in the message, which any page script can set freely.
func (h *browserHost) onMessage(id string, tab *browserTab, raw string, source string) {
	var m aetoxMsg
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m.Aetox == "" {
		return
	}
	switch m.Aetox {
	case "meta":
		// A page can claim any url it likes in the envelope; only trust it if
		// it matches where WebView2 says the message actually came from —
		// otherwise a page could make the address bar show a URL it isn't at.
		if !sameOrigin(source, m.URL) {
			return
		}
		tab.metaMu.Lock()
		tab.title, tab.url = m.Title, m.URL
		tab.metaMu.Unlock()
		if h.app.ctx != nil {
			wailsruntime.EventsEmit(h.app.ctx, "browser:meta:"+id, map[string]string{"title": m.Title, "url": m.URL})
		}
	case "text":
		tab.textMu.Lock()
		ch := tab.textCh
		expectedToken := tab.textToken
		tab.textCh = nil
		tab.textToken = ""
		tab.textMu.Unlock()
		// Reject if nothing is waiting, the token doesn't match this specific
		// BrowserGetText call (stops stale/forged messages from a page), or
		// the claimed url doesn't match the real sending origin.
		if ch == nil || m.Token == "" || m.Token != expectedToken || !sameOrigin(source, m.URL) {
			return
		}
		ch <- browserSnapshot{Text: m.Text, Elements: m.Elements}
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
				procSetWindowPos.Call(t.hwnd, hwndTop, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpShowWindow|swpNoActivate)
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
			t.visMu.Lock()
			t.hidden = !visible
			t.visMu.Unlock()
			host.do(func() {
				if visible {
					procSetWindowPos.Call(t.hwnd, hwndTop, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow|swpNoActivate)
				} else {
					procShowWindow.Call(t.hwnd, uintptr(swHide))
				}
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

// CloseAllBrowserTabs destroys every native browser window this process still
// holds. Called once by the frontend right after it (re)loads (App.svelte
// onMount) — a freshly loaded frontend owns zero workbench tabs by
// definition, so anything still open here is orphaned from a previous
// frontend lifetime: the Go backend is a long-lived process, but a `wails
// dev` Vite HMR full-reload (or any webview reload) wipes the JS-side
// `workbench` store without running BrowserPane's onDestroy, leaving the
// native WebView2 child window behind with nothing left to reposition or
// close it — it just floats, stuck at its last bounds. On a genuine fresh
// app start `a.browsers` is nil and this is a no-op.
func (a *App) CloseAllBrowserTabs() {
	if a.browsers == nil {
		return
	}
	h := a.browsers
	h.mu.Lock()
	ids := make([]string, 0, len(h.tabs))
	for id := range h.tabs {
		ids = append(ids, id)
	}
	h.mu.Unlock()
	for _, id := range ids {
		a.BrowserClose(id)
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
	snap, err := a.browserSnapshot(id)
	if err != nil {
		return "", err
	}
	return snap.Text, nil
}

// browserSnapshot reads page text plus the interactive elements tagged by
// textScript, in one round trip. Used by BrowserGetText and browser_read.
func (a *App) browserSnapshot(id string) (browserSnapshot, error) {
	host, err := a.browserHostLazy()
	if err != nil {
		return browserSnapshot{}, err
	}
	t := host.tab(id)
	if t == nil {
		return browserSnapshot{}, fmt.Errorf("no browser tab %q", id)
	}

	token := newMessageToken()
	ch := make(chan browserSnapshot, 1)
	t.textMu.Lock()
	t.textCh = ch
	t.textToken = token
	t.textMu.Unlock()

	host.do(func() { t.chromium.Eval(textScript(token)) })

	select {
	case snap := <-ch:
		return snap, nil
	case <-time.After(5 * time.Second):
		t.textMu.Lock()
		t.textCh = nil
		t.textToken = ""
		t.textMu.Unlock()
		return browserSnapshot{}, fmt.Errorf("page did not respond (still loading?)")
	}
}

// BrowserClickRef clicks the element tagged with ref by the most recent
// browser_read snapshot (see textScript).
func (a *App) BrowserClickRef(id string, ref int) error {
	host, err := a.browserHostLazy()
	if err != nil {
		return err
	}
	t := host.tab(id)
	if t == nil {
		return fmt.Errorf("no browser tab %q", id)
	}
	host.do(func() { t.chromium.Eval(clickScript(ref)) })
	return nil
}

// BrowserTypeRef sets an input/textarea/contenteditable's value, tagged with
// ref by the most recent browser_read snapshot (see textScript).
func (a *App) BrowserTypeRef(id string, ref int, text string) error {
	host, err := a.browserHostLazy()
	if err != nil {
		return err
	}
	t := host.tab(id)
	if t == nil {
		return fmt.Errorf("no browser tab %q", id)
	}
	host.do(func() { t.chromium.Eval(typeScript(ref, text)) })
	return nil
}
