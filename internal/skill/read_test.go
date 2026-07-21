package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSkillExecute(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi there"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"hello.txt"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Content != "hi there" {
		t.Errorf("Content = %q, want %q", out.Content, "hi there")
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
}

func TestReadSkillMissingFile(t *testing.T) {
	s := &readSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"does-not-exist.txt"}})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestReadSkillRejectsEscape(t *testing.T) {
	s := &readSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"../outside.txt"}})
	if err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestReadSkillEmptyFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty.txt"), []byte(""), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}
	out, err := s.Execute(context.Background(), Input{"args": []string{"empty.txt"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Content != "(empty file)" {
		t.Errorf("Content = %q, want %q", out.Content, "(empty file)")
	}
}

func TestReadSkillDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}
	_, err := s.Execute(context.Background(), Input{"args": []string{"subdir"}})
	if err == nil {
		t.Fatal("expected error reading a directory, got nil")
	}
}

func TestReadSkillExecuteTool(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Error("ExecuteTool with no path: expected error, got nil")
	}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "a.txt"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "content") {
		t.Errorf("Content = %q, want to contain %q", out.Content, "content")
	}
}
