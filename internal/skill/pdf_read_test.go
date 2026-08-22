package skill

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/testpdf"
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

// bundledBinary reads config.DataRoot(), so on a developer's machine it can
// see the tools that machine really has installed. Every test below points it
// at an empty directory first — without that they pass or fail depending on
// whether whoever ran them had pressed the install button, which is not a
// property of the code.
func emptyDataRoot(t *testing.T) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
}

// With no bundled copy present, the bare name must come back so PATH does the
// resolving — and so a missing poppler still surfaces as exec.ErrNotFound,
// which is what turns into the install instructions.
func TestPDFToTextFallsBackToPath(t *testing.T) {
	emptyDataRoot(t)
	if got := bundledBinary("poppler", "pdftotext"); got != "pdftotext" {
		t.Errorf("bundledBinary = %q, want the bare name for PATH lookup", got)
	}
}

// The Windows installer unpacks poppler next to aetox.exe instead of putting
// it on PATH (it ships as a plain zip), so this lookup is the only thing that
// makes a bundled copy reachable at all.
func TestPDFToTextPrefersTheBundledCopy(t *testing.T) {
	emptyDataRoot(t)
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
	emptyDataRoot(t)
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

// DataRoot beats next-to-the-executable, and the order is the point: after a
// version bump the downloaded copy is the one this build pinned and tested,
// while the one beside the exe is whatever some earlier release's installer
// happened to leave behind.
func TestBundledBinaryPrefersDataRootOverTheExecutablesNeighbour(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dataRoot)

	name := "pdftotext"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary: %v", err)
	}
	besideRoot := filepath.Join(filepath.Dir(exe), "poppler")
	beside := filepath.Join(besideRoot, "bin", name)
	if err := os.MkdirAll(filepath.Dir(beside), 0o755); err != nil {
		t.Skipf("cannot write next to the test binary: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(besideRoot) })
	if err := os.WriteFile(beside, []byte("old"), 0o755); err != nil {
		t.Skipf("cannot write next to the test binary: %v", err)
	}

	// Only the old copy exists: it must still be found, which is what keeps an
	// upgrade from re-downloading 160MB it already owns.
	if got := bundledBinary("poppler", "pdftotext"); got != beside {
		t.Fatalf("bundledBinary = %q, want the neighbour copy at %q", got, beside)
	}

	managed := filepath.Join(dataRoot, "tools", "poppler", "bin", name)
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := bundledBinary("poppler", "pdftotext"); got != managed {
		t.Errorf("bundledBinary = %q, want the managed copy at %q", got, managed)
	}
}

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
	if err := os.WriteFile(filepath.Join(root, "tiny.pdf"), testpdf.Minimal("AETOX PDF OK"), 0o644); err != nil {
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

// A converter that dies is not a verdict on the document, and the two must not
// be reported the same way: pdftotext prints warnings on stderr as it goes, so
// when it crashes the last of them is sitting there looking exactly like a
// reason. Reading "Couldn't read xref table" off a process that died of an
// access violation is how a working PDF gets reported as a corrupt one.
func TestAbnormalExitSeparatesACrashFromARefusal(t *testing.T) {
	cases := []struct {
		name string
		code int
		want bool
	}{
		{"clean success", 0, false},
		{"the file was refused", 1, false},
		{"a larger but ordinary status", 99, false},
		{"killed by a signal (unix)", -1, true},
		{"access violation (windows)", 0xC0000005, true},
		{"stack overflow (windows)", 0xC00000FD, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := abnormalExit(tc.code); got != tc.want {
				t.Errorf("abnormalExit(%#x) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

// The retry environment has to be smaller than the one that just killed the
// converter, and still carry the handful of variables the converter genuinely
// needs — dropping PATH would turn a crash into "cannot start".
func TestConverterEnvIsSmallAndKeepsWhatMatters(t *testing.T) {
	t.Setenv("PATH", os.Getenv("PATH"))
	t.Setenv("AETOX_NOT_THE_CONVERTERS_BUSINESS", "should not be passed on")

	env := converterEnv()
	if len(env) >= len(os.Environ()) {
		t.Errorf("converterEnv kept %d entries against a full environment of %d — it is meant to be the short one", len(env), len(os.Environ()))
	}

	var names []string
	for _, kv := range env {
		names = append(names, strings.ToUpper(kv[:strings.IndexByte(kv, '=')]))
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "PATH") {
		t.Errorf("PATH is missing — the converter loads its own libraries through it; got %v", names)
	}
	if strings.Contains(joined, "AETOX_NOT_THE_CONVERTERS_BUSINESS") {
		t.Errorf("converterEnv passed on an unrelated variable: %v", names)
	}
	for _, kv := range env {
		if !strings.Contains(kv, "=") {
			t.Errorf("malformed entry %q — exec would reject the whole block", kv)
		}
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
