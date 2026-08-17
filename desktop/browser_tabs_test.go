package main

// The agent's tabs are plural now, and the rule that made them worth having is
// the one that must survive: they are still only ever the agent's own.

import (
	"strings"
	"testing"
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

	err := app.selectAgentTab("web-3")
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

	if err := app.selectAgentTab("web-agent-2"); err != nil {
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

	if err := app.selectAgentTab("web-agent-1"); err == nil {
		t.Fatal("selected a tab that is gone")
	} else if !strings.Contains(err.Error(), "ปิดไปแล้ว") {
		t.Errorf("the refusal reads like the tab was never the agent's: %v", err)
	}
}

// The description is what the model acts on, so it has to carry the rule the
// code enforces — and must no longer claim the agent has exactly one tab.
func TestBrowserToolOffersTabsWithoutPromisingOnlyOne(t *testing.T) {
	desc := (&browserSkill{}).ToolDefinition().Function.Description

	if strings.Contains(desc, "ONE tab of your own") {
		t.Error("the description still tells the model it has exactly one tab")
	}
	if !strings.Contains(desc, "`tabs`") {
		t.Error("the description does not offer the tabs action")
	}
	if !strings.Contains(desc, "select another tab") {
		t.Error("the description does not warn that selecting invalidates refs — the one real hazard of plural tabs")
	}
	// Two short fragments rather than one long sentence: the wording of this
	// paragraph has been rewritten three times in two days as the rule got more
	// precise, and a test that pins the prose fails on every improvement.
	if !strings.Contains(desc, "own tabs are theirs") || !strings.Contains(desc, "cannot reach") {
		t.Error("the ownership rule fell out of the description")
	}
}
