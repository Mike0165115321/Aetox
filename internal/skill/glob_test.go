package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatchesPathGlob(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		// The form every other tool uses, and the one filepath.Match alone
		// silently fails: * never crosses a separator there.
		{"**/*.go", "internal/skill/glob.go", true},
		{"**/*.go", "main.go", true},
		{"**/*.go", "internal/skill/glob.ts", false},
		{"src/**/*.ts", "src/lib/stores/a.ts", true},
		{"src/**/*.ts", "test/lib/a.ts", false},
		{"*_test.go", "internal/skill/glob_test.go", true}, // bare pattern means "anywhere"
		{"*_test.go", "internal/skill/glob.go", false},
		{"internal/*/glob.go", "internal/skill/glob.go", true},
		{"internal/*/glob.go", "internal/a/b/glob.go", false}, // * is one segment
		{"**/*.{ts,svelte}", "src/lib/Chat.svelte", true},
		{"", "a.go", false},
	}
	for _, c := range cases {
		if got := matchesPathGlob(c.pattern, c.rel); got != c.want {
			t.Errorf("matchesPathGlob(%q, %q) = %v, want %v", c.pattern, c.rel, got, c.want)
		}
	}
}

func TestGlobSkillSortsNewestFirstAndSkipsNoise(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, age time.Duration) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		stamp := time.Now().Add(-age)
		if err := os.Chtimes(full, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	write("old.go", 2*time.Hour)
	write("src/new.go", 1*time.Minute)
	write("node_modules/pkg/dep.go", 0)

	s := &globSkill{root: root}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "**/*.go"})
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.Content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 files, got %v", lines)
	}
	// Newest first: "what did I just touch" is the real question behind a file search.
	if lines[0] != "src/new.go" || lines[1] != "old.go" {
		t.Errorf("not sorted newest first: %v", lines)
	}
	if strings.Contains(out.Content, "node_modules") {
		t.Errorf("walked into node_modules:\n%s", out.Content)
	}
}
