package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hunk.go is tested on its own. These are about the wiring: a diff that is
// built correctly and never leaves the tool is a diff the โค้ด desk cannot
// draw, and the counts would go on looking like the whole story.

func TestEditCarriesTheDiffOut(t *testing.T) {
	root := t.TempDir()
	writeEditFixture(t, root, "a.go", "package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n")
	s := &editSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "a.go",
		"old_string": "println(\"old\")",
		"new_string": "println(\"new\")",
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if out.Diff == "" {
		t.Fatal("Diff is empty; the row would have nothing to unfold")
	}
	if !strings.Contains(out.Diff, "-\tprintln(\"old\")") ||
		!strings.Contains(out.Diff, "+\tprintln(\"new\")") {
		t.Errorf("Diff does not show the swap:\n%s", out.Diff)
	}
	// Line 4 of the file, and the hunk has to say so — a diff that numbers from
	// its own first line sends the reader to the wrong place.
	if !strings.HasPrefix(out.Diff, "@@ -1,5 +1,5 @@") {
		t.Errorf("hunk header = %q, want the file's own numbering", strings.SplitN(out.Diff, "\n", 2)[0])
	}
}

// replace_all is the case the two strings cannot describe: one pair of strings,
// eight places in the file.
func TestEditDiffShowsEveryOccurrenceItReplaced(t *testing.T) {
	root := t.TempDir()
	var body strings.Builder
	for i := 0; i < 3; i++ {
		body.WriteString("call(oldName)\n")
		body.WriteString("filler\nfiller\nfiller\nfiller\nfiller\nfiller\nfiller\n")
	}
	writeEditFixture(t, root, "a.go", body.String())
	s := &editSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":        "a.go",
		"old_string":  "oldName",
		"new_string":  "newName",
		"replace_all": true,
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if n := strings.Count(out.Diff, "+call(newName)"); n != 3 {
		t.Errorf("diff shows %d of 3 replaced call sites:\n%s", n, out.Diff)
	}
}

func TestWriteCarriesTheDiffOut(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &writeSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":    "a.txt",
		"content": "one\nTWO\nthree\n",
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !strings.Contains(out.Diff, "-two") || !strings.Contains(out.Diff, "+TWO") {
		t.Errorf("Diff does not show the one changed line:\n%s", out.Diff)
	}
	// The whole point of diffing a `write`: the tool rewrote three lines and
	// changed one, and the row should not read like a rewrite.
	if strings.Contains(out.Diff, "-one") {
		t.Errorf("untouched line reported as removed:\n%s", out.Diff)
	}
}

func TestApplyPatchNamesEachFileItChanged(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("keep\ntarget\nkeep\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	s := &applyPatchSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"edits": []any{
			map[string]any{"path": "a.txt", "old_string": "target", "new_string": "AAA"},
			map[string]any{"path": "b.txt", "old_string": "target", "new_string": "BBB"},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	for _, want := range []string{"+++ a.txt", "+AAA", "+++ b.txt", "+BBB"} {
		if !strings.Contains(out.Diff, want) {
			t.Errorf("Diff is missing %q:\n%s", want, out.Diff)
		}
	}
}

// Two edits to one file in one call: the diff is measured against what was on
// disk when the call started, not against the first edit's result.
func TestApplyPatchDiffsOneFileOnceForTheWholeCall(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &applyPatchSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"edits": []any{
			map[string]any{"path": "a.txt", "old_string": "alpha", "new_string": "ALPHA"},
			map[string]any{"path": "a.txt", "old_string": "beta", "new_string": "BETA"},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if n := strings.Count(out.Diff, "+++ a.txt"); n != 1 {
		t.Errorf("file named %d times, want once:\n%s", n, out.Diff)
	}
	for _, want := range []string{"-alpha", "+ALPHA", "-beta", "+BETA"} {
		if !strings.Contains(out.Diff, want) {
			t.Errorf("Diff is missing %q:\n%s", want, out.Diff)
		}
	}
}

// Every other tool reports nothing here, and must keep doing so — a diff on a
// row that wrote no file would make the row expandable onto an empty box.
func TestReadingToolsCarryNoDiff(t *testing.T) {
	root := t.TempDir()
	writeEditFixture(t, root, "a.txt", "hello\n")

	out, err := (&readSkill{root: root}).ExecuteTool(context.Background(), map[string]any{"path": "a.txt"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out.Diff != "" {
		t.Errorf("read reported a diff: %q", out.Diff)
	}
}
