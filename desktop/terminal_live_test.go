package main

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// A live check of the one assumption every unit test in terminal_test.go
// fakes away: that startPTY on this machine really produces a shell that
// prints something. The black-pane bug has now been reported twice with the
// attach ordering correct both times, which means the pane can also be black
// because ConPTY itself came up mute — and no fake can catch that.
func TestLivePTYSpeaks(t *testing.T) {
	for _, c := range shellCandidates() {
		resolved, err := exec.LookPath(c.Path)
		if err != nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			pty, err := startPTY(resolved, 80, 24, t.TempDir())
			if err != nil {
				t.Fatalf("startPTY(%s): %v", resolved, err)
			}
			defer pty.Close()

			var collected strings.Builder
			done := make(chan struct{})
			go func() {
				defer close(done)
				buf := make([]byte, 4096)
				for {
					n, err := pty.Read(buf)
					if n > 0 {
						collected.WriteString(string(buf[:n]))
					}
					if err != nil {
						return
					}
					// A banner or a prompt is all the proof needed.
					if collected.Len() > 0 {
						return
					}
				}
			}()
			select {
			case <-done:
			case <-time.After(15 * time.Second):
			}
			if collected.Len() == 0 {
				t.Errorf("%s said nothing in 15s — this is the black pane", c.Name)
			} else {
				t.Logf("%s spoke %d bytes, first line: %q", c.Name, collected.Len(), firstLine(collected.String()))
			}
		})
	}
}
