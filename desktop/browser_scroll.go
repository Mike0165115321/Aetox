package main

// Moving down a page that has not finished existing yet.
//
// Owner, 24 ส.ค., going through what the browser could not do: *"scroll โคตร
// สำคัญแต่ผมดันลืม"*. He is right, and it is the reason a whole family of pages
// was unreadable rather than merely awkward: a feed, a search result list, a
// channel's videos, anything with "load more" at the bottom. `read` returns what
// is in the document, and on those pages the document is one screen deep until
// something scrolls. So the agent was not reading a short page — it was reading
// the first screen of a long one and being given no way to know.
//
// **No report, on purpose.** The obvious version answers with a scroll position
// and a document height so the model can decide whether to go again. That needs
// a round trip and a result type, and it duplicates something the model already
// has a better instrument for: `read`. The loop that already exists — read, act,
// read again — answers "did more arrive" exactly, in the words of the page
// itself, rather than in a number that stands for them. So this acts and says so,
// and the next `read` is the measurement.

import (
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

type browserScrollSkill struct{ app *App }

// scrollSettle is how long the page is given to react before the tool returns.
//
// Lazy loading is fetch-then-render, so returning the instant the scroll is
// queued would hand the model a `read` of the page as it was. Short, because
// this is not `wait` — a page that needs seconds needs `wait`, which exists and
// says what it is waiting for.
const scrollSettle = 700 * time.Millisecond

// maxScrollScreens caps how far one call may travel.
//
// The cap is a time budget, not a distance one: every screen costs a settle, so
// ten of them is seven seconds inside a single tool call. That is still far
// cheaper than ten calls — ten model round trips, ten lots of tokens — which is
// the whole reason `screens` exists (§176). Past ten, `bottom` is the action
// that was wanted.
const maxScrollScreens = 10

// scrollScreens reads the argument into something a page can be moved by.
//
// **The signature asks for it and the runtime does not insist**, and the gap is
// deliberate rather than sloppy. The tool block says `screens` plainly and the
// guidance says to always send it, because a call that names its distance is the
// entire saving here: a model that omits it scrolls one screen, calls again,
// and again, and the schema field it ignored has cost every request in the
// session for nothing.
//
// But refusing an omitted one would cost the very thing it is trying to save.
// A refusal is a round trip — the model reads the error, calls again — so
// enforcing "always say how far" would burn one round trip to save several,
// on the exact action whose reason for existing is that round trips are the
// expensive part. Teaching is free and happens once; refusing is not free and
// happens every time a model forgets.
//
// So zero and nonsense both become one screen, which is what `scroll down` has
// always meant. Over the cap is clamped and SAID, because a caller told nothing
// would believe it had travelled twenty.
func scrollScreens(n int) (screens int, clamped bool) {
	if n < 1 {
		return 1, false
	}
	if n > maxScrollScreens {
		return maxScrollScreens, true
	}
	return n, false
}

// scrollScript moves the window, and moves the right thing.
//
// `window.scrollBy` is wrong on the many apps whose real scroller is a div with
// overflow:auto — the window never scrolls and the call silently does nothing.
// So the tallest scrollable element is found first and the window is the
// fallback, which is the same answer on an ordinary page and the only working
// one on an app.
func scrollScript(to string, screens int) string {
	move := `el===document.scrollingElement?window.innerHeight*0.9:el.clientHeight*0.9`
	switch to {
	case "up":
		move = "-(" + move + ")"
	// The two jumps ignore `screens`, and there is nothing to reconcile: "go to
	// the bottom" five times is "go to the bottom".
	case "top":
		return scrollWrap(`el.scrollTo({top:0,behavior:"instant"})`)
	case "bottom":
		return scrollWrap(`el.scrollTo({top:el.scrollHeight,behavior:"instant"})`)
	}
	// Several presses with a wait between them, never one big jump.
	//
	// A jump of five screens is not five screens of scrolling on the pages this
	// action exists for. Lazy content is fetch-then-render, so the document is
	// only as deep as what has rendered — jump past the end of it and the
	// browser stops at the end of it, and the four screens that would have
	// loaded never do. Pressing five times with a settle between is what a
	// person does, and it is the only version that reaches.
	//
	// The distance is recomputed on each press rather than measured once,
	// because the scroller's own height can change as content arrives.
	//
	// **It does not stop early**, and that is deliberate rather than an
	// oversight. The obvious guard is to give up when the position stops
	// changing — and on a feed that is exactly wrong: hitting the bottom is the
	// event that triggers the next page, so the press that moved nothing is
	// routinely the one that makes the next press possible. On a genuinely short
	// page the extra presses move nothing and cost nothing but the wait.
	return scrollWrap(fmt.Sprintf(`(function(){
    var n=%d, i=0;
    (function press(){
      el.scrollBy({top:%s,behavior:"instant"});
      if(++i>=n)return;
      setTimeout(press,%d);
    })();
  })()`, screens, move, scrollSettle.Milliseconds()))
}

// scrollWrap finds the element that actually scrolls, then runs the move on it.
func scrollWrap(action string) string {
	return `(function(){
  var best=document.scrollingElement||document.body, bestOver=0;
  var nodes=document.querySelectorAll("*");
  for(var i=0;i<nodes.length && i<4000;i++){
    var n=nodes[i], s=getComputedStyle(n);
    if(!/(auto|scroll)/.test(s.overflowY)) continue;
    var over=n.scrollHeight-n.clientHeight;
    if(over>bestOver && n.clientHeight>200){best=n;bestOver=over;}
  }
  var docOver=(document.scrollingElement||document.body).scrollHeight-window.innerHeight;
  var el=(docOver>=bestOver)?(document.scrollingElement||document.body):best;
  ` + action + `;
})()`
}

// scrollWhere are the directions worth having. Not pixels: a model asked for a
// number of pixels picks one, and a number picked without seeing the page is a
// guess. top/bottom are the two jumps that actually get asked for.
var scrollWhere = map[string]string{
	"down":   "down",
	"up":     "up",
	"top":    "to the top",
	"bottom": "to the bottom",
}

// scrollSaid names how far this call went, in the unit it was asked in.
//
// **Screens, and that is the whole of the change §176 makes to the original
// rule.** "No pixels" was right and stays right; what was wrong was reading it
// as "no distance at all". A screen is a unit a model can reason about without
// seeing the page — it read the first one, there is obviously more, go three
// more — and it was already the unit this action moved in. Only the count was
// fixed at one, which turned a ten-screen feed into ten round trips.
func scrollSaid(to string, screens int) string {
	where := scrollWhere[to]
	if to != "down" && to != "up" {
		return where
	}
	if screens == 1 {
		return where + " one screen"
	}
	return fmt.Sprintf("%s %d screens", where, screens)
}

func (s *browserScrollSkill) scroll(to string, screensArg int) (skill.Output, error) {
	start := time.Now()
	to = strings.ToLower(strings.TrimSpace(to))
	if to == "" {
		to = "down"
	}
	screens, clamped := scrollScreens(screensArg)
	_, ok := scrollWhere[to]
	said := scrollSaid(to, screens)
	out := skill.Output{Name: "browser_scroll", Command: "browser scroll " + to}
	if !ok {
		err := fmt.Errorf("browser scroll %q is not one of down, up, top, bottom", to)
		out.Content, out.Stderr = err.Error(), err.Error()
		return out, err
	}

	id, err := s.app.agentTab()
	if err != nil {
		out.Content, out.Stderr = err.Error(), err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}
	// The whole journey, so both the arrow and the wait cover it rather than the
	// first press of it.
	travel := time.Duration(screens) * scrollSettle

	// The arrow before the move, so it is on screen for the whole of it. It is
	// position:fixed, so the page travels underneath it rather than carrying it
	// along — which is what makes it read as a direction rather than as a thing
	// stuck to the document. Held for the length of the scroll: an arrow that
	// left after 1.6s of a seven-second scroll would say the page had stopped
	// while it was still going.
	s.app.markPageScroll(id, to, int(travel.Milliseconds()))
	s.app.browserEval(string(id), scrollScript(to, screens))
	// One settle per press, plus the last one: the script waits between presses
	// and this covers all of them and the tail. Sleeping only the tail would
	// return the tool while the page was still moving.
	time.Sleep(travel)

	out.Success = true
	// Says what to do next, because scrolling on its own tells the model
	// nothing: the page it can see is still the one from the last read.
	out.Content = "เลื่อนหน้า " + said + " แล้ว อ่านใหม่เพื่อดูว่ามีอะไรโหลดเพิ่ม — refs จากการ read ก่อนหน้าใช้ไม่ได้แล้ว"
	// Said rather than silently done. A page shorter than the distance asked for
	// simply stops, and a caller that was told it travelled five screens has no
	// way to tell that from having travelled two — so the next read looking
	// short reads as "no more content" instead of "you were already at the end".
	if screens > 1 {
		out.Content += "\nถ้าหน้าสั้นกว่านั้น ก็คือถึงล่างสุดไปแล้วก่อนครบ " + fmt.Sprintf("%d จอ", screens)
	}
	if clamped {
		out.Content += fmt.Sprintf("\n(ขอมาเกิน %d จอ เลื่อนให้ %d จอ — ถ้าอยากถึงล่างสุดใช้ to=bottom)", maxScrollScreens, screens)
	}
	out.RawOutput = out.Content
	out.DurationMs = time.Since(start).Milliseconds()
	return out, nil
}
