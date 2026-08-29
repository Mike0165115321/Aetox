package main

// Which tab the agent is working — the question three browser actions used to
// get wrong by asking the host for its most recently shown tab.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/statereport"
)

// The bug these tests exist for: the user clicking between their own tabs
// rewrites browserHost.lastID (BrowserSetVisible), and read/click/type used to
// target exactly that. A user glancing at their own page mid-turn handed the
// agent's next click to it.
func TestAgentTabSurvivesTheUserRaisingTheirOwn(t *testing.T) {
	app := &App{}
	app.browsers = &browserHost{app: app, tabs: map[string]*browserTab{
		"web-agent-1": {url: "https://example.com/agent"},
		"web-2":       {url: "https://example.com/user"},
	}, views: map[string]tabView{"web-agent-1": &fakeView{}, "web-2": &fakeView{}}}
	app.browsers.agentID = "web-agent-1" // what open() recorded
	app.browsers.lastID = "web-2"        // what BrowserSetVisible writes when the user's tab is raised

	id, err := app.agentTab()
	if err != nil {
		t.Fatalf("agentTab() = %v", err)
	}
	if id != "web-agent-1" {
		t.Errorf("agentTab() = %q, want web-agent-1 — a click or a keystroke would land on the user's page", id)
	}
}

// And the refusal has to be true from the agent's side. "No browser tab open in
// the workbench" was the old wording, and it becomes a lie the moment the target
// is the agent's own tab: the user can have a screenful of them.
func TestAgentTabRefusesWhenOnlyTheUserHasTabs(t *testing.T) {
	app := &App{}
	app.browsers = &browserHost{app: app, tabs: map[string]*browserTab{
		"web-2": {url: "https://example.com/user"},
	}, views: map[string]tabView{"web-2": &fakeView{}}}
	app.browsers.lastID = "web-2" // and no agentID: the agent has opened nothing

	_, err := app.agentTab()
	if err == nil {
		t.Fatal("the agent was handed a tab the user opened")
	}
	if strings.Contains(err.Error(), "no browser tab open") {
		t.Errorf("the refusal denies the user's tabs exist: %v", err)
	}
}

// A tab the user closed leaves the host's map, and the agent must not be steered
// into a corpse — the reason agentTab checks liveness and not just the
// name.
func TestAgentTabRefusesAfterTheUserClosedIt(t *testing.T) {
	app := &App{}
	app.browsers = &browserHost{app: app, tabs: map[string]*browserTab{}, views: map[string]tabView{}}
	app.browsers.agentID = "web-agent-1" // remembered, but the tab is gone from the map

	if _, err := app.agentTab(); err == nil {
		t.Error("a closed tab still answers as the agent's")
	}
}

// The half of the bug that was not about safety: with the agent's tab findable
// only while it was the tab on screen, a user glancing at their own page made
// `open` believe the agent had nothing to steer, so it minted a second tab and
// left the first stranded. Reuse exists to stop exactly that.
func TestOpenReusesTheAgentTabWhileTheUserLooksElsewhere(t *testing.T) {
	app := &App{}
	app.browsers = &browserHost{app: app, tabs: map[string]*browserTab{
		"web-agent-1": {url: "https://example.com/agent"},
		"web-2":       {url: "https://example.com/user"},
	}, views: map[string]tabView{"web-agent-1": &fakeView{}, "web-2": &fakeView{}}}
	app.browsers.agentID = "web-agent-1"
	app.browsers.lastID = "web-2"

	// An error is what workbenchOpenBrowser reads as "mint a new tab".
	if _, err := app.agentTab(); err != nil {
		t.Errorf("open would mint a second agent tab and strand the first: %v", err)
	}
}

// Reuse steers the agent's existing tab, and steering means a COM call into an
// apartment-threaded engine. Made from the calling goroutine it is refused, not
// delayed — silently, into the engine's error callback — so the page never
// navigates and the caller waits out its whole timeout. Every page after the
// first in a session, which is what it did.
func TestReuseNavigatesOnTheHostThread(t *testing.T) {
	b := &fakeBackend{}
	app := &App{}
	view := &fakeView{}
	app.browsers = &browserHost{app: app, backend: b, tabs: map[string]*browserTab{
		"web-agent-1": {},
	}, views: map[string]tabView{"web-agent-1": view}}
	app.browsers.agentID = "web-agent-1"

	app.onTab("web-agent-1", func(v tabView, _ *browserTab) { v.navigate("https://example.org") })

	if view.lastJS != "" {
		t.Fatalf("navigate ran on the caller's goroutine (lastJS=%q) — the engine refuses that call", view.lastJS)
	}
	b.drain() // the host thread's pump
	if view.lastJS != "navigate:https://example.org" {
		t.Errorf("after the host thread ran, lastJS = %q, want the navigation", view.lastJS)
	}
}

// Arming has to happen before the navigation is asked for, or a completion that
// beats the arm is dropped and the wait hangs to its timeout.
func TestReuseArmsBeforeItAsksToNavigate(t *testing.T) {
	tab := &browserTab{}
	tab.setNavOK(true) // the previous page's verdict
	_, before := tab.latch()
	close(before) // that navigation finished long ago
	tab.armNavigation()

	if tab.navLoaded() {
		t.Error("the tab still reports the previous page's outcome")
	}
	_, after := tab.latch()
	select {
	case <-after:
		t.Error("the latch is still the closed one, so a wait would return instantly on the old page")
	default:
	}
}

// The engine's complaint has to reach the caller, because for a week it reached
// only a log file. Owner, 17 ส.ค.: *"เบาเซอร์ อาจจะพังเพราะมันไม่รายงานอะไรกับ
// เอเจน เลยก็ได้ เอเจนเลยไม่รู้"* — and that is exactly what happened: the agent
// was handed "page did not finish loading", concluded the network was bad, and
// said so to the user, three times, while WebView2 had been refusing the call
// outright every single time.
func TestATimeoutSaysWhatTheEngineRefused(t *testing.T) {
	tab := &browserTab{navDone: make(chan struct{})} // never completes
	tab.noteEngineError(errors.New("This method can only be called from the thread that created the object."))

	err := tab.awaitNavigation(context.Background(), 10*time.Millisecond)
	if err == nil {
		t.Fatal("a navigation that never completed reported success")
	}
	if !strings.Contains(err.Error(), "only be called from the thread") {
		t.Errorf("the engine's own words are missing: %v", err)
	}
	// A refused call is a defect in this program, not weather. statereport is
	// for "the site is slow tonight", and marking this as that is precisely how
	// it survived a week (§127.8).
	if statereport.Is(err) {
		t.Error("an engine refusal is filed as a state report, so nothing will ever treat it as a bug")
	}
}

// And with nothing from the engine it stays weather, which is the common case
// and must not start reading like a bug in Aetox.
func TestATimeoutWithNoEngineComplaintIsStillJustASlowPage(t *testing.T) {
	tab := &browserTab{navDone: make(chan struct{})}

	err := tab.awaitNavigation(context.Background(), 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if !statereport.Is(err) {
		t.Errorf("a slow page is now reported as something to correct: %v", err)
	}
}

// Arming clears it, so a wait never inherits the previous navigation's excuse.
func TestArmingForgetsTheLastNavigationsEngineError(t *testing.T) {
	tab := &browserTab{navDone: make(chan struct{})}
	tab.noteEngineError(errors.New("stale complaint"))
	tab.armNavigation()

	if got := tab.engineError(); got != nil {
		t.Errorf("engineError() = %v after arming, want nil", got)
	}
}

// Steering one tab of your own is half an answer; the other half is knowing what
// is in it. click and type say so now, and after a click the page may have moved.
//
// Since 28 ส.ค. it also says WHICH tab, and that half survives a page that
// cannot be named: a tab that has navigated nowhere yet is still a tab the
// action happened in, and with several of them open that is the fact a reader
// cannot recover from anywhere else.
func TestBrowserWhereNamesThePageTheActionLandedOn(t *testing.T) {
	app := &App{}
	app.browsers = &browserHost{app: app, tabs: map[string]*browserTab{
		"web-agent-1": {url: "https://example.com/after-click"},
		"web-agent-2": {}, // navigated nowhere yet
	}, views: map[string]tabView{"web-agent-1": &fakeView{}, "web-agent-2": &fakeView{}}}

	got := app.browserWhere("web-agent-1")
	if !strings.Contains(got, "https://example.com/after-click") {
		t.Errorf("browserWhere() = %q, want the page named in it", got)
	}
	if !strings.Contains(got, "web-agent-1") {
		t.Errorf("browserWhere() = %q, want the tab named in it too", got)
	}

	nowhere := app.browserWhere("web-agent-2")
	if !strings.Contains(nowhere, "web-agent-2") {
		t.Errorf("browserWhere() on a tab with no URL = %q, want it to still name the tab", nowhere)
	}
	if strings.Contains(nowhere, "อยู่ที่") {
		t.Errorf("browserWhere() on a tab with no URL = %q, want no claim about where it is", nowhere)
	}

	if got := app.browserWhere("web-9"); got != "" {
		t.Errorf("browserWhere() on an unknown tab = %q, want nothing appended", got)
	}
}

// One fact, one spelling. There were four: `open` said "Title (url)", `read`
// wrote a document header, and click/type/capture each invented another as they
// were added. This file already keeps browserOpenedPrefix as a shared constant
// so `open`'s sentence and parseBrowserOpened cannot drift — the same reasoning
// simply had not been applied to the sentences written after it.
func TestEveryBrowserActionNamesAPageTheSameWay(t *testing.T) {
	const (
		title = "Example Domain"
		url   = "https://example.com/"
	)
	ref := browserPageRef(title, url)
	if ref != title+" ("+url+")" {
		t.Fatalf("browserPageRef() = %q", ref)
	}

	// open's sentence is parsed back for the agent-pages panel, so its shape is
	// a contract and not a preference.
	line := browserOpenedLine(title, url)
	if !strings.Contains(line, ref) {
		t.Errorf("open does not use the shared ref: %q", line)
	}
	gotTitle, gotURL := parseBrowserOpened(line)
	if gotTitle != title || gotURL != url {
		t.Errorf("the round trip broke: %q, %q", gotTitle, gotURL)
	}

	app := &App{}
	app.browsers = &browserHost{app: app, tabs: map[string]*browserTab{
		"web-agent-1": {title: title, url: url},
	}, views: map[string]tabView{"web-agent-1": &fakeView{}}}
	if where := app.browserWhere("web-agent-1"); !strings.Contains(where, ref) {
		t.Errorf("click/type do not use the shared ref: %q", where)
	}
}

// A page that has not told us its title yet still has to be nameable, and
// naming it "  ()" would be worse than naming it by its address.
//
// The titleless shape is the half of this change that nearly shipped broken:
// `open` writes it, parseBrowserOpened reads it back to build the "pages the
// agent visited" panel, and for one commit the writer produced a bare address
// while the reader still demanded a closing parenthesis. Every titleless page
// would have vanished from that panel, silently. The round-trip test caught it,
// which is the entire reason that test exists.
func TestAPageWithNoTitleIsNamedByItsAddress(t *testing.T) {
	if got := browserPageRef("", "https://example.com/"); got != "https://example.com/" {
		t.Errorf("browserPageRef(no title) = %q", got)
	}
	if got := browserPageRef("", ""); got != "" {
		t.Errorf("browserPageRef(nothing) = %q, want empty so callers can omit it", got)
	}

	_, url := parseBrowserOpened(browserOpenedLine("", "https://example.com/"))
	if url != "https://example.com/" {
		t.Errorf("a titleless page does not survive the round trip: %q", url)
	}
	// And a genuinely truncated line still has to read as nothing, or a broken
	// sentence becomes a dead row on the desk.
	if _, url := parseBrowserOpened("เปิดแล้ว: Example (https://example.test"); url != "" {
		t.Errorf("a truncated line parsed as a page: %q", url)
	}
}

// A reused tab still holds the last page's title and URL, and `open` polls meta
// until it is non-empty — so on a reused tab the poll succeeded on its first
// read and the tool reported the page it had just LEFT.
//
// Caught in a production log rather than by anything here: "เปิดแล้ว: Example
// Domain" for a navigation to x.com, twice in one session. Every reused open
// since tab reuse shipped had been naming the wrong page, and parseBrowserOpened
// files those names into the visited-pages panel.
func TestArmingForgetsWhatThePageWas(t *testing.T) {
	tab := &browserTab{navDone: make(chan struct{}), title: "Example Domain", url: "https://example.com/"}

	tab.armNavigation()

	if title, url := tab.meta(); title != "" || url != "" {
		t.Errorf("meta() = %q, %q after arming — open would report the page it just left", title, url)
	}
}
