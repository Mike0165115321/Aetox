package main

// Which page it is happening on — the fact ไฟบอกสถานะ could not draw without.
//
// Before this, the panel could tell that the browser was busy and nothing more,
// so the only honest thing it could do was light the whole panel with five tab
// chips sitting in it. The stamp is what turns "the agent is working" into "the
// agent is working HERE", and it is put on by the host because the host is the
// only side that holds both the tool event and the tabs.

import (
	"testing"

	"github.com/Mikedev115/Aetox/internal/turn"
)

// eventsFrom captures what the window would have been sent.
func eventsFrom(app *App) *[]turn.ToolEvent {
	seen := &[]turn.ToolEvent{}
	app.emit = func(_ string, data ...any) {
		if len(data) == 0 {
			return
		}
		stamped, ok := data[0].(sessionEvent[turn.ToolEvent])
		if !ok {
			return
		}
		*seen = append(*seen, stamped.Data)
	}
	return seen
}

func TestABrowserCallIsStampedWithTheTabItIsWorking(t *testing.T) {
	app := hostWithTabs(t, "web-agent-2", []string{"web-agent-1", "web-agent-2"},
		"web-agent-1", "web-agent-2", "web-3")
	seen := eventsFrom(app)
	conv := &conversation{id: "s1"}

	app.recordToolAction(conv, turn.ToolEvent{Action: "call", Ref: "c1", Name: "browser", Act: "click"})

	if len(*seen) != 1 {
		t.Fatalf("want one event, got %d", len(*seen))
	}
	if got := (*seen)[0].Tab; got != "web-agent-2" {
		t.Errorf("Tab = %q, want the tab the agent is working", got)
	}
}

// Every other tool gets nothing. A write stamped with a browser tab would have
// the panel light a page that nobody touched.
func TestOnlyTheBrowserGetsATabStamp(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	seen := eventsFrom(app)

	app.recordToolAction(&conversation{id: "s1"},
		turn.ToolEvent{Action: "call", Ref: "c1", Name: "write", Subject: "a.go"})

	if len(*seen) != 1 {
		t.Fatalf("want one event, got %d", len(*seen))
	}
	if got := (*seen)[0].Tab; got != "" {
		t.Errorf("a write was stamped with tab %q", got)
	}
}

// No tab yet is a real state, and it is the state the very first `open` of a
// session is in. The panel reads an empty stamp as "light yourself, point at
// nothing", which is honest; a guessed id would point at somebody else's page.
func TestABrowserCallBeforeAnyTabIsStampedWithNothing(t *testing.T) {
	app := &App{}
	seen := eventsFrom(app)

	app.recordToolAction(&conversation{id: "s1"},
		turn.ToolEvent{Action: "call", Ref: "c1", Name: "browser", Act: "open"})

	if len(*seen) != 1 {
		t.Fatalf("want one event, got %d", len(*seen))
	}
	if got := (*seen)[0].Tab; got != "" {
		t.Errorf("Tab = %q with no browser open at all", got)
	}
}

// The one that would have been silent and expensive.
//
// agentTab TAKES agentTabClosed — it is said once, to the call that runs into
// it. The busy signal asks after the tab on every single tool call, so if it
// asked the same way, it would eat the sentence telling the model its page was
// closed out from under it, and the model would be told instead that it had
// never opened one. A UI detail deleting a message meant for the agent.
func TestTheStampNeverEatsThePageWasClosedMessage(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	eventsFrom(app)
	app.BrowserClose("web-agent-1") // the user's × on the tab strip

	// The panel asks first, the way it does on every call in the turn.
	if got := app.agentTabPeek(); got != "" {
		t.Errorf("agentTabPeek() = %q for a tab that is gone", got)
	}
	app.recordToolAction(&conversation{id: "s1"},
		turn.ToolEvent{Action: "call", Ref: "c1", Name: "browser", Act: "read"})

	// And the message is still there for the tool that needs it.
	_, err := app.agentTab()
	if err == nil {
		t.Fatal("the agent still has a page after the user closed it")
	}
	if err != errAgentTabClosed {
		t.Errorf("the agent was told %q, want the page-was-closed message", err)
	}
}
