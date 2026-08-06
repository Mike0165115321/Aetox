package debuglog

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// A debug log is pasted into bug reports and screenshotted by definition — it
// is the one artifact whose whole purpose is being shown to someone else. So
// the scrub sits in the single funnel every line goes through, not at the call
// sites: a rule enforced at call sites is one the next logging line can be
// written without, and it would be written by someone debugging, in a hurry.
func TestRegisteredSecretsNeverReachTheFile(t *testing.T) {
	dir := t.TempDir()
	if err := Enable(filepath.Join(dir, "log.txt")); err != nil {
		t.Fatalf("enable: %v", err)
	}
	const key = "sk-1b63c2055de84ff7ac209a5fd53823fd"
	Redact(key)

	Msg("resolved provider with key %s", key)
	Info("api key", key)
	done := Block("connect " + key)
	done()
	if err := Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "log.txt"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(raw), key) {
		t.Fatalf("the key reached the log:\n%s", raw)
	}
	// Redacted, not vanished: telling "the key was there and correct" from
	// "the key was missing" is the one thing a credentials bug needs this for.
	if !strings.Contains(string(raw), "[redacted]") {
		t.Errorf("the line lost its shape instead of being redacted:\n%s", raw)
	}
}

// Anything short enough to collide with ordinary words is refused. Replacing
// every occurrence of a four-character string would shred the log into
// something nobody can read, and an unreadable log is its own failure.
func TestRedactIgnoresValuesTooShortToBeSecrets(t *testing.T) {
	dir := t.TempDir()
	if err := Enable(filepath.Join(dir, "log.txt")); err != nil {
		t.Fatalf("enable: %v", err)
	}
	Redact("th")
	Msg("locale is th, the desk is specialized")
	if err := Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "log.txt"))
	if !strings.Contains(string(raw), "locale is th") {
		t.Errorf("a two-character value was treated as a secret:\n%s", raw)
	}
}
