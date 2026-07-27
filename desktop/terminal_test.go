package main

import (
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// These tests are platform-neutral on purpose: the helper below starts a real
// pty through startPTY and picks its shell from shellCandidates(), so the same
// file exercises ConPTY on Windows and creack/pty on Linux and macOS.
//
// TerminalStart itself still isn't called here — it spawns the read goroutine,
// and this file has no business racing it. pumpTerminalOutput is covered
// directly instead, which it could not be before App.emit existed:
// wailsruntime.EventsEmit does log.Fatalf (a hard os.Exit, not a recoverable
// error) when ctx isn't Wails-bound, so a test that reached an emit site used
// to kill the test binary outright. See TEST-REPORT.md Module 5.

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

// The picker must never list the same binary twice — on Unix $SHELL and the
// named fallback below it usually resolve to the identical path.
func TestTerminalShellsHasNoDuplicatePaths(t *testing.T) {
	a := &App{}
	seen := map[string]bool{}
	for _, s := range a.TerminalShells() {
		if seen[s.Path] {
			t.Errorf("TerminalShells() offers %q twice", s.Path)
		}
		seen[s.Path] = true
	}
}

func newTestTerminalSession(t *testing.T) (*App, string) {
	t.Helper()
	a := &App{terminals: map[string]*TerminalSession{}}
	shells := a.TerminalShells()
	if len(shells) == 0 {
		t.Skip("no shell found on this machine")
	}
	// No working directory on purpose: a shell holds its cwd open, and on
	// Windows t.TempDir()'s cleanup then cannot delete the directory out from
	// under it — the shell does not always die before cleanup runs.
	p, err := startPTY(shells[0].Path, 80, 24, "")
	if err != nil {
		t.Skipf("startPTY failed (no pty support in this environment?): %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	id := nextTerminalID()
	a.terminals[id] = &TerminalSession{id: id, pty: p}
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

// fakePTY hands back a scripted sequence of reads and then EOF, which is what
// a shell exiting looks like from the read loop's side.
type fakePTY struct {
	chunks [][]byte
	closes int
}

func (f *fakePTY) Read(p []byte) (int, error) {
	if len(f.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, f.chunks[0])
	f.chunks = f.chunks[1:]
	return n, nil
}

func (f *fakePTY) Write(p []byte) (int, error) { return len(p), nil }
func (f *fakePTY) Resize(int, int) error       { return nil }
func (f *fakePTY) Close() error                { f.closes++; return nil }

// The read loop is what a busy shell drives thousands of times a second, and
// it is also the piece most likely to behave differently per kernel: ConPTY
// hands over coalesced buffers, while a Unix pty master returns the moment the
// slave writes. Pin down what it must do everywhere — forward every chunk,
// then close exactly once when the shell goes away.
func TestPumpTerminalOutputForwardsThenClosesOnce(t *testing.T) {
	var mu sync.Mutex
	var data []string
	closed := 0

	a := &App{terminals: map[string]*TerminalSession{}}
	id := nextTerminalID()
	a.emit = func(event string, payload ...any) {
		mu.Lock()
		defer mu.Unlock()
		switch event {
		case "terminal:data:" + id:
			data = append(data, payload[0].(string))
		case "terminal:closed:" + id:
			closed++
		default:
			t.Errorf("unexpected event %q", event)
		}
	}

	f := &fakePTY{chunks: [][]byte{[]byte("hello "), []byte("world")}}
	s := &TerminalSession{id: id, pty: f}
	a.terminals[id] = s

	a.pumpTerminalOutput(s) // returns once the fake reports EOF

	if got := strings.Join(data, ""); got != "hello world" {
		t.Errorf("streamed %q, want %q", got, "hello world")
	}
	if closed != 1 {
		t.Errorf("emitted %d closed events, want exactly 1", closed)
	}
	if f.closes != 1 {
		t.Errorf("closed the pty %d times, want exactly 1", f.closes)
	}
	if len(a.terminals) != 0 {
		t.Errorf("session still in the map after the shell exited: %v", a.terminals)
	}
}

// A user closing the tab while the shell is exiting must not double-close.
// closeSession's map deletion is the claim that makes that safe.
func TestCloseSessionIsIdempotent(t *testing.T) {
	a := &App{terminals: map[string]*TerminalSession{}, emit: func(string, ...any) {}}
	f := &fakePTY{}
	id := nextTerminalID()
	a.terminals[id] = &TerminalSession{id: id, pty: f}

	a.closeSession(id)
	a.closeSession(id)

	if f.closes != 1 {
		t.Errorf("closed the pty %d times, want exactly 1", f.closes)
	}
}
