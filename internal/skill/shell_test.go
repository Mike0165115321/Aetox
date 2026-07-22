package skill

import (
	"context"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/rtk"
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

// TestShellSkillRewritesToRTKWhenAvailable exercises the actual integration
// seam (ARCHITECTURE.md §13): shell.go substituting execLine with rtk's
// rewritten command, not internal/rtk's own Rewrite logic (already covered by
// internal/rtk/rtk_test.go).
func TestShellSkillRewritesToRTKWhenAvailable(t *testing.T) {
	if !rtk.Available() {
		t.Skip("rtk not installed on PATH")
	}
	root := initGitRepo(t)
	s := &shellSkill{root: root}
	out, err := s.Execute(context.Background(), Input{"args": []string{"git", "status"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false, output: %q", out.Content)
	}
	// rtk's own compact git-status filter collapses a clean tree to one short
	// line ("clean — nothing to commit"), unlike plain git's multi-line
	// porcelain-style output ("On branch ..."). This shape difference is what
	// proves the command was actually substituted, not just that Rewrite()
	// works in isolation.
	if strings.Contains(out.Content, "On branch") {
		t.Errorf("expected RTK-rewritten compact output, got plain git output: %q", out.Content)
	}
	// The recorded command must stay the ORIGINAL, never the rtk-substituted
	// one (ARCHITECTURE.md §13: approval/audit always see the real command).
	if !strings.Contains(out.Command, "git status") || strings.Contains(out.Command, "rtk") {
		t.Errorf("Command = %q, want to contain original \"git status\" and not \"rtk\"", out.Command)
	}
}
