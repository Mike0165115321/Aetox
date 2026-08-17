package skill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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

// The pair that has to stay split. Same missing directory, two callers, two
// different answers — because only one of them made a claim about the disk.
func TestGlobSeparatesAMissingFolderFromAPatternThatMatchesNothing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &globSkill{root: root}

	// The caller named the folder. It is not there, and answering "nothing
	// matched" would report an empty project instead of a wrong path.
	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "**/*", "path": "not-here"})
	if err == nil {
		t.Fatalf("a missing folder globbed clean: %q", out.Content)
	}
	if !strings.Contains(err.Error(), "not-here") || !strings.Contains(err.Error(), root) {
		t.Errorf("error should name what was asked for and where it resolved to, got: %v", err)
	}

	// Nobody named anything: the leading directory came out of the pattern, and
	// a guess that does not exist is exactly what "no files matched" means.
	out, err = s.ExecuteTool(context.Background(), map[string]any{"pattern": "not-here/**/*.go"})
	if err != nil {
		t.Fatalf("a pattern whose prefix is absent should match nothing, not fail: %v", err)
	}
	if !strings.Contains(out.Content, "(no files matched)") {
		t.Errorf("Content = %q, want (no files matched)", out.Content)
	}
}

// Paging, and the property that makes it worth having: page two continues
// where page one stopped, in the same newest-first order, with no overlap.
func TestGlobPagesThroughResults(t *testing.T) {
	root := t.TempDir()
	for i, name := range []string{"oldest.go", "middle.go", "newest.go"} {
		full := filepath.Join(root, name)
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		stamp := time.Now().Add(-time.Duration(2-i) * time.Hour)
		if err := os.Chtimes(full, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	s := &globSkill{root: root}

	first, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "*.go", "head_limit": 2})
	if err != nil {
		t.Fatalf("page one: %v", err)
	}
	if !strings.HasPrefix(first.Content, "newest.go\nmiddle.go") {
		t.Errorf("page one = %q, want the two newest in order", first.Content)
	}
	if !strings.Contains(first.Content, "offset=2") {
		t.Errorf("page one = %q, want the offset to continue from", first.Content)
	}

	second, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "*.go", "head_limit": 2, "offset": 2})
	if err != nil {
		t.Fatalf("page two: %v", err)
	}
	if strings.TrimSpace(second.Content) != "oldest.go" {
		t.Errorf("page two = %q, want only what page one did not show", second.Content)
	}

	past, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "*.go", "offset": 99})
	if err != nil {
		t.Fatalf("past the end: %v", err)
	}
	if !strings.Contains(past.Content, "no files past offset") {
		t.Errorf("past the end = %q, want it said so rather than looking like no match at all", past.Content)
	}
}

func TestGlobPrefix(t *testing.T) {
	cases := []struct{ pattern, want string }{
		{"aetox/output/**/*.html", "aetox/output"}, // the 51-second case
		{"**/*.go", ""},
		{"*.go", ""},
		{"src/**/*.{ts,svelte}", "src"},
		{"src/lib/App.svelte", "src/lib"},
		{`src\lib\**\*.ts`, "src/lib"},
		{"*/output/*.html", ""}, // meta in the first segment: cannot narrow
	}
	for _, c := range cases {
		if got := globPrefix(c.pattern); got != c.want {
			t.Errorf("globPrefix(%q) = %q, want %q", c.pattern, got, c.want)
		}
	}
}

// The walk must start at the pattern's literal prefix, not at the sandbox root:
// a narrow pattern never reaches maxResults, so a root-anchored walk visits
// every file under it before returning one hit.
func TestGlobDoesNotWalkOutsideThePatternPrefix(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("aetox/output/s1/index.html")
	mustWrite("elsewhere/decoy.html")

	// Cancelled up front: the walk aborts on the first callback, so the only way
	// to still return the hit is never to have walked outside the prefix... and
	// the only way to fail is to start at the root, where "elsewhere" comes first.
	out, err := (&globSkill{root: root}).ExecuteTool(context.Background(), map[string]any{
		"pattern": "aetox/output/**/*.html",
	})
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	body := out.Content
	if !strings.Contains(body, "aetox/output/s1/index.html") {
		t.Fatalf("missed the file it was pointed at: %q", body)
	}
	if strings.Contains(body, "decoy") {
		t.Fatalf("matched outside the prefix: %q", body)
	}
}

func TestGlobStopsWhenCancelled(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 50; i++ {
		dir := filepath.Join(root, "d", strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&globSkill{root: root}).ExecuteTool(ctx, map[string]any{"pattern": "**/*.go"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled glob returned %v, want context.Canceled", err)
	}
}
