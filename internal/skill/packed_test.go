package skill

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// definitionOf pulls a tool's schema back apart, because what the model is
// offered is JSON and asserting on the Go value that built it would pass on a
// day the marshalling broke.
func definitionOf(t *testing.T, s Skill) (string, map[string]any) {
	t.Helper()
	tool, ok := s.(Tool)
	if !ok {
		t.Fatalf("%s is not offered to the model as a tool", s.Name())
	}
	def := tool.ToolDefinition()
	var schema map[string]any
	if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
		t.Fatalf("%s: parameters are not JSON: %v", def.Function.Name, err)
	}
	return def.Function.Description, schema
}

func enumOf(t *testing.T, schema map[string]any) []string {
	t.Helper()
	props, _ := schema["properties"].(map[string]any)
	action, _ := props["action"].(map[string]any)
	raw, ok := action["enum"].([]any)
	if !ok {
		t.Fatal("the schema has no action enum — a packed tool that cannot be told which act to perform is one tool doing nothing")
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}

// A packed tool is one entry in the block and several rights inside it.
//
// The half worth pinning is the second: the per-action names did not stop
// existing when they stopped being registry entries — they are still what a
// profile narrows with, what a permission rule names, and what the approval
// gate judges. A refactor that "cleaned up" the leftover names would take all
// three away at once and nothing else would fail.
func TestAPackedToolIsOneRegistryEntryWithItsActionsInside(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})

	for tool, actions := range map[string][]string{
		"shell":  {"shell", "shell_output", "shell_kill", "shell_list"},
		"github": {"github_search", "github_repo_summary", "github_list_files", "github_read_file"},
	} {
		s, ok := registry.Get(tool)
		if !ok {
			t.Errorf("%s is not registered — the pack has no outside", tool)
			continue
		}
		packed, ok := s.(Packed)
		if !ok {
			t.Errorf("%s does not report its actions — nothing can narrow it", tool)
			continue
		}
		if got := packed.Actions(); !slices.Equal(got, actions) {
			t.Errorf("%s actions = %v, want %v", tool, got, actions)
		}
		for _, action := range actions {
			if action == tool {
				continue // `shell` names both the tool and its first action
			}
			if _, found := registry.Get(action); found {
				t.Errorf("%s is still its own registry entry — packing it was supposed to take it out of the tool block", action)
			}
			// Every action name still has to be filed somewhere, or a desk that
			// narrows with it falls through to CategoryAgent, which every desk
			// carries — quietly widening the desk that was being narrowed.
			if !HasCategory(action) {
				t.Errorf("%s has no category — a desk narrowing with it would widen instead", action)
			}
		}
	}
}

// The gates below the tool block judge a call by the name the act always had.
//
// This is the whole reason packed.go exists rather than four lines in each
// tool. internal/safety keys on the literal "shell" and answers RiskHigh to a
// call it can find no command in; internal/turn stretches a turn's patience
// only for a "shell_output" that was asked to wait. Both would start
// misjudging the moment three tools became one name, and both fail silently:
// the first as an approval prompt in front of reading a log, the second as a
// wait cut off at sixty seconds.
func TestTheGatesBelowTheBlockStillSeeThePerActionName(t *testing.T) {
	for _, c := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"shell", map[string]any{"command": "go test ./..."}, "shell"},
		{"shell", map[string]any{"action": "run", "command": "go build"}, "shell"},
		{"shell", map[string]any{"action": "output", "shell_id": "bg_1"}, "shell_output"},
		{"shell", map[string]any{"action": "kill", "shell_id": "bg_1"}, "shell_kill"},
		{"github", map[string]any{"action": "search", "query": "aetox"}, "github_search"},
		{"github", map[string]any{"action": "read_file", "repo_url": "u", "path": "p"}, "github_read_file"},
		// An action this pack does not have keeps the packed name. The call is
		// about to be refused, and naming it after an action that exists
		// nowhere would put that word in an approval prompt.
		{"shell", map[string]any{"action": "detonate"}, "shell"},
		// A tool that is not packed is its own answer, unchanged.
		{"read", map[string]any{"path": "notes.txt"}, "read"},
	} {
		if got := Unpack(c.tool, c.args); got != c.want {
			t.Errorf("Unpack(%q, %v) = %q, want %q", c.tool, c.args, got, c.want)
		}
	}
}

// Narrowing is how a profile's `tools:` line reaches a tool that has never
// heard of a profile — internal/skill cannot import the package that knows what
// an agent is, so the answer arrives from outside.
//
// Both halves are checked, because the description is guidance and the gate is
// a gate: an action left out of the enum must also be refused when a model
// names it anyway, and a model that has been told "no" by a schema will still
// try it eventually.
func TestANarrowedToolOffersAndRunsOnlyWhatItWasGiven(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	s, ok := registry.Get("shell")
	if !ok {
		t.Fatal("shell is not registered")
	}
	packed := s.(Packed)

	whole, wholeSchema := definitionOf(t, s)
	if got := enumOf(t, wholeSchema); !slices.Equal(got, []string{"run", "output", "kill", "list"}) {
		t.Fatalf("an unnarrowed shell offers %v, want all four", got)
	}
	if !strings.Contains(whole, "`kill`") {
		t.Error("an unnarrowed shell does not describe kill")
	}

	looker := packed.Narrow([]string{"shell_output"})
	desc, schema := definitionOf(t, looker)
	if got := enumOf(t, schema); !slices.Equal(got, []string{"output"}) {
		t.Errorf("a narrowed shell offers %v, want only output", got)
	}
	// A tool that advertises what it will refuse is a wasted turn.
	if strings.Contains(desc, "`kill`") || strings.Contains(desc, "`run`") {
		t.Errorf("the narrowed description still describes actions it would refuse:\n%s", desc)
	}

	out, err := looker.(Tool).ExecuteTool(context.Background(), map[string]any{
		"action": "run", "command": "echo nope",
	})
	if err == nil {
		t.Fatalf("a narrowed shell ran an action it was not given: %q", out.Content)
	}
	if !strings.Contains(err.Error(), "not available here") {
		t.Errorf("the refusal does not say this session cannot: %v", err)
	}

	// And narrowing is a copy. The registry is shared by every session in the
	// process, so a profile that asks for less must not take anything away from
	// everyone else.
	if got := enumOf(t, mustSchema(t, s)); !slices.Equal(got, []string{"run", "output", "kill", "list"}) {
		t.Errorf("narrowing one caller's shell changed the shared one: %v", got)
	}
}

func mustSchema(t *testing.T, s Skill) map[string]any {
	t.Helper()
	_, schema := definitionOf(t, s)
	return schema
}

// Every action a pack declares has to be wired to something that does it.
//
// The switch that routes an action and the table that declares it are two
// lists, and the failure when they disagree is the quietest kind: the action
// appears in the enum, the model calls it, and the answer is an internal error
// about a tool that was supposed to work.
func TestEveryDeclaredActionIsWiredToAnImplementation(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})

	for tool := range packs {
		s, ok := registry.Get(tool)
		if !ok {
			t.Errorf("%s is declared as a pack but is not registered", tool)
			continue
		}
		for _, call := range PackedCalls(tool) {
			// Called with nothing but the action, so what comes back is about
			// the arguments — "command is required", "repo_url is required" —
			// and never about the action being unknown. That distinction is the
			// assertion: the first means it routed, the second means it did not.
			_, err := s.(Tool).ExecuteTool(context.Background(), map[string]any{"action": call.Action})
			if err == nil {
				continue // it ran on empty arguments; it certainly routed
			}
			for _, unrouted := range []string{"unknown", "no implementation", "not available here"} {
				if strings.Contains(err.Error(), unrouted) {
					t.Errorf("%s action %q (%s) did not route: %v", tool, call.Action, call.Permission, err)
				}
			}
		}
	}
}
