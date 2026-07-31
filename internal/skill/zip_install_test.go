package skill

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZip builds an archive from path -> contents, in map order.
func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "skill.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, body := range files {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

// A skill has to arrive whole — SKILL.md plus every script, reference and
// asset it sends the model to, at any depth.
func TestZipInstallTakesTheWholeSet(t *testing.T) {
	archive := writeZip(t, map[string]string{
		"SKILL.md":            "---\nname: pdf\ndescription: pdfs\n---\nbody",
		"scripts/fill.py":     "print(1)",
		"scripts/lib/util.py": "helper",
		"references/forms.md": "reference",
		"assets/template.pdf": "%PDF-1.4",
	})
	root := filepath.Join(t.TempDir(), "skills")
	res, err := InstallSkillsFromZip(archive, root)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if res.Files != 5 {
		t.Errorf("wrote %d files, want all 5", res.Files)
	}
	for _, rel := range []string{"SKILL.md", "scripts/fill.py", "scripts/lib/util.py", "references/forms.md", "assets/template.pdf"} {
		if _, err := os.Stat(filepath.Join(root, res.Names[0], filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s missing after install: %v", rel, err)
		}
	}
	if got := ListDiscovered([]string{root}); len(got) != 1 {
		t.Fatalf("discovery found %d skills, want 1", len(got))
	}
}

// GitHub's "Download ZIP" wraps everything in <repo>-<branch>/. That wrapper is
// not part of the skill and must not become its folder name.
func TestZipInstallStripsTheDownloadWrapper(t *testing.T) {
	archive := writeZip(t, map[string]string{
		"my-skill-main/SKILL.md":        "---\nname: x\ndescription: y\n---\nb",
		"my-skill-main/references/a.md": "a",
	})
	root := filepath.Join(t.TempDir(), "skills")
	res, err := InstallSkillsFromZip(archive, root)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(res.Names) != 1 || res.Names[0] != "my-skill-main" {
		t.Fatalf("names = %v", res.Names)
	}
	// The wrapper is stripped from the *contents*, so SKILL.md sits at the top
	// of the installed folder where discovery looks for it.
	if _, err := os.Stat(filepath.Join(root, "my-skill-main", "SKILL.md")); err != nil {
		t.Errorf("SKILL.md is not at the root of the installed skill: %v", err)
	}
}

func TestZipInstallHandlesAFolderPerSkill(t *testing.T) {
	archive := writeZip(t, map[string]string{
		"bundle/pdf/SKILL.md":  "---\nname: pdf\ndescription: a\n---\nb",
		"bundle/xlsx/SKILL.md": "---\nname: xlsx\ndescription: a\n---\nb",
		"bundle/README.md":     "not a skill",
	})
	root := filepath.Join(t.TempDir(), "skills")
	res, err := InstallSkillsFromZip(archive, root)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(res.Names) != 2 {
		t.Fatalf("names = %v, want two skills", res.Names)
	}
	// README.md sits beside the skills, not inside one.
	if _, err := os.Stat(filepath.Join(root, "pdf", "README.md")); err == nil {
		t.Error("a file from outside the skill folder was installed into it")
	}
}

// Zip-slip: an entry that escapes the install root. The archive is refused
// whole, because one such entry is not a mistake.
func TestZipInstallRefusesPathsThatEscape(t *testing.T) {
	archive := writeZip(t, map[string]string{
		"SKILL.md":          "---\nname: x\ndescription: y\n---\nb",
		"../../../evil.txt": "pwned",
	})
	root := filepath.Join(t.TempDir(), "skills")
	if _, err := InstallSkillsFromZip(archive, root); err == nil {
		t.Fatal("an archive with an escaping path was accepted")
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("files were written despite the refusal: %v", entries)
	}
}

func TestZipInstallRejectsAnArchiveWithNoSkill(t *testing.T) {
	archive := writeZip(t, map[string]string{"README.md": "hello"})
	_, err := InstallSkillsFromZip(archive, filepath.Join(t.TempDir(), "skills"))
	if err == nil {
		t.Fatal("an archive with no SKILL.md was accepted")
	}
	if !strings.Contains(err.Error(), skillFileName) {
		t.Errorf("error %q does not name what was looked for", err)
	}
}

// Reinstalling replaces the previous copy — that is how a skill is updated —
// and the old one only goes once the new one is staged and proven.
func TestZipInstallReplacesAnExistingSkill(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	first := writeZip(t, map[string]string{
		"thing/SKILL.md": "---\nname: thing\ndescription: v1\n---\nold",
		"thing/gone.md":  "stale",
	})
	if _, err := InstallSkillsFromZip(first, root); err != nil {
		t.Fatal(err)
	}
	second := writeZip(t, map[string]string{
		"thing/SKILL.md": "---\nname: thing\ndescription: v2\n---\nnew",
	})
	if _, err := InstallSkillsFromZip(second, root); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "thing", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "v2") {
		t.Error("the old copy survived the reinstall")
	}
	// A file the new version dropped must not linger from the old one.
	if _, err := os.Stat(filepath.Join(root, "thing", "gone.md")); err == nil {
		t.Error("a file removed in the new version was left behind")
	}
}

// A large archive installs. There is no size or file-count ceiling: the zip is
// a local file the user chose, and how much of their own disk to spend on it is
// theirs to decide — same reasoning as the GitHub route.
func TestZipInstallHasNoSizeCeiling(t *testing.T) {
	files := map[string]string{"big/SKILL.md": "---\nname: big\ndescription: a\n---\nb"}
	const extra = 500
	for i := 0; i < extra; i++ {
		files[fmt.Sprintf("big/f%04d.md", i)] = strings.Repeat("x", 4096)
	}
	archive := writeZip(t, files)
	root := filepath.Join(t.TempDir(), "skills")
	res, err := InstallSkillsFromZip(archive, root)
	if err != nil {
		t.Fatalf("a large archive was refused: %v", err)
	}
	if res.Files != extra+1 {
		t.Errorf("wrote %d files, want all %d", res.Files, extra+1)
	}
	if _, err := os.Stat(filepath.Join(root, "big", "f0499.md")); err != nil {
		t.Errorf("the last file was not written: %v", err)
	}
}
