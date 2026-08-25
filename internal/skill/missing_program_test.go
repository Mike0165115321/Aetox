package skill

import (
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/statereport"
)

// "This machine does not have X installed" is weather, not behaviour. No way of
// calling image_ocr conjures Tesseract onto the disk, so the sentence teaches
// nothing that would still be true after one install — and the learning floor,
// which reads every unmarked failure as a possible lesson, drafted exactly that
// card: three missing-Tesseract runs became "เลี่ยงรูปแบบที่ชนเงื่อนไขนี้ตั้งแต่
// ครั้งแรก" in the approval queue (2026-08-18), a permanent rule against OCR
// written from an install step that had been skipped.
//
// The mark is what keeps them out (internal/statereport -> turn.ErrorFromWorld
// -> the summarizer's error_kind filter). Asserted here at the author, because
// the author is the only place that knows: downstream, every one of these is
// just a Thai sentence carrying a remedy, indistinguishable from a real refusal.
func TestMissingProgramErrorsAreStateReports(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"tesseract", missingTesseractError()},
		{"poppler", missingPopplerError()},
		{"ffmpeg", missingFFmpegError()},
	} {
		if !statereport.Is(tc.err) {
			t.Errorf("%s: %q is not marked as a state report — the summarizer will offer it as a permanent lesson", tc.name, tc.err)
		}
	}
}

// The counterexample, kept next to the rule so the line between them stays
// visible. A call with no path is the caller's own mistake: the remedy is a
// different next call, and it will be just as true on a machine with every
// binary installed. Unmarked on purpose — unmarked is what "this may be a
// lesson" means, and marking it would hide a real one.
func TestARemedyTheCallerCanActOnStaysALesson(t *testing.T) {
	s := &pdfReadSkill{root: t.TempDir()}
	_, err := s.Execute(t.Context(), Input{})
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected the usage refusal for a call with no path, got %v", err)
	}
	if statereport.Is(err) {
		t.Errorf("a usage refusal was marked as weather: %q", err)
	}
}
