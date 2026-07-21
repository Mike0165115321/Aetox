package skill

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepo creates a minimal git repo with one commit in a temp dir.
func initGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestGitSkillStatus(t *testing.T) {
	dir := initGitRepo(t)
	s := &gitSkill{root: dir}
	out, err := s.Execute(context.Background(), Input{"args": []string{"status"}})
	if err != nil {
		t.Fatalf("git status: unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
}

func TestGitSkillLog(t *testing.T) {
	dir := initGitRepo(t)
	s := &gitSkill{root: dir}
	out, err := s.Execute(context.Background(), Input{"args": []string{"log"}})
	if err != nil {
		t.Fatalf("git log: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "init") {
		t.Errorf("Content = %q, want to contain commit message %q", out.Content, "init")
	}
}

func TestGitSkillRejectsUnsupportedAction(t *testing.T) {
	dir := initGitRepo(t)
	s := &gitSkill{root: dir}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"push"}}); err == nil {
		t.Fatal("expected error for unsupported git action (push), got nil")
	}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"commit"}}); err == nil {
		t.Fatal("expected error for unsupported git action (commit), got nil")
	}
}

func TestGitSkillBlocksUnsafeFlags(t *testing.T) {
	dir := initGitRepo(t)
	s := &gitSkill{root: dir}
	for _, args := range [][]string{
		{"status", "--git-dir=/etc"},
		{"status", "-C", "/etc"},
		{"log", "-c", "core.pager=evil"},
	} {
		if _, err := s.Execute(context.Background(), Input{"args": args}); err == nil {
			t.Errorf("args %v: expected unsafe-option error, got nil", args)
		}
	}
}

func TestGitSkillNotARepo(t *testing.T) {
	dir := t.TempDir() // no git init
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	s := &gitSkill{root: dir}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"status"}}); err == nil {
		t.Fatal("expected error outside a git repository, got nil")
	}
}
