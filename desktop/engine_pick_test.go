package main

import (
	"slices"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/connect"
	"github.com/Mike0165115321/Aetox/internal/oauth"
)

// Choosing which automation engine the specialist works on.
//
// The choice is written as placement — the same `for:` list the settings
// register edits — rather than as a new per-session "active engine". That is the
// whole design, and these tests are what keeps it: a second piece of state
// answering "which engine" would be a second truth, and this one already decides
// which tools the agent is handed, so the engine you did not pick is not in the
// model's list at all.

func engineApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	return bootDeskApp(t, "assistant")
}

func connectN8N(t *testing.T) {
	t.Helper()
	if err := config.SetConnectionBaseURL("n8n", "http://localhost:5678"); err != nil {
		t.Fatalf("SetConnectionBaseURL: %v", err)
	}
	if err := oauth.Set("n8n", oauth.Credential{Type: "api", Key: "k", Account: "localhost:5678"}); err != nil {
		t.Fatalf("oauth.Set: %v", err)
	}
}

// Every engine in the family, connected or not, with its state on the row.
//
// This filtered to the connected ones once, on the reasoning that a picker
// offering something that does not work is a menu of dead options. What it
// actually produced was worse: with nothing connected the list came back empty,
// the chip drew nothing, and the person who had set up least lost the one
// control that could have told them so. The filtering is the caller's, and the
// caller's job is to say "ยังไม่ได้เชื่อม" beside the name.
func TestEnginesForOffersTheWholeFamilyWithItsState(t *testing.T) {
	a := engineApp(t)

	got := a.EnginesFor("automation", "automation")
	if len(got) != 2 {
		t.Fatalf("EnginesFor with nothing connected = %d rows, want both engines so the picker can still be drawn", len(got))
	}
	for _, row := range got {
		if row.Connected {
			t.Errorf("%s reported connected on a machine with no accounts", row.ID)
		}
		// GitHub belongs to no family — grouping a service nothing substitutes
		// for would be inventing a set with one member.
		if row.ID == "github" {
			t.Error("github is in the automation family; it is not an automation engine")
		}
	}

	connectN8N(t)
	got = a.EnginesFor("automation", "automation")
	var n8nRow int
	for i, row := range got {
		if row.ID == "n8n" {
			n8nRow = i
		}
	}
	if !got[n8nRow].Connected {
		t.Error("n8n is attached but the row does not say so — the chip would offer it as unconnected")
	}
}

// Picking one engine points the agent at it AND away from the others. Leaving
// the old one placed would hand the model two engines' tools and let it write to
// whichever it reached for first.
func TestUseEnginePointsTheAgentAtOneAndAwayFromTheRest(t *testing.T) {
	a := engineApp(t)
	connectN8N(t)

	if err := a.UseEngine("automation", "automation", "n8n"); err != nil {
		t.Fatalf("UseEngine: %v", err)
	}
	held := config.ConnectionsForAgent("automation", connect.IDs(), nil)
	if !slices.Contains(held, "n8n") {
		t.Fatalf("the agent holds %v after picking n8n", held)
	}

	// Picking nothing in the family — an id that is not one of them — takes the
	// agent off every engine rather than silently leaving the last one on.
	if err := a.UseEngine("automation", "automation", ""); err != nil {
		t.Fatalf("UseEngine(none): %v", err)
	}
	if held := config.ConnectionsForAgent("automation", connect.IDs(), nil); len(held) != 0 {
		t.Fatalf("the agent still holds %v after being pointed at nothing", held)
	}
}

// Placement is what the tool gate reads, so picking an engine has to change what
// the agent can actually call — not just what a chip says.
func TestPickingAnEngineDecidesWhichToolsTheAgentGets(t *testing.T) {
	a := engineApp(t)
	connectN8N(t)

	target := connect.AgentTarget("automation")
	if target != "agent:automation" {
		t.Fatalf("AgentTarget = %q, want the `for:` spelling the config matcher reads", target)
	}

	if err := a.UseEngine("automation", "automation", "n8n"); err != nil {
		t.Fatalf("UseEngine: %v", err)
	}
	held := config.ConnectionsForAgent("automation", connect.IDs(), nil)
	if !connect.Allows("n8n_workflow_create", held) {
		t.Error("the agent cannot call n8n's tools after being pointed at n8n")
	}

	if err := a.UseEngine("automation", "automation", ""); err != nil {
		t.Fatalf("UseEngine(none): %v", err)
	}
	held = config.ConnectionsForAgent("automation", connect.IDs(), nil)
	if connect.Allows("n8n_workflow_create", held) {
		t.Error("the agent kept n8n's tools after being pointed away — the gate is not reading placement")
	}
}

// A caller that forgets to say who is asking must be refused rather than
// writing a placement nobody holds.
func TestUseEngineRefusesAnUnnamedAgent(t *testing.T) {
	a := engineApp(t)
	if err := a.UseEngine("automation", "  ", "n8n"); err == nil {
		t.Error("UseEngine accepted a blank agent name")
	}
}

// The state the picker exists for, and the one that was invisible.
//
// A connection reaches an agent only when placed on it BY NAME — silence is not
// a grant for an agent, deliberately. So the ordinary path (connect an engine,
// walk into its room) produces an agent holding nothing, and every answer it
// gives is about what it cannot do. The picker has to be offered in exactly that
// state, which is why it is drawn on one connected engine and not only on two.
func TestAConnectedEngineIsOfferedEvenBeforeItIsPlaced(t *testing.T) {
	a := engineApp(t)
	connectN8N(t)

	var offered int
	for _, row := range a.EnginesFor("automation", "automation") {
		if row.Connected {
			offered++
		}
	}
	if offered != 1 {
		t.Fatalf("connected engines = %d, want the one attached offered before it is placed", offered)
	}
	// Offered, and genuinely not yet in play: this is what the chip reads as
	// "ยังไม่ได้เลือก", and what one click repairs.
	held := config.ConnectionsForAgent("automation", connect.IDs(), nil)
	if len(held) != 0 {
		t.Fatalf("the agent already holds %v — this test no longer covers the unplaced state", held)
	}
	if connect.Allows("n8n_workflow_create", held) {
		t.Error("an unplaced engine's tools reached the agent — silence became a grant")
	}
}
