package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteSkillRemovesFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "gone.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &deleteSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"gone.txt"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("file still exists after delete: %v", statErr)
	}
}

func TestDeleteSkillMissingFile(t *testing.T) {
	s := &deleteSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"nope.txt"}}); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestDeleteSkillRefusesDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "adir"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &deleteSkill{root: root}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"adir"}}); err == nil {
		t.Fatal("expected error deleting a directory, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(root, "adir")); statErr != nil {
		t.Errorf("directory should still exist: %v", statErr)
	}
}

func TestDeleteSkillRejectsEscape(t *testing.T) {
	s := &deleteSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"../escape.txt"}}); err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}
