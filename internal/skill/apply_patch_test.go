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
