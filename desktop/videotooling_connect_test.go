package main

// connectVideoEditor is the half of the install button nobody sees: the
// kinocut download lands and the MCP entry that reaches it gets written. These
// tests are the promises the function's own comment makes — it finishes the
// press on a fresh machine, and it never overrides a decision on one that
// already made some.

import (
	"slices"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
)

func TestConnectVideoEditorWritesTheWholeEntry(t *testing.T) {
	a := newMCPTestApp(t)

	if err := a.connectVideoEditor(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	servers, err := config.LoadMCPServers()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %+v, want the one entry", servers)
	}
	got := servers[0]
	if got.Name != VideoEditorServer {
		t.Fatalf("name = %q", got.Name)
	}
	// The command names where Aetox installs to even before the download has
	// landed (VideoEditorCommand's "will have, not has"), so it is never empty.
	if len(got.Command) == 0 {
		t.Fatal("command is empty — the entry cannot spawn anything")
	}
	if want := config.MCPAgentPrefix + "editor"; !slices.Contains(got.For, want) {
		t.Fatalf("for = %v, want %q — an unplaced server meets nobody's needs", got.For, want)
	}
	// The measured allowlist, not everything the server offers: 196 schemas are
	// ~37,600 tokens on every request (videoEditorTools' own comment).
	if len(got.Tools) != len(videoEditorTools) {
		t.Fatalf("tools = %d names, want the %d-tool bill", len(got.Tools), len(videoEditorTools))
	}
	if !slices.Contains(got.Tools, "search_tools") {
		t.Fatal("search_tools missing — the trim would be silent")
	}
}

func TestConnectVideoEditorAddsOnlyTheMissingPlacement(t *testing.T) {
	a := newMCPTestApp(t)

	// An entry the user shaped themselves: their own command, their own trim,
	// switched off, pointed at nobody.
	theirs := config.MCPServerConfig{
		Name:     "kinocut",
		Command:  []string{"C:\\their\\own\\kino.exe", "--mcp"},
		Tools:    []string{"video_trim"},
		Disabled: true,
		For:      []string{},
	}
	if err := config.SaveMCPServers([]config.MCPServerConfig{theirs}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := a.connectVideoEditor(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	servers, err := config.LoadMCPServers()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %+v, want the existing entry and no second one", servers)
	}
	got := servers[0]
	if !slices.Equal(got.Command, theirs.Command) || !slices.Equal(got.Tools, theirs.Tools) || !got.Disabled {
		t.Fatalf("their entry was rewritten: %+v", got)
	}
	if want := config.MCPAgentPrefix + "editor"; !slices.Contains(got.For, want) {
		t.Fatalf("for = %v, want the editor placed", got.For)
	}
}

func TestConnectVideoEditorIsANoOpWhenAlreadyPlaced(t *testing.T) {
	a := newMCPTestApp(t)

	if err := a.connectVideoEditor(); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	before, err := config.LoadMCPServers()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := a.connectVideoEditor(); err != nil {
		t.Fatalf("second connect: %v", err)
	}
	after, err := config.LoadMCPServers()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(after) != len(before) || len(after[0].For) != len(before[0].For) {
		t.Fatalf("a second press changed the file: %+v -> %+v", before, after)
	}
}

// The shelf reads the same list this side writes, so a copy that drifts cannot
// exist — and the bridge method must hand out a copy, not the backing array.
func TestVideoEditorToolsIsACopy(t *testing.T) {
	a := newMCPTestApp(t)
	got := a.VideoEditorTools()
	if len(got) == 0 || got[0] != "search_tools" {
		t.Fatalf("tools = %v", got)
	}
	got[0] = "mutated"
	if videoEditorTools[0] != "search_tools" {
		t.Fatal("the caller's write reached the backing list")
	}
}
