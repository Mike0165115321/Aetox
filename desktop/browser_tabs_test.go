package main

// The agent's tabs are plural now, and the rule that made them worth having is
// the one that must survive: they are still only ever the agent's own.

import (
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/skill"
)

func hostWithTabs(t *testing.T, current string, order []string, ids ...string) *App {
	t.Helper()
	app := &App{}
	tabs := map[string]*browserTab{}
	views := map[string]tabView{}
	for _, id := range ids {
		tabs[id] = &browserTab{title: "Page " + id, url: "https://example.com/" + id}
		views[id] = &fakeView{}
	}
	app.browsers = &browserHost{app: app, backend: &fakeBackend{}, tabs: tabs, views: views}
	app.browsers.agentID = current
	app.browsers.agentOrder = order
	return app
}

// The whole point of keeping ownership while dropping the one-tab rule: a list
// of "your tabs" that included the user's would hand the agent a select away
// from everything §127 spent a week protecting.
func TestTabsListNamesOnlyTheAgentsOwn(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2", "web-3")

	got := app.agentTabs()
	if len(got) != 2 {
		t.Fatalf("agentTabs() = %v, want the two the agent opened", got)
	}
	for _, id := range got {
		if !isAgentTabID(id) {
			t.Errorf("agentTabs() offered %q, which the user opened", id)
		}
	}

	list := app.agentTabList()
	if strings.Contains(list, "web-3") {
		t.Errorf("the list shows the user's tab:\n%s", list)
	}
	if !strings.Contains(list, "* web-agent-1") {
		t.Errorf("the list does not mark which tab the other actions work:\n%s", list)
	}
}

// And the refusal has to say WHY. "Unknown tab" makes a model try another id;
// "that one is the user's" makes it stop.
func TestSelectingTheUsersTabIsRefusedAsTheirs(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1", "web-3")

	err := app.selectAgentTab("web-3", "")
	if err == nil {
		t.Fatal("the agent was given the user's tab")
	}
	if !strings.Contains(err.Error(), "ของผู้ใช้") {
		t.Errorf("the refusal does not say whose tab it is: %v", err)
	}
	if id, _ := app.agentTab(); id != "web-agent-1" {
		t.Errorf("a refused select still moved the current tab to %q", id)
	}
}

func TestSelectMovesWhichTabTheOtherActionsWork(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2")
	app.emit = func(string, ...any) {} // selecting raises the tab, which emits

	if err := app.selectAgentTab("web-agent-2", ""); err != nil {
		t.Fatalf("selectAgentTab() = %v", err)
	}
	id, err := app.agentTab()
	if err != nil || id != "web-agent-2" {
		t.Errorf("agentTab() = %q, %v — every other action still works the old page", id, err)
	}
}

// A closed tab must leave the list, because the list is read by a model that
// will try to select what it names.
func TestClosingATabLeavesTheListAndHandsTheCurrentOnOver(t *testing.T) {
	app := hostWithTabs(t, "web-agent-2", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2")

	if err := app.closeAgentTab("web-agent-2"); err != nil {
		t.Fatalf("closeAgentTab() = %v", err)
	}
	if got := app.agentTabs(); len(got) != 1 || got[0] != "web-agent-1" {
		t.Errorf("agentTabs() = %v, want only web-agent-1", got)
	}
	// Falls back rather than to nothing: closing one of several must not strand
	// the agent mid-task.
	if id, err := app.agentTab(); err != nil || id != "web-agent-1" {
		t.Errorf("agentTab() = %q, %v, want the surviving tab", id, err)
	}
}

// Closing the last one is the other half: there is genuinely nothing left, and
// `open` reads that as "mint a fresh tab".
func TestClosingTheLastTabLeavesTheAgentWithNone(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")

	if err := app.closeAgentTab("web-agent-1"); err != nil {
		t.Fatalf("closeAgentTab() = %v", err)
	}
	if _, err := app.agentTab(); err == nil {
		t.Error("a closed last tab still answers as the agent's")
	}
	if list := app.agentTabList(); !strings.Contains(list, "open") {
		t.Errorf("the empty list does not say what to do next: %q", list)
	}
}

// The user closing one is the same path, and the agent must not be steered into
// a corpse afterwards.
func TestATabTheUserClosedIsNoLongerOffered(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2")

	app.BrowserClose("web-agent-1") // the user pressing × on the tab strip

	if err := app.selectAgentTab("web-agent-1", ""); err == nil {
		t.Fatal("selected a tab that is gone")
	} else if !strings.Contains(err.Error(), "ปิดไปแล้ว") {
		t.Errorf("the refusal reads like the tab was never the agent's: %v", err)
	}
}

// What the model is told, and WHERE, after the block was split into signature
// and guidance (internal/skill/guidance.go). Both halves are checked here
// because the failure this guards against is a rule falling into the gap
// between them and being told to nobody.
func TestBrowserToolOffersTabsAndSaysTheRestOnce(t *testing.T) {
	desc := (&browserSkill{}).ToolDefinition().Function.Description
	s := &browserSkill{}
	guide := func(action string) string { return s.Guidance(map[string]any{"action": action}) }

	// In the block: the signature, and the two things that must never be lost.
	if !strings.Contains(desc, "`tabs`") {
		t.Error("the tabs action is not offered")
	}
	if strings.Contains(desc, "ONE tab of your own") {
		t.Error("the description still tells the model it has exactly one tab")
	}
	if !strings.Contains(desc, "password") {
		t.Error("the safety rule left the block — guidance can be lost to a summary, so this one may not live there")
	}

	// Moved to guidance, and each with the action that spends it.
	if !strings.Contains(guide("tabs"), "cannot reach them") {
		t.Error("tab ownership is told to nobody")
	}
	if !strings.Contains(guide("tabs"), "invalidates your refs") {
		t.Error("selecting a tab no longer warns that refs die with it")
	}
	if !strings.Contains(guide("read"), "stale") {
		t.Error("the ref rule is told to nobody")
	}

	// And the judgment really is out of the block, or nothing was saved.
	for _, moved := range []string{"stale", "cannot reach", "Read first"} {
		if strings.Contains(desc, moved) {
			t.Errorf("%q is still in the block, so it is still paid for on every message", moved)
		}
	}
}

// The window was never told a tab had closed, and the chip stayed on the strip
// pointing at a native view that no longer existed (owner, 24 ส.ค., with five
// dead tabs on screen). `workbench:open-browser` had a partner on the file side
// all along — `workbench:close-file` — and none here.
func TestClosingATabTellsTheWindow(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	events := captureEvents(app)

	app.closeTab("web-agent-1", closedByApp)

	closed := []string{}
	for _, ev := range events.all() {
		if ev.Name != "workbench:close-browser" {
			continue
		}
		payload, ok := ev.Data[0].(map[string]string)
		if !ok {
			t.Fatalf("close carried %T", ev.Data[0])
		}
		closed = append(closed, payload["id"])
	}
	if len(closed) != 1 || closed[0] != "web-agent-1" {
		t.Fatalf("window told about %v, want web-agent-1", closed)
	}
}

// Closing something that is not there says nothing. A chip removed by this very
// event unmounts its pane, which calls BrowserClose on the way out — and an
// event for that would come straight back round.
func TestClosingATabTwiceOnlyAnnouncesItOnce(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	events := captureEvents(app)

	app.closeTab("web-agent-1", closedByApp)
	app.browsers.backend.(*fakeBackend).drain()
	app.closeTab("web-agent-1", closedByApp)

	n := 0
	for _, ev := range events.all() {
		if ev.Name == "workbench:close-browser" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("announced %d times, want 1", n)
	}
}

// The owner's other half: *"บางทีโมเดลปิดหน้าต่างแต่เบราว์เซอร์จริงหรือแท็บไม่ได้ถูกลบจริง"*.
//
// The registry entry has to go now, so the agent stops being told about a tab it
// cannot use. The native window cannot go now — destroying it only happens on
// the browser thread. Deleting both together left a window whose destroy had not
// run yet already unreachable: no id in `views`, so nothing could hide it, move
// it or try again.
func TestTheViewOutlivesTheTabUntilItIsActuallyDestroyed(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	view := app.browsers.views["web-agent-1"].(*fakeView)

	app.closeTab("web-agent-1", closedByApp)

	// Closed to everyone who asks — immediately.
	if app.browsers.live("web-agent-1") {
		t.Error("the tab still reads as live")
	}
	if len(app.agentTabs()) != 0 {
		t.Error("the agent is still being offered it")
	}
	// But still reachable, because the window it names is still on screen.
	app.browsers.mu.Lock()
	_, held := app.browsers.views["web-agent-1"]
	app.browsers.mu.Unlock()
	if !held {
		t.Fatal("the view was forgotten before it was destroyed — nothing could reach that window again")
	}
	if view.destroyed {
		t.Fatal("destroy ran on the calling goroutine rather than the browser thread")
	}

	app.browsers.backend.(*fakeBackend).drain()

	if !view.destroyed {
		t.Error("the window was never destroyed")
	}
	app.browsers.mu.Lock()
	_, stillHeld := app.browsers.views["web-agent-1"]
	app.browsers.mu.Unlock()
	if stillHeld {
		t.Error("the view outlived its own destroy")
	}
}

// The sweep after a window reload used to take everything, on the reasoning that
// a freshly loaded frontend owns no tabs. True of the frontend, false of the
// app: the engine works across a reload, so a turn in flight can be holding
// tabs of its own — and killing them mid-task is what produced five "แท็บ
// ปิดไปแล้ว" in a row.
func TestTheOrphanSweepSparesARunningTurnsTabs(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2", "web-3")
	app.turns = map[string]*liveTurn{"20260824-000000.001": {}}

	app.CloseAllBrowserTabs()

	if got := app.agentTabs(); len(got) != 2 {
		t.Errorf("the working turn lost its tabs: %v", got)
	}
	if app.browsers.live("web-3") {
		t.Error("the user's orphaned tab survived the sweep")
	}
}

// With nothing working, an agent tab is as orphaned as any other — otherwise the
// leftovers of a turn that died would sit there for the rest of the app's life
// with nothing left to close them.
func TestTheOrphanSweepTakesEverythingWhenNothingIsRunning(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1", "web-3")

	app.CloseAllBrowserTabs()

	if len(app.agentTabs()) != 0 || app.browsers.live("web-3") {
		t.Error("a sweep with no turn running must clear the lot")
	}
}

// "close" with no id shuts the tab being worked.
//
// The first thing that happened when the guidance started telling a model to
// tidy up after itself was `which tab? use list first` — a refusal for the one
// case the tool could have answered by itself. It had just opened a page and
// wanted it shut; the id was never in doubt (owner's run, 24 ส.ค.).
func TestClosingWithNoIDClosesTheTabInHand(t *testing.T) {
	app := hostWithTabs(t, "web-agent-2", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2")

	out, err := (&browserTabsSkill{app: app}).run("close", "")
	if err != nil {
		t.Fatalf("close with no id: %v", err)
	}
	if !out.Success {
		t.Errorf("Success = false: %s", out.Content)
	}
	if app.browsers.live("web-agent-2") {
		t.Error("the tab in hand is still open")
	}
	if !app.browsers.live("web-agent-1") {
		t.Error("it closed a tab that was not the one in hand")
	}
	// The row has to name what it closed, or the timeline shows a close with no
	// subject for the one case where the tool worked the subject out itself.
	if !strings.Contains(out.Command, "web-agent-2") {
		t.Errorf("Command = %q, want the resolved id in it", out.Command)
	}
}

// With nothing open, "close" says so instead of asking for a list that would
// come back empty.
func TestClosingWithNoIDAndNoTabsSaysSo(t *testing.T) {
	app := hostWithTabs(t, "", nil)

	_, err := (&browserTabsSkill{app: app}).run("close", "")
	if err == nil {
		t.Fatal("want a refusal")
	}
	if strings.Contains(err.Error(), "list") {
		t.Errorf("it still sends the model to `list`: %q", err)
	}
}

// `select` keeps needing an id, and that is not an inconsistency: "go to the tab
// I am already on" is not a thing anybody asks for.
func TestSelectStillNeedsToBeToldWhich(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")

	if _, err := (&browserTabsSkill{app: app}).run("select", ""); err == nil {
		t.Error("select with no id was accepted")
	}
}

// Scrolling, and the family of pages that were unreadable without it: a feed, a
// search result list, a channel's videos. `read` returns what is in the
// document, and on those pages the document is one screen deep until something
// scrolls — so the agent was reading the first screen of a long page and had no
// way to know that is what it was doing.
func TestScrollTakesTheFourDirectionsAndRefusesTheRest(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	s := &browserScrollSkill{app: app}

	for _, to := range []string{"down", "up", "top", "bottom", ""} {
		out, err := s.scroll(to, 0)
		if err != nil {
			t.Errorf("scroll %q: %v", to, err)
			continue
		}
		if !out.Success {
			t.Errorf("scroll %q reported no success", to)
		}
		// It has to send the model back to `read`: scrolling on its own tells it
		// nothing, because the page it can see is still the one it last read.
		if !strings.Contains(out.Content, "อ่านใหม่") {
			t.Errorf("scroll %q does not say to read again: %q", to, out.Content)
		}
	}
	if _, err := s.scroll("sideways", 0); err == nil {
		t.Error("an unknown direction was accepted")
	}
}

// The scroller on a modern app is usually a div with overflow:auto, not the
// window — a bare window.scrollBy silently does nothing there, which is the
// failure that looks exactly like a page with no more content.
func TestScrollLooksForTheElementThatActuallyScrolls(t *testing.T) {
	js := scrollScript("down", 1)
	for _, want := range []string{"overflowY", "scrollHeight", "scrollingElement"} {
		if !strings.Contains(js, want) {
			t.Errorf("the script does not consider %s:\n%s", want, js)
		}
	}
	if !strings.Contains(scrollScript("bottom", 1), "el.scrollHeight") {
		t.Error("bottom does not go to the bottom of the scroller it found")
	}
	if !strings.Contains(scrollScript("up", 1), "-(") {
		t.Error("up does not move the other way")
	}
}

// Scrolling reveals what reading was always going to reach. It rides on
// browser_read's right rather than earning one of its own, and a profile that
// can read pages must not need a second grant to see their second screen.
func TestScrollRidesOnTheReadRight(t *testing.T) {
	for _, call := range skill.PackedCalls("browser") {
		if call.Action != "scroll" {
			continue
		}
		if call.Permission != "browser_read" {
			t.Errorf("scroll maps to %q, want browser_read", call.Permission)
		}
		return
	}
	t.Error("scroll is not in the browser pack at all")
}
