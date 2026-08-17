package main

// Native in-app browser: each browser tab is a real webview of whatever engine
// the OS ships (WebView2, WebKitGTK, WKWebView), positioned over the dock's
// browser pane. This exists because iframes can't render sites that send
// X-Frame-Options/CSP deny (YouTube, Google, anything with bot checks), and
// because the AI needs to read real page content (BrowserGetText).
//
// This file is everything that does not care which engine that is: the injected
// scripts, the message bridge's security model, tab bookkeeping, and every
// Wails binding. One platform file behind it supplies the engine —
// browser_windows.go today, browser_linux.go and browser_darwin.go later. See
// PLATFORM-SUPPORT.md for the file map and ARCHITECTURE.md §48 for why the
// bindings themselves are never behind a build tag.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/statereport"
)

// tabView is one platform's live webview for one tab. Every method is called
// on the thread that owns the webview — that is, from inside hostBackend.do.
type tabView interface {
	navigate(url string)
	eval(js string)
	setBounds(x, y, w, h int)
	// setVisible(true) both shows the view and raises it: on Windows two
	// webviews in the same top-level window composite independently, so a tab
	// that is merely shown can stay behind the app's own webview — loaded,
	// painting, invisible.
	setVisible(visible bool)
	setZoom(factor float64)
	openDevTools()
	destroy()
	// capture asks the engine for a PNG of the visible page. Called on the
	// webview thread like everything else here, but answered on the channel
	// rather than returned: the engine delivers its answer through the same
	// message pump this thread is running, so a caller that blocked here would
	// be blocking the thing it is waiting for. See browser_shot.go.
	capture() <-chan shotResult
}

// tabCallbacks are the portable reactions a platform host wires into a tab it
// creates.
type tabCallbacks struct {
	// onMessage carries one postMessage envelope plus the sending frame's real
	// origin as the engine reports it — never what the page claims. See
	// aetoxMsg for why that distinction is the whole security model.
	onMessage func(raw, source string)
	// onNavDone fires when a navigation finishes. ok is whether the page
	// actually loaded, as opposed to the engine stopping on its own error
	// page; view is passed in because this can fire before the caller has
	// finished storing it.
	onNavDone func(view tabView, ok bool)
	// onEngineError carries the engine's own complaint about a call we made —
	// a refused COM call, a controller that would not create, a bad browser
	// path. Not a page error: the page is what onNavDone reports on.
	//
	// This existed as a log line and nothing else, which is how §127.8 stayed
	// invisible for a week. The engine said "This method can only be called
	// from the thread that created the object" every single time, into a file
	// nobody was reading, while the agent was handed "page did not finish
	// loading" and reasonably concluded the network was bad — then told the
	// user so. The tool's answer has to be able to carry what the engine said,
	// or the agent is guessing about its own tools.
	onEngineError func(err error)
}

// hostBackend is one platform's webview host.
type hostBackend interface {
	// start brings the host up — its owning thread, message pump, window
	// class, whatever the platform needs. Idempotent; blocks until ready.
	start() error
	// do runs fn on the thread that owns the webviews.
	//
	// ALWAYS asynchronous, on every platform. Windows has a dedicated STA
	// thread for this; GTK and Cocoa require the webviews on the app's *main*
	// thread, which makes the obvious dispatch_sync/g_main_context_invoke_sync
	// spelling deadlock — browserSnapshot calls do() and then blocks up to
	// five seconds waiting for the page to answer, and that is the path the
	// agent reads pages through. ARCHITECTURE.md §48 Decision 3.
	do(fn func())
	// openTab creates a webview at the given physical-pixel bounds and starts
	// it navigating. Called from inside do, so it is already on the owning
	// thread. Returns nil if the platform could not create it, having logged
	// why.
	openTab(id, url string, x, y, w, h int, cb tabCallbacks) tabView
}

// aetoxMsg is the JSON envelope pages post back to Go via the platform's script
// bridge (see metaScript / textScript).
//
// SECURITY: any page loaded in the tab can call that bridge itself, at any
// time, with an arbitrary __aetox envelope — it is not exclusive to our own
// injected scripts. Two checks guard against that (see onMessage): the "meta"
// case cross-checks the claimed URL against the sending frame's real origin as
// reported by the engine itself (a page cannot forge that), so a page can't
// make the address bar show a URL it isn't actually at (phishing-enabling
// spoof). The "text" case additionally requires a per-request Token minted by
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
	Images   []browserImage   `json:"images,omitempty"`
	// "pick" only: what the user pointed at, whether they left the mode without
	// pointing at anything, and whether they drew — in which case the marks are
	// still on the page waiting to be photographed. See browser_pick.go.
	Picks     []browserPick `json:"picks,omitempty"`
	Cancelled bool          `json:"cancelled,omitempty"`
	Drawn     bool          `json:"drawn,omitempty"`
	// "wait": whether what was waited for turned up before the deadline.
	Found bool `json:"found,omitempty"`
	// "dialog": which of alert/confirm/prompt the page called, what it said, and
	// what we answered on its behalf. See dialogScript.
	Dialog  string `json:"dialog,omitempty"`
	Message string `json:"message,omitempty"`
	Answer  string `json:"answer,omitempty"`
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

// browserImage is one meaningful image found on the page — its absolute URL
// and alt text, so the model can show it in chat with markdown ![alt](src).
type browserImage struct {
	Src string `json:"src"`
	Alt string `json:"alt,omitempty"`
}

// browserSnapshot is the result of one textScript round trip: page text plus
// the interactive elements and images found on it.
type browserSnapshot struct {
	Text     string
	Elements []browserElement
	Images   []browserImage
}

// metaScript reports the page's real title and URL back over the bridge. The
// call itself is the one part of these scripts that is engine-specific —
// bridgePost is `window.chrome.webview.postMessage` on WebView2 and
// `window.webkit.messageHandlers.aetox.postMessage` on both WebKits — so it
// comes from the platform file and everything below is shared.
func metaScript() string {
	return bridgePost + `(JSON.stringify({__aetox:"meta",title:document.title,url:location.href}))`
}

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
    if(el.tagName==='SELECT'){
      var op=[];
      for(var k=0;k<el.options.length&&k<8;k++)op.push(el.options[k].text.trim());
      txt=((txt?txt+' ':'')+'[options: '+op.join(' | ')+']').slice(0,200);
    }
    out.push({ref:ref,tag:el.tagName.toLowerCase(),role:el.getAttribute('role')||'',text:txt});
  }
  var imgs=[];
  var seen={};
  var imels=document.querySelectorAll('img[src]');
  for(var j=0;j<imels.length&&imgs.length<20;j++){
    var im=imels[j];
    var ir=im.getBoundingClientRect();
    if(ir.width<64||ir.height<64)continue; /* skip icons/trackers */
    var src=im.currentSrc||im.src||'';
    if(!src||src.indexOf('data:')===0||seen[src])continue;
    seen[src]=1;
    imgs.push({src:src.slice(0,600),alt:(im.alt||'').trim().replace(/\s+/g,' ').slice(0,120)});
  }
  %s(JSON.stringify({__aetox:"text",token:%q,title:document.title,url:location.href,text:(document.body&&document.body.innerText||"").slice(0,200000),elements:out,images:imgs}));
})()`, bridgePost, token)
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

// typeScript sets an input/textarea/select/contenteditable's value via the
// native setter (so React/Vue-controlled inputs pick it up) and fires
// input+change. A SELECT matches the text against option value or label
// instead of overwriting content. enter additionally presses Enter — synthetic
// keydown first (skipped requestSubmit if the page preventDefault'ed it), then
// the form's requestSubmit, because untrusted KeyboardEvents never trigger the
// browser's own implicit submission.
func typeScript(ref int, text string, enter bool) string {
	encoded, _ := json.Marshal(text)
	enterJS := ""
	if enter {
		enterJS = `
  var ke={key:"Enter",code:"Enter",keyCode:13,which:13,bubbles:true,cancelable:true};
  var notHandled=el.dispatchEvent(new KeyboardEvent("keydown",ke));
  el.dispatchEvent(new KeyboardEvent("keyup",ke));
  if(notHandled&&el.form&&typeof el.form.requestSubmit==="function"){el.form.requestSubmit();}`
	}
	return fmt.Sprintf(`(function(){
  var el=document.querySelector('[data-aetox-ref="%d"]');
  if(!el)return;
  el.focus();
  var val=%s;
  if(el.tagName==="SELECT"){
    var want=val.trim().toLowerCase();
    for(var i=0;i<el.options.length;i++){
      var o=el.options[i];
      if(o.value.toLowerCase()===want||o.text.trim().toLowerCase()===want){
        Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype,"value").set.call(el,o.value);
        break;
      }
    }
  } else if(el.tagName==="INPUT"||el.tagName==="TEXTAREA"){
    var proto=el.tagName==="TEXTAREA"?window.HTMLTextAreaElement.prototype:window.HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto,"value").set.call(el,val);
  } else {
    el.textContent=val;
  }
  el.dispatchEvent(new Event("input",{bubbles:true}));
  el.dispatchEvent(new Event("change",{bubbles:true}));%s
})()`, ref, encoded, enterJS)
}

// sameOrigin reports whether a and b share a scheme+host — used to check a
// page's claimed URL against its real origin as reported by the engine.
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

// browserTab is the portable bookkeeping for one tab: latches, meta, zoom,
// visibility, the pick in progress. Everything about a tab EXCEPT the webview.
//
// The webview used to be a field here, and that is what made §127.8 possible:
// anything holding a *browserTab could reach the engine directly, on whatever
// goroutine it happened to be on, and WebView2 answers that by refusing the
// call silently. The views now live in browserHost.views and are handed out
// only inside onTab, which runs on the thread that owns them. There is no
// second way to get one, so the call that broke does not compile.
type browserTab struct {
	// One navigation's completion latch, replaced on every new navigation so a
	// reused tab can be awaited again. Guarded because navCompleted closes it
	// from the host thread while a tool call waits on it: without the mutex,
	// re-arming races the close and a waiter can hold the previous latch.
	navMu   sync.Mutex
	navDone chan struct{} // closed after the current navigation completes
	navOnce *sync.Once

	metaMu sync.Mutex
	title  string
	url    string
	// navOK is the last completed navigation's real outcome. False means the
	// window is showing the engine's own error page — a state that used to be
	// indistinguishable from a loaded page, since navigation-completed fires
	// either way.
	navOK bool

	visMu  sync.Mutex
	hidden bool // BrowserSetVisible(false); nav-completed re-glue must not surface hidden tabs

	zoomMu sync.Mutex
	zoom   float64 // device-size emulation (see BrowserSetZoom); 0 = never set

	textMu    sync.Mutex
	textCh    chan browserSnapshot
	textToken string // token BrowserGetText is currently waiting on; empty = none pending

	pickMu sync.Mutex
	// pickToken is the token the live point-at-the-page mode will answer with;
	// empty = no mode running. Unlike textToken there is no channel waiting on
	// it — a pick arrives as an event whenever the user gets round to pointing,
	// or never. See browser_pick.go.
	pickToken string

	waitMu    sync.Mutex
	waitCh    chan bool
	waitToken string

	// dlgMu guards what the page's dialogs said since anyone last looked. A
	// dialog cannot block here — see dialogScript — so the only way the agent
	// ever learns one happened is that the next answer it gets mentions it.
	dlgMu   sync.Mutex
	dialogs []string

	engMu sync.Mutex
	// engErr is the engine's last complaint about a call made SINCE this tab's
	// current navigation was armed. Cleared by armNavigation, so it is always
	// about the navigation being waited on and never a leftover from the last
	// one. See tabCallbacks.onEngineError for why it exists at all.
	engErr error
}

// noteEngineError records what the engine said. Called from whatever thread the
// engine chose; the mutex is the whole of the synchronisation.
func (t *browserTab) noteEngineError(err error) {
	if err == nil {
		return
	}
	t.engMu.Lock()
	t.engErr = err
	t.engMu.Unlock()
}

func (t *browserTab) engineError() error {
	t.engMu.Lock()
	defer t.engMu.Unlock()
	return t.engErr
}

func (t *browserTab) meta() (title, url string) {
	t.metaMu.Lock()
	defer t.metaMu.Unlock()
	return t.title, t.url
}

// noteDialog remembers one dialog the page raised. Bounded, because a page in a
// loop can raise them forever and the point is to tell the agent something
// happened, not to transcribe an attack.
func (t *browserTab) noteDialog(line string) {
	t.dlgMu.Lock()
	defer t.dlgMu.Unlock()
	if len(t.dialogs) < 8 {
		t.dialogs = append(t.dialogs, line)
	}
}

// takeDialogs hands back what the page said and forgets it, so one dialog is
// reported once rather than on every action for the rest of the session.
func (t *browserTab) takeDialogs() []string {
	t.dlgMu.Lock()
	defer t.dlgMu.Unlock()
	out := t.dialogs
	t.dialogs = nil
	return out
}

// dialogNote is the sentence an action appends when the page said something
// while it was working, or "" when it did not.
func (a *App) dialogNote(id AgentTabID) string {
	if a.browsers == nil {
		return ""
	}
	t := a.browsers.tab(string(id))
	if t == nil {
		return ""
	}
	lines := t.takeDialogs()
	if len(lines) == 0 {
		return ""
	}
	return "\nหน้าเว็บขึ้นกล่องข้อความ:\n" + strings.Join(lines, "\n")
}

// armNavigation readies the tab for one more navigation, so the caller that
// is about to call view.navigate can await *that* one.
//
// Without it a reused tab answers instantly with the previous page's verdict:
// the latch is closed from the first load and never reopens, so "did the page
// I just asked for arrive" becomes "did the page before it arrive". Called
// before navigate, never after — arming late would drop a completion that beat
// the arm and hang the wait until its timeout.
// latch hands back the current navigation latch, creating one if this tab has
// never had it set. The zero-value tab is a real state — tests build one, and
// a completion can arrive before open() finishes storing its fields — so the
// pair is resolved here rather than assumed to exist at every use.
func (t *browserTab) latch() (*sync.Once, chan struct{}) {
	t.navMu.Lock()
	defer t.navMu.Unlock()
	if t.navDone == nil {
		t.navDone = make(chan struct{})
	}
	if t.navOnce == nil {
		t.navOnce = &sync.Once{}
	}
	return t.navOnce, t.navDone
}

func (t *browserTab) armNavigation() {
	t.setNavOK(false)
	t.navMu.Lock()
	t.navDone, t.navOnce = make(chan struct{}), &sync.Once{}
	t.navMu.Unlock()
	// Cleared here so anything the engine says from now on belongs to the
	// navigation about to be asked for, and a wait that times out can tell the
	// difference between "the site is slow" and "we made a call the engine
	// threw away".
	t.engMu.Lock()
	t.engErr = nil
	t.engMu.Unlock()
	// And the same for what we think the page IS, which is the half that was
	// wrong in production before anyone noticed.
	//
	// meta arrives from the page a beat after it loads, so `open` polls until it
	// is non-empty. On a fresh tab that works. On a REUSED one the previous
	// page's title and URL are already sitting there, so the poll succeeds on
	// its first read and `open` reports the page it just left — seen in the log
	// as "เปิดแล้ว: Example Domain" for a navigation to x.com. Every reused open
	// since tab reuse shipped has been naming the wrong page, and
	// parseBrowserOpened has been filing those into the visited-pages panel.
	t.metaMu.Lock()
	t.title, t.url = "", ""
	t.metaMu.Unlock()
}

func (t *browserTab) setNavOK(ok bool) {
	t.metaMu.Lock()
	t.navOK = ok
	t.metaMu.Unlock()
}

func (t *browserTab) navLoaded() bool {
	t.metaMu.Lock()
	defer t.metaMu.Unlock()
	return t.navOK
}

// awaitNavigation blocks until the tab's first navigation completes, then
// reports whether the page actually loaded. Waiting alone is not enough:
// "navigation completed" only says the engine stopped, not that it stopped on
// the page that was asked for, so a caller that trusted navDone reported
// success over a File-not-found page and kept working from it.
func (t *browserTab) awaitNavigation(ctx context.Context, timeout time.Duration) error {
	_, done := t.latch()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(timeout):
		// If the engine complained about a call we made during this navigation,
		// that is the answer and it is OURS, not the page's.
		//
		// A plain errors.New, deliberately, where the line below is a
		// statereport: the two sentences describe different worlds. "The site
		// is slow tonight" is a report about a moment and nothing to correct.
		// "The engine refused our call" is a defect in this program, and
		// marking it as weather is how it survived a week of being logged
		// every twenty seconds — §127.8, where the agent read the weather
		// report it was handed and told the user the network was bad.
		if engErr := t.engineError(); engErr != nil {
			return fmt.Errorf("the browser engine refused what Aetox asked it to do: %w", engErr)
		}
		// Both are reports about a page at a moment — slow tonight, down
		// tonight — not behaviour to correct (statereport, not errors.New):
		// left unmarked, three of these became a permanent-memory card about
		// avoiding a localhost URL whose server simply was not running yet.
		return statereport.New("page did not finish loading")
	}
	if !t.navLoaded() {
		return statereport.New("page failed to load — not found, or unreachable")
	}
	return nil
}

type browserHost struct {
	app     *App
	backend hostBackend

	mu   sync.Mutex
	tabs map[string]*browserTab
	// views holds the live webviews, and is the reason browserTab no longer
	// does. Nothing outside this file reads it, and onTab is the only thing
	// that hands one out — always from inside backend.do, which is the whole
	// point (see browserTab's own comment, and browser_shot.go's header for
	// what the engine does when that rule is broken).
	views map[string]tabView
	// Two fields because there are two questions, and one field answering both
	// is what put the agent's keystrokes on the user's page.
	//
	// lastID is "the one on screen": most recently opened *or shown*, rewritten
	// by BrowserSetVisible every time a tab is raised, including by the user
	// clicking between their own. The frontend asks this; nothing the agent does
	// may.
	//
	// agentID is "the one the agent is working" and agentOrder is every tab the
	// agent owns, oldest first. Only a web-agent- tab reaches either, so raising
	// a user's tab cannot move them and the agent cannot lose track of itself.
	//
	// Two fields for the same reason lastID and agentID are two: "which of mine
	// am I working" and "which are mine" are different questions. The agent had
	// exactly one tab until 2026-08-17, which fused a rule about OWNERSHIP (the
	// agent's tabs are its own, never the user's) with a rule about COUNT (there
	// is one of them) — and only the first was ever load-bearing. The prefix
	// separates the agent's tabs from the user's at any number.
	//
	// agentID is not cleared by whoever closes a tab: one the user closed is gone
	// from tabs, and agentTab reads that as no tab, which is also how `open`
	// learns to mint a fresh one. agentOrder IS pruned, because a list is read
	// and a list that names a dead tab is a list that lies.
	lastID     string
	agentID    string
	agentOrder []string
}

func newBrowserHost(app *App) *browserHost {
	return &browserHost{
		app:     app,
		backend: newHostBackend(),
		tabs:    map[string]*browserTab{},
		views:   map[string]tabView{},
	}
}

func (h *browserHost) start() error { return h.backend.start() }

func (h *browserHost) tab(id string) *browserTab {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tabs[id]
}

// open creates the webview for a tab, on the thread that owns webviews.
func (h *browserHost) open(id, url string, x, y, w, hgt int) {
	debuglog.Msg("browser.open(%s): queueing (url=%s)", id, url)
	h.backend.do(func() {
		debuglog.Msg("browser.open(%s): running on the host thread", id)
		if h.tab(id) != nil {
			return
		}
		tab := &browserTab{navDone: make(chan struct{}), navOnce: &sync.Once{}}
		view := h.backend.openTab(id, url, x, y, w, hgt, tabCallbacks{
			onMessage:     func(raw, source string) { h.onMessage(id, tab, raw, source) },
			onNavDone:     func(v tabView, ok bool) { h.navCompleted(tab, v, ok) },
			onEngineError: tab.noteEngineError,
		})
		if view == nil {
			return // the backend has already logged why
		}

		h.mu.Lock()
		h.tabs[id] = tab
		h.views[id] = view
		h.lastID = id
		if isAgentTabID(id) {
			h.agentID = id
			if !slices.Contains(h.agentOrder, id) {
				h.agentOrder = append(h.agentOrder, id)
			}
		}
		h.mu.Unlock()
	})
}

// agentTabPrefix marks the ids workbenchOpenBrowser mints. It is the only thing
// that distinguishes a tab the agent opened from one the user did, so it is
// tested in exactly one place — here — and everything downstream reads agentID.
const agentTabPrefix = "web-agent-"

func isAgentTabID(id string) bool { return strings.HasPrefix(id, agentTabPrefix) }

// navCompleted is the portable half of "a navigation finished". It takes the
// view rather than reading tab.view because a fast first navigation can land
// before open() has stored it.
func (h *browserHost) navCompleted(tab *browserTab, view tabView, ok bool) {
	// Recorded before navDone is closed, so a waiter that wakes on it reads
	// this navigation's outcome and not the previous one's.
	tab.setNavOK(ok)
	once, done := tab.latch()
	once.Do(func() { close(done) })

	// Raise the tab now that the page has rendered. The frontend's
	// browser:meta handler used to be the only thing doing this, which made
	// visibility depend on page JS delivering a message that passes the origin
	// check — never true for file:// before the sameOrigin fix, and fragile in
	// general: the page stayed loaded but composited invisibly behind the
	// app's own webview.
	tab.visMu.Lock()
	hidden := tab.hidden
	tab.visMu.Unlock()
	if !hidden {
		view.setVisible(true)
	}

	// Engines keep zoom per origin, so a cross-site navigation drops the
	// device-emulation factor back to 1 — re-assert it here.
	tab.zoomMu.Lock()
	z := tab.zoom
	tab.zoomMu.Unlock()
	if z > 0 {
		view.setZoom(z)
	}
	view.eval(metaScript())
}

// onMessage handles one postMessage envelope from a tab's page. source is the
// sending frame's real origin as the engine reports it — trustworthy, unlike
// anything else in the message, which any page script can set freely.
func (h *browserHost) onMessage(id string, tab *browserTab, raw string, source string) {
	var m aetoxMsg
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m.Aetox == "" {
		return
	}
	switch m.Aetox {
	case "meta":
		// A page can claim any url it likes in the envelope; only trust it if
		// it matches where the engine says the message actually came from —
		// otherwise a page could make the address bar show a URL it isn't at.
		if !sameOrigin(source, m.URL) {
			return
		}
		tab.metaMu.Lock()
		tab.title, tab.url = m.Title, m.URL
		tab.metaMu.Unlock()
		if h.app.ctx != nil {
			h.app.emitEvent("browser:meta:"+id, map[string]string{"title": m.Title, "url": m.URL})
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
		ch <- browserSnapshot{Text: m.Text, Elements: m.Elements, Images: m.Images}
	case "wait":
		tab.waitMu.Lock()
		ch, want := tab.waitCh, tab.waitToken
		tab.waitCh, tab.waitToken = nil, ""
		tab.waitMu.Unlock()
		// Same two checks the text case makes, for the same reason: a page can
		// post this envelope itself, and a forged one must not end a wait the
		// real script is still running.
		if ch == nil || m.Token == "" || m.Token != want || !sameOrigin(source, m.URL) {
			return
		}
		ch <- m.Found
	case "dialog":
		// No token here, and that is deliberate: nobody asked for this message,
		// the page raised it. The origin check is what keeps a frame from
		// putting words in another page's mouth, and the text is quoted rather
		// than obeyed — it is a report about the page, never an instruction.
		if !sameOrigin(source, m.URL) {
			return
		}
		tab.noteDialog(fmt.Sprintf("- %s(%q) — Aetox ตอบว่า %s", m.Dialog, m.Message, m.Answer))
	case "pick":
		// Origin before token, deliberately: a message from the wrong origin
		// must not consume the token the real page is still going to answer
		// with. Order the other way round, and any page in any frame could end
		// a pick the user is halfway through.
		if !sameOrigin(source, m.URL) || !tab.claimPick(m.Token) {
			return
		}
		h.app.emitEvent("browser:pick:"+id, map[string]any{
			"url": m.URL, "cancelled": m.Cancelled, "drawn": m.Drawn, "picks": m.Picks,
		})
	}
}

// ---------------------------------------------------------------------------
// Wails bindings
// ---------------------------------------------------------------------------
//
// Every method below exists on every platform, and none of them is behind a
// build tag. desktop/frontend/wailsjs/go/main/App.d.ts is generated from this
// set and committed; a platform that dropped one would regenerate that file
// without it and break BrowserPane.svelte's imports at vite build time.
// ARCHITECTURE.md §48 Decision 2.

func (a *App) browserHostLazy() (*browserHost, error) {
	a.terminalsMu.Lock()
	if a.browsers == nil {
		a.browsers = newBrowserHost(a)
	}
	h := a.browsers
	a.terminalsMu.Unlock()
	return h, h.start()
}

// onTab runs fn against one tab's webview on the thread that owns webviews.
//
// It is the ONLY way to reach a tabView, and that is the design rather than a
// convenience: an engine call made from any other goroutine is not slow and not
// racy, it is refused outright, and the refusal arrives as a page that never
// finishes loading twenty seconds later (§127.8). Holding the views in the host
// instead of on browserTab is what makes this the only door — there is no field
// left to reach past it.
//
// The view handed to fn is valid for the duration of fn and no longer. Nothing
// in Go stops a caller squirrelling it away to call later; what this prevents
// is the accident, which is the one that happened.
//
// The lookup happens HERE, on the host thread, not at the call site. open()
// only registers a tab at the very end of its own queued command, so anything
// that checked on the caller's goroutine found nil for every call made in the
// moments after BrowserOpen and dropped it silently — which is what left a
// freshly opened tab's window at the rect the pane had before the address bar
// existed, covering the toolbar until something forced a resize. do() is FIFO,
// so by the time this runs the open ahead of it has finished.
func (h *browserHost) onTab(id string, fn func(tabView, *browserTab)) {
	h.backend.do(func() {
		h.mu.Lock()
		v, t := h.views[id], h.tabs[id]
		h.mu.Unlock()
		if v != nil && t != nil {
			fn(v, t)
		}
	})
}

func (a *App) onTab(id string, fn func(tabView, *browserTab)) {
	if host, err := a.browserHostLazy(); err == nil {
		host.onTab(id, fn)
	}
}

// live reports whether id names a tab that still has a webview — the question
// browserTab.view == nil used to answer at call sites. A tab without a view was
// never a tab; it was an openTab that failed and got stored anyway.
func (h *browserHost) live(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tabs[id] != nil && h.views[id] != nil
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
	a.onTab(id, func(v tabView, _ *browserTab) { v.navigate(url) })
}

// BrowserSetBounds moves/resizes a tab's view (physical pixels, relative to
// the main window client area).
func (a *App) BrowserSetBounds(id string, x, y, w, h int) {
	a.onTab(id, func(v tabView, _ *browserTab) { v.setBounds(x, y, w, h) })
}

// BrowserSetZoom scales the page inside a tab — this is what makes the device
// presets real emulation rather than a small window: with the tab's view sized
// to deviceWidth*factor, a zoom of `factor` leaves the page a CSS viewport of
// exactly deviceWidth, so its media queries fire as they would on that device.
// 1 = no emulation (the pane-filling default).
func (a *App) BrowserSetZoom(id string, factor float64) {
	if factor <= 0 {
		factor = 1
	}
	a.onTab(id, func(v tabView, t *browserTab) {
		t.zoomMu.Lock()
		t.zoom = factor
		t.zoomMu.Unlock()
		v.setZoom(factor)
	})
}

// BrowserSetVisible shows/hides a tab (hidden when its dock tab is inactive or
// the settings overlay is open — a native view always floats above the UI).
func (a *App) BrowserSetVisible(id string, visible bool) {
	a.onTab(id, func(v tabView, t *browserTab) {
		t.visMu.Lock()
		t.hidden = !visible
		t.visMu.Unlock()
		if visible {
			a.browsers.mu.Lock()
			a.browsers.lastID = id
			a.browsers.mu.Unlock()
		}
		v.setVisible(visible)
	})
}

// BrowserBack / BrowserForward / BrowserReload drive history via script — not
// every engine wrapper exposes GoBack/GoForward, and the script works on all
// of them.
func (a *App) BrowserBack(id string)    { a.browserEval(id, "history.back()") }
func (a *App) BrowserForward(id string) { a.browserEval(id, "history.forward()") }
func (a *App) BrowserReload(id string)  { a.browserEval(id, "location.reload()") }

// BrowserOpenDevTools opens the engine's own DevTools on a tab — find-in-page,
// console, network, element inspection and its screenshot tools, none of which
// are worth reimplementing in our toolbar.
func (a *App) BrowserOpenDevTools(id string) {
	a.onTab(id, func(v tabView, _ *browserTab) { v.openDevTools() })
}

func (a *App) browserEval(id, js string) {
	a.onTab(id, func(v tabView, _ *browserTab) { v.eval(js) })
}

// CloseAllBrowserTabs destroys every native browser view this process still
// holds. Called once by the frontend right after it (re)loads (App.svelte
// onMount) — a freshly loaded frontend owns zero workbench tabs by
// definition, so anything still open here is orphaned from a previous
// frontend lifetime: the Go backend is a long-lived process, but a `wails
// dev` Vite HMR full-reload (or any webview reload) wipes the JS-side
// `workbench` store without running BrowserPane's onDestroy, leaving the
// native view behind with nothing left to reposition or close it — it just
// floats, stuck at its last bounds. On a genuine fresh app start `a.browsers`
// is nil and this is a no-op.
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

func (a *App) BrowserClose(id string) {
	if host, err := a.browserHostLazy(); err == nil {
		host.mu.Lock()
		v := host.views[id]
		delete(host.tabs, id)
		delete(host.views, id)
		// The current tab falls back to whatever is left rather than to nothing,
		// so closing one of several does not strand the agent mid-task.
		host.agentOrder = slices.DeleteFunc(host.agentOrder, func(open string) bool { return open == id })
		if host.agentID == id {
			host.agentID = ""
			if len(host.agentOrder) > 0 {
				host.agentID = host.agentOrder[len(host.agentOrder)-1]
			}
		}
		host.mu.Unlock()
		if v != nil {
			host.backend.do(func() { v.destroy() })
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

	// This blocks below, so do() must not: see hostBackend.do.
	host.onTab(id, func(v tabView, _ *browserTab) { v.eval(textScript(token)) })

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
	host.onTab(id, func(v tabView, _ *browserTab) { v.eval(clickScript(ref)) })
	return nil
}

// BrowserTypeRef sets an input/textarea/select/contenteditable's value,
// tagged with ref by the most recent browser_read snapshot (see textScript).
// enter presses Enter afterwards (for forms with no submit button).
func (a *App) BrowserTypeRef(id string, ref int, text string, enter bool) error {
	host, err := a.browserHostLazy()
	if err != nil {
		return err
	}
	t := host.tab(id)
	if t == nil {
		return fmt.Errorf("no browser tab %q", id)
	}
	host.onTab(id, func(v tabView, _ *browserTab) { v.eval(typeScript(ref, text, enter)) })
	return nil
}
