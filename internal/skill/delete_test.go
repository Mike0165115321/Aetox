package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// A folder is only removed when the caller said so. "delete" of a path that
// turned out to be a directory is the one mistake with no undo at the tool
// level, so it is named rather than inferred.
func TestDeleteSkillNeedsRecursiveForAFolder(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &deleteSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{"path": "sub"})
	if err == nil || !strings.Contains(err.Error(), "recursive") {
		t.Fatalf("err = %v, want it to name the flag that would allow this", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "sub")); statErr != nil {
		t.Fatal("the folder was removed despite the refusal")
	}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{"path": "sub", "recursive": true}); err != nil {
		t.Fatalf("recursive delete: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "sub")); !os.IsNotExist(statErr) {
		t.Error("the folder is still there after a recursive delete")
	}
}

// The sandbox root is the project, not a thing to delete.
func TestDeleteSkillRefusesTheRoot(t *testing.T) {
	s := &deleteSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"path": ".", "recursive": true}); err == nil {
		t.Fatal("deleting the sandbox root must be refused")
	}
}
