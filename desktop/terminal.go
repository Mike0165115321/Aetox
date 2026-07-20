package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync/atomic"

	"github.com/UserExistsError/conpty"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// TerminalSession wraps one live shell process attached to a ConPTY.
type TerminalSession struct {
	id  string
	pty *conpty.ConPty
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

// TerminalShells detects which shells are actually available on this
// machine, so the "+" picker never offers one that doesn't exist.
func (a *App) TerminalShells() []ShellProfile {
	candidates := []ShellProfile{
		{Name: "PowerShell 7", Path: "pwsh.exe"},
		{Name: "Windows PowerShell", Path: "powershell.exe"},
		{Name: "Command Prompt", Path: "cmd.exe"},
		{Name: "Git Bash", Path: "bash.exe"},
		{Name: "WSL", Path: "wsl.exe"},
	}
	out := []ShellProfile{}
	for _, c := range candidates {
		if resolved, err := exec.LookPath(c.Path); err == nil {
			out = append(out, ShellProfile{Name: c.Name, Path: resolved})
		}
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
	pty, err := conpty.Start(
		shellPath,
		conpty.ConPtyDimensions(cols, rows),
		conpty.ConPtyWorkDir(a.cfg.SandboxRoot),
	)
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
			wailsruntime.EventsEmit(a.ctx, "terminal:data:"+s.id, string(buf[:n]))
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
	wailsruntime.EventsEmit(a.ctx, "terminal:closed:"+id, nil)
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

// shutdown is the Wails OnShutdown hook (wired in main.go) — sweeps every
// live terminal session so shell processes never orphan when the app quits.
func (a *App) shutdown(_ context.Context) {
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
