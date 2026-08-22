package main

// Closing a tab is something the USER did, and the agent is told so.
//
// Owner, 22 ส.ค.: *"มันไม่ควรหยุดสิ การเปลี่ยนแปลงอะไรในเบราว์เซอร์หรือการกดอะไร
// คือแอ็คชั่นฝั่งผู้ใช้ โมเดลยังรับรู้และทำงานของตัวเองได้สิ เว้นแต่จะถูกกดหยุด"*.
//
// Before this, the × on the agent's tab and the agent never having opened one
// were the same sentence: "the agent has no page open — use open first". Read
// mid-task that is an accusation, and the model answers it by explaining that
// it DID open one. The fact it needed was that the page went away underneath
// it, which is not a failure and not a reason to stop.

import (
	"strings"
	"testing"
)

func TestTheUserClosingTheAgentsTabIsToldToTheAgent(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")

	app.BrowserClose("web-agent-1") // the × on the tab strip

	_, err := app.agentTab()
	if err == nil {
		t.Fatal("the agent still has a page after the user closed it")
	}
	if !strings.Contains(err.Error(), "closed while you worked") {
		t.Errorf("the agent was not told the user closed its page: %v", err)
	}
	if !strings.Contains(err.Error(), "carry on") {
		t.Errorf("the message does not say the work continues: %v", err)
	}
}

// Said once, to the call that runs into it. Repeated, it would read as the page
// being closed again on every action for the rest of the turn.
func TestThePageWasClosedIsSaidOnce(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	app.BrowserClose("web-agent-1")

	if _, err := app.agentTab(); err == nil || !strings.Contains(err.Error(), "closed while you worked") {
		t.Fatalf("first call = %v, want the reason", err)
	}
	_, again := app.agentTab()
	if again == nil {
		t.Fatal("the agent has a page it was never given")
	}
	if strings.Contains(again.Error(), "closed while you worked") {
		t.Errorf("the reason was repeated to a call that had already heard it: %v", again)
	}
}

// The agent closing its own tab is not the user doing anything, and telling it
// otherwise would be a lie it would then act on.
func TestTheAgentClosingItsOwnTabIsNotBlamedOnTheUser(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")

	if err := app.closeAgentTab("web-agent-1"); err != nil {
		t.Fatalf("closeAgentTab() = %v", err)
	}
	_, err := app.agentTab()
	if err == nil {
		t.Fatal("the tab the agent closed is still its page")
	}
	if strings.Contains(err.Error(), "the user closed the tab") {
		t.Errorf("the agent's own close was reported as the user's: %v", err)
	}
}

// And a tab of the user's own is none of the agent's business either way: its
// page is still there, and nothing is owed to it.
func TestClosingAUserTabLeavesTheAgentsPageAlone(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1", "web-3")

	app.BrowserClose("web-3")

	id, err := app.agentTab()
	if err != nil {
		t.Fatalf("closing the user's own tab took the agent's away: %v", err)
	}
	if id != "web-agent-1" {
		t.Errorf("the agent moved to %q when the user closed their own tab", id)
	}
}
