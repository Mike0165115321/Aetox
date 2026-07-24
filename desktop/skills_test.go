package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// newSkillsTestApp isolates both the config dir and the skill discovery roots
// (~/.agents/skills via USERPROFILE/HOME) into temp dirs.
func newSkillsTestApp(t *testing.T) *App {
	t.Helper()
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("LOCALAPPDATA", base)
	t.Setenv("USERPROFILE", base)
	t.Setenv("HOME", base)
	return &App{cfg: config.Config{ModelProvider: "noop", SandboxRoot: t.TempDir()}}
}

func writeTestSkill(t *testing.T, name string) string {
	t.Helper()
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".agents", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	md := "---\nname: " + name + "\ndescription: a test skill\n---\nDo the thing.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return dir
}

func TestListAndRemoveExternalSkill(t *testing.T) {
	a := newSkillsTestApp(t)
	dir := writeTestSkill(t, "helper")

	skills := a.ListExternalSkills()
	if len(skills) != 1 || skills[0].Name != "helper" || skills[0].Dir != dir {
		t.Fatalf("list = %+v, want one skill at %s", skills, dir)
	}

	if err := a.RemoveExternalSkill("helper"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("skill dir still exists after remove (err=%v)", err)
	}
	if got := a.ListExternalSkills(); len(got) != 0 {
		t.Fatalf("expected empty list after remove, got %+v", got)
	}

	if err := a.RemoveExternalSkill("missing"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestInstallSkillValidation(t *testing.T) {
	a := newSkillsTestApp(t)
	if _, err := a.InstallSkillFromGitHub("  "); err == nil {
		t.Fatal("expected error for empty url")
	}
	// Engine not bootstrapped yet → registry nil → clear error, no panic.
	if _, err := a.InstallSkillFromGitHub("https://github.com/x/y"); err == nil {
		t.Fatal("expected engine-not-ready error")
	}
}
