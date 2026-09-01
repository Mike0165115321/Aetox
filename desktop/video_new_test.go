package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/subagent"
)

// A refusal must name the caller's actual mistake.
//
// On 31 ส.ค. an agent asked for a folder-shaped scene into a path that already
// existed, and was told "ไม่มีฉากชื่อ ... ในคลัง" — with the scene's own name
// sitting in the list the refusal attached. It retried the same call twice,
// tried a name that really was absent, then routed around the tool with a new
// path. The old videoCopyTemplate decided a scene's shape by trying the folder
// copy and reading any failure as "not a folder", so the dest-exists refusal
// from CopySkillDir became a template-not-found refusal here. The shape now
// comes from the shelf listing, and each mistake keeps its own sentence.
func TestVideoNewNamesTheRealMistake(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	shelf, err := subagent.ListSkillDir(videoLibraryAgent, videoLibrarySkill, videoLibraryDir)
	if err != nil {
		t.Fatalf("the scene library did not open: %v", err)
	}
	var folder, flat string
	for _, name := range shelf {
		if strings.HasSuffix(name, ".html") {
			if flat == "" {
				flat = strings.TrimSuffix(name, ".html")
			}
		} else if folder == "" {
			folder = name
		}
	}
	if folder == "" || flat == "" {
		t.Fatalf("the shelf no longer holds both shapes (folder=%q, flat=%q)", folder, flat)
	}

	// A destination that already exists is refused as exactly that, for both
	// shapes — never as a scene missing from a library that lists it.
	for _, template := range []string{folder, flat} {
		dest := filepath.Join(t.TempDir(), "taken")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := videoCopyTemplate(template, dest)
		if err == nil {
			t.Fatalf("videoCopyTemplate(%q) into an existing folder did not refuse", template)
		}
		if !strings.Contains(err.Error(), "มีอยู่แล้ว") {
			t.Errorf("videoCopyTemplate(%q) into an existing folder said %q — not that the folder is taken", template, err)
		}
		if strings.Contains(err.Error(), "ไม่มีฉากชื่อ") {
			t.Errorf("videoCopyTemplate(%q) blamed the library for a folder that is merely taken: %q", template, err)
		}
	}

	// A scene that truly is not on the shelf still gets the listing.
	_, err = videoCopyTemplate("no-such-scene", filepath.Join(t.TempDir(), "fresh"))
	if err == nil || !strings.Contains(err.Error(), "ไม่มีฉากชื่อ") {
		t.Errorf("a scene absent from the shelf was not refused as absent: %v", err)
	}

	// The spellings our own index table teaches are accepted, not refused:
	// `motion/<name>.html` for a flat scene, `motion/<name>/index.html` for a
	// folder. Three refusals in one evening (problem queue, 31 ส.ค.) were
	// agents copying the table verbatim.
	for _, spelling := range []string{
		videoLibraryDir + "/" + flat + ".html",
		videoLibraryDir + "/" + folder + "/index.html",
		folder + "/index.html",
	} {
		dest := filepath.Join(t.TempDir(), "spelled")
		if _, err := videoCopyTemplate(spelling, dest); err != nil {
			t.Errorf("videoCopyTemplate(%q) refused a spelling our own table teaches: %v", spelling, err)
		}
	}

	// And both shapes still copy into a fresh destination.
	for _, template := range []string{folder, flat} {
		dest := filepath.Join(t.TempDir(), "fresh")
		written, err := videoCopyTemplate(template, dest)
		if err != nil || written < 1 {
			t.Fatalf("videoCopyTemplate(%q) into a fresh folder = %d, %v", template, written, err)
		}
		if _, err := os.Stat(filepath.Join(dest, "index.html")); err != nil {
			t.Errorf("the %q project has no index.html for the renderer to find: %v", template, err)
		}
	}
}

// A session's video work lives in that session's folder.
//
// Every file-producing tool steers an unfocused session's output into
// output/<session>; this one skipped the rule, so three aetox-intro variants
// from three chats sat side by side at the sandbox root (measured 31 ส.ค.
// 19:24, during the owner's own test), and the chat under test globbed up a
// render another chat had made. The receipt names the placed path, the read
// side resolves the short name back, and two sessions using the same project
// name never meet.
func TestAVideoProjectLandsInItsSessionsFolder(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	root := t.TempDir()
	shelf, err := subagent.ListSkillDir(videoLibraryAgent, videoLibrarySkill, videoLibraryDir)
	if err != nil {
		t.Fatalf("the scene library did not open: %v", err)
	}
	flat := ""
	for _, name := range shelf {
		if strings.HasSuffix(name, ".html") {
			flat = strings.TrimSuffix(name, ".html")
			break
		}
	}
	if flat == "" {
		t.Fatal("no flat scene on the shelf")
	}

	a := newTestApp(t, root)
	s := &videoToolSkill{app: a}
	out, err := s.newProject(map[string]any{"action": "new", "template": flat, "path": "clip"})
	if err != nil {
		t.Fatalf("video new: %v", err)
	}
	sub := "output/" + a.cur().id
	if _, err := os.Stat(filepath.Join(root, sub, "clip", "index.html")); err != nil {
		t.Fatalf("the project is not in the session folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "clip")); err == nil {
		t.Error("the project also landed at the sandbox root")
	}
	if !strings.Contains(out.Content, sub+"/clip") {
		t.Errorf("the receipt does not name the placed path:\n%s", out.Content)
	}

	// The short name the caller originally used still finds the project.
	full, err := s.projectDir("clip")
	if err != nil {
		t.Fatalf("projectDir(clip) = %v", err)
	}
	if !strings.Contains(filepath.ToSlash(full), sub) {
		t.Errorf("projectDir resolved outside the session folder: %s", full)
	}

	// A second session reusing the same name is a fresh project, not a
	// collision with the first session's work.
	b := newTestApp(t, root)
	if _, err := (&videoToolSkill{app: b}).newProject(map[string]any{"action": "new", "template": flat, "path": "clip"}); err != nil {
		t.Fatalf("the same name in a second session collided: %v", err)
	}
}

// The index is the library, or it is a lie.
//
// SKILL.md's motion table is what an agent reads instead of opening scenes,
// so a scene on the shelf that the table does not carry is invisible, and one
// the table carries that the shelf lost is a refusal waiting at `video new`.
// The same standard the release checklist holds version numbers to.
func TestTheMotionTableMatchesTheShelf(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	doc, err := subagent.ReadSkillFile(videoLibraryAgent, videoLibrarySkill, "SKILL.md")
	if err != nil {
		t.Fatalf("the library has no SKILL.md: %v", err)
	}
	shelf, err := subagent.ListSkillDir(videoLibraryAgent, videoLibrarySkill, videoLibraryDir)
	if err != nil {
		t.Fatalf("the scene library did not open: %v", err)
	}
	table := string(doc)
	rows := 0
	for _, name := range shelf {
		entry := videoLibraryDir + "/" + name
		if !strings.HasSuffix(name, ".html") {
			entry += "/index.html"
		}
		if !strings.Contains(table, "`"+entry+"`") {
			t.Errorf("%s is on the shelf and not in SKILL.md's table", entry)
		}
		rows++
	}
	if rows == 0 {
		t.Fatal("the shelf is empty, so this test checked nothing")
	}
	for _, path := range regexp.MustCompile("`"+videoLibraryDir+`/[^`+"`"+`]+`+"`").FindAllString(table, -1) {
		entry := strings.Trim(path, "`")
		probe := strings.TrimSuffix(entry, "/index.html")
		probe = strings.TrimPrefix(probe, videoLibraryDir+"/")
		found := false
		for _, name := range shelf {
			if name == probe || name == probe+".html" || strings.TrimSuffix(name, ".html") == probe {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SKILL.md's table names %s and the shelf does not hold it", entry)
		}
	}
}

// The scaffold report reads the copy so the agent does not have to.
//
// Measured 31 ส.ค. (session 164630): after `video new` the agent spent six
// calls learning what the copy held and eight more reading library sub-scenes
// for where the words live. The inventory is the tool saying what it wrote.
func TestVideoNewReportsWhatItWrote(t *testing.T) {
	dir := t.TempDir()
	page := `<html><head><title>ignored</title><style>body{color:red}</style></head>
<body><h1>Too many tools</h1><p>Your team is &amp; drowning</p>
<script>var hidden = "not words";</script></body></html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "logo.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	// What `video new` itself put there stays out of the agent's reading list.
	if err := os.MkdirAll(filepath.Join(dir, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vendor", "gsap.min.js"), []byte("/* gsap */"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := videoProjectInventory(dir)
	for _, want := range []string{"index.html", "Too many tools", "Your team is & drowning", "logo.svg"} {
		if !strings.Contains(got, want) {
			t.Errorf("the inventory does not carry %q:\n%s", want, got)
		}
	}
	for _, leak := range []string{"ignored", "color:red", "not words", "gsap.min.js"} {
		if strings.Contains(got, leak) {
			t.Errorf("the inventory carries %q, which no browser paints:\n%s", leak, got)
		}
	}
}
