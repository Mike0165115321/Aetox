package debuglog

import (
	"bytes"
	"regexp"
	"testing"
	"time"
)

// Block's exit line must carry the elapsed duration — that number is the whole
// point of the profiler, so pin its presence and format.
func TestBlockLogsElapsed(t *testing.T) {
	var buf bytes.Buffer
	writer = &buf
	indent = 0
	t.Cleanup(func() { writer = nil; indent = 0 })

	done := Block("phase")
	time.Sleep(2 * time.Millisecond)
	done()

	out := buf.String()
	if !regexp.MustCompile(`--- phase \(\d+\.\dms\) ---`).MatchString(out) {
		t.Fatalf("exit line missing elapsed ms: %q", out)
	}
}
