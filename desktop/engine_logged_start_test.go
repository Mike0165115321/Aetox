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

// A tail is a snapshot; a path is somewhere to look again.
//
// The night this was added, n8n was still coming up — its log said
// "Initializing n8n process", which is a server working — and the agent
// reported it as unreachable and asked the user to go and read the terminal.
// The terminal is one-way by design (desk_terminal says so in its own
// description), and it was tailing the very file the agent could have opened
// with `read`. So the sentence now hands over the address, says that "still
// starting" is not "failed", and closes the door it walked through.
func TestTheUnreachableMessageHandsOverTheLogRatherThanASnapshotOfIt(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "engine-n8n.log")
	if err := os.WriteFile(logPath, []byte("Initializing n8n process"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := unreachableMessage("n8n", "http://localhost:5678", logPath)

	if !strings.Contains(msg, "Initializing n8n process") {
		t.Errorf("the server's own last words are gone: %q", msg)
	}
	if !strings.Contains(msg, logPath) {
		t.Errorf("no address to look again at — only a snapshot: %q", msg)
	}
	// The two things it must actually say, or the agent draws the wrong
	// conclusion from the right evidence.
	for _, want := range []string{"read", "ไม่ใช่ล้ม", "เทอร์มินัล"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the message never mentions %q: %q", want, msg)
		}
	}
}

// Two starts must not fight over one file, and the reader following it must not
// be dragged backwards.
//
// The incident: a second `n8n_server_start` while the first was still coming up
// truncated the log to zero under a live `Get-Content -Wait`, whose byte offset
// was then past the end of a file that no longer had one. The pane thrashed.
// Appending is what makes a second write harmless to a reader; the guard above
// it is what stops the second server existing at all.
func TestASecondStartAppendsRatherThanTruncating(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "engine-n8n.log")
	first := "Initializing n8n process\nEditor is now accessible\n"
	if err := os.WriteFile(logPath, []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := launchLogged("Write-Output 'second boot'", logPath); err != nil {
		t.Fatalf("launchLogged: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Initializing n8n process") {
		t.Fatalf("the first boot's log was truncated away — a live tail would be reading past EOF:\n%q", raw)
	}
	if !strings.Contains(string(raw), ": starting ===") {
		t.Errorf("nothing marks where the second boot begins:\n%q", raw)
	}
}

// The flag that stops a second launch expires on its own, or an engine that
// failed to come up could never be started again without restarting the app.
func TestTheStartGuardLetsGoAfterThePatienceRunsOut(t *testing.T) {
	a := &App{}
	if _, busy := a.engineStarting("n8n"); busy {
		t.Fatal("nothing has started yet and the guard is already closed")
	}
	a.markEngineStarting("n8n")
	if _, busy := a.engineStarting("n8n"); !busy {
		t.Fatal("a start was just fired and the guard is open")
	}
	// Older than the starter's own patience: the start is over, whatever it did.
	a.engineStartMu.Lock()
	a.engineStartedAt["n8n"] = time.Now().Add(-serverStartPatience - time.Second)
	a.engineStartMu.Unlock()
	if _, busy := a.engineStarting("n8n"); busy {
		t.Error("the guard outlived the start it was guarding — the engine is now unstartable")
	}
	// And answering clears it at once, so a real restart later is not refused.
	a.markEngineStarting("windmill")
	a.clearEngineStarting("windmill")
	if _, busy := a.engineStarting("windmill"); busy {
		t.Error("the guard survived the server answering")
	}
}
