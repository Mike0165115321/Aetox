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

// deskHolding is deskAt's counterpart for the one question a stance does not
// get to answer: what the DESK is for. bootstrap.withStance narrows Carries and
// leaves Holds standing, so a คู่คิด session carries nothing and still knows
// which room it is sitting in.
func deskHolding(toolLess bool, holds ...string) Desk {
	d := deskAt("DESK", "", toolLess)
	d.Holds = func(name string) bool {
		for _, h := range holds {
			if h == name {
				return true
			}
		}
		return false
	}
	return d
}

// Asked what it could do, the assistant answered "ไฟล์ เชลล์ desk_open" while
// the window beside it was showing four buttons reading เทอร์มินัล, เบราว์เซอร์,
// ไฟล์ and สไลด์. Owner, 22 ส.ค.: "มันควรจะพูดถึงพวกนี้ได้นะ ไม่ใช่พูด desk_open
// ชื่อแบบนี้ตรงๆ".
//
// It answered with the only names in its context. Nothing in the prompt had
// ever described the panels, so the model's whole inventory was one tool id
// left in a desk manifest.
func TestThePromptNamesThePanelsBesideTheChat(t *testing.T) {
	got := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()},
		deskHolding(false, "desk_terminal", "browser", "desk_open"))

	for _, pane := range []string{"a terminal", "a browser", "a file view", "a slide view"} {
		if !strings.Contains(got, pane) {
			t.Errorf("the prompt never mentions %q, so the model cannot talk about it:\n%s", pane, got)
		}
	}
}

// The §116 line, held from the side it was crossed on. A tool id is wiring, and
// the user has never seen one and cannot act on one — so nothing the model is
// invited to repeat may contain one.
func TestThePanelsAreNamedWithoutNamingATool(t *testing.T) {
	got := workbench(SurfaceDesktop, deskHolding(false, "desk_terminal", "browser", "desk_open"))
	if got == "" {
		t.Fatal("no workbench layer at a desk that holds all three panes")
	}
	for _, id := range []string{"desk_open", "desk_terminal", "desk_list", "desk_close"} {
		if strings.Contains(got, id) {
			t.Errorf("the layer spells the tool id %q, which is the thing that reached the user:\n%s", id, got)
		}
	}
}

// The whole reason Holds exists. Under คู่คิด, Carries is false for every name
// there is — correctly, the turn carries nothing — and a layer reading it would
// answer "what can you do" with "nothing", which is true of the turn and false
// of the desk. The question the user asks when they turn the dial is about the
// desk.
func TestASessionCarryingNoToolsStillKnowsWhatTheDeskIsFor(t *testing.T) {
	got := BuildForDesk(SurfaceDesktop, Scope{Root: t.TempDir()},
		deskHolding(true, "desk_terminal", "browser", "desk_open"))

	if !strings.Contains(got, "a browser") {
		t.Errorf("คู่คิด lost the panels along with the tools:\n%s", got)
	}
	// It keeps its tool-less silence everywhere it was already silent, though —
	// this layer must not be a door back in for the paragraphs Desk.ToolLess
	// exists to withhold.
	if strings.Contains(got, "skills_list returns them on request") {
		t.Errorf("the workbench layer let the tool block back in:\n%s", got)
	}
}

// Panels in a window. A terminal session has none of them, and a prompt that
// described them there would be describing furniture that is not in the room.
func TestTheCLIIsNotToldAboutPanels(t *testing.T) {
	got := BuildForDesk(SurfaceCLI, Scope{Root: t.TempDir()},
		deskHolding(false, "desk_terminal", "browser", "desk_open"))
	if strings.Contains(got, "panels of its own") {
		t.Errorf("the terminal prompt describes a window it is not running in:\n%s", got)
	}
}

// A desk that does not hold a pane is not told it has one. The specialized desk
// refuses shell on purpose, so the terminal is not its to offer.
func TestADeskWithoutAPaneIsNotToldItHasOne(t *testing.T) {
	got := workbench(SurfaceDesktop, deskHolding(false, "browser", "desk_open"))
	if strings.Contains(got, "a terminal") {
		t.Errorf("a desk that refuses shell was offered a terminal:\n%s", got)
	}
	if !strings.Contains(got, "a browser") {
		t.Errorf("the panes it does hold went missing with the one it does not:\n%s", got)
	}
}
