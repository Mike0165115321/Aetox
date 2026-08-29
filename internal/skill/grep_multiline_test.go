package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGrepFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
}

// A pattern crossing a newline finds nothing line by line and everything with
// multiline on. Both halves are asserted: the point of the flag is the
// difference between them.
func TestGrepMultilineCrossesNewlines(t *testing.T) {
	root := t.TempDir()
	writeGrepFiles(t, root, map[string]string{
		"a.go": "package main\n\ntype Options struct {\n\tRoot string\n}\n",
	})
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"pattern": `type Options struct \{\n\tRoot`,
		"show":    grepModeContent,
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !strings.Contains(out.Content, "(no matches)") {
		t.Errorf("line-by-line Content = %q, want no matches", out.Content)
	}

	out, err = s.ExecuteTool(context.Background(), map[string]any{
		"pattern":   `type Options struct \{\n\tRoot`,
		"show":      grepModeContent,
		"multiline": true,
	})
	if err != nil {
		t.Fatalf("ExecuteTool multiline: %v", err)
	}
	if !strings.Contains(out.Content, "a.go:3:type Options struct {") {
		t.Errorf("Content = %q, want the struct's opening line", out.Content)
	}
	// Every line the match spans is a match line, not context.
	if !strings.Contains(out.Content, "a.go:4:\tRoot string") {
		t.Errorf("Content = %q, want the second line of the span marked as a match", out.Content)
	}
}

// A match that ends on a newline belongs to the line before it, not the empty
// one after — the off-by-one this span arithmetic exists to get right.
func TestGrepMultilineSpanStopsAtLastMatchedLine(t *testing.T) {
	root := t.TempDir()
	writeGrepFiles(t, root, map[string]string{"a.txt": "one\ntwo\nthree\n"})
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"pattern":   "one\ntwo\n",
		"show":      grepModeContent,
		"multiline": true,
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if strings.Contains(out.Content, "a.txt:3:") {
		t.Errorf("Content = %q, want the span to stop at line 2", out.Content)
	}
}

// CRLF is folded before matching, so a pattern written with \n works on a file
// checked out on Windows — the promise lineendings.go already makes for edit.
func TestGrepMultilineFoldsCRLF(t *testing.T) {
	root := t.TempDir()
	writeGrepFiles(t, root, map[string]string{"a.txt": "alpha\r\nbeta\r\n"})
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"pattern":   "alpha\nbeta",
		"show":      grepModeContent,
		"multiline": true,
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !strings.Contains(out.Content, "a.txt:1:alpha") {
		t.Errorf("Content = %q, want a match across the CRLF", out.Content)
	}
}

// The dot crosses a newline with multiline on, and ^ and $ still mean line
// edges — together they are ripgrep's -U --multiline-dotall.
func TestGrepMultilineDotallAndAnchors(t *testing.T) {
	root := t.TempDir()
	writeGrepFiles(t, root, map[string]string{"a.txt": "start\nmiddle\nend\n"})
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"pattern":   "start.*end",
		"show":      grepModeContent,
		"multiline": true,
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !strings.Contains(out.Content, "a.txt:1:start") {
		t.Errorf("Content = %q, want . to cross newlines", out.Content)
	}

	out, err = s.ExecuteTool(context.Background(), map[string]any{
		"pattern":   "^middle$",
		"multiline": true,
	})
	if err != nil {
		t.Fatalf("ExecuteTool anchors: %v", err)
	}
	if !strings.Contains(out.Content, "a.txt") {
		t.Errorf("Content = %q, want ^ and $ to still mean line edges", out.Content)
	}
}

func TestGrepTypeFiltersByLanguage(t *testing.T) {
	root := t.TempDir()
	writeGrepFiles(t, root, map[string]string{
		"a.go":         "needle\n",
		"b.ts":         "needle\n",
		"c.tsx":        "needle\n",
		"d.md":         "needle\n",
		"sub/e.svelte": "needle\n",
	})
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "type": "go"})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !strings.Contains(out.Content, "a.go") || strings.Contains(out.Content, "b.ts") {
		t.Errorf("Content = %q, want only a.go", out.Content)
	}

	// One name, every extension the language is spelled with.
	out, err = s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "type": "ts"})
	if err != nil {
		t.Fatalf("ExecuteTool ts: %v", err)
	}
	if !strings.Contains(out.Content, "b.ts") || !strings.Contains(out.Content, "c.tsx") {
		t.Errorf("Content = %q, want both .ts and .tsx", out.Content)
	}
}

// type and glob are both filters, not a pair where the later one wins.
func TestGrepTypeComposesWithGlob(t *testing.T) {
	root := t.TempDir()
	writeGrepFiles(t, root, map[string]string{
		"store.ts": "needle\n",
		"view.ts":  "needle\n",
		"store.go": "needle\n",
	})
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"pattern": "needle", "type": "ts", "glob": "store.*",
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !strings.Contains(out.Content, "store.ts") {
		t.Errorf("Content = %q, want store.ts", out.Content)
	}
	if strings.Contains(out.Content, "view.ts") || strings.Contains(out.Content, "store.go") {
		t.Errorf("Content = %q, want both filters applied", out.Content)
	}
}

// An unknown type is refused by name. Matching nothing would answer
// "(no matches)", which reads as "that string is not in your repository".
func TestGrepUnknownTypeIsRefused(t *testing.T) {
	root := t.TempDir()
	writeGrepFiles(t, root, map[string]string{"a.go": "needle\n"})
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "type": "golang"})
	if err == nil {
		t.Fatal("ExecuteTool: want an error for an unknown type")
	}
	if out.Success {
		t.Error("Success = true, want false")
	}
	if !strings.Contains(err.Error(), "go") {
		t.Errorf("error = %q, want it to list the known types", err)
	}
}

func TestGrepTypeNamesAreSortedAndComplete(t *testing.T) {
	names := GrepTypeNames()
	if len(names) != len(grepTypes) {
		t.Fatalf("GrepTypeNames() has %d entries, table has %d", len(names), len(grepTypes))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("GrepTypeNames() is not sorted: %q before %q", names[i-1], names[i])
		}
	}
}
