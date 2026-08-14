package prompt

import (
	"strings"
	"testing"
)

// deskAt builds the Desk a session at a stance produces, without importing
// internal/mode — which this package must not do (see the Desk doc). The two
// fields are what bootstrap.withStance sets, so what is exercised here is the
// shape prompt actually receives.
func deskAt(direction, stanceDirection string, toolLess bool) Desk {
	return Desk{
		Name:            "assistant",
		Direction:       direction,
		StanceDirection: stanceDirection,
		ToolLess:        toolLess,
		Carries:         func(string) bool { return !toolLess },
		Delegates:       true,
	}
}

// The ordering is the policy (§106.4): a desk is fixed for the session and a
// stance is the dial the user just turned, so where they disagree the stance
// has to be the later text. Models weight later context more heavily, and
// position is the only mechanism there is for saying which one wins.
func TestTheStanceIsReadAfterTheDeskItNarrows(t *testing.T) {
	got := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()},
		deskAt("DESK-DIRECTION-MARKER", "STANCE-DIRECTION-MARKER", false))

	deskAt := strings.Index(got, "DESK-DIRECTION-MARKER")
	stanceAt := strings.Index(got, "STANCE-DIRECTION-MARKER")
	if deskAt < 0 || stanceAt < 0 {
		t.Fatalf("both directions must reach the prompt (desk=%d stance=%d)", deskAt, stanceAt)
	}
	if stanceAt < deskAt {
		t.Error("the stance must be read after the desk it narrows — later context is the only way to say which wins")
	}
	// And not filed at the end with the machine rules. The desk direction is
	// second on purpose ("an answer filed after ten thousand characters of
	// machine rules is not one"); a stance is the same kind of answer.
	if stanceAt > len(got)/2 {
		t.Errorf("the stance landed at %d of %d — that is the burial the desk direction was moved up to avoid",
			stanceAt, len(got))
	}
}

// A session carrying no tools must not be handed paragraphs about calling
// them. This is the failure Desk.Carries was added to stop, arriving through
// the door Carries cannot watch: batchWork is about shell, narration is about
// the pause before a tool round, and neither names a tool Carries could be
// asked about.
func TestASessionWithNoToolsIsNotTaughtHowToUseThem(t *testing.T) {
	scope := Scope{Root: t.TempDir()}
	full := BuildForDesk(SurfaceDesktop, scope, deskAt("d", "", false))
	toolLess := BuildForDesk(SurfaceDesktop, scope, deskAt("d", "s", true))

	// Phrases from the layers that are only ever instructions for using tools.
	for _, phrase := range []string{
		"skills_list",            // capability
		"apply_patch",            // fileEditing
		"one shell script",       // batchWork
		"Reach for calc",         // computing
		"write it to a .md file", // longform
		"about to call tools",    // narration
	} {
		if !strings.Contains(full, phrase) {
			t.Fatalf("test is stale: %q is no longer in the ordinary prompt", phrase)
		}
		if strings.Contains(toolLess, phrase) {
			t.Errorf("a toolless session was still told %q", phrase)
		}
	}

	// The saving is the point of คู่คิด, not a side effect — assert it is real
	// rather than trusting the phrase checks above.
	if len(toolLess) >= len(full) {
		t.Errorf("the toolless prompt (%d) must be shorter than the ordinary one (%d)", len(toolLess), len(full))
	}
}

// drawing and panel describe how the *answer* is rendered, not how a tool is
// called. A conversation that produces a diagram is exactly what คู่คิด is for,
// so these two must survive a stance that takes everything else away.
func TestAToollessSessionCanStillDraw(t *testing.T) {
	got := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()}, deskAt("d", "s", true))
	for _, phrase := range []string{"viewBox", "draw it instead of describing it"} {
		if !strings.Contains(got, phrase) {
			t.Errorf("drawing/panel must survive a toolless stance (%q missing) — a picture is an answer, not a tool call", phrase)
		}
	}
}

// The zero Desk is every session from before stances existed and must produce
// the prompt byte-for-byte as it was.
func TestAStancelessDeskIsUnchanged(t *testing.T) {
	scope := Scope{Root: t.TempDir()}
	before := BuildForDesk(SurfaceDesktop, scope, Desk{Name: "assistant", Direction: "d", Delegates: true})
	after := BuildForDesk(SurfaceDesktop, scope, deskAt("d", "", false))
	if before != after {
		t.Error("a desk with no stance must build the prompt exactly as it did before stances")
	}
}
