package main

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// The tool block is the one cost nobody sees and everybody pays. Every schema
// in it is sent on every request, before the user has typed a word — so it is
// subtracted from the context window of every model Aetox runs on, and paid
// again on every turn by every provider without prompt caching (which is most
// local runtimes, i.e. exactly the cheap-model case this project is built for).
//
// It grows the way this kind of thing always grows: one tool at a time, each
// obviously worth its own 200 tokens, none of them ever weighed against the
// total. This test is the weighing. It is not a limit on what Aetox may do —
// it is a limit on what Aetox may carry *everywhere*, which is a different
// question and the one that was never being asked.
//
// When it fails, the fix is a decision, not a bigger number: does this belong
// in front of every model on every request, or behind `task`, a profile, or a
// skill? Raising the ceiling is a legitimate answer — but it has to be
// deliberate, which is the whole point.
const (
	// Measured 2026-08-04: 44 tools, 35,215 bytes ≈ 8,803 tokens.
	//
	// Raised to 9,600 on 2026-08-07 for `calc`, and the argument is on the
	// record because that is what this test asks for. It is ~130 tokens on
	// every request, and it was weighed against the two cheaper answers and
	// beat both: putting it behind `task` means a sum costs a delegated agent,
	// and leaving it to `shell` means arithmetic costs the right to touch the
	// user's machine and depends on what they happen to have installed. What
	// makes it worth carrying everywhere rather than in a profile is that
	// nothing else in the block fails invisibly — a wrong number arrives in the
	// same confident sentence as a right one, on any kind of task, so there is
	// no profile it could sit in that would be the right one.
	//
	// Its description was cut to capability alone first (occasions belong in
	// internal/prompt), which is the order the fix should always take.
	maxToolBlockTokens = 9600
	// At 48 the count is now full: the next tool has to displace one, and that
	// is the intended reading of this number rather than a nuisance.
	maxToolCount = 48
)

func TestTheToolBlockStaysWithinItsBudget(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir(), sessionID: newSessionID()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	defs := skill.NewDispatcher(a.registry).ToolDefinitions()
	type row struct {
		name  string
		bytes int
	}
	rows := make([]row, 0, len(defs))
	total := 0
	for _, d := range defs {
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("%s: schema does not marshal: %v", d.Function.Name, err)
		}
		rows = append(rows, row{d.Function.Name, len(b)})
		total += len(b)
	}
	// 4 bytes/token is the rough rate for the English JSON these schemas are.
	// Approximate on purpose: the budget is about order of magnitude, and a
	// real tokenizer here would make the test depend on which model is loaded.
	tokens := total / 4

	sort.Slice(rows, func(i, j int) bool { return rows[i].bytes > rows[j].bytes })
	if len(rows) > maxToolCount || tokens > maxToolBlockTokens {
		t.Errorf("tool block is %d tools / ~%d tokens, budget is %d / %d",
			len(rows), tokens, maxToolCount, maxToolBlockTokens)
		t.Log("largest first — this is the list to argue with:")
		for i, r := range rows {
			if i == 12 {
				t.Logf("  … and %d more", len(rows)-i)
				break
			}
			t.Logf("  %5d B  %4d tok  %s", r.bytes, r.bytes/4, r.name)
		}
	}
}

// Every tool the assistant can be handed has to say what it is for, or the
// surfaces that group them quietly drop it into a catch-all and the page stops
// being a complete answer.
//
// This lives here rather than in internal/skill because only the desktop can
// see the whole registry at once — builtins, the workbench tools, and the
// delegation trio are registered from three different places, and a table
// checked against only one of them would look complete while missing a third.
func TestEveryToolHasACategory(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir(), sessionID: newSessionID()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	var missing []string
	for _, name := range a.registry.Names() {
		if src, ok := a.registry.SourceOf(name); ok && src == skill.SourceSkill {
			continue // a SKILL.md document is not a tool and is listed elsewhere
		}
		if !skill.HasCategory(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("these tools are not in internal/skill/category.go, so they fall into the catch-all: %v", missing)
	}
}
