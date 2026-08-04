package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
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
}

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
	session := &TerminalSession{id: id, pty: pty}

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
			a.emitEvent("terminal:data:"+s.id, string(buf[:n]))
		}
		if err != nil {
			break
		}
	}
	a.closeSession(s.id)
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
func (a *App) TerminalResize(sessionID string, cols, rows int) error {
	s, err := a.getSession(sessionID)
	if err != nil {
		return err
	}
	return s.pty.Resize(cols, rows)
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
