package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Mike0165115321/Aetox/internal/lsp"
)

// ptySession is the whole of what differs between platforms in this file:
// Windows drives a ConPTY, Unix an ordinary pty master. *conpty.ConPty already
// has exactly this method set, so terminal_windows.go needs no adapter type at
// all; terminal_unix.go wraps creack/pty's *os.File only because resizing there
// is a package function rather than a method, and because closing has to sweep
// the shell's process group by hand.
type ptySession interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// TerminalSession wraps one live shell process attached to a pty.
type TerminalSession struct {
	id  string
	pty ptySession

	// A shell starts talking the moment it exists — banner, then prompt — and
	// for the first stretch of its life nothing is listening: the pane that will
	// show it has not mounted yet, and an event with no subscriber is simply
	// gone. That is a race the user's own "+" menu usually wins by luck and
	// desk_terminal lost outright, which is what a black pane with a cursor in
	// it actually was.
	//
	// So the session holds its output until a pane says it is there. Nothing is
	// emitted before that, which means there is no window in which a chunk can
	// be dropped and none in which one can arrive twice.
	mu       sync.Mutex
	backlog  []byte
	attached bool

	// The last dimensions the PTY actually received — the birth size at first,
	// then whatever the latest delivered Resize carried. Kept because resizing
	// a ConPTY is not idempotent: every ResizePseudoConsole call makes it
	// re-emit the whole visible screen, same dimensions or not (measured
	// 2026-08-12: ten same-size resizes replayed the shell banner ten times).
	// The pane's ResizeObserver fires on sub-pixel layout jitter that never
	// changes the column count, and during an engine boot those replays landed
	// between live output lines — a desk terminal full of half-shifted
	// duplicates was the whole of that morning's "terminal is broken".
	lastCols int
	lastRows int
}

// maxBacklog caps what an unattached session keeps. A shell nobody has attached
// to yet has printed a banner and a prompt; anything past this is a runaway
// process filling memory for a pane that may never open.
const maxBacklog = 256 << 10

// ShellProfile is one shell the terminal picker can offer.
type ShellProfile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

var terminalSeq int64

func nextTerminalID() string {
	return "term-" + strconv.FormatInt(atomic.AddInt64(&terminalSeq, 1), 10)
}

// emitEvent sends a frontend event, through a.emit when a test has installed
// one. See App.emit for why the indirection has to exist at all.
//
// A nil ctx is silently nothing rather than a crash: an event with no window
// to reach is not an error, and every emitter used to carry its own `if a.ctx
// != nil` for exactly that reason. Holding it here is what lets them all go
// through one door.
func (a *App) emitEvent(event string, data ...any) {
	if a.emit != nil {
		a.emit(event, data...)
		return
	}
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, event, data...)
}

// TerminalShells detects which shells are actually available on this
// machine, so the "+" picker never offers one that doesn't exist. The list of
// what to look for is per-OS (terminal_windows.go / terminal_unix.go).
//
// Deduplicating on the resolved path matters on Unix, where $SHELL almost
// always points at the same binary as one of the named fallbacks — without it
// the picker offers "bash (default)" and "Bash" as two separate choices that
// start the identical shell.
func (a *App) TerminalShells() []ShellProfile {
	out := []ShellProfile{}
	seen := map[string]bool{}
	for _, c := range shellCandidates() {
		resolved, err := exec.LookPath(c.Path)
		if err != nil || seen[resolved] {
			continue
		}
		seen[resolved] = true
		out = append(out, ShellProfile{Name: c.Name, Path: resolved})
	}
	return out
}

// TerminalStart spawns a new interactive shell session and starts streaming
// its output back as "terminal:data:<id>" events. Returns the new session id.
func (a *App) TerminalStart(shellPath string, cols, rows int) (string, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	pty, err := startPTY(shellPath, cols, rows, a.cfg.SandboxRoot)
	if err != nil {
		return "", fmt.Errorf("start terminal: %w", err)
	}

	id := nextTerminalID()
	session := &TerminalSession{id: id, pty: pty, lastCols: cols, lastRows: rows}

	a.terminalsMu.Lock()
	if a.terminals == nil {
		a.terminals = make(map[string]*TerminalSession)
	}
	a.terminals[id] = session
	a.terminalsMu.Unlock()

	go a.pumpTerminalOutput(session)
	return id, nil
}

// pumpTerminalOutput streams PTY output to the frontend until the shell
// exits or the session is closed, then cleans itself up.
func (a *App) pumpTerminalOutput(s *TerminalSession) {
	buf := make([]byte, 4096)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.mu.Lock()
			if s.attached {
				s.mu.Unlock()
				a.emitEvent("terminal:data:"+s.id, string(buf[:n]))
			} else {
				s.backlog = append(s.backlog, buf[:n]...)
				if len(s.backlog) > maxBacklog {
					// Keep the tail: the prompt the user is about to type at
					// matters more than the banner above it.
					s.backlog = s.backlog[len(s.backlog)-maxBacklog:]
				}
				s.mu.Unlock()
			}
		}
		if err != nil {
			break
		}
	}
	a.closeSession(s.id)
}

// TerminalAttach hands a newly mounted pane everything the session has said so
// far, and switches it to live events from here on.
//
// Called by Terminal.svelte after it subscribes and before it draws anything.
// That order is the whole design: while unattached the pump emits nothing, so
// subscribing first cannot miss a chunk, and attaching second cannot replay one
// that already arrived live.
func (a *App) TerminalAttach(sessionID string) string {
	a.terminalsMu.Lock()
	s := a.terminals[sessionID]
	a.terminalsMu.Unlock()
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attached = true
	backlog := string(s.backlog)
	s.backlog = nil
	return backlog
}

// closeSession removes a session from the map and closes its PTY exactly
// once, however it's triggered — the map deletion below is the atomic claim
// that a natural shell-exit (from pumpTerminalOutput) and a user-initiated
// TerminalClose race safely against, so only one of them ever calls
// pty.Close()/emits the closed event.
func (a *App) closeSession(id string) {
	a.terminalsMu.Lock()
	s, ok := a.terminals[id]
	if ok {
		delete(a.terminals, id)
	}
	a.terminalsMu.Unlock()
	if !ok {
		return
	}
	_ = s.pty.Close()
	a.emitEvent("terminal:closed:"+id, nil)
}

func (a *App) getSession(id string) (*TerminalSession, error) {
	a.terminalsMu.Lock()
	s, ok := a.terminals[id]
	a.terminalsMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("terminal session %q not found", id)
	}
	return s, nil
}

// TerminalWrite sends keystrokes/input to a running session.
func (a *App) TerminalWrite(sessionID, data string) error {
	s, err := a.getSession(sessionID)
	if err != nil {
		return err
	}
	_, err = s.pty.Write([]byte(data))
	return err
}

// TerminalResize adjusts a running session's console dimensions.
//
// Dimensions the PTY has already received are dropped here, and this is the
// only layer that may drop them. The frontend deliberately reports every fit
// (Terminal.svelte — a size the shell never hears about is a broken terminal,
// reverted the same hour it shipped), so the filter has to live with the one
// party that knows what was actually delivered. Skipping is safe precisely
// because it is conditioned on that knowledge: a size can only be skipped
// when the PTY already heard exactly that size, so the wrap-mismatch that
// killed the frontend attempt cannot happen. What skipping buys is on
// lastCols — a same-size ConPTY resize replays the whole screen.
func (a *App) TerminalResize(sessionID string, cols, rows int) error {
	s, err := a.getSession(sessionID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cols == s.lastCols && rows == s.lastRows {
		return nil
	}
	if err := s.pty.Resize(cols, rows); err != nil {
		return err
	}
	s.lastCols, s.lastRows = cols, rows
	return nil
}

// TerminalClose ends a session (user closed the tab).
func (a *App) TerminalClose(sessionID string) error {
	if _, err := a.getSession(sessionID); err != nil {
		return err
	}
	a.closeSession(sessionID)
	return nil
}

// shutdown is the Wails OnShutdown hook (wired in main.go) — closes the local
// store and MCP servers, then sweeps every live terminal session so shell and
// server processes never orphan when the app quits. (Chat turns are persisted
// as they happen.)
func (a *App) shutdown(_ context.Context) {
	if a.db != nil {
		_ = a.db.Close()
	}
	if a.mcp != nil {
		_ = a.mcp.Close()
	}
	// Language servers are process-wide and shared per workspace root, so
	// nothing else owns them: without this, gopls/node keep running and only
	// the Windows job object reaps them — off Windows there is no job object
	// and they orphan for good.
	lsp.CloseShared()
	a.terminalsMu.Lock()
	ids := make([]string, 0, len(a.terminals))
	for id := range a.terminals {
		ids = append(ids, id)
	}
	a.terminalsMu.Unlock()
	for _, id := range ids {
		a.closeSession(id)
	}
}
