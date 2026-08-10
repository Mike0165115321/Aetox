package main

// The engine's start must not depend on any UI plumbing being healthy — the
// 2026-08-11 lesson, learned at the user's expense: an engine typed into the
// desk terminal's ConPTY froze mid-boot when the plumbing wedged, while the
// same command outside it came up in nine seconds. So the server writes to a
// file and the pane only reads. These tests drive the two halves that carry
// that design, with a real child process and a real file.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// launchLogged must run the command through a real shell, put everything it
// prints into the log, and come back without waiting for the process to end.
func TestALoggedLaunchWritesTheCommandsOutputToTheFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "engine-test.log")
	if err := launchLogged("echo aetox-engine-boot-line", logPath); err != nil {
		t.Fatalf("launchLogged: %v", err)
	}
	// Released, not waited on — so the test polls the way waitReachable would.
	deadline := time.Now().Add(20 * time.Second)
	for {
		raw, _ := os.ReadFile(logPath)
		if strings.Contains(string(raw), "aetox-engine-boot-line") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the log never carried the command's output; it holds: %q", string(raw))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// The failure message carries the server's own last words. This is what turns
// "ยังไม่ตอบใน 90 วินาที" from a dead end into a diagnosis — the night this was
// built, the model spent twenty-nine tool calls hunting for a reason that was
// sitting in a file nothing had handed to it.
func TestTheFailureTailCarriesTheServersOwnWords(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "engine-test.log")
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, "boot line")
	}
	lines = append(lines, "Error: port 5678 is already in use")
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	tail := tailOfFile(logPath, 15)
	if !strings.Contains(tail, "port 5678 is already in use") {
		t.Errorf("the tail lost the one line that mattered: %q", tail)
	}
	if strings.Count(tail, "\n") >= 30 {
		t.Error("the tail is the whole file — a chatty server would drown the sentence the model needs")
	}

	// And the two empty shapes stay tellable-apart: no file, and no words.
	if got := tailOfFile(filepath.Join(t.TempDir(), "absent.log"), 15); !strings.Contains(got, "อ่าน log ไม่ได้") {
		t.Errorf("a missing log did not say so: %q", got)
	}
	empty := filepath.Join(t.TempDir(), "empty.log")
	_ = os.WriteFile(empty, nil, 0o644)
	if got := tailOfFile(empty, 15); !strings.Contains(got, "ว่างเปล่า") {
		t.Errorf("an empty log did not say so: %q", got)
	}
}
