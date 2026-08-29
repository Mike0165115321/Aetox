package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mikedev115/Aetox/internal/subagent"
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

// The false positive that actually shipped, and the one that made this warning
// worthless: after §99 packed the tools, the registry hands back the PACK's name
// while every profile still names the per-action ones — the names those tools
// had before packing, and the names every desk manifest and permission rule
// still uses. Compared flatly, an agent holding `change` was reported as missing
// write, edit, edits AND delete.
//
// Measured in the running app on 30 ส.ค.: `doc` reported 14 missing tools, of
// which 13 it was holding. Every card on the roster printed nearly the same
// list, because the list had stopped being about the agent at all. Reported as
// "อันนี้อ่ะจะมาแสดงทำไมว่ะ".
func TestMissingToolsResolvesAPackedToolToTheActionsItCarries(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	p := subagent.Profile{Name: "doc", Tools: []string{
		"write", "edit", "edits", "delete", // change
		"grep", "list", "glob", // search
		"image_ocr", "video_ocr", "audio_transcribe", // media_read
		"desk_open", "desk_list", // desk
		"read", "ตัวที่ไม่มีจริง",
	}}

	got := missingTools(p, []string{"read", "change", "search", "media_read", "desk"})

	if len(got) != 1 || got[0] != "ตัวที่ไม่มีจริง" {
		t.Fatalf("missing = %v — every name but the invented one is carried by a pack the agent holds", got)
	}
}

// The other half, which the fix must not trade away: a pack the agent does NOT
// hold cannot excuse the actions inside it. Without this, resolving packs would
// turn the warning off for everything and the tool that left the build would go
// silent again — the exact failure this whole file exists for.
func TestMissingToolsStillAccusesAnActionWhosePackIsAbsent(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	p := subagent.Profile{Name: "reader", Tools: []string{"read", "write", "grep"}}

	got := missingTools(p, []string{"read", "search"})

	if len(got) != 1 || got[0] != "write" {
		t.Fatalf("missing = %v — grep comes with the search pack, write does not come with anything held", got)
	}
}
