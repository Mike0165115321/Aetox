package skill

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImageOCRSkillRejectsEscape(t *testing.T) {
	s := &imageOCRSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"../outside.png"}})
	if err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestImageOCRSkillUsageError(t *testing.T) {
	s := &imageOCRSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{}}); err == nil {
		t.Fatal("expected usage error for missing path, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected usage error for missing path arg, got nil")
	}
}

// Exercises the real "tesseract not installed" path when nothing can be found
// (true for most machines, including CI) — asserts a clear, actionable error
// rather than a raw exec.ErrNotFound.
//
// Guarded on tesseractAvailable, not exec.LookPath: a Windows box where our
// installer put Tesseract in Program Files without touching PATH has one, and
// would otherwise fall through to running it for real against the fake file
// below and fail on tesseract's own complaint.
func TestImageOCRSkillMissingBinaryGivesActionableError(t *testing.T) {
	if tesseractAvailable() {
		t.Skip("tesseract is installed on this machine — not exercising the missing-binary path")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "img.png"), []byte("not a real png, just needs to exist"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &imageOCRSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"img.png"}})
	if err == nil {
		t.Fatal("expected error when tesseract is not installed, got nil")
	}
	if out.Success {
		t.Error("Success = true, want false")
	}
	if !strings.Contains(err.Error(), "ติดตั้ง") {
		t.Errorf("error should tell the user to install Tesseract, got %q", err.Error())
	}
}

func TestMissingTesseractErrorNeverEmpty(t *testing.T) {
	// Exercises whichever OS branch this test happens to run on — the point
	// is just that every branch returns a real, non-empty message.
	if err := missingTesseractError(); err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Errorf("missingTesseractError() = %v, want a non-empty actionable message", err)
	}
}

func TestCommandExistsFalseForBogusName(t *testing.T) {
	if commandExists("this-command-definitely-does-not-exist-aetox-test") {
		t.Error("commandExists returned true for a name that can't possibly be on PATH")
	}
}

// The bug this guards: Tesseract installed by our own NSIS step, which runs
// the UB-Mannheim setup with /S and so never adds it to PATH. Before
// resolveTesseract looked here, image_ocr told the user to go install a
// Tesseract that was already sitting in Program Files.
func TestTesseractInInstallDirFindsSilentWindowsInstall(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("install-directory lookup is Windows-only by design")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "Tesseract-OCR")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	want := filepath.Join(dir, "tesseract.exe")
	if err := os.WriteFile(want, []byte("stand-in for tesseract.exe"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Cleared, not just overridden: the real machine may well have a genuine
	// install under one of the others, and finding that instead would let this
	// test pass without proving anything.
	for _, env := range []string{"ProgramW6432", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		t.Setenv(env, "")
	}
	t.Setenv("ProgramFiles", base)

	if got := tesseractInInstallDir(); got != want {
		t.Errorf("tesseractInInstallDir() = %q, want %q", got, want)
	}
}

func TestTesseractInInstallDirEmptyWhenNothingInstalled(t *testing.T) {
	for _, env := range []string{"ProgramW6432", "ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		t.Setenv(env, t.TempDir())
	}
	if got := tesseractInInstallDir(); got != "" {
		t.Errorf("tesseractInInstallDir() = %q, want \"\" when no install exists", got)
	}
}

// The bare name is the sentinel resolveTesseract falls back to, and what
// tesseractAvailable reads to mean "found nothing" — a resolver that returned
// a guessed absolute path instead would turn a missing Tesseract into a
// file-not-found about a path the user never typed.
func TestResolveTesseractFallsBackToBareName(t *testing.T) {
	if tesseractAvailable() {
		t.Skip("tesseract is present on this machine — nothing to fall back from")
	}
	if got := resolveTesseract(); got != tesseractCommand {
		t.Errorf("resolveTesseract() = %q, want the bare %q", got, tesseractCommand)
	}
}

// The defect these guard: Tesseract asked for tha+eng answers for a page of
// Chinese as readily as for Thai. It returns plausible Thai letters, exit code
// 0, and correct Arabic digits that make the surrounding nonsense look
// trustworthy — measured 2026-08-18 at a mean confidence of 47.7 against 94.2
// for real Thai and 93.2 for the same Thai after being downscaled and blown
// back up. Before this, the tool handed that text to the model with nothing
// said about it.

func TestMeanWordConfidenceAveragesOnlyWordRows(t *testing.T) {
	// Columns are Tesseract's twelve: conf at index 10, the word at 11. The
	// first four rows are the page/block/paragraph/line levels, which carry
	// conf -1 and no text; counting them would drag every mean toward -1.
	tsv := strings.Join([]string{
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext",
		"1\t1\t0\t0\t0\t0\t0\t0\t600\t200\t-1\t",
		"2\t1\t1\t0\t0\t0\t10\t10\t580\t180\t-1\t",
		"3\t1\t1\t1\t0\t0\t10\t10\t580\t90\t-1\t",
		"4\t1\t1\t1\t1\t0\t10\t10\t580\t40\t-1\t",
		"5\t1\t1\t1\t1\t1\t10\t10\t100\t40\t90\tโอน",
		"5\t1\t1\t1\t1\t2\t120\t10\t100\t40\t80\tเงิน",
	}, "\n")

	mean, words := meanWordConfidence(tsv)
	if words != 2 {
		t.Errorf("words = %d, want 2 (only the level-5 rows carrying text)", words)
	}
	if mean != 85 {
		t.Errorf("mean = %v, want 85 (the average of 90 and 80)", mean)
	}
}

// -1 is "nothing to be confident about", not "confident it is bad". The
// difference decides whether an image holding no text gets a warning under it.
func TestMeanWordConfidenceNegativeWhenNoWords(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"1\t1\t0\t0\t0\t0\t0\t0\t600\t200\t-1\t"
	if mean, words := meanWordConfidence(tsv); mean != -1 || words != 0 {
		t.Errorf("meanWordConfidence() = (%v, %d), want (-1, 0)", mean, words)
	}
}

// A truncated or absent TSV must not take the text down with it: the text is
// what was asked for, the confidence is the footnote.
func TestMeanWordConfidenceSurvivesMalformedRows(t *testing.T) {
	tsv := "level\tconf\ttext\nnot\ta\tvalid\trow\n5\t1\t1\t1\t1\t1\t10\t10\t100\t40\t75\tบาท"
	if mean, words := meanWordConfidence(tsv); words != 1 || mean != 75 {
		t.Errorf("meanWordConfidence() = (%v, %d), want (75, 1)", mean, words)
	}
}

func TestAppendConfidenceNoteWarnsBelowThreshold(t *testing.T) {
	// 47.7 over 17 words is the real reading of a Chinese slip through tha+eng.
	got := appendConfidenceNote("SEM AKIN ธร พ 1.250.00", 47.7, 17)
	if !strings.Contains(got, "SEM AKIN") {
		t.Error("the text itself must survive the note being added")
	}
	if !strings.Contains(got, "48%") {
		t.Errorf("note should state the measured confidence, got %q", got)
	}
	// Naming the two languages is the actionable half: a reader told only that
	// confidence was low retries the same unreadable page.
	if !strings.Contains(got, "ไทย") || !strings.Contains(got, "อังกฤษ") {
		t.Errorf("note should name the two languages it can read, got %q", got)
	}
}

func TestAppendConfidenceNoteSilentOnGoodReading(t *testing.T) {
	text := "โอนเงินสำเร็จ 1,250.00 บาท"
	for _, tc := range []struct {
		name  string
		mean  float64
		words int
	}{
		{"sharp Thai", 94.2, 33},
		{"Thai degraded by a downscale round trip", 93.2, 33},
		{"exactly at the threshold", ocrLowConfidence, 10},
		{"no words to judge", -1, 0},
	} {
		if got := appendConfidenceNote(text, tc.mean, tc.words); got != text {
			t.Errorf("%s: appendConfidenceNote added a warning it should not have: %q", tc.name, got)
		}
	}
}

// The temp directory is new plumbing: runTesseract now writes txt+tsv to disk
// rather than reading stdout, so a leaked directory per OCR is a failure mode
// that did not exist before.
func TestRunTesseractLeavesNoTempDirBehind(t *testing.T) {
	if !tesseractAvailable() {
		t.Skip("tesseract not installed on this machine")
	}
	tmp := t.TempDir()
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	t.Setenv("TMPDIR", tmp)

	img := filepath.Join(t.TempDir(), "blank.png")
	if err := os.WriteFile(img, pngBytes(t, 60, 30), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := runTesseract(context.Background(), img); err != nil {
		t.Fatalf("runTesseract: %v", err)
	}

	left, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("reading temp root: %v", err)
	}
	for _, e := range left {
		if strings.HasPrefix(e.Name(), "aetox-ocr-") {
			t.Errorf("runTesseract left %s behind in the temp root", e.Name())
		}
	}
}
