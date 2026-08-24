package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// screenshotPNG is the thing this feature exists for: flat colour, hard edges,
// fully opaque, stored losslessly for no reason. Big enough that the two
// encoders can actually disagree about it.
func screenshotPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	// Deterministic noise, not a gradient. A gradient is the one thing PNG
	// stores brilliantly, and a fixture PNG that is already small is a fixture
	// the compressor is right to leave alone — which tests nothing. Detail at
	// pixel scale is what a screenshot of anti-aliased text looks like to an
	// encoder, and it is where the 75-90% measured on the real files comes from.
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(12345)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			seed = seed*1664525 + 1013904223
			img.Set(x, y, color.RGBA{
				R: uint8(seed >> 24), G: uint8(seed >> 16), B: uint8(seed >> 8), A: 255,
			})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// logoPNG is the thing that must survive it untouched in format: transparent
// corners, which JPEG cannot carry at all.
func logoPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			alpha := uint8(255)
			if x < 4 && y < 4 {
				alpha = 0
			}
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: 40, B: 200, A: alpha})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func writeBytes(t *testing.T, a *App, session, name string, body []byte) string {
	t.Helper()
	dir := filepath.Join(a.cur().cfg.SandboxRoot, "output", session)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The whole promise of the button: the picture gets smaller, and the number it
// reports is the number the user can check against the disk.
func TestCompressingAScreenshotGivesBackMostOfIt(t *testing.T) {
	a := bootGalleryApp(t)
	path := writeBytes(t, a, "s1", "page-1.png", screenshotPNG(t, 320, 240))
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	report, err := a.CompressArtifacts([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if report.Files != 1 {
		t.Fatalf("compressed %d files, want 1 (skipped %d, err %q)", report.Files, report.Skipped, report.Error)
	}
	if report.After >= report.Before {
		t.Errorf("reported %d -> %d bytes, which gives nothing back", report.Before, report.After)
	}
	if report.Before != before.Size() {
		t.Errorf("reported a starting size of %d, disk said %d", report.Before, before.Size())
	}

	// An opaque screenshot is allowed to become a JPEG, and the PNG it was must
	// not be left behind as a second copy of the same picture.
	jpg := strings.TrimSuffix(path, ".png") + ".jpg"
	got, err := os.Stat(jpg)
	if err != nil {
		t.Fatalf("no .jpg where the .png was: %v", err)
	}
	if got.Size() != report.After {
		t.Errorf("the file on disk is %d bytes, the report said %d", got.Size(), report.After)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the original .png is still there — the gallery would show the same picture twice")
	}
}

// A logo is a PNG because of what is NOT painted in it. JPEG cannot hold that,
// so the format is not the compressor's to change.
func TestSomethingTransparentStaysAPNG(t *testing.T) {
	a := bootGalleryApp(t)
	path := writeBytes(t, a, "s1", "โลโก้.png", logoPNG(t, 160, 160))

	if _, err := a.CompressArtifacts([]string{path}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the .png is gone: %v", err)
	}
	if _, err := os.Stat(strings.TrimSuffix(path, ".png") + ".jpg"); err == nil {
		t.Error("a transparent image was turned into a JPEG, which flattens what it was transparent for")
	}
	// And it is still readable as an image with its alpha intact.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("what came out is not a PNG any more: %v", err)
	}
	if _, _, _, alpha := img.At(1, 1).RGBA(); alpha != 0 {
		t.Errorf("the transparent corner came back with alpha %d", alpha)
	}
}

// Anything that is not a picture is left exactly as it was, and says so.
func TestCompressLeavesEverythingElseAlone(t *testing.T) {
	a := bootGalleryApp(t)
	doc := writeBytes(t, a, "s1", "สรุป.md", []byte(strings.Repeat("ก", 2000)))
	before, err := os.ReadFile(doc)
	if err != nil {
		t.Fatal(err)
	}

	report, err := a.CompressArtifacts([]string{doc})
	if err != nil {
		t.Fatal(err)
	}
	if report.Files != 0 || report.Skipped != 1 {
		t.Errorf("report says files=%d skipped=%d, want 0 and 1", report.Files, report.Skipped)
	}
	after, err := os.ReadFile(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("a markdown file came back changed")
	}
}

// Same door, same bound as opening and deleting: a path from the frontend is
// not a reason to touch a file.
func TestCompressRefusesAnythingOutsideTheGallery(t *testing.T) {
	a := bootGalleryApp(t)
	outside := filepath.Join(t.TempDir(), "somebody-elses.png")
	if err := os.WriteFile(outside, screenshotPNG(t, 32, 32), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := a.CompressArtifacts([]string{outside}); err == nil {
		t.Fatal("compressing a file outside the output folders was allowed")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("the refused file was touched anyway: %v", err)
	}
}

// One bad file must not cost the user the rest of the run.
func TestOneUnreadableImageDoesNotStopTheOthers(t *testing.T) {
	a := bootGalleryApp(t)
	broken := writeBytes(t, a, "s1", "เสีย.png", []byte("this is not a PNG"))
	good := writeBytes(t, a, "s1", "page-2.png", screenshotPNG(t, 200, 150))

	report, err := a.CompressArtifacts([]string{broken, good})
	if err != nil {
		t.Fatal(err)
	}
	if report.Files != 1 {
		t.Errorf("compressed %d, want the one good file", report.Files)
	}
	if report.Skipped != 1 || report.Error == "" {
		t.Errorf("the broken file was not reported: skipped=%d err=%q", report.Skipped, report.Error)
	}
	if _, err := os.Stat(broken); err != nil {
		t.Error("the file that would not decode was removed")
	}
}
