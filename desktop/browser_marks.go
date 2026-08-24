package main

// ลูกศรและวงแหวนบนหน้าเว็บ — the one busy-signal layer the window cannot draw.
//
// The other three live in territory the frontend owns: the panel's border, the
// strip under the toolbar, the mark on a tab chip. This one does not exist
// there at all, and the reason is the workbench browser's shape. A tab is a
// native WebView2 window glued over the pane by BrowserSetBounds, so it
// composites ABOVE the app's own webview, and everything the app draws over
// that rect is behind it. A ring pointing at a button would be a ring pointing
// at the back of a window.
//
// So the only way into those pixels is the way `read` already goes: a script
// evaluated inside the page. This file is that script, and it obeys three rules
// that are not style choices.
//
//   - **Cleared before `capture`.** BrowserCapturePNG photographs the real
//     page, and the click ring sits directly over the control it is pointing
//     at. A picture the model then reads would have a bright circle drawn
//     across the very thing it was looking for.
//   - **Mounted on the document element, never inside the page's own tree.**
//     A `transform` or a `filter` anywhere up the ancestor chain makes a new
//     containing block, and `position:fixed` inside one is no longer fixed to
//     the viewport — the mark drifts with the page instead of pointing at it.
//     documentElement rather than body for the same reason one step further:
//     plenty of sites put `transform:translateZ(0)` on body to force a
//     compositing layer, which is exactly the hazard. It is also where
//     pickScript mounts, so the two things Aetox draws on somebody else's page
//     agree with each other.
//   - **The previous one removed before the next is drawn.** Exactly what
//     textScript does with stale data-aetox-ref attributes, for the same
//     reason: rapid clicks and scrolls would otherwise leave a page stacked
//     with rings and arrows for actions that finished a second ago, which
//     reads as the agent still doing all of them at once.
//
// **Never a timer, always an action.** Nothing here draws on a clock. A mark is
// made by a click or a scroll and takes itself off shortly after, which is not
// the same thing: a mark on screen means an action happened, and no action
// means an empty page.
//
// **prefers-reduced-motion outranks the switch.** The switch says what Aetox
// would like to draw; the machine says whether it moves. A page whose user has
// asked for no motion gets the mark and none of the travel, which is the same
// answer the stylesheet gives every other animation in the app.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// markElementID is the id the mark carries, and the whole of its identity: the
// clear reads it, the mount overwrites whatever wore it before, and the delayed
// removal checks it is still holding the mark it was given before taking
// anything away.
const markElementID = "__aetox-busy-mark"

// markAccent is the blue the pick overlay defaults to, spelled the same way for
// the same reason: these are the two things Aetox ever draws on somebody else's
// page, and a user who has seen one should recognise the other.
//
// Not read from the user's theme, unlike pickScript's, and that is a real
// difference rather than an oversight. Pick is started by a click in the
// window, so the frontend is right there to hand its accent down. A mark is
// made from inside a tool call on the engine's goroutine, where there is no
// frontend anywhere in the call chain to ask.
const markAccent = "#378add"

// markLifetimeMS is how long a mark stays before it takes itself off.
//
// Long enough to be caught by somebody who was looking elsewhere when it
// appeared, short enough to be gone before the next action in an ordinary
// sequence: a click already sleeps 300ms afterwards and a scroll 700ms, so a
// run of them keeps a live mark on screen continuously without ever leaving a
// stale one beside a fresh one.
//
// It is the floor rather than the rule, because one action is no longer one
// move: `scroll` takes a number of screens now (§176), and an arrow that left
// after 1.6s of a seven-second scroll would say the page had stopped while it
// was still going. A caller that knows its action is long says so.
const markLifetimeMS = 1600

// aetoxMarkJS is the half both marks share: take the old one down, put the new
// one up, and arrange for it to leave.
//
// Styles are set through cssText rather than setAttribute("style", …) because
// the CSSOM path is not what a page's Content-Security-Policy governs — the
// same reason pickScript has always been able to draw on sites that forbid
// inline styles. Motion goes through element.animate() for the same reason one
// step further: a @keyframes rule needs a <style> element, which style-src can
// refuse, while the Web Animations API needs nothing from the page at all.
func aetoxMarkJS(lifeMS int) string {
	if lifeMS < markLifetimeMS {
		lifeMS = markLifetimeMS
	}
	return fmt.Sprintf(`
  var AETOX_MARK=%q, AETOX_ACC=%q;
  function aetoxMarkClear(){
    var old=document.getElementById(AETOX_MARK);
    if(old&&old.parentNode)old.parentNode.removeChild(old);
  }
  function aetoxMarkQuiet(){
    try{return !!(window.matchMedia&&window.matchMedia("(prefers-reduced-motion: reduce)").matches);}
    catch(e){return false;}
  }
  function aetoxMarkMount(el,css){
    aetoxMarkClear();
    var root=document.documentElement;
    if(!root)return null;
    el.id=AETOX_MARK;
    el.style.cssText="position:fixed;z-index:2147483601;pointer-events:none;box-sizing:border-box;margin:0;padding:0;"+css;
    root.appendChild(el);
    var quiet=aetoxMarkQuiet();
    if(!quiet&&el.animate){
      try{el.animate([{opacity:0},{opacity:1}],{duration:120,easing:"ease-out"});}catch(e){}
    }
    setTimeout(function(){
      /* Only ever takes down the mark it put up. A later action has already
         cleared this one and hung its own under the same id, and a timer that
         fired blind would be deleting somebody else's. */
      if(document.getElementById(AETOX_MARK)!==el)return;
      if(quiet||!el.animate){if(el.parentNode)el.parentNode.removeChild(el);return;}
      try{
        var out=el.animate([{opacity:1},{opacity:0}],{duration:220,easing:"ease-in"});
        out.onfinish=function(){if(el.parentNode)el.parentNode.removeChild(el);};
      }catch(e){if(el.parentNode)el.parentNode.removeChild(el);}
    },%d);
    return el;
  }
`, markElementID, markAccent, lifeMS)
}

// markClickScript draws a ring around the element a click is about to land on.
//
// **Before the click, not after**, and centred first. clickScript itself calls
// scrollIntoView before pressing, so a ring measured beforehand would be drawn
// where the button used to be. Doing the centring here means the rect this
// measures is the rect the click hits, and clickScript's own call then has
// nothing left to move. behavior:"instant" rather than the default, because a
// site with scroll-behavior:smooth in its CSS would otherwise still be
// travelling when the rect is read.
//
// Drawing before also means a click that navigates takes its own ring down with
// the document, which is right: the page it was pointing at is gone.
func markClickScript(ref int) string {
	return fmt.Sprintf(`(function(){%s%s
  var el=aetoxFind(%d);
  if(!el){aetoxMarkClear();return;}
  try{el.scrollIntoView({block:"center",behavior:"instant"});}catch(e){}
  var r=el.getBoundingClientRect();
  if(r.width<=0&&r.height<=0){aetoxMarkClear();return;}
  var pad=6;
  var d=document.createElement("div");
  var mounted=aetoxMarkMount(d,
    "left:"+(r.left-pad)+"px;top:"+(r.top-pad)+"px;"+
    "width:"+(r.width+pad*2)+"px;height:"+(r.height+pad*2)+"px;"+
    "border:2px solid "+AETOX_ACC+";border-radius:8px;"+
    "box-shadow:0 0 0 3px "+AETOX_ACC+"33, 0 0 14px 2px "+AETOX_ACC+"66;");
  if(mounted&&d.animate&&!aetoxMarkQuiet()){
    /* Closing in rather than pulsing. A ring that grows and shrinks forever
       says "still going", and this one is a single press that has already
       happened: it arrives a little wide and settles onto the control. */
    try{d.animate([{transform:"scale(1.14)"},{transform:"scale(1)"}],{duration:260,easing:"cubic-bezier(.2,.8,.3,1)"});}catch(e){}
  }
})()`, aetoxScanJS, aetoxMarkJS(markLifetimeMS), ref)
}

// markScrollScript draws an arrow in the direction the page is about to move,
// and holds it for as long as the move will take.
//
// Two chevrons for the jumps (top, bottom) and one for a screen at a time, so
// the four directions read as two distances without a word being written. The
// words are the action bar's job, and this mark is drawn on a page whose
// language is not the app's.
func markScrollScript(to string, holdMS int) string {
	up := to == "up" || to == "top"
	// The chevron is a square wearing two of its four borders, turned. Built
	// this way rather than as an SVG because innerHTML is the one thing a page
	// with Trusted Types enforced refuses outright, and a mark that vanishes on
	// exactly the strictest sites is a mark nobody can rely on.
	turn, place, travel := "rotate(45deg)", "bottom:44px", "12px"
	if up {
		turn, place, travel = "rotate(-135deg)", "top:44px", "-12px"
	}
	count := 1
	if to == "top" || to == "bottom" {
		count = 2
	}
	return fmt.Sprintf(`(function(){%s
  var box=document.createElement("div");
  var mounted=aetoxMarkMount(box,
    "left:0;right:0;%s;display:flex;flex-direction:column;align-items:center;gap:4px;"+
    "filter:drop-shadow(0 0 8px "+AETOX_ACC+"88);");
  if(!mounted)return;
  for(var i=0;i<%d;i++){
    var c=document.createElement("div");
    c.style.cssText="width:20px;height:20px;box-sizing:border-box;"+
      "border-right:5px solid "+AETOX_ACC+";border-bottom:5px solid "+AETOX_ACC+";"+
      "transform:%s;opacity:"+(i===0?"1":".55");
    box.appendChild(c);
  }
  if(box.animate&&!aetoxMarkQuiet()){
    /* One trip in the direction of travel, then still. The page is moving
       underneath it; the arrow says which way, once. */
    try{box.animate([{transform:"translateY(0)"},{transform:"translateY(%s)"}],
      {duration:420,easing:"cubic-bezier(.3,.7,.4,1)"});}catch(e){}
  }
})()`, aetoxMarkJS(holdMS), place, count, turn, travel)
}

// clearMarksScript takes down whatever is up, draws nothing, and says when it
// is done.
//
// **The report is the whole point of this one.** Every other mark script is
// fire and forget, which is right for a courtesy nobody waits on. This one has
// a caller that genuinely cannot proceed without it: `capture` photographs the
// real page, and a ring left standing goes into the picture the model then
// reads, with nothing to tell it the circle is not part of the site.
//
// Ordering by queue is not enough to prevent that, which is the part worth
// writing down. Both this and the screenshot go through host.onTab, so they are
// ENQUEUED in order — but v.eval is ExecuteScript, which hands the script to the
// page and returns; the page runs it whenever its own thread next gets to it.
// So "queued first" says nothing about "finished first", and the only thing
// standing between a stale ring and the photograph was a 400ms sleep put there
// for something else entirely. A sleep is not an ordering primitive; it is a
// guess that has been right so far.
//
// It reuses aetoxActJS's envelope rather than inventing a second one, so this
// rides the same token/channel/timeout the click report has always used
// (browserActOn). ref 0 and a null element: nothing was aimed at.
func clearMarksScript(token string) string {
	tok, _ := json.Marshal(token)
	return fmt.Sprintf(`(function(){%s
  var old=document.getElementById(%q);
  if(old&&old.parentNode)old.parentNode.removeChild(old);
  aetoxReport(%s,0,null);
})()`, aetoxActJS(), markElementID, string(tok))
}

// pageMarksOn is the switch, read at the moment of drawing rather than held.
//
// a.cfg is the config a new chat is born with, and SetBusyLayer writes every
// live conversation and this field together, so all of them agree and any one
// of them is the answer. This one is chosen because a tool call does not know
// which conversation it belongs to, and the honest reading of the setting is
// that it is a fact about the window rather than about a chat.
func (a *App) pageMarksOn() bool { return !a.cfg.BusyPageMarksOff }

// markPageClick and markPageScroll are the two doors, and both are no-ops when
// the layer is off — the caller does not check, so a caller cannot forget to.
//
// Fire and forget: browserEval queues the script on the thread that owns
// webviews and does not wait for it, so a page that is busy, gone, or refusing
// to run scripts costs the action nothing. A mark is a courtesy, and a courtesy
// must never be something a tool call can fail on.
func (a *App) markPageClick(id AgentTabID, ref int) {
	if !a.pageMarksOn() {
		return
	}
	a.browserEval(string(id), markClickScript(ref))
}

func (a *App) markPageScroll(id AgentTabID, to string, holdMS int) {
	if !a.pageMarksOn() {
		return
	}
	a.browserEval(string(id), markScrollScript(strings.ToLower(strings.TrimSpace(to)), holdMS))
}

// clearPageMarks runs whether or not the layer is on, and that is deliberate.
// The switch decides whether a mark is DRAWN; one already on the page has to
// come off regardless, or turning the layer off mid-run would leave the last
// mark sitting there for the rest of that page's life.
//
// It WAITS, unlike the two that draw. browserActOn bounds that wait at two
// seconds and treats silence as a third outcome rather than a failure — a page
// that is busy, gone, or refusing to run scripts simply never answers, and the
// photograph is still taken. Every return value is dropped on purpose: there is
// nothing a caller could do differently with any of them, and a capture that
// refused because a mark could not be confirmed gone would be a courtesy that
// had grown into a gate.
func (a *App) clearPageMarks(id AgentTabID) {
	_, _, _ = a.browserActOn(string(id), func(token string) string {
		return clearMarksScript(token)
	})
}
