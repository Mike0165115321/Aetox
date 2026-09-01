package subagent

import (
	"slices"
	"testing"

	"github.com/Mikedev115/Aetox/internal/mode"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// The office keeps the whole deliverables group for its agents, and the video
// agent's own tools are in it.
//
// They were not, and nothing said so. `video_new`/`video_check`/`video_render`
// are deliverables; the office carried `media, web, agent` plus four named
// files tools, and `chairs:` listed nine tool names that included neither the
// pack nor any action of it. So the profile asked for them, its allowlist let
// them through, and the ceiling cut them one line later — the agent that exists
// to render scenes shipped unable to. Found by measuring the ceiling on
// 31 ส.ค.; `desktop/video_gate_test.go` had been green throughout because it
// reads the profile's text and never asks the desk.
func TestTheOfficeKeepsTheDeliverablesGroupForItsAgents(t *testing.T) {
	office, ok := mode.Load(mode.Office)
	if !ok {
		t.Fatal("the office desk did not load")
	}
	// The pack the model is offered, and the actions a manifest may narrow with.
	for _, name := range []string{"video", "video_new", "video_check", "video_render", "doc_write", "sheet_write"} {
		if !office.CarriesForChair(name, skill.SourceBuiltin) {
			t.Errorf("the office does not keep %s in the room for its agents", name)
		}
	}
	// And the line that is deliberately still drawn: assistant parity means the
	// developer group stays out (owner, 31 ส.ค.).
	for _, name := range []string{"codebase", "repo_map", "pr", "symbol"} {
		if office.CarriesForChair(name, skill.SourceBuiltin) {
			t.Errorf("the office handed %s to its agents — that is the group it refuses", name)
		}
	}
	// The desk's own assistant is unchanged by all of it: `chairs:` is what is
	// in the room, never what is on the desk.
	if office.Carries("doc_write", skill.SourceBuiltin) {
		t.Error("the office's own assistant carries doc_write — chairs: leaked onto the desk")
	}
}

// The panel belongs to whoever is watching, and nobody watches a delegate.
//
// `desk` and `desk_terminal` do not read or change anything: their whole output
// is what the person is looking at, on the one surface this app has. A delegate
// has no human attached to its loop, and several delegates run at once, so a
// job that opened a file on the panel would write over the screen of somebody
// following the main agent's work — and over the other delegates. It costs a
// delegate nothing, `shell` runs the same command without the screen.
//
// The other half is what makes it a rule about who is listening rather than
// about reach: an agent being spoken to directly gets both back.
func TestTheDeskSurfaceIsDeniedToDelegatesAndReturnedWhenWatched(t *testing.T) {
	isolate(t)
	office, ok := mode.Load(mode.Office)
	if !ok {
		t.Fatal("the office desk did not load")
	}
	parent := skill.NewRegistry()
	for _, s := range []skill.Skill{stubTool("desk"), stubTool("desk_terminal"), stubTool("read")} {
		if err := parent.Register(s, skill.SourceWorkbench); err != nil {
			t.Fatal(err)
		}
	}
	p, ok := Load("doc")
	if !ok {
		t.Fatal("the doc agent did not load")
	}

	delegated := FilterRegistry(parent, p, office).Names()
	for _, name := range []string{"desk", "desk_terminal"} {
		if slices.Contains(delegated, name) {
			t.Errorf("a delegate was handed %s — it can write on a panel nobody asked it to touch: %v", name, delegated)
		}
	}
	if !slices.Contains(delegated, "read") {
		t.Errorf("the delegate lost an ordinary tool as well: %v", delegated)
	}

	watched := AttendedRegistry(parent, p, office).Names()
	for _, name := range []string{"desk", "desk_terminal"} {
		if !slices.Contains(watched, name) {
			t.Errorf("an agent in a direct chat cannot put its work on the desk: %v", watched)
		}
	}

	// `deny:` in the file still wins. This hands back what the mechanism took
	// away; it does not overrule the author.
	refuses := p
	refuses.Deny = []string{"desk_terminal"}
	if got := AttendedRegistry(parent, refuses, office).Names(); slices.Contains(got, "desk_terminal") {
		t.Errorf("a profile's own deny was overruled by the hand-back: %v", got)
	}

	// And it can only give back what the desk already carried. A room with no
	// terminal does not grow one because somebody is watching.
	noTerminal := *office
	noTerminal.Deny = append(slices.Clone(noTerminal.Deny), "desk_terminal")
	if got := AttendedRegistry(parent, p, &noTerminal).Names(); slices.Contains(got, "desk_terminal") {
		t.Errorf("the hand-back reached past the desk's own ceiling: %v", got)
	}
}
