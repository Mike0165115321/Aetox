package stt

import (
	"testing"

	"github.com/Mikedev115/Aetox/internal/statereport"
)

// Same rule as the tool layer's missing binaries (internal/skill): a whisper
// binary that was never installed and a model file that was never downloaded
// describe this machine, not the call. Marked, so three transcribe attempts on
// a machine without them cannot become a permanent memory line telling the
// agent that audio_transcribe is a route to avoid.
func TestMissingWhisperPiecesAreStateReports(t *testing.T) {
	if err := missingBinaryError(catalog[0]); !statereport.Is(err) {
		t.Errorf("missing binary: %q is not marked as a state report", err)
	}
	if err := missingModelError(t.TempDir()); !statereport.Is(err) {
		t.Errorf("missing model: %q is not marked as a state report", err)
	}
}
