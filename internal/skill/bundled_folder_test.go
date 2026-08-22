package skill

import (
	"context"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// shippedPack is a skill of the shape every published one has: a SKILL.md that
// routes, and the files it routes to.
func shippedPack() fstest.MapFS {
	return fstest.MapFS{
		"pack/architect/SKILL.md": &fstest.MapFile{Data: []byte(
			"---\nname: architect\ndescription: draws the current system\n---\nRead references/inspect.md before mapping.\n")},
		"pack/architect/references/inspect.md":   &fstest.MapFile{Data: []byte("count the modules first")},
		"pack/architect/templates/module-map.md": &fstest.MapFile{Data: []byte("# Module map")},
		"pack/onedoc/SKILL.md": &fstest.MapFile{Data: []byte(
			"---\nname: onedoc\ndescription: fits in one file\n---\nAll of it is here.\n")},
	}
}

func shippedView(t *testing.T) *skillViewSkill {
	t.Helper()
	found, errs := EmbeddedSkills(shippedPack(), "pack")
	if len(errs) > 0 {
		t.Fatalf("EmbeddedSkills: %v", errs)
	}
	if len(found) != 2 {
		t.Fatalf("found %d skills; want 2", len(found))
	}
	return &skillViewSkill{paths: []string{t.TempDir()}, extra: found}
}

// The whole feature. A published skill is a router — SKILL.md names
// references/, templates/ and rules/ beside it — and until 2026-08-22 the
// bundle carried skills/*/SKILL.md and nothing else. A shipped skill of that
// shape was a document full of doors that opened onto nothing: the sentence
// "read references/inspect.md" reached every machine and the file reached none.
func TestABundledSkillServesTheFilesBesideIt(t *testing.T) {
	view := shippedView(t)

	out, err := view.ExecuteTool(context.Background(), map[string]any{"name": "architect"})
	if err != nil {
		t.Fatalf("skill_view: %v", err)
	}
	for _, want := range []string{"references/inspect.md", "templates/module-map.md"} {
		if !strings.Contains(out.Content, want) {
			t.Fatalf("the body does not list %q, so the model cannot ask for it:\n%s", want, out.Content)
		}
	}
	if strings.Contains(out.Content, "SKILL.md") {
		t.Error("the listing offers SKILL.md, which is the document the model is already holding")
	}

	out, err = view.ExecuteTool(context.Background(), map[string]any{"name": "architect", "path": "references/inspect.md"})
	if err != nil {
		t.Fatalf("reading a file that ships inside the binary failed: %v", err)
	}
	if !strings.Contains(out.Content, "count the modules first") {
		t.Fatalf("wrong content for references/inspect.md: %q", out.Content)
	}
}

// It has no folder on disk and that stays true: glob and shell cannot reach
// inside the binary, and a model told otherwise spends rounds proving it.
func TestABundledSkillWithAFolderStillSaysItHasNoneOnDisk(t *testing.T) {
	view := shippedView(t)
	out, err := view.ExecuteTool(context.Background(), map[string]any{"name": "architect"})
	if err != nil {
		t.Fatalf("skill_view: %v", err)
	}
	if !strings.Contains(out.Content, "no folder on disk") || !strings.Contains(out.Content, "glob or shell") {
		t.Errorf("a skill inside the binary no longer says where not to look:\n%s", out.Content)
	}
	if !strings.Contains(out.Content, "skill_view with a path is the only door") {
		t.Errorf("it says the files cannot be found and never says how they can:\n%s", out.Content)
	}

	// A skill that really is one document says so, rather than reporting the
	// file it does not have as missing.
	single, err := view.ExecuteTool(context.Background(), map[string]any{"name": "onedoc"})
	if err != nil {
		t.Fatalf("skill_view: %v", err)
	}
	if strings.Contains(single.Content, "only door") {
		t.Errorf("a one-document skill points at files it does not carry:\n%s", single.Content)
	}
}

// `path` is a string the model wrote. On disk the guard is filepath.Rel against
// the skill's own directory; inside the binary the root is an fs.FS that cannot
// express a parent at all — but the spelling still has to be refused, because
// path.Clean folds "references/../../" into a name that leaves the root and
// fs.ValidPath is what catches it.
func TestABundledSkillRefusesAPathThatClimbsOut(t *testing.T) {
	view := shippedView(t)
	for _, sub := range []string{
		"../onedoc/SKILL.md",
		"references/../../onedoc/SKILL.md",
		"../../../go.mod",
		"/etc/passwd",
	} {
		out, err := view.ExecuteTool(context.Background(), map[string]any{"name": "architect", "path": sub})
		if err == nil {
			t.Errorf("%q was served out of the skill: %q", sub, out.Content)
		}
	}
}

// The bundle embeds a folder now, so a reference file added beside a shipped
// SKILL.md ships with it. This is the guard on that: every file the binary
// carries for a skill has to be one the model can be handed, or it is weight in
// the installer that nothing can reach.
func TestEveryFileBundledBesideASkillIsReachable(t *testing.T) {
	for _, d := range bundledSkills() {
		listed := map[string]bool{}
		for _, name := range supportingFiles(d) {
			listed[name] = true
		}
		root := path.Join(bundledSkillRoot, d.Name)
		err := fs.WalkDir(bundledSkillFS, root, func(name string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			rel := strings.TrimPrefix(name, root+"/")
			if rel == skillFileName {
				return nil
			}
			if !listed[rel] {
				t.Errorf("%s ships %s and skill_view never offers it", d.Name, rel)
				return nil
			}
			if _, readErr := readSkillFile(d, rel); readErr != nil {
				t.Errorf("%s offers %s and cannot serve it: %v", d.Name, rel, readErr)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

// namedFiles pulls the supporting files a skill body tells the model to open.
//
// namedFiles pulls the supporting files a skill body tells the model to open.
//
// The filter is the skill's own shipped folders, not a list of folder names
// written here, and that is what makes it usable on a real skill body. A
// document is full of paths that are not doors: `aetox` names the repo file
// docs/architecture/agent-package-standard-2026-08-08.md, and
// `aetox-design-system` names assets/design-tokens.css and
// docs/brand-guidelines.md, which are files in the *user's project* that the
// skill is telling the model to go and read there. None of those first
// segments is a folder the skill ships, so none of them is a promise it made.
// An extension is still required, so a bare "references/" reads as prose.
func namedFiles(d DiscoveredSkill) []string {
	ships := map[string]bool{}
	for _, f := range supportingFiles(d) {
		if i := strings.Index(f, "/"); i > 0 {
			ships[f[:i]] = true
		}
	}
	pattern := regexp.MustCompile(`(^|[^A-Za-z0-9._/-])([A-Za-z0-9._-]+/[A-Za-z0-9._/-]*[A-Za-z0-9._-]+\.[A-Za-z0-9]+)`)
	seen := map[string]bool{}
	var out []string
	for _, m := range pattern.FindAllStringSubmatch(d.body, -1) {
		name := m[2]
		if seen[name] || !ships[name[:strings.Index(name, "/")]] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// The other side of TestEveryFileBundledBesideASkillIsReachable: that one
// catches weight in the installer nothing can reach, this one catches a door
// the model is invited through and then refused at.
//
// Both matter more now than they did. Before the folder travelled with the
// binary a shipped skill was one document and neither mistake was possible;
// with references/ in the bundle, a file renamed in one place and named in the
// other is a silent dead end that only shows up mid-task.
func TestNoBundledSkillNamesAFileItDoesNotShip(t *testing.T) {
	for _, d := range bundledSkills() {
		for _, named := range namedFiles(d) {
			if _, err := readSkillFile(d, named); err != nil {
				t.Errorf("%s points the model at %q and does not ship it: %v", d.Name, named, err)
			}
		}
	}
}

// A guard whose matcher silently matches nothing is a test that passes for the
// wrong reason, and the two above are only as good as this function.
func TestNamedFilesFindsDoorsAndNotProse(t *testing.T) {
	d := shippedDoc(t)
	got := namedFiles(d)
	want := "references/inspect.md|templates/module-map.md"
	if strings.Join(got, "|") != want {
		t.Fatalf("namedFiles = %v; want [%s]", got, want)
	}
}

// shippedDoc is the architect skill of shippedPack with a body that mixes the
// two kinds of path: files it ships, and files it is telling the model to go
// and find in the user's project or in the repo.
func shippedDoc(t *testing.T) DiscoveredSkill {
	t.Helper()
	found, _ := EmbeddedSkills(fstest.MapFS{
		"pack/architect/SKILL.md": &fstest.MapFile{Data: []byte(
			"---\nname: architect\ndescription: draws the current system\n---\n" +
				"Read `references/inspect.md`, then templates/module-map.md.\n" +
				"Written up in docs/architecture/agent-package-standard-2026-08-08.md,\n" +
				"and the project's own assets/design-tokens.css.\n")},
		"pack/architect/references/inspect.md":   &fstest.MapFile{Data: []byte("count the modules first")},
		"pack/architect/templates/module-map.md": &fstest.MapFile{Data: []byte("# Module map")},
	}, "pack")
	if len(found) != 1 {
		t.Fatalf("found %d skills; want 1", len(found))
	}
	return found[0]
}

// The aetox skill lists what ships, and a list of shipped things is the exact
// shape of fact that goes stale in silence: a skill is added, the document that
// introduces it is not, and the model never learns the skill exists — which for
// a progressively-loaded shelf is the same as it not shipping at all.
func TestTheAetoxSkillNamesEveryBundledSkill(t *testing.T) {
	body := bundledDoc(t, "aetox").body
	bundled := map[string]bool{}
	for _, d := range bundledSkills() {
		bundled[d.Name] = true
		if !strings.Contains(body, "`"+d.Name+"`") && d.Name != "aetox" {
			t.Errorf("%s ships and the aetox skill never names it", d.Name)
		}
	}
	// And the other direction: a skill named there and removed from the binary
	// sends the model looking for a door that is gone.
	for _, m := range regexp.MustCompile("`(aetox-[a-z-]+)`").FindAllStringSubmatch(body, -1) {
		if !bundled[m[1]] {
			t.Errorf("the aetox skill names %s and nothing ships it", m[1])
		}
	}
}
