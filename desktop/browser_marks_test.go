package main

// ลูกศรและวงแหวนบนหน้าเว็บ, checked without a browser.
//
// These are scripts, so what can be pinned here is what the script SAYS, and
// the three things it says are the three rules the layer would be wrong
// without: it takes the previous mark down before putting one up, it mounts
// clear of the page's own tree, and it asks the machine about motion rather
// than the switch. Each of those failing is silent in a browser — a stack of
// stale rings, a mark that drifts, an animation on a machine that asked for
// none — so they are worth a test that runs in a second.

import (
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

func TestMarkScriptsClearBeforeTheyDraw(t *testing.T) {
	for name, js := range map[string]string{
		"click":  markClickScript(3),
		"scroll": markScrollScript("down", 0),
	} {
		mount := strings.Index(js, "function aetoxMarkMount")
		clear := strings.Index(js, "aetoxMarkClear();\n    var root")
		if mount < 0 || clear < 0 || clear < mount {
			t.Errorf("%s: the mount does not begin by clearing what is already there", name)
		}
	}
}

// documentElement, never a node inside the page. A transform or a filter
// anywhere up the ancestor chain makes a new containing block, and
// position:fixed inside one stops being fixed to the viewport — the mark then
// drifts with the page instead of pointing at it. Plenty of sites put
// transform:translateZ(0) on body, so body is not far enough up.
func TestMarksMountAtTheRootAndStayFixed(t *testing.T) {
	for name, js := range map[string]string{
		"click":  markClickScript(1),
		"scroll": markScrollScript("up", 0),
	} {
		if !strings.Contains(js, "var root=document.documentElement;") {
			t.Errorf("%s: does not mount on the document element", name)
		}
		if !strings.Contains(js, "position:fixed") {
			t.Errorf("%s: the mark is not fixed to the viewport", name)
		}
		if !strings.Contains(js, "pointer-events:none") {
			t.Errorf("%s: the mark can be clicked, and would eat the user's click", name)
		}
	}
}

// The switch says what Aetox would LIKE to draw; the machine says whether it
// moves. Both scripts have to ask the page itself, because the page is the one
// with the user's setting.
func TestMarksAskThePageAboutMotion(t *testing.T) {
	for name, js := range map[string]string{
		"click":  markClickScript(2),
		"scroll": markScrollScript("bottom", 0),
	} {
		if !strings.Contains(js, "prefers-reduced-motion: reduce") {
			t.Errorf("%s: never asks the machine whether it may move", name)
		}
		if !strings.Contains(js, "aetoxMarkQuiet()") {
			t.Errorf("%s: asks and does not act on the answer", name)
		}
	}
}

// A jump gets two chevrons and a screen at a time gets one, so the four
// directions read as two distances without a word being written — the words are
// the action bar's job, on a page whose language is not the app's.
func TestScrollMarkPointsTheRightWay(t *testing.T) {
	for _, tc := range []struct {
		to      string
		place   string
		turn    string
		chevron string
	}{
		{"down", "bottom:44px", "rotate(45deg)", "i<1"},
		{"bottom", "bottom:44px", "rotate(45deg)", "i<2"},
		{"up", "top:44px", "rotate(-135deg)", "i<1"},
		{"top", "top:44px", "rotate(-135deg)", "i<2"},
	} {
		js := markScrollScript(tc.to, 0)
		for _, want := range []string{tc.place, tc.turn, tc.chevron} {
			if !strings.Contains(js, want) {
				t.Errorf("scroll %s: script is missing %q", tc.to, want)
			}
		}
	}
}

// A ref that matches nothing leaves the page clean rather than leaving the last
// mark up — a ring still pointing at the button from the previous action is a
// ring that lies about which one is being pressed.
func TestClickMarkGivesUpQuietlyOnAMissingRef(t *testing.T) {
	js := markClickScript(9)
	if !strings.Contains(js, "if(!el){aetoxMarkClear();return;}") {
		t.Error("a ref matching nothing does not clear the previous mark")
	}
	if !strings.Contains(js, `behavior:"instant"`) {
		t.Error("the element is centred without instant behaviour, so the rect can be read mid-scroll")
	}
}

// The switch gates DRAWING and nothing else. Clearing has to run either way, or
// turning the layer off mid-run would leave the last mark on the page for the
// rest of its life.
func TestPageMarksSwitchGatesDrawingOnly(t *testing.T) {
	on := &App{cfg: config.Config{}}
	if !on.pageMarksOn() {
		t.Error("the layer ships on and reports off")
	}
	off := &App{cfg: config.Config{BusyPageMarksOff: true}}
	if off.pageMarksOn() {
		t.Error("the layer was switched off and reports on")
	}
	// Neither door panics with no browser host behind it, which is the state
	// every one of this package's tests constructs an App in — and, more to the
	// point, the state a session is in before anybody has opened a page.
	off.markPageClick(AgentTabID("web-agent-1"), 1)
	off.markPageScroll(AgentTabID("web-agent-1"), "down", 0)
	off.clearPageMarks(AgentTabID("web-agent-1"))
	on.markPageClick(AgentTabID("web-agent-1"), 1)
	on.markPageScroll(AgentTabID("web-agent-1"), "down", 0)
}

// Nothing of Aetox's own in the photograph. The ring sits directly over the
// control it points at, and a model handed that picture has no way to know the
// circle is not part of the site.
func TestClearScriptRemovesTheMarkByID(t *testing.T) {
	js := clearMarksScript("tok-1")
	if !strings.Contains(js, markElementID) {
		t.Errorf("the clear does not name the mark it is meant to remove")
	}
	if strings.Contains(js, "createElement") {
		t.Error("the clear draws something")
	}
	// The report is what makes this one different from every other mark script,
	// and it is what capture waits on. Without it the only thing between a stale
	// ring and the photograph is a sleep that was put there for the raise.
	if !strings.Contains(js, "aetoxReport(\"tok-1\",0,null)") {
		t.Errorf("the clear never says it is done:\n%s", js)
	}
	// Reported AFTER the removal, or it would be answering for work it had not
	// done yet.
	if strings.Index(js, "removeChild") > strings.Index(js, "aetoxReport(\"tok-1\"") {
		t.Error("it reports before it removes")
	}
}
