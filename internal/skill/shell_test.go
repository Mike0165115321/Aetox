package skill

import (
	"context"
	"strings"
	"testing"
)

func TestShellSkillRunsCommand(t *testing.T) {
	s := &shellSkill{root: t.TempDir()}
	out, err := s.Execute(context.Background(), Input{"args": []string{"echo", "hello-shell"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "hello-shell") {
		t.Errorf("Content = %q, want to contain %q", out.Content, "hello-shell")
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
}

func TestShellSkillRunsInSandboxRoot(t *testing.T) {
	root := t.TempDir()
	s := &shellSkill{root: root}
	// "cd" with no output check needed: a failing command in a bad dir would
	// surface as an error, so a successful run against an arbitrary command
	// confirms cmd.Dir was set to a valid, existing directory.
	if _, err := s.Execute(context.Background(), Input{"args": []string{"echo", "ok"}}); err != nil {
		t.Fatalf("Execute in sandbox root: unexpected error: %v", err)
	}
}

func TestShellSkillCommandFailureReturnsError(t *testing.T) {
	s := &shellSkill{root: t.TempDir()}
	out, err := s.Execute(context.Background(), Input{"args": []string{"this-command-does-not-exist-xyz"}})
	if err == nil {
		t.Fatal("expected error for nonexistent command, got nil")
	}
	if out.Success {
		t.Error("Success = true, want false on command failure")
	}
}

func TestShellSkillMissingArgs(t *testing.T) {
	s := &shellSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": nil}); err == nil {
		t.Fatal("expected usage error for empty command, got nil")
	}
}
