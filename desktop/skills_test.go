package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// newSkillsTestApp isolates both the config dir and the skill discovery root
// (~/.aetox/skills via USERPROFILE/HOME) into temp dirs.
func newSkillsTestApp(t *testing.T) *App {
	t.Helper()
	isolateUserDirs(t)
	return seed(&App{cfg: config.Config{ModelProvider: "noop", SandboxRoot: t.TempDir()}}, newConversation())
}

func writeTestSkill(t *testing.T, name string) string {
	t.Helper()
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".aetox", "skills", name)
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

	// Only the skills on disk are this test's business — a listing also carries
	// the ones compiled into the binary, which have no folder and no Remove.
	onDisk := func() []skill.DiscoveredSkill {
		var out []skill.DiscoveredSkill
		for _, s := range a.ListExternalSkills() {
			if !s.Bundled {
				out = append(out, s)
			}
		}
		return out
	}

	skills := onDisk()
	if len(skills) != 1 || skills[0].Name != "helper" || skills[0].Dir != dir {
		t.Fatalf("list = %+v, want one skill at %s", skills, dir)
	}

	if err := a.RemoveExternalSkill("helper"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("skill dir still exists after remove (err=%v)", err)
	}
	if got := onDisk(); len(got) != 0 {
		t.Fatalf("expected empty list after remove, got %+v", got)
	}

	if err := a.RemoveExternalSkill("missing"); err == nil {
		t.Fatal("expected not-found error")
	}
}

// A bundled skill has no folder, and os.RemoveAll("") returns nil — so the
// delete button would have reported success while the skill stayed exactly
// where it was. Refused with the override road named instead.
func TestRemoveBundledSkillIsRefusedNotSilentlyIgnored(t *testing.T) {
	a := newSkillsTestApp(t)

	var bundled string
	for _, s := range a.ListExternalSkills() {
		if s.Bundled {
			bundled = s.Name
			break
		}
	}
	if bundled == "" {
		t.Fatal("no bundled skill in the listing — bundled_skills.go embeds none")
	}

	err := a.RemoveExternalSkill(bundled)
	if err == nil {
		t.Fatalf("removing bundled skill %q reported success", bundled)
	}
	if !strings.Contains(err.Error(), skill.DefaultSkillsDir()) {
		t.Errorf("the refusal does not say where to put an override: %v", err)
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
