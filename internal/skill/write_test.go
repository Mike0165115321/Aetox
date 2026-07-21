package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSkillCreatesFile(t *testing.T) {
	root := t.TempDir()
	s := &writeSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"a.txt", "hello", "world"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
	data, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("file content = %q, want %q", string(data), "hello world")
	}
}

func TestWriteSkillCreatesParentDirs(t *testing.T) {
	root := t.TempDir()
	s := &writeSkill{root: root}

	if _, err := s.Execute(context.Background(), Input{"args": []string{"nested/dir/b.txt", "x"}}); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nested", "dir", "b.txt")); err != nil {
		t.Errorf("nested file not created: %v", err)
	}
}

func TestWriteSkillOverwrites(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "c.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &writeSkill{root: root}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"c.txt", "new"}}); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "c.txt"))
	if string(data) != "new" {
		t.Errorf("content = %q, want %q", string(data), "new")
	}
}

func TestWriteSkillRejectsEscape(t *testing.T) {
	s := &writeSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"../escape.txt", "x"}})
	if err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestWriteSkillMissingArgs(t *testing.T) {
	s := &writeSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"onlypath"}}); err == nil {
		t.Fatal("expected error for missing content arg, got nil")
	}
}

func TestWriteSkillExecuteToolMissingPath(t *testing.T) {
	s := &writeSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"content": "x"}); err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}
