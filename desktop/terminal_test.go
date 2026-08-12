package main

import (
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
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

// A ConPTY replays its whole screen on every resize call, same dimensions or
// not, so dimensions the PTY has already received must never reach it again —
// while a genuinely new size always must (the frontend half of that contract
// is documented in Terminal.svelte's observer).
func TestTerminalResizeSkipsDimensionsAlreadyDelivered(t *testing.T) {
	f := &fakePTY{}
	a := &App{terminals: map[string]*TerminalSession{
		"t1": {id: "t1", pty: f, lastCols: 80, lastRows: 24},
	}}

	if err := a.TerminalResize("t1", 80, 24); err != nil {
		t.Fatalf("same-size resize: unexpected error: %v", err)
	}
	if f.resizes != 0 {
		t.Errorf("resize to the size the PTY was born with reached the PTY %d times, want 0", f.resizes)
	}

	if err := a.TerminalResize("t1", 100, 30); err != nil {
		t.Fatalf("new-size resize: unexpected error: %v", err)
	}
	if f.resizes != 1 {
		t.Errorf("resize to a new size reached the PTY %d times, want 1", f.resizes)
	}

	for i := 0; i < 5; i++ {
		if err := a.TerminalResize("t1", 100, 30); err != nil {
			t.Fatalf("repeat resize: unexpected error: %v", err)
		}
	}
	if f.resizes != 1 {
		t.Errorf("repeated same-size resizes reached the PTY %d times, want 1", f.resizes)
	}
}

// fakePTY hands back a scripted sequence of reads and then EOF, which is what
// a shell exiting looks like from the read loop's side.
type fakePTY struct {
	chunks  [][]byte
	closes  int
	resizes int
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
func (f *fakePTY) Resize(int, int) error       { f.resizes++; return nil }
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
	// Attached, because that is the state this test is about: a pane is mounted
	// and every chunk must reach it. The unattached half is
	// TestTerminalHoldsOutputUntilAPaneAttaches below.
	s := &TerminalSession{id: id, pty: f, attached: true}
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

// The black-pane bug, pinned.
//
// A shell prints its banner and prompt the moment it starts, and for the first
// stretch of its life no pane is listening — a Wails event with no subscriber is
// simply gone. The user's own "+" menu usually won that race by luck;
// desk_terminal, which starts the session from Go before the frontend has heard
// anything, lost it every time and showed a black rectangle with a cursor in it.
func TestTerminalHoldsOutputUntilAPaneAttaches(t *testing.T) {
	var mu sync.Mutex
	var emitted []string

	a := &App{terminals: map[string]*TerminalSession{}}
	id := nextTerminalID()
	a.emit = func(event string, payload ...any) {
		if event != "terminal:data:"+id {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		emitted = append(emitted, payload[0].(string))
	}

	// A shell that stays alive after printing its prompt, which is what every
	// real one does — fakePTY reports EOF instead, and an EOF tears the session
	// out of the map before a pane could ever attach to it.
	f := &livePTY{chunks: [][]byte{[]byte("PowerShell 7\r\n"), []byte("PS D:\\> ")}, idle: make(chan struct{})}
	s := &TerminalSession{id: id, pty: f}
	a.terminals[id] = s
	go a.pumpTerminalOutput(s)
	f.waitUntilRead(t)

	// Nothing may go out before a pane is there to receive it...
	mu.Lock()
	sent := append([]string(nil), emitted...)
	mu.Unlock()
	if len(sent) != 0 {
		t.Errorf("emitted %q before any pane attached — those bytes are the ones that go missing", sent)
	}

	// ...and the pane must get all of it when it arrives.
	if got := a.TerminalAttach(id); got != "PowerShell 7\r\nPS D:\\> " {
		t.Errorf("TerminalAttach = %q, want the banner and prompt", got)
	}
	// Handed over once. A second pane (or a re-mount) must not redraw a prompt
	// the first one already has.
	if got := a.TerminalAttach(id); got != "" {
		t.Errorf("second TerminalAttach = %q, want empty", got)
	}
	close(f.idle) // let the pump finish so the goroutine does not outlive the test
}

// livePTY is fakePTY for the case that actually matters here: a shell that
// prints and then sits there. It blocks after its chunks instead of reporting
// EOF, so the session stays in the map the way a real one does.
type livePTY struct {
	chunks [][]byte
	idle   chan struct{}
	read   int
	mu     sync.Mutex
	done   chan struct{}
}

func (f *livePTY) Read(p []byte) (int, error) {
	f.mu.Lock()
	if f.read < len(f.chunks) {
		n := copy(p, f.chunks[f.read])
		f.read++
		last := f.read == len(f.chunks)
		f.mu.Unlock()
		if last && f.done != nil {
			close(f.done)
		}
		return n, nil
	}
	f.mu.Unlock()
	<-f.idle
	return 0, io.EOF
}

func (f *livePTY) Write(p []byte) (int, error) { return len(p), nil }
func (f *livePTY) Resize(int, int) error       { return nil }
func (f *livePTY) Close() error                { return nil }

// waitUntilRead blocks until the pump has consumed every chunk, which is the
// moment the assertion above is meaningful.
func (f *livePTY) waitUntilRead(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	f.done = make(chan struct{})
	if f.read == len(f.chunks) {
		close(f.done)
	}
	done := f.done
	f.mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pump never read the shell's output")
	}
}

// After attaching, the session is live: output goes straight out as events and
// stops accumulating, or a long-running shell would grow the backlog forever.
func TestTerminalAttachSwitchesToLiveEvents(t *testing.T) {
	a := &App{terminals: map[string]*TerminalSession{}}
	id := nextTerminalID()
	var got []string
	a.emit = func(event string, payload ...any) {
		if event == "terminal:data:"+id {
			got = append(got, payload[0].(string))
		}
	}
	s := &TerminalSession{id: id, pty: &fakePTY{chunks: [][]byte{[]byte("live")}}}
	a.terminals[id] = s

	a.TerminalAttach(id)
	a.pumpTerminalOutput(s)

	if strings.Join(got, "") != "live" {
		t.Errorf("emitted %q, want %q", got, "live")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.backlog) != 0 {
		t.Errorf("backlog = %q, want empty once attached", s.backlog)
	}
}

// A session id that is not there answers empty rather than panicking: a pane
// can outlive its shell by a frame when the shell exits on its own.
func TestTerminalAttachUnknownSession(t *testing.T) {
	a := &App{terminals: map[string]*TerminalSession{}}
	if got := a.TerminalAttach("term-gone"); got != "" {
		t.Errorf("TerminalAttach = %q, want empty", got)
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
