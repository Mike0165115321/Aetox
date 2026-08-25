package subagent

import (
	"encoding/json"

	"slices"
	"testing"

	"github.com/Mikedev115/Aetox/internal/skill"
)

// writeSubagent drops one worker in the agents home and loads it back — the
// same trip a file the user edited makes, rather than a Profile built by hand.
// A struct literal would skip the parser, and `tools:` is a line the parser
// owns.
func writeSubagent(t *testing.T, name, frontmatter string) Profile {
	t.Helper()
	isolate(t)
	writeProfile(t, AgentsDir, name, "---\ndescription: a worker\n"+frontmatter+"---\n\nDo the job.\n")
	p, ok := Load(name)
	if !ok {
		t.Fatalf("%s did not load", name)
	}
	return p
}

// actionsOffered reads back the enum a caller would actually be sent — the
// question "which acts does this worker have", asked of the thing that answers
// it for the model rather than of the profile that asked for them.
func actionsOffered(t *testing.T, reg *skill.Registry, tool string) []string {
	t.Helper()
	s, ok := reg.Get(tool)
	if !ok {
		return nil
	}
	var schema struct {
		Properties struct {
			Action struct {
				Enum []string `json:"enum"`
			} `json:"action"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(s.(skill.Tool).ToolDefinition().Function.Parameters, &schema); err != nil {
		t.Fatalf("%s: parameters are not JSON: %v", tool, err)
	}
	return schema.Properties.Action.Enum
}

// The seam packing opened: a `tools:` line is written by one hand — an AGENT.md
// on disk, or the settings page that edits it — and read by the one function
// that hands a worker its tools. Everything in between is what broke on
// 2026-08-10 (see TestAConnectionPlacedOnTheAgentReachesItsTools): every piece
// had a test, every piece passed, and nothing walked the pipe.
//
// So this test writes what a manifest writes and asks what a dispatch asks. No
// Profile built in Go, no allowedActions called directly.
//
// Four sentences a `tools:` line can say, and each of them silent when it
// breaks — the worker still loads, still answers, and simply holds the wrong
// set of acts.
func TestAToolsLineNarrowsAPackedToolToTheActionsItNames(t *testing.T) {
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})

	for _, c := range []struct {
		what        string
		frontmatter string
		want        []string
	}{
		{
			// The ordinary case, and the one every profile written before the
			// packing says: name the tool, get the tool.
			what:        "naming the tool asks for all of it",
			frontmatter: "tools: read, shell\n",
			want:        []string{"run", "output", "kill", "list"},
		},
		{
			// The sentence that has to keep meaning what it said. `shell_output`
			// was a whole tool name until this release; a profile that asked for
			// it asked to look in on a background command and nothing else.
			what:        "naming only an action asks for only that action",
			frontmatter: "tools: read, shell_output\n",
			want:        []string{"output"},
		},
		{
			what:        "naming the tool and an action still means the tool",
			frontmatter: "tools: read, shell, shell_kill\n",
			want:        []string{"run", "output", "kill", "list"},
		},
		{
			// deny outranks any grant (§44.0), and it has to reach inside the
			// pack or a refusal the author wrote is one the worker never hears.
			what:        "denying an action takes it out of a tool that was granted whole",
			frontmatter: "tools: read, shell\ndeny: shell_kill\n",
			want:        []string{"run", "output", "list"},
		},
		{
			// No allowlist at all is the profile saying "whatever the desk has",
			// which cannot quietly become "nothing".
			what:        "no tools line at all is still the whole tool",
			frontmatter: "",
			want:        []string{"run", "output", "kill", "list"},
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			p := writeSubagent(t, "packworker", c.frontmatter)
			got := actionsOffered(t, FilterRegistry(parent, p, nil), "shell")
			if !slices.Equal(got, c.want) {
				t.Errorf("%s: the worker is offered %v, want %v", c.what, got, c.want)
			}
		})
	}
}

// A worker left with no actions is handed nothing, rather than a tool that
// refuses every call.
//
// The two look identical from inside the model — it calls, it is refused, it
// tries again — and they are different faults with different fixes. A tool that
// is absent is a `tools:` line to change; a tool that is present and says no to
// everything is the state the 2026-08-10 incident actually shipped, where every
// screen said equipped and the toolbox was empty.
func TestAWorkerDeniedEveryActionIsNotHandedTheToolAtAll(t *testing.T) {
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	p := writeSubagent(t, "packworker", "tools: read, shell\ndeny: shell, shell_output, shell_kill\n")

	if _, found := FilterRegistry(parent, p, nil).Get("shell"); found {
		t.Error("a worker with every shell action denied still carries shell — it would refuse every call it makes")
	}
}

// github is the same mechanism on a tool whose actions are all equals, so it is
// the one that catches a pack wired only for shell's shape — an action that
// doubles as the tool's own name.
func TestAToolsLineNarrowsGithubTheSameWay(t *testing.T) {
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	p := writeSubagent(t, "packworker", "tools: read, github_read_file, github_list_files\n")

	got := actionsOffered(t, FilterRegistry(parent, p, nil), "github")
	if !slices.Equal(got, []string{"list_files", "read_file"}) {
		t.Errorf("the worker is offered %v, want the two it named, in the pack's own order", got)
	}
}
