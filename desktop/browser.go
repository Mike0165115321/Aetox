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

	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/statereport"
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
	// callEngine runs one of the engine's own protocol methods and hands back
	// its answer as JSON. Same threading and same channel-not-return reason as
	// capture.
	//
	// It exists because exporting a deck needs three things no portable
	// vocabulary covers — print this page to PDF, photograph this rectangle,
	// measure where the slides are — and inventing a portable spelling for each
	// would be inventing an abstraction over one implementation. On Windows
	// these are Chrome DevTools Protocol methods (browser_windows.go). A future
	// WebKit host answers the same three questions its own way or not at all,
	// and the caller finds out through an error rather than through a lie.
	//
	// Unlike capture, printing does NOT need the page on screen: it is a
	// separate pipeline from compositing. That is the whole reason a deck can be
	// exported from a webview nobody ever sees (deck_render.go).
	callEngine(method, paramsJSON string) <-chan engineReply
	// sendBehind drops this tab to the bottom of the Z order, leaving it laid
	// out and painting with the app's own webview drawn over it. The one caller
	// is the deck export, which needs a page that composites (so a capture has
	// frames) and that nobody sees (so nothing flashes). See browser_windows.go
	// for why the obvious alternatives — hidden, or parked off-screen — both
	// come back blank.
	sendBehind()
}

// engineReply is one finished (or failed) engine call, in the portable
// vocabulary tabView is written in — browser.go must not name any one engine's
// types. JSON is the protocol's own answer object, undecoded.
type engineReply struct {
	JSON string
	Err  error
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
	// "text": how many there really were, and how much of the page could not
	// be entered at all. The lists above stop at a cap; these do not, so the
	// read can say what it left out instead of presenting a truncated list as
	// the whole page. See textScript.
	ElementsTotal int `json:"elementsTotal,omitempty"`
	ImagesTotal   int `json:"imagesTotal,omitempty"`
	Frames        int `json:"frames,omitempty"`
	// "pick" only: what the user pointed at, whether they left the mode without
	// pointing at anything, and whether they drew — in which case the marks are
	// still on the page waiting to be photographed. See browser_pick.go.
	Picks     []browserPick `json:"picks,omitempty"`
	Cancelled bool          `json:"cancelled,omitempty"`
	Drawn     bool          `json:"drawn,omitempty"`
	// "wait": whether what was waited for turned up before the deadline.
	// "act": whether the ref a click or a type was aimed at matched anything,
	// and what it turned out to be. Sent BEFORE the action runs, so a click
	// that navigates cannot destroy the document before its own report gets
	// out. See aetoxActJS.
	Found bool   `json:"found,omitempty"`
	Ref   int    `json:"ref,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Label string `json:"label,omitempty"`
	// "log": one of the page's own recorders, read on demand. Armed travels
	// with the entries deliberately — an empty buffer and a page the recorder
	// never reached are the same list, and only one of them is evidence. See
	// browser_log.go.
	Kind    string            `json:"kind,omitempty"`
	Log     []browserLogEntry `json:"log,omitempty"`
	Dropped int               `json:"dropped,omitempty"`
	Armed   bool              `json:"armed,omitempty"`
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
	// ElementsTotal and ImagesTotal are how many the page really had. They are
	// counted past the caps the lists above stop at, which is the whole point:
	// a list of 150 and a page of 150 look identical otherwise, and the reader
	// of a silently-cut list has no way to know it is missing the button it is
	// looking for.
	ElementsTotal int
	ImagesTotal   int
	// BlockedFrames is how many cross-origin frames this page has. Their
	// contents cannot be reached from JS by any means, so they are counted and
	// reported rather than quietly omitted.
	BlockedFrames int
}

// metaScript reports the page's real title and URL back over the bridge. The
// call itself is the one part of these scripts that is engine-specific —
// bridgePost is `window.chrome.webview.postMessage` on WebView2 and
// `window.webkit.messageHandlers.aetox.postMessage` on both WebKits — so it
// comes from the platform file and everything below is shared.
func metaScript() string {
	return bridgePost + `(JSON.stringify({__aetox:"meta",title:document.title,url:location.href}))`
}

// aetoxScanJS is the walker every page script shares: the document roots this
// page really has, and the way to find a tagged element inside any of them.
//
// `document` alone was the whole search until 2026-08-22, and `document` is not
// the page. An element inside a shadow root is invisible to
// document.querySelectorAll, and so is one inside an iframe — which is where
// checkout forms, embedded editors and most component-library controls live.
//
// The failure was silent and, worse, disguised as a different one: `read` came
// back with no elements, and read's own guidance teaches the model that an
// empty page means "not finished loading, use wait". So the honest response to
// a page we could not see was to wait for it, and then wait again.
//
// A cross-origin frame cannot be entered from JS by any technique. Those are
// counted rather than opened, so a read can report how much of the page it was
// unable to reach instead of implying there was nothing there.
//
// The node budget is what keeps this inside browserSnapshot's five-second
// deadline on a page with a very large DOM. It bounds the walk rather than the
// collecting, because the walk is the part whose cost the caller cannot
// predict — and `cut` is reported for the same reason every other cap here is.
const aetoxScanJS = `
  var AETOX_BUDGET=20000;
  function aetoxScan(){
    var roots=[document],blocked=0,seen=0;
    for(var i=0;i<roots.length;i++){
      var all;
      try{all=roots[i].querySelectorAll('*');}catch(e){continue;}
      for(var j=0;j<all.length;j++){
        if(++seen>AETOX_BUDGET)return{roots:roots,blocked:blocked,cut:true};
        var el=all[j];
        if(el.shadowRoot)roots.push(el.shadowRoot);
        if(el.tagName==='IFRAME'){
          var d=null;
          try{d=el.contentDocument;}catch(e){d=null;}
          if(d&&d.body)roots.push(d);else blocked++;
        }
      }
    }
    return{roots:roots,blocked:blocked,cut:false};
  }
  function aetoxFind(ref){
    var s=aetoxScan();
    for(var i=0;i<s.roots.length;i++){
      var el;
      try{el=s.roots[i].querySelector('[data-aetox-ref="'+ref+'"]');}catch(e){el=null;}
      if(el)return el;
    }
    return null;
  }
`

// aetoxTextJS is what "the text of this page" means, in one place.
//
// document.body.innerText is not it, and it is short in two different ways.
// An iframe's document is a separate one and its text is in none of its
// parent's. A shadow root's text is in none of its host's either: innerText is
// built from an element's light-DOM descendants, and shadow content is not
// among them however plainly it is on screen. Measured on a fixture page
// 2026-08-22 — a host whose shadow root renders a paragraph reports
// host.innerText === "" and document.body.innerText does not contain a word of
// it, while the shadow root's own children report it normally. That last part
// is what this function is built on.
//
// One definition because `read` and `wait` both need the same answer or they
// contradict each other: a `wait` for a word `read` can see would poll until
// its deadline and then report the word absent, which is the worst kind of
// wrong here because an absent word reads as a page still loading, and the
// response to that is to wait again.
//
// The cost is the whole walk, on every poll, and it was measured before being
// accepted rather than argued about. On en.wikipedia.org/wiki/World_War_II
// (16,844 nodes): aetoxScan 3.755 ms, document.body.innerText 7.08 ms. The
// scan is cheaper than the innerText call `wait` was already paying five times
// a second, so this is not a new order of cost — and AETOX_BUDGET still bounds
// the pathological page.
const aetoxTextJS = `
  function aetoxText(){
    var roots=aetoxScan().roots;
    var t=document.body?(document.body.innerText||""):"";
    for(var i=1;i<roots.length;i++){
      var r=roots[i];
      /* A document root (a same-origin frame) has a body; a shadow root does
         not, and is read through its own children — each of which reports
         rendered text normally even though its host reports none. */
      if(r.body){if(r.body.innerText)t+='\n'+r.body.innerText;continue;}
      for(var j=0;j<r.children.length;j++){
        var c=r.children[j];
        if(c.innerText)t+='\n'+c.innerText;
      }
    }
    return t;
  }
`

// textScript reads page text and, in the same pass, tags every visible
// interactive element with a data-aetox-ref so browser_click/browser_type can
// target it later. Refs are reassigned fresh each call.
//
// filter, when non-empty, tags only the elements whose text contains it,
// case-insensitively. It exists because the cap below is a real ceiling on a
// real page and not a theoretical one: 150 refs are assigned in DOM order, and
// on most sites the first 150 elements are the nav and the sidebar, so the
// control the model is actually looking for can sit past the cap on every read
// no matter how many times it reads. Raising the cap would buy that with
// context on every read; a filter is paid only by the read that needs it.
//
// Both caps now count past themselves. What stopped is said out loud in the
// tool output rather than left for the model to discover by finding nothing —
// the same rule skill_view's end marker was written for
// (internal/skill/progressive.go), applied to the tool that was breaking it.
func textScript(token, filter string) string {
	return fmt.Sprintf(`(function(){%s%s
  var want=%s.trim().toLowerCase();
  var scan=aetoxScan();
  var roots=scan.roots;
  var sel='a[href],button,input,select,textarea,[role="button"],[role="link"],[contenteditable="true"]';
  /* Stale refs from the previous read are cleared first, in every root. A
     filtered read tags far fewer nodes than an unfiltered one, so without this
     a ref could resolve to a node the last read tagged and this one did not. */
  for(var r=0;r<roots.length;r++){
    var stale;
    try{stale=roots[r].querySelectorAll('[data-aetox-ref]');}catch(e){continue;}
    for(var q=0;q<stale.length;q++)stale[q].removeAttribute('data-aetox-ref');
  }
  var out=[],elTotal=0;
  for(var r2=0;r2<roots.length;r2++){
    var els;
    try{els=roots[r2].querySelectorAll(sel);}catch(e){continue;}
    for(var i=0;i<els.length;i++){
      var el=els[i];
      var rect=el.getBoundingClientRect();
      if(rect.width<=0||rect.height<=0)continue;
      var txt=(el.innerText||el.value||el.getAttribute('aria-label')||el.getAttribute('placeholder')||'').trim().replace(/\s+/g,' ').slice(0,80);
      if(el.tagName==='SELECT'){
        var op=[];
        for(var k=0;k<el.options.length&&k<8;k++)op.push(el.options[k].text.trim());
        txt=((txt?txt+' ':'')+'[options: '+op.join(' | ')+']').slice(0,200);
      }
      if(want&&txt.toLowerCase().indexOf(want)<0)continue;
      elTotal++;
      if(out.length>=150)continue;
      var ref=out.length+1;
      el.setAttribute('data-aetox-ref',String(ref));
      out.push({ref:ref,tag:el.tagName.toLowerCase(),role:el.getAttribute('role')||'',text:txt});
    }
  }
  var imgs=[],seenSrc={},imgTotal=0;
  for(var r3=0;r3<roots.length;r3++){
    var imels;
    try{imels=roots[r3].querySelectorAll('img[src]');}catch(e){continue;}
    for(var j=0;j<imels.length;j++){
      var im=imels[j];
      var ir=im.getBoundingClientRect();
      if(ir.width<64||ir.height<64)continue; /* skip icons/trackers */
      var src=im.currentSrc||im.src||'';
      if(!src||src.indexOf('data:')===0||seenSrc[src])continue;
      seenSrc[src]=1;
      imgTotal++;
      if(imgs.length>=20)continue;
      imgs.push({src:src.slice(0,600),alt:(im.alt||'').trim().replace(/\s+/g,' ').slice(0,120)});
    }
  }
  var text=aetoxText();
  %s(JSON.stringify({__aetox:"text",token:%q,title:document.title,url:location.href,text:text.slice(0,200000),elements:out,images:imgs,elementsTotal:elTotal,imagesTotal:imgTotal,frames:scan.blocked}));
})()`, aetoxScanJS, aetoxTextJS, mustJSONString(filter), bridgePost, token)
}

// browserActResult is what a click or a type says about the element it was
// aimed at. Reported before the action, never after: see aetoxActJS.
type browserActResult struct {
	Found bool
	Ref   int
	Tag   string
	Label string
}

// aetoxActJS is the one sentence a click and a type both have to say: whether
// the ref they were given matched anything on this page.
//
// It exists because of a real turn, on 2026-08-22, recorded in tool_runs. The
// model sent {"action":"click","ref":"1"}, the quoted number came through the
// desktop's own intArg as 0, and clickScript's `if(!el)return` swallowed the
// miss without a sound — so the tool answered "คลิก ref 0 แล้ว", ok=1, and the
// page had not been touched. The model read the page, saw nothing had changed,
// clicked again, reopened the page, clicked again: six rounds of a loop whose
// exit was a sentence nobody was saying. The type coercion is fixed too, and it
// is the smaller half. A tool that reports success for work it did not do turns
// every bug upstream of it into a loop.
//
// **Reported before the click, not after.** A click can navigate, and a
// navigation tears down the document that would have sent the message. Sending
// first means the report is about what was aimed at rather than about what
// happened, which is the half that was missing — whether the action landed at
// all is what `read` is for, and the guidance already says to read afterwards.
func aetoxActJS() string {
	return `
  function aetoxReport(token,ref,el){
    ` + bridgePost + `(JSON.stringify({__aetox:"act",token:token,url:location.href,ref:ref,
      found:!!el,
      tag:el?el.tagName.toLowerCase():"",
      label:el?String(el.innerText||el.value||el.getAttribute('aria-label')||'').trim().replace(/\s+/g,' ').slice(0,80):""}));
  }
`
}

// clickScript clicks the element tagged with the given ref (see textScript).
//
// aetoxFind rather than document.querySelector, because textScript now tags
// nodes inside shadow roots and same-origin frames as well. A ref handed out by
// a read this tool could not then act on would be worse than not handing it out.
func clickScript(token string, ref int) string {
	tok, _ := json.Marshal(token)
	return fmt.Sprintf(`(function(){%s%s
  var el=aetoxFind(%d);
  aetoxReport(%s,%d,el);
  if(!el)return;
  el.scrollIntoView({block:"center"});
  el.click();
})()`, aetoxScanJS, aetoxActJS(), ref, string(tok), ref)
}

// typeScript sets an input/textarea/select/contenteditable's value via the
// native setter (so React/Vue-controlled inputs pick it up) and fires
// input+change. A SELECT matches the text against option value or label
// instead of overwriting content. enter additionally presses Enter — synthetic
// keydown first (skipped requestSubmit if the page preventDefault'ed it), then
// the form's requestSubmit, because untrusted KeyboardEvents never trigger the
// browser's own implicit submission.
func typeScript(token string, ref int, text string, enter bool) string {
	encoded, _ := json.Marshal(text)
	enterJS := ""
	if enter {
		enterJS = `
  var ke={key:"Enter",code:"Enter",keyCode:13,which:13,bubbles:true,cancelable:true};
  var notHandled=el.dispatchEvent(new KeyboardEvent("keydown",ke));
  el.dispatchEvent(new KeyboardEvent("keyup",ke));
  if(notHandled&&el.form&&typeof el.form.requestSubmit==="function"){el.form.requestSubmit();}`
	}
	tok, _ := json.Marshal(token)
	return fmt.Sprintf(`(function(){%s%s
  var el=aetoxFind(%d);
  aetoxReport(%s,%d,el);
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
})()`, aetoxScanJS, aetoxActJS(), ref, string(tok), ref, encoded, enterJS)
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

	// What the last read left behind for click and type to aim at. Every read
	// reassigns refs from 1 and strips the ones before it, so "ref 11" means
	// nothing except in the numbering of the read that produced it — and a
	// FILTERED read tags only its matches, which is a short list with its own
	// numbers on a page that never changed.
	//
	// Kept because the tool could not otherwise say why a ref missed. It used
	// to answer every miss with "refs expire when the page changes", which is
	// true and was the wrong cause: the page was the same, a narrower read had
	// simply replaced the numbering underneath it.
	refMu     sync.Mutex
	refCount  int    // how many elements the last read tagged
	refFilter string // the filter that read used; "" for none
	refRead   bool   // whether any read has happened on this page yet

	pickMu sync.Mutex
	// pickToken is the token the live point-at-the-page mode will answer with;
	// empty = no mode running. Unlike textToken there is no channel waiting on
	// it — a pick arrives as an event whenever the user gets round to pointing,
	// or never. See browser_pick.go.
	pickToken string

	waitMu    sync.Mutex
	waitCh    chan bool
	waitToken string

	logMu    sync.Mutex
	logCh    chan browserLogReport
	logToken string // token browserLog is waiting on; empty = none pending

	actMu    sync.Mutex
	actCh    chan browserActResult
	actToken string // token a click/type is waiting on; empty = none pending

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
	// goneID and goneWhy remember that a page the agent was working stopped
	// existing, and who ended it, so the next thing the agent asks of the
	// browser can say so instead of answering "you have no page open" as though
	// the agent had forgotten to open one. Owner, 22 ส.ค.: a click in the
	// browser is a user-side action, and the model is meant to hear about it
	// and carry on.
	//
	// The id is half the record and not decoration. Without it this was a bare
	// bool that anything could raise, which could only answer "did something get
	// closed recently" — while the question being asked is "what happened to the
	// page I am holding". The two come apart the moment the agent opens a new
	// page: a bool set before that survives the reopen and lands on the first
	// action of a fresh, perfectly live tab. So the id is cleared the moment the
	// agent has a page again (see open), and the reason is written only by the
	// call that actually removed a tab (see closeTab).
	goneID  string
	goneWhy closeReason
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
			// The agent has a page again, so whatever happened to the last one
			// has stopped being news. Cleared HERE rather than when the message
			// is read, because a record that outlives the reopen is a sentence
			// about a tab that no longer exists, delivered against one that
			// does — which is the whole failure this pair replaced.
			h.goneID, h.goneWhy = "", 0
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
		ch <- browserSnapshot{
			Text: m.Text, Elements: m.Elements, Images: m.Images,
			ElementsTotal: m.ElementsTotal, ImagesTotal: m.ImagesTotal, BlockedFrames: m.Frames,
		}
	case "act":
		tab.actMu.Lock()
		ch := tab.actCh
		expectedToken := tab.actToken
		tab.actCh = nil
		tab.actToken = ""
		tab.actMu.Unlock()
		if ch == nil || m.Token == "" || m.Token != expectedToken || !sameOrigin(source, m.URL) {
			return
		}
		ch <- browserActResult{Found: m.Found, Ref: m.Ref, Tag: m.Tag, Label: m.Label}
	case "log":
		tab.logMu.Lock()
		ch := tab.logCh
		expectedToken := tab.logToken
		tab.logCh = nil
		tab.logToken = ""
		tab.logMu.Unlock()
		// Same three gates as "text", for the same three reasons: nothing
		// waiting, a token that is not the one this call minted, or a page
		// claiming to be somewhere it is not.
		if ch == nil || m.Token == "" || m.Token != expectedToken || !sameOrigin(source, m.URL) {
			return
		}
		ch <- browserLogReport{Kind: m.Kind, Entries: m.Log, Dropped: m.Dropped, Armed: m.Armed}
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

// CloseAllBrowserTabs destroys the native browser views this process is still
// holding, except the ones a turn that is still running is working in.
//
// Called by the frontend right after it (re)loads (App.svelte onMount). The Go
// backend outlives a webview reload — a `wails dev` Vite HMR full-reload, or a
// plain Ctrl+R — which wipes the JS-side `workbench` store without running
// BrowserPane's onDestroy, leaving the native view behind with nothing left to
// reposition or close it. It just floats, stuck at its last bounds. Sweeping
// those is what this is for, and on a genuine fresh start `a.browsers` is nil
// and it is a no-op.
//
// **The exception is new, and the reasoning it replaces was wrong.** This used
// to close everything, on the grounds that *"a freshly loaded frontend owns
// zero workbench tabs by definition"*. True of the frontend, false of the app:
// the engine keeps working across a reload, so a turn in flight can be holding
// half a dozen tabs of its own — and reloading the window killed them all
// mid-task, after which the agent walked its own list and was told, correctly
// and uselessly, that every one of them had been closed (owner, 24 ส.ค.).
//
// Only while a turn is running. With nothing working, an agent tab is as
// orphaned as any other and the sweep should take it — otherwise the leftovers
// of a turn that died would sit there for the rest of the app's life with
// nothing left to close them.
func (a *App) CloseAllBrowserTabs() {
	if a.browsers == nil {
		return
	}
	keep := map[string]bool{}
	if a.anyTurnRunning() {
		for _, id := range a.agentTabs() {
			keep[id] = true
		}
	}
	h := a.browsers
	h.mu.Lock()
	ids := make([]string, 0, len(h.tabs))
	for id := range h.tabs {
		if !keep[id] {
			ids = append(ids, id)
		}
	}
	h.mu.Unlock()
	for _, id := range ids {
		// The app tidying up after a reload, which is not the user and must not
		// be reported as one: the agent is told its page is gone, not that
		// somebody closed it.
		a.closeTab(id, closedByApp)
	}
}

// closeReason says who ended a tab.
//
// It has no zero value on purpose. A tab can stop existing for three different
// reasons and the agent needs to be told a different thing about each, so a
// close that does not say which is not a close this program knows how to
// report — and it should not compile.
//
// The rule this replaces was prose: "BrowserClose is the frontend's door, so
// every call to it is the user." True when it was written and false five weeks
// later, because a lifecycle hook in the window and a sweep in the engine both
// reached it and neither is a user. See
// docs/architecture/browser-tab-lifetime-2026-08-25.md.
type closeReason int

const (
	closedByUser  closeReason = iota + 1 // the × on the tab strip
	closedByAgent                        // the agent's own `browser tabs close`
	closedByApp                          // a sweep, a teardown, a view that died
)

// BrowserClose is the × on the tab strip, and nothing else. It is a Wails
// binding, so the window is its only caller; every close inside the engine
// names its own reason through closeTab.
func (a *App) BrowserClose(id string) {
	a.closeTab(id, closedByUser)
}

// closeTab is the close itself, with nothing said about who asked for it.
//
// Two things are true at once and they are why this is not four lines: the
// registry entry has to go NOW, so the agent stops being told about a tab it
// can no longer use, and the native window cannot go now, because destroying it
// only happens on the browser thread (backend.do posts a message). Deleting
// both together — which is what this did — meant a window whose destroy had not
// run yet was already unreachable: no id in `views`, so nothing could hide it,
// move it or try again. It just sat there, black, over the pane.
//
// So `tabs` and `agentOrder` are dropped immediately (live() reads `tabs`, so
// the tab is closed the instant this returns) and `views` is dropped by the
// queued func, after the window is actually gone.
func (a *App) closeTab(id string, why closeReason) {
	host, err := a.browserHostLazy()
	if err != nil {
		return
	}
	host.mu.Lock()
	v := host.views[id]
	_, wasOpen := host.tabs[id]
	delete(host.tabs, id)
	// Written by the call that actually removed the tab, and only that one.
	// A second pass over an id already gone writes nothing, so re-entering
	// this function cannot overwrite the reason the first pass gave — which
	// matters because closing a tab tells the window, the window drops the
	// chip, and dropping the chip used to come straight back round here. That
	// echo is a no-op now by construction rather than by a guard somebody has
	// to remember to keep.
	if wasOpen && isAgentTabID(id) {
		host.goneID, host.goneWhy = id, why
	}
	// The current tab falls back to whatever is left rather than to nothing,
	// so closing one of several does not strand the agent mid-task.
	host.agentOrder = slices.DeleteFunc(host.agentOrder, func(open string) bool { return open == id })
	if host.agentID == id {
		host.agentID = ""
		if len(host.agentOrder) > 0 {
			host.agentID = host.agentOrder[len(host.agentOrder)-1]
		}
	}
	if v == nil {
		delete(host.views, id)
	}
	host.mu.Unlock()
	if v != nil {
		host.backend.do(func() {
			v.destroy()
			host.mu.Lock()
			delete(host.views, id)
			host.mu.Unlock()
		})
	}
	// The window is told, which it never was. `workbench:open-browser` had no
	// partner, so a tab closed from this side stayed on the strip forever —
	// pointing at a native view that no longer existed, and with the pane
	// latched open (BrowserPane's `opened`) so nothing would ever re-open it.
	// The file side has said both halves all along (workbench:close-file).
	if wasOpen || v != nil {
		a.deskEvent("", "close-browser", map[string]string{"id": id})
	}
}

// BrowserGetText returns the visible text content of a tab's current page —
// this is the read-path the AI agent uses to work with the browser.
func (a *App) BrowserGetText(id string) (string, error) {
	// No filter: this is the frontend's own read of the whole page, not the
	// agent's search for one control on it.
	snap, err := a.browserSnapshot(id, "")
	if err != nil {
		return "", err
	}
	return snap.Text, nil
}

// browserSnapshot reads page text plus the interactive elements tagged by
// textScript, in one round trip. Used by BrowserGetText and browser_read.
// noteRefs records what a read left for click and type to aim at. Called on
// every read, including a filtered one, because a filtered read is the case
// that made this worth keeping.
func (t *browserTab) noteRefs(count int, filter string) {
	if t == nil {
		return
	}
	t.refMu.Lock()
	t.refCount, t.refFilter, t.refRead = count, filter, true
	t.refMu.Unlock()
}

func (t *browserTab) refs() (count int, filter string, read bool) {
	if t == nil {
		return 0, "", false
	}
	t.refMu.Lock()
	defer t.refMu.Unlock()
	return t.refCount, t.refFilter, t.refRead
}

func (a *App) browserSnapshot(id, filter string) (browserSnapshot, error) {
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
	host.onTab(id, func(v tabView, _ *browserTab) { v.eval(textScript(token, filter)) })

	select {
	case snap := <-ch:
		t.noteRefs(len(snap.Elements), filter)
		return snap, nil
	case <-time.After(5 * time.Second):
		t.textMu.Lock()
		t.textCh = nil
		t.textToken = ""
		t.textMu.Unlock()
		return browserSnapshot{}, fmt.Errorf("page did not respond (still loading?)")
	}
}

// browserActOn runs one ref-aimed script and collects the report it sends
// before acting. answered is false when the page never said anything, which is
// a third outcome and not a failure: the report is a courtesy the page performs
// and a page can be busy, gone, or refusing to run scripts at all.
func (a *App) browserActOn(id string, build func(token string) string) (res browserActResult, answered bool, err error) {
	host, err := a.browserHostLazy()
	if err != nil {
		return browserActResult{}, false, err
	}
	t := host.tab(id)
	if t == nil {
		return browserActResult{}, false, fmt.Errorf("no browser tab %q", id)
	}

	token := newMessageToken()
	ch := make(chan browserActResult, 1)
	t.actMu.Lock()
	t.actCh = ch
	t.actToken = token
	t.actMu.Unlock()

	host.onTab(id, func(v tabView, _ *browserTab) { v.eval(build(token)) })

	select {
	case r := <-ch:
		return r, true, nil
	// Short, because the report is sent before the click rather than after it:
	// nothing here is waiting on a page to finish loading or a handler to run.
	case <-time.After(2 * time.Second):
		t.actMu.Lock()
		t.actCh = nil
		t.actToken = ""
		t.actMu.Unlock()
		return browserActResult{}, false, nil
	}
}

// browserClickRef clicks the element tagged with ref by the most recent
// browser_read snapshot (see textScript), and reports what it was aimed at.
func (a *App) browserClickRef(id string, ref int) (browserActResult, bool, error) {
	return a.browserActOn(id, func(token string) string { return clickScript(token, ref) })
}

// BrowserClickRef is the binding the frontend was generated against. It keeps
// its signature because the frontend has no use for the report; the tool, which
// has to tell the model whether the ref matched anything, calls the one above.
func (a *App) BrowserClickRef(id string, ref int) error {
	_, _, err := a.browserClickRef(id, ref)
	return err
}

// browserTypeRef sets an input/textarea/select/contenteditable's value, tagged
// with ref by the most recent browser_read snapshot (see textScript), and
// reports what it was aimed at. enter presses Enter afterwards (for forms with
// no submit button).
func (a *App) browserTypeRef(id string, ref int, text string, enter bool) (browserActResult, bool, error) {
	return a.browserActOn(id, func(token string) string { return typeScript(token, ref, text, enter) })
}

// BrowserTypeRef is the binding the frontend was generated against; see
// BrowserClickRef for why it keeps its shape.
func (a *App) BrowserTypeRef(id string, ref int, text string, enter bool) error {
	_, _, err := a.browserTypeRef(id, ref, text, enter)
	return err
}
