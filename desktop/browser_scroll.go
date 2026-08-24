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

// scrollScript moves the window, and moves the right thing.
//
// `window.scrollBy` is wrong on the many apps whose real scroller is a div with
// overflow:auto — the window never scrolls and the call silently does nothing.
// So the tallest scrollable element is found first and the window is the
// fallback, which is the same answer on an ordinary page and the only working
// one on an app.
func scrollScript(to string) string {
	move := `el===document.scrollingElement?window.innerHeight*0.9:el.clientHeight*0.9`
	switch to {
	case "up":
		move = "-(" + move + ")"
	case "top":
		return scrollWrap(`el.scrollTo({top:0,behavior:"instant"})`)
	case "bottom":
		return scrollWrap(`el.scrollTo({top:el.scrollHeight,behavior:"instant"})`)
	}
	return scrollWrap(fmt.Sprintf(`el.scrollBy({top:%s,behavior:"instant"})`, move))
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
// number picks one, and a number picked without seeing the page is a guess. A
// screen at a time is what a person does, and top/bottom are the two jumps that
// actually get asked for.
var scrollWhere = map[string]string{
	"down":   "down one screen",
	"up":     "up one screen",
	"top":    "to the top",
	"bottom": "to the bottom",
}

func (s *browserScrollSkill) scroll(to string) (skill.Output, error) {
	start := time.Now()
	to = strings.ToLower(strings.TrimSpace(to))
	if to == "" {
		to = "down"
	}
	said, ok := scrollWhere[to]
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
	// The arrow before the move, so it is on screen for the whole of it. It is
	// position:fixed, so the page travels underneath it rather than carrying it
	// along — which is what makes it read as a direction rather than as a thing
	// stuck to the document.
	s.app.markPageScroll(id, to)
	s.app.browserEval(string(id), scrollScript(to))
	time.Sleep(scrollSettle)

	out.Success = true
	// Says what to do next, because scrolling on its own tells the model
	// nothing: the page it can see is still the one from the last read.
	out.Content = "เลื่อนหน้า " + said + " แล้ว อ่านใหม่เพื่อดูว่ามีอะไรโหลดเพิ่ม — refs จากการ read ก่อนหน้าใช้ไม่ได้แล้ว"
	out.RawOutput = out.Content
	out.DurationMs = time.Since(start).Milliseconds()
	return out, nil
}
