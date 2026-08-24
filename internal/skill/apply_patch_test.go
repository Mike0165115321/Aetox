package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func patchRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestApplyPatchAcrossFilesInOneCall(t *testing.T) {
	root := patchRoot(t, map[string]string{
		"a.go": "package a\n\nconst Name = \"old\"\n",
		"b.go": "package b\n\nconst Name = \"old\"\n",
	})
	s := &applyPatchSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"edits": []any{
		map[string]any{"path": "a.go", "old_string": `"old"`, "new_string": `"new"`},
		map[string]any{"path": "b.go", "old_string": `"old"`, "new_string": `"new"`},
	}})
	if err != nil {
		t.Fatalf("apply_patch failed: %v", err)
	}
	for _, rel := range []string{"a.go", "b.go"} {
		body, _ := os.ReadFile(filepath.Join(root, rel))
		if !strings.Contains(string(body), `"new"`) {
			t.Errorf("%s was not patched: %s", rel, body)
		}
	}
	if out.LinesAdded != 2 || out.LinesRemoved != 2 {
		t.Errorf("line delta = +%d -%d, want +2 -2", out.LinesAdded, out.LinesRemoved)
	}
}

// The whole point of a patch over N edit calls: a half-applied one leaves the
// tree in a state the model then reasons about wrongly.
func TestApplyPatchWritesNothingWhenAnyEditFails(t *testing.T) {
	root := patchRoot(t, map[string]string{
		"a.go": "package a\n\nconst Name = \"old\"\n",
		"b.go": "package b\n",
	})
	s := &applyPatchSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{"edits": []any{
		map[string]any{"path": "a.go", "old_string": `"old"`, "new_string": `"new"`},
		map[string]any{"path": "b.go", "old_string": "nope", "new_string": "x"},
	}})
	if err == nil {
		t.Fatal("expected the patch to be rejected")
	}
	if !strings.Contains(err.Error(), "nothing was written") {
		t.Errorf("error should say nothing was written, got: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "a.go"))
	if strings.Contains(string(body), `"new"`) {
		t.Error("the first edit landed even though the patch failed — not atomic")
	}
}

// Two edits to one file in a single patch: the second must see the first.
func TestApplyPatchSequencesEditsToTheSameFile(t *testing.T) {
	root := patchRoot(t, map[string]string{"a.go": "one\ntwo\n"})
	s := &applyPatchSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{"edits": []any{
		map[string]any{"path": "a.go", "old_string": "one", "new_string": "1"},
		map[string]any{"path": "a.go", "old_string": "two", "new_string": "2"},
	}}); err != nil {
		t.Fatalf("apply_patch failed: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "a.go"))
	if string(body) != "1\n2\n" {
		t.Errorf("got %q, want \"1\n2\n\"", body)
	}
}

func TestApplyPatchRejectsAmbiguousMatch(t *testing.T) {
	root := patchRoot(t, map[string]string{"a.go": "x\nx\n"})
	s := &applyPatchSkill{root: root}
	_, err := s.ExecuteTool(context.Background(), map[string]any{"edits": []any{
		map[string]any{"path": "a.go", "old_string": "x", "new_string": "y"},
	}})
	if err == nil || !strings.Contains(err.Error(), "matches 2 times") {
		t.Errorf("ambiguous match should be refused, got: %v", err)
	}
}

// The shape a model actually sends when every edit is in one file.
//
// From the owner's own log (24 ส.ค. 03:16): `{"path": "notes-test/03-...md",
// "edits": [{old_string, new_string}, ...]}` came back as "edit 1: path is
// required" — a refusal about shape, for a call whose meaning was never in
// doubt. Several edits to one file is the commonest patch there is, and a model
// that has just named the file does not expect to name it again on every item.
func TestApplyPatchTakesThePathOnceForTheWholeCall(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &applyPatchSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"path": "notes.md",
		"edits": []any{
			map[string]any{"old_string": "one", "new_string": "ONE"},
			map[string]any{"old_string": "three", "new_string": "THREE"},
		},
	})
	if err != nil {
		t.Fatalf("apply_patch refused the shape it was sent: %v", err)
	}
	if !out.Success {
		t.Errorf("Success = false: %+v", out)
	}
	data, _ := os.ReadFile(filepath.Join(root, "notes.md"))
	if string(data) != "ONE\ntwo\nTHREE\n" {
		t.Errorf("notes.md = %q", string(data))
	}
}

// The default is a default, not an override: an edit that names its own file
// still goes to that file, so one call can name the common case once and still
// reach elsewhere.
func TestAnEditsOwnPathBeatsTheCallDefault(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &applyPatchSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path": "a.md",
		"edits": []any{
			map[string]any{"old_string": "x", "new_string": "A"},
			map[string]any{"path": "b.md", "old_string": "x", "new_string": "B"},
		},
	}); err != nil {
		t.Fatalf("apply_patch: %v", err)
	}
	a, _ := os.ReadFile(filepath.Join(root, "a.md"))
	b, _ := os.ReadFile(filepath.Join(root, "b.md"))
	if string(a) != "A\n" || string(b) != "B\n" {
		t.Errorf("a.md = %q, b.md = %q", string(a), string(b))
	}
}

// Naming no file anywhere is still the error it always was, and the message now
// says both places one could go.
func TestApplyPatchStillNeedsAFileSomewhere(t *testing.T) {
	s := &applyPatchSkill{root: t.TempDir()}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"edits": []any{map[string]any{"old_string": "x", "new_string": "y"}},
	})
	if err == nil {
		t.Fatal("want a refusal")
	}
	if !strings.Contains(err.Error(), "top level") {
		t.Errorf("the refusal does not say where the path may go: %q", err)
	}
}
