package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// What `tools:` asked for and the registry did not hand over. The case this
// exists for is a tool leaving the build — `slides_write` did — and every agent
// naming it quietly getting one tool less.
func TestMissingToolsNamesWhatWasAskedForAndNotGot(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	p := subagent.Profile{Name: "deck", Tools: []string{"read", "slides_write", "grep"}}

	got := missingTools(p, []string{"read", "grep", "skills_list"})
	if len(got) != 1 || got[0] != "slides_write" {
		t.Fatalf("missing = %v, want just the tool that is gone", got)
	}
}

// An empty `tools:` asks for nothing by name — it means "whatever this desk
// carries". A warning there would land on every agent that never narrowed
// itself, which is how a warning stops being read.
func TestMissingToolsSaysNothingWhenNothingWasAskedFor(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if got := missingTools(subagent.Profile{Name: "doc"}, []string{"read"}); got != nil {
		t.Fatalf("missing = %v, want nothing", got)
	}
}

// The false positive worth guarding: a server placed on one agent is
// deliberately not connected until that agent runs, so its tools are legitimately
// absent from the registry while the roster is being drawn. Reporting those as
// missing would put a red line on a working agent every time the app started.
func TestMissingToolsDoesNotAccuseADeferredMCPServer(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	servers := `[{"name":"github","command":["npx","x"],"for":["agent:gh"]}]`
	if err := os.WriteFile(filepath.Join(root, "mcp-servers.json"), []byte(servers), 0o600); err != nil {
		t.Fatal(err)
	}
	p := subagent.Profile{Name: "gh", Tools: []string{"read", "github_create_pull_request", "ตัวที่ไม่มีจริง"}}

	got := missingTools(p, []string{"read"})
	if len(got) != 1 || got[0] != "ตัวที่ไม่มีจริง" {
		t.Fatalf("missing = %v — the MCP tool of a placed server must not be accused, the invented one must be", got)
	}
}
