package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFsSkillPwd(t *testing.T) {
	root := t.TempDir()
	s := &fsSkill{root: root}
	out, err := s.Execute(context.Background(), Input{"args": []string{"pwd"}})
	if err != nil {
		t.Fatalf("fs pwd: unexpected error: %v", err)
	}
	want, _ := filepath.Abs(root)
	if out.Content != want {
		t.Errorf("Content = %q, want %q", out.Content, want)
	}
}

func TestFsSkillLs(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &fsSkill{root: root}
	out, err := s.Execute(context.Background(), Input{"args": []string{"ls"}})
	if err != nil {
		t.Fatalf("fs ls: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "sub/") || !strings.Contains(out.Content, "file.txt") {
		t.Errorf("Content = %q, want to contain %q and %q", out.Content, "sub/", "file.txt")
	}
}

func TestFsSkillCat(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("body"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &fsSkill{root: root}
	out, err := s.Execute(context.Background(), Input{"args": []string{"cat", "a.txt"}})
	if err != nil {
		t.Fatalf("fs cat: unexpected error: %v", err)
	}
	if out.Content != "body" {
		t.Errorf("Content = %q, want %q", out.Content, "body")
	}
}

func TestFsSkillFind(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "needle.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &fsSkill{root: root}
	out, err := s.Execute(context.Background(), Input{"args": []string{"find", "needle"}})
	if err != nil {
		t.Fatalf("fs find: unexpected error: %v", err)
	}
	if out.Content != "needle.txt" {
		t.Errorf("Content = %q, want %q", out.Content, "needle.txt")
	}
}

func TestFsSkillFindRejectsGlob(t *testing.T) {
	s := &fsSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"find", "*.txt"}}); err == nil {
		t.Fatal("expected error for glob pattern, got nil")
	}
}

func TestFsSkillUnsupportedAction(t *testing.T) {
	s := &fsSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"rm", "x"}}); err == nil {
		t.Fatal("expected error for unsupported action, got nil")
	}
}
