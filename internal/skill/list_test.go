package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestListSkillExecute(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"b.txt", "a.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	s := &listSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": nil})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	want := "a.txt\nb.txt"
	if out.Content != want {
		t.Errorf("Content = %q, want %q (sorted)", out.Content, want)
	}
}

// Without the "/" a model cannot tell a folder from a file and has to call
// list again on each name to find out.
func TestListSkillMarksDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &listSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	// The pair is deliberate: one name is a prefix of the other, which is
	// exactly when guessing from the name alone goes wrong.
	want := "sub.txt\nsub/"
	if out.Content != want {
		t.Errorf("Content = %q, want %q", out.Content, want)
	}
}

func TestListSkillMissingDir(t *testing.T) {
	s := &listSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"nope"}})
	if err == nil {
		t.Fatal("expected error for missing directory, got nil")
	}
}

func TestListSkillExecuteToolDefaultsToRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "only.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &listSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if out.Content != "only.txt" {
		t.Errorf("Content = %q, want %q", out.Content, "only.txt")
	}
}
