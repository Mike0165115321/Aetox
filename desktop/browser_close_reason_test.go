package main

// Who ended a tab, and whether the agent can be told a lie about it.
//
// The failure these pin is in the app's own history: three `browser scroll`
// calls in forty seconds, each answered "the page you were working on was
// closed (the user closed the tab)", one of them six seconds after a
// successful `open`. Nobody had touched a tab. The message was produced by the
// engine's own close coming back round through the window and re-entering the
// door that means "the user did this".
//
// Design and the whole reconstruction: docs/architecture/browser-tab-lifetime-2026-08-25.md.

import (
	"strings"
	"testing"
)

// The loop, end to end.
//
// Go closes a tab for its own reasons -> tells the window -> the window drops
// the chip -> dropping the chip unmounts the pane -> the pane used to close on
// the way out, through the binding that means the user pressed ×.
//
// The second call is the echo. It must not be able to rewrite what the first
// one said, and it cannot: a reason is recorded only by the call that actually
// removed the tab.
func TestAnEchoOfTheEnginesOwnCloseIsNotBlamedOnTheUser(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")

	app.closeTab("web-agent-1", closedByApp) // the sweep after a reload
	app.BrowserClose("web-agent-1")          // the echo back through the window

	_, err := app.agentTab()
	if err == nil {
		t.Fatal("the agent still has a page after both calls")
	}
	if strings.Contains(err.Error(), "the user closed") {
		t.Errorf("the agent was told a person closed its page, and nobody did: %v", err)
	}
	if !strings.Contains(err.Error(), "carry on") {
		t.Errorf("the message does not say the work continues: %v", err)
	}
}

// Same echo, one door over: the agent closing its own tab. `closeAgentTab` has
// always been careful to use the reason-carrying path rather than the user's
// binding — and the round trip through the window defeated that care, because
// the window's teardown reached the user's binding anyway.
func TestAnEchoOfTheAgentsOwnCloseIsNotBlamedOnTheUser(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")

	if err := app.closeAgentTab("web-agent-1"); err != nil {
		t.Fatalf("closeAgentTab() = %v", err)
	}
	app.BrowserClose("web-agent-1") // the echo

	_, err := app.agentTab()
	if err == nil {
		t.Fatal("the agent still has a page it closed itself")
	}
	if strings.Contains(err.Error(), "closed while you worked") {
		t.Errorf("the agent closed its own page and was told the user did it: %v", err)
	}
}

// The user really did close it, and that must still be said. The point of
// typing the reason is not to stop reporting — it is to report the right one.
func TestTheUserClosingItIsStillSaid(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")

	app.BrowserClose("web-agent-1") // the × on the strip, and only that

	_, err := app.agentTab()
	if err == nil || !strings.Contains(err.Error(), "the user closed") {
		t.Fatalf("agentTab() = %v, want the user's action named", err)
	}
}

// The half that put the message six seconds after a successful `open`.
//
// A record with no id can only say "something was closed recently", so it
// outlives the reopen and is delivered against a tab that is not the one it is
// about. It is cleared the moment the agent has a page again — so a page lost
// some other way afterwards (§171.4: a view that died, with nothing calling
// closeTab) reports what it can, and does not replay a stale accusation.
func TestAReasonDoesNotOutliveTheReopen(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	app.BrowserClose("web-agent-1") // the user closes it: a reason is recorded

	// The agent does what the message told it to and opens another page —
	// through the real open(), because the clearing of the record is part of
	// what is under test and doing it by hand here would hide its absence.
	h := app.browsers
	h.open("web-agent-2", "https://example.com", 0, 0, 100, 100)
	h.backend.(*fakeBackend).drain()

	if _, err := app.agentTab(); err != nil {
		t.Fatalf("the freshly opened page does not answer: %v", err)
	}

	// Now lose that page without anybody closing it — the case §171.4 left open.
	h.mu.Lock()
	delete(h.tabs, "web-agent-2")
	h.mu.Unlock()

	_, err := app.agentTab()
	if err == nil {
		t.Fatal("a page that is gone still answers as the agent's")
	}
	if strings.Contains(err.Error(), "the user closed") {
		t.Errorf("a reason from the previous page was replayed against this one: %v", err)
	}
}

// open() is where the clear has to live, and this is the assertion that keeps
// it there: registering an agent tab wipes the record, whatever it held.
func TestOpeningAnAgentTabClearsTheRecord(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	app.BrowserClose("web-agent-1")

	app.browsers.open("web-agent-2", "https://example.com", 0, 0, 100, 100)
	app.browsers.backend.(*fakeBackend).drain()

	app.browsers.mu.Lock()
	goneID := app.browsers.goneID
	app.browsers.mu.Unlock()
	if goneID != "" {
		t.Errorf("the record survived the reopen: %q", goneID)
	}
}
