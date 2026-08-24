package main

// How far one scroll travels (§176).
//
// Owner, 24 ส.ค., after §173.2 shipped one screen per call: *"ผมว่าสกอ ให้ AI
// เลือกความเร็วได้ด้วยดีมั้ย"*, and then the clarification that settles what
// this is: *"หมายถึงเลื่อน ระยะสกออ่ะครับ"* — distance, not animation speed.
//
// The original rule stays: no pixels. What was wrong was reading it as no
// distance at all, which turned a ten-screen feed into ten round trips.

import (
	"strings"
	"testing"
	"time"
)

func TestScrollScreensReadsTheArgumentForgivingly(t *testing.T) {
	for _, tc := range []struct {
		in      int
		want    int
		clamped bool
	}{
		// Omitted, and every nonsense that arrives as a zero, is one screen.
		// An optional refinement with a good default must not be able to fail
		// the action it refines.
		{0, 1, false},
		{-4, 1, false},
		{1, 1, false},
		{7, 7, false},
		{maxScrollScreens, maxScrollScreens, false},
		// Over the cap is clamped, and the caller is told: one that believed it
		// had gone twenty screens would read the next short page as the end.
		{40, maxScrollScreens, true},
	} {
		got, clamped := scrollScreens(tc.in)
		if got != tc.want || clamped != tc.clamped {
			t.Errorf("scrollScreens(%d) = %d/%v, want %d/%v", tc.in, got, clamped, tc.want, tc.clamped)
		}
	}
}

// Several presses with a wait between them, never one jump.
//
// This is the whole reason the feature is not `scrollBy(5 * screenHeight)`. On
// the pages this action exists for the document is only as deep as what has
// rendered, so a jump past the end of it stops at the end of it and the four
// screens that would have loaded never do.
func TestScrollPressesOncePerScreenWithAWaitBetween(t *testing.T) {
	js := scrollScript("down", 5)
	if !strings.Contains(js, "var n=5") {
		t.Errorf("the script does not carry the count:\n%s", js)
	}
	if !strings.Contains(js, "setTimeout(press,") {
		t.Error("the presses are not spaced out, so lazy content cannot load between them")
	}
	if !strings.Contains(js, "scrollBy") {
		t.Error("it does not move by a screen at a time")
	}
	// The distance is inside the loop, so a scroller whose height changes as
	// content arrives is measured again rather than once.
	press := strings.Index(js, "function press()")
	move := strings.Index(js, "innerHeight*0.9")
	if press < 0 || move < 0 || move < press {
		t.Error("the distance is measured once, outside the loop")
	}
}

// It does not stop early, and that is deliberate. Hitting the bottom is the
// event that triggers the next page on a feed, so the press that moved nothing
// is routinely the one that makes the next press possible.
func TestScrollDoesNotGiveUpWhenAPressMovesNothing(t *testing.T) {
	js := scrollScript("down", 4)
	for _, bail := range []string{"scrollTop===", "scrollTop ==", "if(before"} {
		if strings.Contains(js, bail) {
			t.Errorf("the script gives up when a press moves nothing (%q), which is exactly the press that loads a feed", bail)
		}
	}
}

// The two jumps ignore the count, and there is nothing to reconcile: "go to the
// bottom" five times is "go to the bottom".
func TestTheJumpsIgnoreTheCount(t *testing.T) {
	for _, to := range []string{"top", "bottom"} {
		js := scrollScript(to, 6)
		if strings.Contains(js, "var n=6") {
			t.Errorf("scroll %s repeated itself six times", to)
		}
		if !strings.Contains(js, "scrollTo") {
			t.Errorf("scroll %s is not a jump", to)
		}
	}
}

func TestScrollSaysHowFarItWent(t *testing.T) {
	for _, tc := range []struct {
		to      string
		screens int
		want    string
	}{
		{"down", 1, "down one screen"},
		{"down", 6, "down 6 screens"},
		{"up", 3, "up 3 screens"},
		// The count is not part of a jump's sentence, because it was not part of
		// the jump.
		{"bottom", 4, "to the bottom"},
		{"top", 1, "to the top"},
	} {
		if got := scrollSaid(tc.to, tc.screens); got != tc.want {
			t.Errorf("scrollSaid(%q, %d) = %q, want %q", tc.to, tc.screens, got, tc.want)
		}
	}
}

// A short page just stops at its end, and the caller has to be told that is
// possible — otherwise the next read looking short reads as "no more content"
// when the truth is "you were already at the end two screens ago".
func TestAMultiScreenScrollWarnsThatThePageMayHaveEnded(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	s := &browserScrollSkill{app: app}

	out, err := s.scroll("down", 3)
	if err != nil {
		t.Fatalf("scroll down 3: %v", err)
	}
	if !strings.Contains(out.Content, "3 จอ") {
		t.Errorf("the result does not name the distance: %q", out.Content)
	}
	if !strings.Contains(out.Content, "ล่างสุด") {
		t.Errorf("the result does not allow for a page that ended early: %q", out.Content)
	}

	// One screen says nothing extra. The caveat is only true of a distance that
	// could have overshot, and a note on every single scroll is a note nobody
	// reads by the third one.
	one, err := s.scroll("down", 1)
	if err != nil {
		t.Fatalf("scroll down 1: %v", err)
	}
	if strings.Contains(one.Content, "ล่างสุด") {
		t.Errorf("a one-screen scroll carried the overshoot caveat: %q", one.Content)
	}
}

func TestClampedScrollSaysSoAndPointsAtBottom(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	out, err := (&browserScrollSkill{app: app}).scroll("down", 99)
	if err != nil {
		t.Fatalf("scroll down 99: %v", err)
	}
	if !strings.Contains(out.Content, "to=bottom") {
		t.Errorf("a clamped scroll does not name the action that was wanted: %q", out.Content)
	}
}

// The wait covers the whole journey, not the first press of it. Returning while
// the page is still moving would hand the next `read` a page mid-scroll, which
// is the one thing the settle exists to prevent.
func TestTheToolWaitsForTheWholeJourney(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	s := &browserScrollSkill{app: app}

	start := time.Now()
	if _, err := s.scroll("down", 3); err != nil {
		t.Fatalf("scroll down 3: %v", err)
	}
	if took := time.Since(start); took < 3*scrollSettle {
		t.Errorf("three screens returned after %v, want at least %v", took, 3*scrollSettle)
	}
}

// The arrow has to outlive the scroll it is describing. One that left after
// 1.6s of a seven-second scroll would say the page had stopped while it was
// still going.
func TestTheArrowIsHeldForTheLengthOfTheScroll(t *testing.T) {
	long := markScrollScript("down", 6000)
	if !strings.Contains(long, "},6000);") {
		t.Errorf("a long scroll's arrow is not held for it:\n%s", long)
	}
	// And a short one still gets the floor rather than nothing.
	short := markScrollScript("down", 0)
	if !strings.Contains(short, "},1600);") {
		t.Error("a short scroll's arrow lost its default lifetime")
	}
}

// Where each half of "always say how far" is said, and where it is not.
//
// Owner, 24 ส.ค.: *"เดี๋ยว call จะหนักกว่า ตอนมันเรียก scroll อ่ะ ให้ใส่ค่ามาเลย"*. He is
// right about the risk, and it is the one that would make this feature worse
// than nothing: a model that never sends `screens` scrolls one screen at a
// time exactly as before, while every request in the session now carries a
// schema field nobody uses.
//
// So the block states the parameter and stops — it is paid on every request —
// and the argument for using it lives in Guidance, which is paid once.
func TestTheBlockNamesScreensAndTheTeachingLivesElsewhere(t *testing.T) {
	def := (&browserSkill{}).ToolDefinition()
	desc, _ := def.Function.Description, def.Function.Parameters
	if !strings.Contains(desc, "screens)") {
		t.Errorf("the signature does not ask for screens:\n%s", desc)
	}
	// Not "screens?": a parameter the model reads as optional is one it omits,
	// and an omitted one is the whole cost with none of the saving.
	if strings.Contains(desc, "screens?") {
		t.Error("the signature offers screens as optional")
	}
	// The reasoning is expensive and belongs where it is sent once. If any of
	// this leaks into the block it is paid on every request for the life of
	// every conversation that carries the browser.
	for _, teaching := range []string{"round trip", "ten calls", "1 to 10", "has not loaded yet"} {
		if strings.Contains(desc, teaching) {
			t.Errorf("guidance leaked into the tool block: %q", teaching)
		}
	}
}

func TestScrollGuidanceTellsTheModelToDecideTheDistance(t *testing.T) {
	g := (&browserSkill{}).Guidance(map[string]any{"action": "scroll"})
	if g == "" {
		t.Fatal("scroll has no guidance at all")
	}
	for _, want := range []string{"ALWAYS send `screens`", "round trip"} {
		if !strings.Contains(g, want) {
			t.Errorf("the guidance does not say %q:\n%s", want, g)
		}
	}
	// It has to say how to CHOOSE, not just that there is a number. "Use it" is
	// advice a model cannot act on without a rule for picking one.
	if !strings.Contains(g, "Send 1 when") {
		t.Errorf("the guidance never says how to pick the number:\n%s", g)
	}
}

// A refusal would cost the round trip it is trying to save: the model reads the
// error and calls again. Teaching is free and happens once. So an omitted
// `screens` still scrolls, and this is the test that stops somebody tightening
// it into a refusal later because the signature looks required.
func TestAnOmittedDistanceStillScrollsRatherThanRefusing(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	out, err := (&browserScrollSkill{app: app}).scroll("down", 0)
	if err != nil {
		t.Fatalf("scroll with no distance was refused: %v", err)
	}
	if !out.Success {
		t.Error("scroll with no distance reported no success")
	}
}
