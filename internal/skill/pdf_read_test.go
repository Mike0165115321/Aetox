package skill

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPDFReadRequiresPath(t *testing.T) {
	s := &pdfReadSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{}}); err == nil {
		t.Fatal("expected usage error for missing path, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected usage error for missing path arg, got nil")
	}
}

func TestPDFReadRejectsEscape(t *testing.T) {
	s := &pdfReadSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"path": "../outside.pdf"}); err == nil {
		t.Fatal("expected a sandbox-escape error, got nil")
	}
}

// With no bundled copy present, the bare name must come back so PATH does the
// resolving — and so a missing poppler still surfaces as exec.ErrNotFound,
// which is what turns into the install instructions.
func TestPDFToTextFallsBackToPath(t *testing.T) {
	if got := bundledBinary("poppler", "pdftotext"); got != "pdftotext" {
		t.Errorf("bundledBinary = %q, want the bare name for PATH lookup", got)
	}
}

// The Windows installer unpacks poppler next to aetox.exe instead of putting
// it on PATH (it ships as a plain zip), so this lookup is the only thing that
// makes a bundled copy reachable at all.
func TestPDFToTextPrefersTheBundledCopy(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary: %v", err)
	}
	name := "pdftotext"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	root := filepath.Join(filepath.Dir(exe), "poppler")
	dir := filepath.Join(root, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skipf("cannot write next to the test binary: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	bundled := filepath.Join(dir, name)
	if err := os.WriteFile(bundled, []byte("stub"), 0o755); err != nil {
		t.Skipf("cannot write next to the test binary: %v", err)
	}

	if got := bundledBinary("poppler", "pdftotext"); got != bundled {
		t.Errorf("bundledBinary = %q, want the bundled copy at %q", got, bundled)
	}
}

// The same lookup carries ffmpeg for video_ocr and audio_transcribe, which are
// the tools that were dead on a fresh install: the model can call them, but
// nothing put an ffmpeg on the machine for them to use.
func TestBundledBinaryFindsFFmpegNextToTheExecutable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary: %v", err)
	}
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	root := filepath.Join(filepath.Dir(exe), "ffmpeg")
	dir := filepath.Join(root, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skipf("cannot write next to the test binary: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	bundled := filepath.Join(dir, name)
	if err := os.WriteFile(bundled, []byte("stub"), 0o755); err != nil {
		t.Skipf("cannot write next to the test binary: %v", err)
	}

	if got := bundledBinary("ffmpeg", "ffmpeg"); got != bundled {
		t.Errorf("bundledBinary = %q, want the bundled copy at %q", got, bundled)
	}
}

// A hand-written minimal PDF — one text-drawing operator, no xref table
// (poppler reconstructs one and warns on stderr, which never reaches the
// model). Inline rather than a testdata blob so the thing being extracted is
// readable next to the assertion that looks for it.
const tinyPDF = `%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj
4 0 obj<</Length 46>>stream
BT /F1 18 Tf 20 100 Td (AETOX PDF OK) Tj ET
endstream
endobj
5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj
trailer<</Root 1 0 R>>
`

// End-to-end through the real binary: the skill spawns pdftotext, passes the
// arguments it means to, and gets usable text back out of stdout. Skips where
// poppler isn't installed, the same shape as TestAudioTranscribeLive — this is
// the only test that proves the actual subprocess contract rather than the
// decisions around it.
func TestPDFReadLive(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil && bundledBinary("poppler", "pdftotext") == "pdftotext" {
		t.Skip("poppler is not installed on this machine")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tiny.pdf"), []byte(tinyPDF), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &pdfReadSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "tiny.pdf"})
	if err != nil {
		t.Fatalf("pdf_read against a real PDF: %v", err)
	}
	if !out.Success {
		t.Error("Success = false on a PDF that extracted cleanly")
	}
	if !strings.Contains(out.Content, "AETOX PDF OK") {
		t.Errorf("Content = %q, want the text the PDF actually draws", out.Content)
	}
}

// The "poppler isn't here" path must hand back the fix, not a raw
// exec.ErrNotFound — that error is the only thing telling the user what to do.
func TestPDFReadMissingBinaryGivesActionableError(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin tries a real `brew install` on this path — not running a package install from a test")
	}
	if _, err := exec.LookPath("pdftotext"); err == nil {
		t.Skip("pdftotext is installed on this machine — not exercising the missing-binary path")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "doc.pdf"), []byte("%PDF-1.7"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &pdfReadSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "doc.pdf"})
	if err == nil {
		t.Fatal("expected an error when pdftotext is unavailable, got nil")
	}
	if !strings.Contains(err.Error(), "pdftotext") {
		t.Errorf("error should name the missing program so the user can act on it, got: %v", err)
	}
	if out.Success {
		t.Error("Success = true with no pdftotext available")
	}
}
