package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bundledDoc returns one compiled-in skill's body, or fails the test.
func bundledDoc(t *testing.T, name string) DiscoveredSkill {
	t.Helper()
	for _, s := range bundledSkills() {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no bundled skill named %q — go:embed pattern and the folder name have to agree", name)
	return DiscoveredSkill{}
}

func TestTheAetoxSkillIsBundledAndParses(t *testing.T) {
	s := bundledDoc(t, "aetox")
	if strings.TrimSpace(s.Description) == "" {
		t.Error("no description — skills_list prints it, and a skill with no line there is one nobody opens")
	}
	if !s.Bundled {
		t.Error("Bundled is false; the delete and reveal surfaces key on it")
	}
	if s.Dir != "" {
		t.Errorf("Dir = %q, want empty — a bundled skill has no folder", s.Dir)
	}
	if len(strings.TrimSpace(s.body)) < 500 {
		t.Errorf("body is %d bytes; that is not a document", len(s.body))
	}
}


// The bundled skill has to show up through the ordinary door, not a second one.
// A separate path for bundled skills would be exactly the "two places answering
// the same question" this codebase treats as debt.
func TestBundledSkillsAppearInDiscovery(t *testing.T) {
	found := ListDiscovered([]string{filepath.Join(t.TempDir(), "nothing-here")})
	for _, s := range found {
		if s.Name == "aetox" {
			return
		}
	}
	t.Fatal("ListDiscovered did not include the bundled aetox skill")
}

// §35's rule, applied to skills: a user folder of the same name replaces the
// shipped document outright. Editing a bundled skill is copying it out.
func TestAUserSkillOverridesTheBundledOneOfTheSameName(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "aetox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("fixture dir: %v", err)
	}
	body := "mine, not the shipped one"
	content := "---\nname: aetox\ndescription: the user's own\n---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, skillFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	found := ListDiscovered([]string{root})
	var seen int
	for _, s := range found {
		if !strings.EqualFold(s.Name, "aetox") {
			continue
		}
		seen++
		if s.Bundled {
			t.Error("the bundled entry is still listed — the user's folder must replace it, not join it")
		}
		if s.Dir != dir {
			t.Errorf("Dir = %q, want %q", s.Dir, dir)
		}
	}
	if seen != 1 {
		t.Fatalf("found %d skills named aetox, want exactly 1", seen)
	}
}

// skill_view's `path` argument is a string the model wrote, and a bundled skill
// is read out of an FS rooted at itself — so `go.mod` is a file that exists,
// sits one directory above the root, and must not be served.
//
// The second half is the older guard, still load-bearing for a skill with
// neither a folder on disk nor one in the binary: its Dir is the empty string,
// and filepath.Abs turns that into the process's working directory, so without
// the check the containment test would be measuring cwd and every file under it
// would pass.
func TestBundledSkillRefusesAFileRead(t *testing.T) {
	view := &skillViewSkill{paths: []string{filepath.Join(t.TempDir(), "nothing-here")}}
	out, err := view.ExecuteTool(context.Background(), map[string]any{"name": "aetox", "path": "go.mod"})
	if err == nil {
		t.Fatalf("reading a file out of a bundled skill succeeded: %q", out.Content)
	}

	if _, err := readSkillFile(DiscoveredSkill{Name: "rootless", Bundled: true}, "go.mod"); err == nil {
		t.Fatal("a skill with no folder anywhere read a file from the working directory")
	}
}

// The skill tells the model which folders its own file tools always refuse, so
// it can say so instead of walking into the refusal and relaying it as a
// product limitation. That list is only useful while it matches the gate — and
// the gate is the kind of thing that gains an entry years after the document
// was written.
//
// So the document is checked against the real denylists rather than against a
// copy of them. Adding a credential store now makes this test red, and the fix
// is one line in SKILL.md.
func TestTheAetoxSkillNamesEveryRefusedPath(t *testing.T) {
	body := bundledDoc(t, "aetox").body
	for _, entry := range credentialStores {
		if !strings.Contains(body, entry) {
			t.Errorf("credentialStores has %q and the aetox skill never names it — "+
				"the assistant will try to read it and relay the refusal as a limitation", entry)
		}
	}
	for _, name := range ownSecretFiles {
		if !strings.Contains(body, name) {
			t.Errorf("ownSecretFiles has %q and the aetox skill never names it", name)
		}
	}
	// The shelf left credentialStores on 2026-08-20 and is refused by its own
	// gate now. It has to stay in the document for the same reason every entry
	// above does, and it is the one an assistant is most likely to walk into:
	// it is the only refused path that holds something the assistant genuinely
	// wants and can genuinely have, through the other door.
	if !strings.Contains(body, skillShelf) {
		t.Errorf("the skill shelf %q is refused to file tools and the aetox skill never names it", skillShelf)
	}
}

// The skills directory is the one Aetox-owned location that is deliberately not
// under DataRoot, and the Skills page already shipped three wrong copies of it
// (they said ~/.agents/skills, which belongs to opencode). A path spelled by
// hand in a document is a fourth copy — so it is checked against the function.
func TestTheAetoxSkillSpellsTheSkillsDirectoryCorrectly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	dir := DefaultSkillsDir()
	if dir == "" {
		t.Skip("no home directory resolvable on this machine")
	}
	rel, err := filepath.Rel(home, dir)
	if err != nil {
		t.Fatalf("skills dir %q is not under home %q: %v", dir, home, err)
	}
	want := "~/" + filepath.ToSlash(rel)

	body := bundledDoc(t, "aetox").body
	if !strings.Contains(body, want) {
		t.Errorf("the aetox skill never writes %q — DefaultSkillsDir moved and the document did not", want)
	}
}
