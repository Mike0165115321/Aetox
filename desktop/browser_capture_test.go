package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// A screenshot is a byproduct of the agent looking at something, not a file the
// user asked for by name — so it goes to output/<session> even with a project
// focused, where a.outputSubdir() would put a new file in the project root.
// Getting this wrong drops page-1.png into the root of somebody's repository.
func TestBrowserShotStaysOutOfTheProjectRoot(t *testing.T) {
	root := t.TempDir()
	a := &App{cfg: config.Config{SandboxRoot: root}, projectFocused: true, sessionID: "s1"}

	rel, err := a.writeBrowserShot([]byte("\x89PNG fake"))
	if err != nil {
		t.Fatalf("writeBrowserShot() = %v", err)
	}
	if !strings.HasPrefix(rel, "output/s1/") {
		t.Errorf("writeBrowserShot() = %q, want it under output/s1 — output/<session> is the one folder ListArtifacts sweeps", rel)
	}
	if !strings.HasSuffix(rel, ".png") {
		t.Errorf("writeBrowserShot() = %q, want a .png", rel)
	}
	if strings.Contains(rel, "..") {
		t.Errorf("writeBrowserShot() = %q, which climbs out of the sandbox", rel)
	}

	abs := filepath.Join(root, filepath.FromSlash(rel))
	got, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("the path it reported does not exist: %v", err)
	}
	if string(got) != "\x89PNG fake" {
		t.Errorf("the file holds %q, not the bytes handed in", got)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 1 || entries[0].Name() != "output" {
		t.Errorf("the project root gained something other than output/: %v", entries)
	}
}

// Two shots in one turn are two files. They were one for as long as the name
// was fixed, which reads as the second capture having failed silently.
func TestTwoShotsAreTwoFiles(t *testing.T) {
	a := &App{cfg: config.Config{SandboxRoot: t.TempDir()}, sessionID: "s1"}

	first, err := a.writeBrowserShot([]byte("one"))
	if err != nil {
		t.Fatalf("writeBrowserShot() = %v", err)
	}
	second, err := a.writeBrowserShot([]byte("two"))
	if err != nil {
		t.Fatalf("writeBrowserShot() = %v", err)
	}
	if first == second {
		t.Errorf("both shots went to %q — the first is gone", first)
	}
}

// A chat that has not been saved yet still has to be able to photograph a page.
func TestBrowserShotWorksBeforeASessionHasAnID(t *testing.T) {
	a := &App{cfg: config.Config{SandboxRoot: t.TempDir()}}

	if _, err := a.writeBrowserShot([]byte("png")); err != nil {
		t.Errorf("writeBrowserShot() with no session id = %v", err)
	}
}

// No working folder is the one case where there is nowhere to put it, and the
// refusal has to be a sentence rather than a file written who-knows-where.
func TestBrowserShotRefusesWithNoWorkingFolder(t *testing.T) {
	a := &App{sessionID: "s1"}

	if rel, err := a.writeBrowserShot([]byte("png")); err == nil {
		t.Errorf("writeBrowserShot() with no sandbox root wrote to %q", rel)
	}
}

// capture asks whose tab it is before it asks the engine for a picture, so a
// session with no page open is told so instead of waiting on a webview.
func TestCaptureRefusesWithNoPageOpen(t *testing.T) {
	a := &App{cfg: config.Config{SandboxRoot: t.TempDir()}, sessionID: "s1"}
	a.browsers = &browserHost{app: a, tabs: map[string]*browserTab{}}

	out, err := (&browserCaptureSkill{app: a}).capture(t.Context())
	if err == nil {
		t.Fatal("capture answered without a page open")
	}
	if out.Success {
		t.Error("the output claims success")
	}
	if len(out.Images) != 0 {
		t.Error("a failed capture still handed the model an image")
	}
}
