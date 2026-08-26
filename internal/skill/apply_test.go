package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempHome points DefaultSkillsDir at a throwaway directory so an edit never
// touches the developer's real ~/.aetox/skills. os.UserHomeDir reads USERPROFILE
// on Windows and HOME elsewhere; set both.
func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	return filepath.Join(home, ".aetox", "skills")
}

// The one thing this writer does that memory's does not: a bundled skill is
// copied out whole on its first edit, because withBundled drops the bundled copy
// the moment a disk folder of its name exists — a SKILL.md without its
// references/ would be the "crippled skill" the folder embed exists to prevent.
func TestCopyBundledSkillOutBringsTheWholeFolder(t *testing.T) {
	skills := tempHome(t)
	dir := filepath.Join(skills, "aetox-ui-design")
	if err := copyBundledSkillOut("aetox-ui-design", dir); err != nil {
		t.Fatalf("copy: %v", err)
	}
	for _, rel := range []string{"SKILL.md", filepath.Join("references", "responsive-design.md")} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("copy-out is missing %s: %v", rel, err)
		}
	}
}

func TestApplyCopiesABundledSkillOutOnFirstEdit(t *testing.T) {
	skills := tempHome(t)
	if err := Apply("aetox-ui-design", "add", "", "APPENDED-MARKER"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	dir := filepath.Join(skills, "aetox-ui-design")
	data, err := os.ReadFile(filepath.Join(dir, skillFileName))
	if err != nil {
		t.Fatalf("read edited skill: %v", err)
	}
	if !strings.Contains(string(data), "APPENDED-MARKER") {
		t.Error("the appended body did not land in the skill on disk")
	}
	if _, err := os.Stat(filepath.Join(dir, "references", "responsive-design.md")); err != nil {
		t.Errorf("the first edit copied SKILL.md without its references: %v", err)
	}
}

func TestApplyReplacesAPassage(t *testing.T) {
	tempHome(t)
	if err := Apply("aetox-ui-design", "add", "", "ORIGINAL-LINE"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := Apply("aetox-ui-design", "replace", "ORIGINAL-LINE", "CHANGED-LINE"); err != nil {
		t.Fatalf("replace: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(DefaultSkillsDir(), "aetox-ui-design", skillFileName))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "ORIGINAL-LINE") || !strings.Contains(s, "CHANGED-LINE") {
		t.Errorf("replace did not swap the passage")
	}
}

// A replace whose anchor is gone — the SKILL.md was hand-edited, or an earlier
// approval already changed it — is refused, not guessed at.
func TestApplyRefusesAStaleReplace(t *testing.T) {
	tempHome(t)
	if err := Apply("aetox-ui-design", "replace", "TEXT-THAT-IS-NOT-PRESENT", "x"); err == nil {
		t.Error("a replace with no matching anchor should error")
	}
}

// Nothing to copy and nothing to edit.
func TestApplyRefusesAnUnknownSkill(t *testing.T) {
	tempHome(t)
	if err := Apply("no-such-skill-xyz", "add", "", "x"); err == nil {
		t.Error("editing a skill that does not exist should error")
	}
}
