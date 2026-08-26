package prompt

import (
	"testing"
)

// The system prompt is the tool block's twin, and it had no scale.
//
// desktop/tool_budget_test.go has weighed every tool schema since 2026-08-04,
// on the argument that a cost paid on every request before the user has typed
// a word is a cost nobody sees. All of that is equally true one field over:
// the system prompt rides the same request, is subtracted from the same
// context window, and is paid again on every turn by every provider without
// prompt caching. The difference was only that one of them was being measured.
//
// It grows the same way too — one layer at a time, each obviously worth its
// own paragraph, none of them ever weighed against the total. DECISIONS.md
// §106.8 recorded 11,440 B for the assistant desk on 2026-08-14; twelve days
// later it was 14,608, and the only reason anybody knew is that somebody
// counted it by hand while answering an unrelated question. A number that has
// to be rediscovered is not a baseline.
//
// When this fails the fix is a decision, not a bigger number: does this
// paragraph belong in front of the model on every single request, or does it
// belong in a skill, a tool description, or nowhere? Raising the ceiling is a
// legitimate answer — it has to be a deliberate one, which is the whole point.
const (
	// Measured 2026-08-26: assistant desk, desktop surface, no project open,
	// every tool carried — 14,608 bytes ≈ 3,650 tokens, against a tool block of
	// ~7,747. Ceiling set at 16,000 by the owner, about 10% of headroom: room
	// for a layer or two before the next conversation, not room to drift.
	maxPromptBytes = 16000

	// A session carrying nothing must stay far below the full prompt. §106.8
	// measured 6,248 B against 11,440 — 55% — and the saving is structural:
	// Desk.ToolLess skips the layers that are only ever instructions for using
	// tools. Held as a ratio rather than a byte count because what is being
	// protected is the skipping, not any particular size.
	maxToolLessShare = 0.70
)

// promptBytes is the whole prompt for a desk holding everything, which is the
// honest worst case: a stance narrows it, a project widens it, and neither is
// the number to build a ceiling on.
func promptBytes(t *testing.T, desk Desk) string {
	t.Helper()
	return BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()}, desk)
}

func TestTheSystemPromptStaysWithinItsBudget(t *testing.T) {
	full := promptBytes(t, Desk{
		Name:      "assistant",
		Direction: "This session is assistant work.",
		Carries:   func(string) bool { return true },
	})
	// 4 bytes/token is the rough rate for the English prose these layers are —
	// the same approximation, for the same reason, as the tool block's.
	// Thai in a folded identity file counts about three bytes a character and
	// would read high here; the engine layers this measures are all English.
	tokens := len(full) / 4

	// Logged on a pass, not only on a failure — the lesson tool_budget_test.go
	// learned when two docs quoted two different figures for the tool block and
	// neither could be checked. `-run TestTheSystemPromptStaysWithinItsBudget
	// -v` is the source anything quoting this number copies from.
	t.Logf("assistant-desk system prompt: %d B / ~%d tokens (budget %d B)", len(full), tokens, maxPromptBytes)
	if len(full) > maxPromptBytes {
		t.Errorf("system prompt is %d B / ~%d tokens, budget is %d B", len(full), tokens, maxPromptBytes)
		t.Log("largest first — this is the list to argue with:")
		logLayerSizes(t, full)
	}
}

// What a session carrying nothing pays. The saving §106.8 measured was never
// held by anything, so a layer added outside the ToolLess block would quietly
// hand four thousand characters of tool instructions to a chat that cannot call
// a tool — which is the exact failure Desk.ToolLess was added to stop.
func TestASessionCarryingNothingIsNotHandedTheToolInstructions(t *testing.T) {
	desk := Desk{Name: "assistant", Direction: "This session is assistant work."}
	full := promptBytes(t, Desk{Name: desk.Name, Direction: desk.Direction, Carries: func(string) bool { return true }})
	desk.ToolLess = true
	bare := promptBytes(t, desk)

	share := float64(len(bare)) / float64(len(full))
	t.Logf("tool-less system prompt: %d B / ~%d tokens — %.0f%% of the full one", len(bare), len(bare)/4, share*100)
	if share > maxToolLessShare {
		t.Errorf("a tool-less session is being handed %.0f%% of the full prompt (ceiling %.0f%%); "+
			"a layer that is instruction for using tools has been added outside the ToolLess block",
			share*100, maxToolLessShare*100)
		logLayerSizes(t, bare)
	}
}

// logLayerSizes prints what the engine layers cost, largest first.
//
// The list is written out rather than derived, because Build assembles the
// prompt as one string and there is nothing in it to split on. That means it
// can fall behind — so the last line reports what share of the prompt the
// listed layers actually account for. A new layer nobody added here shows up
// as that share dropping, which is a hint in the right direction rather than a
// false claim of completeness.
func logLayerSizes(t *testing.T, prompt string) {
	t.Helper()
	desk := Desk{Name: "assistant", Carries: func(string) bool { return true }}
	layers := []struct {
		name string
		text string
	}{
		{"identity", identity()},
		{"surfaceLayer", surfaceLayer(SurfaceDesktop)},
		{"workbench", workbench(SurfaceDesktop, desk)},
		{"capability", capability()},
		{"fileEditing", fileEditing(desk)},
		{"parallelCalls", parallelCalls()},
		{"batchWork", batchWork()},
		{"computing", computing()},
		{"panel", panel()},
		{"planCard", planCard()},
		{"drawing", drawing()},
		{"longform", longform(desk)},
		{"narration", narration()},
		{"clarify", clarify()},
		{"evidence", evidence(desk)},
	}
	accounted := 0
	for i := range layers {
		accounted += len(layers[i].text)
	}
	for i := 0; i < len(layers); i++ {
		for j := i + 1; j < len(layers); j++ {
			if len(layers[j].text) > len(layers[i].text) {
				layers[i], layers[j] = layers[j], layers[i]
			}
		}
		t.Logf("  %5d B  %4d tok  %s", len(layers[i].text), len(layers[i].text)/4, layers[i].name)
	}
	t.Logf("  listed layers account for %d of %d B — the rest is the desk's direction, the environment and whatever is folded in",
		accounted, len(prompt))
}
