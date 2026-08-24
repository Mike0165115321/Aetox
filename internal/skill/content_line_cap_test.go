package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func linesOf(n int) string {
	rows := make([]string, n)
	for i := range rows {
		rows[i] = "line"
	}
	return strings.Join(rows, "\n")
}

// A file ending in a newline has as many lines as it looks like it has. Off by
// one here turns the boundary into a lie in whichever direction it errs.
func TestContentLinesDoesNotInventATrailingLine(t *testing.T) {
	cases := map[string]int{"": 0, "a": 1, "a\n": 1, "a\nb": 2, "a\nb\n": 2}
	for input, want := range cases {
		if got := contentLines(input); got != want {
			t.Errorf("contentLines(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestContentLineCapBoundary(t *testing.T) {
	if err := checkContentLineCap("content", linesOf(contentLineCap)); err != nil {
		t.Fatalf("exactly the cap must pass, got %v", err)
	}
	err := checkContentLineCap("content", linesOf(contentLineCap+1))
	if err == nil {
		t.Fatal("one line over the cap must be refused")
	}
	for _, want := range []string{"301", "300", "Nothing was written", "mode=append"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal is missing %q: %s", want, err.Error())
		}
	}
}

// The point of a cap over a salvage: an over-long write leaves nothing behind.
// A partial file that opens cleanly and is missing its second half is the
// failure this whole design exists to avoid.
func TestWriteSkillOverCapWritesNothing(t *testing.T) {
	root := t.TempDir()
	s := &writeSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":    "big.html",
		"content": linesOf(contentLineCap + 50),
	})
	if err == nil {
		t.Fatal("expected a refusal over the cap")
	}
	if out.Success {
		t.Error("Success = true on a refused write")
	}
	if _, statErr := os.Stat(filepath.Join(root, "big.html")); statErr == nil {
		t.Fatal("a refused write must not leave a file on disk")
	}
}

func TestWriteSkillAtCapStillWrites(t *testing.T) {
	root := t.TempDir()
	s := &writeSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":    "ok.txt",
		"content": linesOf(contentLineCap),
	}); err != nil {
		t.Fatalf("a file exactly at the cap must be written: %v", err)
	}
}

// The other door. A cap that only watched write would be satisfied by moving
// the same content into one enormous append.
func TestEditAppendOverCapChangesNothing(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "cont.txt", "start\n")
	s := &editSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":    "cont.txt",
		"replace": linesOf(contentLineCap + 1),
		"mode":    "append",
	})
	if err == nil {
		t.Fatal("append must obey the same cap as write")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "start\n" {
		t.Fatalf("a refused append must leave the file untouched, got %q", string(data))
	}
}

// Replace is deliberately uncapped. One substitution cannot be split in half,
// so a cap there would refuse correct work and offer nothing in its place.
func TestEditReplaceIsNotCapped(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "big.go", "OLD_BLOCK\n")
	s := &editSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":    "big.go",
		"find":    "OLD_BLOCK",
		"replace": linesOf(contentLineCap + 100),
	}); err != nil {
		t.Fatalf("a large replace must still be allowed: %v", err)
	}
	data, _ := os.ReadFile(path)
	if contentLines(string(data)) != contentLineCap+100 {
		t.Errorf("replacement did not land whole, got %d lines", contentLines(string(data)))
	}
}
