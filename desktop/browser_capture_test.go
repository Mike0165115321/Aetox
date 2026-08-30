package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
)

// A screenshot is a byproduct of the agent looking at something, not a file the
// user asked for by name — so it goes to output/<session> even with a project
// focused, where a.outputSubdir() would put a new file in the project root.
// Getting this wrong drops page-1.png into the root of somebody's repository.
func TestBrowserShotStaysOutOfTheProjectRoot(t *testing.T) {
	root := t.TempDir()
	a := seed(&App{cfg: config.Config{SandboxRoot: root}, projectFocused: true}, &conversation{id: "s1"})

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

// A screenshot is not a deliverable, and the gallery has one way of knowing
// that: the folder it is in. If this ever writes flat into output/<session>
// again, the shots go back to sitting level with the document somebody asked
// for — 46 of 244 cards on the owner's machine, the day this was written.
func TestBrowserShotGoesInTheWorkFolderSoTheGalleryCanStackIt(t *testing.T) {
	root := t.TempDir()
	a := seed(&App{cfg: config.Config{SandboxRoot: root}, projectFocused: true}, &conversation{id: "s1"})

	rel, err := a.writeBrowserShot([]byte("PNG fake"))
	if err != nil {
		t.Fatalf("writeBrowserShot() = %v", err)
	}
	if want := "output/s1/" + workSubdir + "/"; !strings.HasPrefix(rel, want) {
		t.Errorf("writeBrowserShot() = %q, want it under %q", rel, want)
	}

	// And the gallery has to read that folder back as the deck key. Going
	// through ListArtifacts rather than asserting on folderUnder directly is
	// the point: the two halves are only useful if they agree.
	found := false
	for _, art := range sweepSession(filepath.Join(root, "output", "s1"), "s1", root) {
		if filepath.Base(art.Path) != filepath.Base(rel) {
			continue
		}
		found = true
		if art.Folder != workSubdir {
			t.Errorf("the shot reports Folder %q, want %q — the gallery groups on this", art.Folder, workSubdir)
		}
	}
	if !found {
		t.Error("the shot never came back from the sweep at all")
	}
}

// Two shots in one turn are two files. They were one for as long as the name
// was fixed, which reads as the second capture having failed silently.
func TestTwoShotsAreTwoFiles(t *testing.T) {
	a := seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}}, &conversation{id: "s1"})

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
	a := seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}}, newConversation())

	if _, err := a.writeBrowserShot([]byte("png")); err != nil {
		t.Errorf("writeBrowserShot() with no session id = %v", err)
	}
}

// No working folder is the one case where there is nowhere to put it, and the
// refusal has to be a sentence rather than a file written who-knows-where.
func TestBrowserShotRefusesWithNoWorkingFolder(t *testing.T) {
	a := seed(&App{}, &conversation{id: "s1"})

	if rel, err := a.writeBrowserShot([]byte("png")); err == nil {
		t.Errorf("writeBrowserShot() with no sandbox root wrote to %q", rel)
	}
}

// capture asks whose tab it is before it asks the engine for a picture, so a
// session with no page open is told so instead of waiting on a webview.
func TestCaptureRefusesWithNoPageOpen(t *testing.T) {
	a := seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}}, &conversation{id: "s1"})
	a.browsers = &browserHost{app: a, tabs: map[string]*browserTab{}}

	out, err := (&browserCaptureSkill{app: a}).capture(t.Context(), false)
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

// pagePNG is a capture of a given shape. Only the header is ever read, so the
// pixels are left as whatever image.NewRGBA starts with.
func pagePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encoding the test capture: %v", err)
	}
	return buf.Bytes()
}

// A full-page capture of a fifteen-slide deck came back 1280 x 10800 on
// 30 ส.ค. Every provider downsizes a picture before the model reads it, so a
// column that shape arrives with nothing in it legible — and the model, told
// nothing, reported on slides it could not see.
func TestTallCaptureSaysItCannotBeRead(t *testing.T) {
	note := tallStripNote(pagePNG(t, 1280, 10800))
	if note == "" {
		t.Fatal("a 1280x10800 capture must say the text in it will not be readable")
	}
	if !strings.Contains(note, "10800") || !strings.Contains(note, "1280") {
		t.Errorf("the note should carry the measurements it is about, got %q", note)
	}
}

// The ordinary capture says nothing extra. A note on every screenshot is a note
// nobody reads.
func TestOrdinaryCaptureCarriesNoStripNote(t *testing.T) {
	if note := tallStripNote(pagePNG(t, 1280, 720)); note != "" {
		t.Errorf("a viewport capture got %q, want no note", note)
	}
	if note := tallStripNote(pagePNG(t, 573, 871)); note != "" {
		t.Errorf("a tall-ish page capture got %q, want no note", note)
	}
	if note := tallStripNote([]byte("not a png")); note != "" {
		t.Errorf("unreadable bytes got %q, want no note", note)
	}
}
