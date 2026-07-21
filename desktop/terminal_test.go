package main

import (
	"os/exec"
	"testing"

	"github.com/UserExistsError/conpty"
)

// terminal.go's TerminalStart/TerminalClose/pumpTerminalOutput are NOT covered
// here: they call wailsruntime.EventsEmit(a.ctx, ...), and Wails' getEvents()
// does log.Fatalf (a hard os.Exit, not a recoverable error) when ctx isn't a
// real Wails-bound context — which it never is in a unit test. Calling
// TerminalStart in a test would kill the test binary the moment the shell
// produces any output. See TEST-REPORT.md Module 5.
//
// What's covered instead: the pure helpers, and TerminalWrite/TerminalResize
// against a real conpty session inserted directly into a.terminals (bypassing
// TerminalStart, so pumpTerminalOutput/EventsEmit are never invoked).

func TestNextTerminalIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		id := nextTerminalID()
		if seen[id] {
			t.Fatalf("nextTerminalID() returned duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestTerminalShellsOnlyResolvedPaths(t *testing.T) {
	a := &App{}
	for _, s := range a.TerminalShells() {
		if _, err := exec.LookPath(s.Path); err != nil {
			t.Errorf("TerminalShells() returned %q, but LookPath fails: %v", s.Path, err)
		}
	}
}

func newTestTerminalSession(t *testing.T) (*App, string) {
	t.Helper()
	shell, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Skip("cmd.exe not found in PATH")
	}
	pty, err := conpty.Start(shell, conpty.ConPtyDimensions(80, 24))
	if err != nil {
		t.Skipf("conpty.Start failed (no ConPTY support in this environment?): %v", err)
	}
	t.Cleanup(func() { _ = pty.Close() })

	a := &App{terminals: map[string]*TerminalSession{}}
	id := nextTerminalID()
	a.terminals[id] = &TerminalSession{id: id, pty: pty}
	return a, id
}

func TestTerminalWrite(t *testing.T) {
	a, id := newTestTerminalSession(t)
	if err := a.TerminalWrite(id, "echo hi\r\n"); err != nil {
		t.Errorf("TerminalWrite: unexpected error: %v", err)
	}
}

func TestTerminalWriteUnknownSession(t *testing.T) {
	a := &App{terminals: map[string]*TerminalSession{}}
	if err := a.TerminalWrite("no-such-id", "x"); err == nil {
		t.Error("expected error for unknown session id, got nil")
	}
}

func TestTerminalResize(t *testing.T) {
	a, id := newTestTerminalSession(t)
	if err := a.TerminalResize(id, 100, 30); err != nil {
		t.Errorf("TerminalResize: unexpected error: %v", err)
	}
}

func TestTerminalResizeUnknownSession(t *testing.T) {
	a := &App{terminals: map[string]*TerminalSession{}}
	if err := a.TerminalResize("no-such-id", 80, 24); err == nil {
		t.Error("expected error for unknown session id, got nil")
	}
}
