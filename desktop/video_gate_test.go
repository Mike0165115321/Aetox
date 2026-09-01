package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/subagent"
)

// The card that makes videos must be veiled on a machine with no renderer, and
// nothing in that agent's profile says so.
//
// This is the check that went missing for an afternoon. Every other gate in
// AgentGate reads a `needs:` line, and `needs:` is how an MCP server is named —
// so when the video agent stopped needing kinocut and started calling the
// renderer directly, its card lost the only thing that was veiling it. Nothing
// failed: the card looked ready, the agent accepted the job, and the first
// render came back saying the renderer was not installed.
func TestTheVideoAgentIsJudgedByItsToolsNotItsNeeds(t *testing.T) {
	video, ok := subagent.Load("video")
	if !ok {
		t.Fatal("the video agent is not in the bundled profiles")
	}
	if len(video.Needs) != 0 {
		t.Errorf("video still declares needs %v — it renders through its own tool now", video.Needs)
	}
	if !profileNeedsSceneRenderer(video) {
		t.Error("video does not carry video_render, so nothing will ever check its renderer")
	}

	// The other half of the same rule. Cutting footage that already exists uses
	// the editor and never the renderer, so veiling that card over a missing
	// renderer would be a fault report about a job nobody is doing.
	editor, ok := subagent.Load("editor")
	if !ok {
		t.Fatal("the editor agent is not in the bundled profiles")
	}
	if profileNeedsSceneRenderer(editor) {
		t.Error("editor carries video_render — it cuts footage and should not")
	}
}

// A machine with nothing installed must report the renderer as missing.
func TestAMachineWithNoRendererSaysSo(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if !sceneRendererMissing() {
		t.Error("sceneRendererMissing() = false on a data root with nothing in it")
	}
}

// A scene whose motion never ends, or never starts, has to say how long it is.
//
// The renderer works out a clip's length from where the last animation stops.
// A scene built entirely from `infinite` loops has no such point and neither
// does one with no `@keyframes` at all, so the render refuses both outright:
// "Composition has zero duration". Four of the library's scenes are like that
// and each states a length on its own `<body>`; this is the rule rather than
// the list, so the fifth one somebody adds fails here instead of failing on a
// user's machine after they picked it.
//
// Only the flat scenes are asked. The nine folders drive GSAP timelines the
// renderer reads directly, which is a different answer to the same question.
func TestASceneWithNoEndOfItsOwnStatesOne(t *testing.T) {
	scenes, err := subagent.ListSkillDir(videoLibraryAgent, videoLibrarySkill, videoLibraryDir)
	if err != nil {
		t.Fatalf("the scene library did not open: %v", err)
	}
	checked := 0
	for _, name := range scenes {
		if !strings.HasSuffix(name, ".html") {
			continue // a folder: its length lives in a GSAP timeline
		}
		body, err := subagent.ReadSkillFile(videoLibraryAgent, videoLibrarySkill, videoLibraryDir+"/"+name)
		if err != nil {
			t.Errorf("%s is on the shelf and could not be read: %v", name, err)
			continue
		}
		checked++
		text := string(body)
		finite := false
		for _, decl := range regexp.MustCompile(`animation:[^;}]*`).FindAllString(text, -1) {
			if !strings.Contains(decl, "infinite") && !strings.Contains(decl, "none") {
				finite = true
				break
			}
		}
		if finite || strings.Contains(text, "data-duration=") {
			continue
		}
		t.Errorf("%s has no animation that ends and no data-duration — the renderer refuses it with "+
			"\"Composition has zero duration\"", name)
	}
	if checked == 0 {
		t.Fatal("no flat scenes were read, so this test checked nothing")
	}
}

// The library may name exactly one GSAP, and it must be the one we pinned.
//
// `video new` swaps that address for the copy on this machine. A scene naming a
// different version would be silently handed our 3.14.2 instead, which is the
// kind of substitution that works until the day it does not. This fails on the
// scene rather than on the render.
func TestEveryGSAPInTheLibraryIsThePinnedOne(t *testing.T) {
	loose := regexp.MustCompile(`https://cdn\.jsdelivr\.net/npm/gsap@[^"']+`)
	if _, err := subagent.ListSkillDir(videoLibraryAgent, videoLibrarySkill, videoLibraryDir); err != nil {
		t.Fatalf("the scene library did not open: %v", err)
	}
	seen := 0
	var walk func(sub string)
	walk = func(sub string) {
		entries, err := subagent.ListSkillDir(videoLibraryAgent, videoLibrarySkill, sub)
		if err != nil {
			return
		}
		for _, name := range entries {
			path := sub + "/" + name
			if !strings.Contains(name, ".") {
				walk(path)
				continue
			}
			if !strings.HasSuffix(name, ".html") {
				continue
			}
			body, err := subagent.ReadSkillFile(videoLibraryAgent, videoLibrarySkill, path)
			if err != nil {
				continue
			}
			for _, found := range loose.FindAllString(string(body), -1) {
				seen++
				if found != gsapCDNURL {
					t.Errorf("%s loads %s, and the pinned copy is %s", path, found, gsapCDNURL)
				}
			}
		}
	}
	walk(videoLibraryDir)
	if seen == 0 {
		t.Fatal("no GSAP reference was read, so this test checked nothing")
	}
}

// A copied scene points at the GSAP on this machine, and at the CDN when there
// is none.
//
// Both halves matter. The rewrite is what makes nine of the library's scenes
// render on a machine with no network; the fallback is what stops a missing
// optional download from turning `video new` into a refusal.
func TestACopiedSceneUsesTheLocalGSAPWhenThereIsOne(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)

	scene := func() string {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "compositions"), 0o755); err != nil {
			t.Fatal(err)
		}
		page := `<html><head><script src="` + gsapCDNURL + `"></script></head><body></body></html>`
		for _, name := range []string{"index.html", filepath.Join("compositions", "intro.html")} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(page), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}

	// Nothing installed: the address upstream wrote is the address that stays.
	bare := scene()
	if n, err := videoUseLocalGSAP(bare); err != nil || n != 0 {
		t.Fatalf("videoUseLocalGSAP with no GSAP installed = %d, %v; want 0, nil", n, err)
	}
	if body, _ := os.ReadFile(filepath.Join(bare, "index.html")); !strings.Contains(string(body), gsapCDNURL) {
		t.Error("the CDN address was rewritten with nothing to rewrite it to")
	}

	// Installed: every file that named the CDN now names the copy beside it,
	// spelled relative to the file doing the loading.
	if err := os.MkdirAll(filepath.Join(root, "tools", "gsap"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tools", "gsap", "gsap-3.14.2.min.js"), []byte("/* gsap */"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := scene()
	n, err := videoUseLocalGSAP(dir)
	if err != nil || n != 2 {
		t.Fatalf("videoUseLocalGSAP = %d, %v; want 2, nil", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor", "gsap.min.js")); err != nil {
		t.Fatalf("the project has no local copy to point at: %v", err)
	}
	for name, want := range map[string]string{
		"index.html": `src="./vendor/gsap.min.js"`,
		filepath.Join("compositions", "intro.html"): `src="../vendor/gsap.min.js"`,
	} {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), want) {
			t.Errorf("%s does not load %s: %s", name, want, body)
		}
		if strings.Contains(string(body), gsapCDNURL) {
			t.Errorf("%s still names the CDN after the rewrite", name)
		}
	}
}
