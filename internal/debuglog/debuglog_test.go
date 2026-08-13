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

// The ring is the bug-report bundle's source, and bug reports come from the
// installs where nobody enabled file logging — so it must fill with no writer
// open, stay capped, and hold lines that went through the same scrub as the
// file.
func TestRecentRemembersWithoutAFileAndStaysScrubbed(t *testing.T) {
	recentMu.Lock()
	recent = nil
	recentMu.Unlock()
	t.Cleanup(func() {
		recentMu.Lock()
		recent = nil
		recentMu.Unlock()
	})

	const key = "sk-9f52c81e77aa4bd3b60f2b1c33d94a01"
	Redact(key)
	Msg("no writer, key %s", key)
	for i := 0; i < recentCap+50; i++ {
		Msg("line %d", i)
	}

	got := Recent(0)
	if len(got) != recentCap {
		t.Fatalf("ring holds %d lines, want the cap %d", len(got), recentCap)
	}
	joined := strings.Join(Recent(recentCap), "\n")
	if strings.Contains(joined, key) {
		t.Fatal("a registered secret reached the ring")
	}
	// Newest survive, oldest fall off, order oldest-first.
	tail := Recent(2)
	if !strings.Contains(tail[1], "line 249") || !strings.Contains(tail[0], "line 248") {
		t.Errorf("tail = %q, want the two newest lines oldest-first", tail)
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
