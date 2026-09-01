package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/repomap"
	"github.com/Mikedev115/Aetox/internal/safety"
)

// The pane's whole promise is nodes for a focused project and an honest empty
// for anything else — this is the test that would have caught the first field
// report ("มันว่างเปล่าเลยครับ") before it was a field report.
func TestGetRepoMapGraphServesTheFocusedProject(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/demo\n")
	write("core/core.go", "package core\n\nfunc Answer() int { return 42 }\n")
	write("main.go", "package main\n\nimport \"example.com/demo/core\"\n\nfunc main() { core.Answer() }\n")

	a := seed(&App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir()}, &conversation{id: newSessionID()})
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.projectFocused = true
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   root,
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	g := a.GetRepoMapGraph(0)
	if !g.Focused {
		t.Fatal("a focused project must map, not refuse")
	}
	if g.Error != "" {
		t.Fatalf("unexpected error: %s", g.Error)
	}
	if len(g.Nodes) == 0 || g.TotalFiles < 2 {
		t.Fatalf("expected nodes from the seeded tree, got %d nodes / %d files", len(g.Nodes), g.TotalFiles)
	}
	if g.Nodes[0].Path != "core/core.go" {
		t.Errorf("the imported package should lead, got %+v", g.Nodes[0])
	}

	// The ceiling is the caller's, and it has to bite in both directions: a
	// small number must actually cut, and AllNodes must actually stop cutting.
	if one := a.GetRepoMapGraph(1); len(one.Nodes) != 1 {
		t.Errorf("a ceiling of one must keep one node, got %d", len(one.Nodes))
	}
	all := a.GetRepoMapGraph(repomap.AllNodes)
	if len(all.Nodes) < len(g.Nodes) {
		t.Errorf("AllNodes must not keep fewer than the default, got %d vs %d", len(all.Nodes), len(g.Nodes))
	}

	a.projectFocused = false
	if g := a.GetRepoMapGraph(0); g.Focused {
		t.Fatal("unfocused must be an honest empty, not a map of the machine")
	}
}
