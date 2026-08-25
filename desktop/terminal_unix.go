//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/creack/pty"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// unixPTY adapts creack/pty to ptySession. The master is already an
// io.ReadWriteCloser, so Read and Write come free from the embedded file; only
// resizing and closing need code of their own.
type unixPTY struct {
	*os.File
	cmd *exec.Cmd
}

func (p *unixPTY) Resize(cols, rows int) error {
	return pty.Setsize(p.File, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

// Close ends everything this tab started — the shell's whole session, not just
// the shell.
//
// The session, rather than the process group, is the boundary that matters,
// and that is a measured result rather than a guess. Closing the master alone
// only raises SIGHUP for the foreground job. Killing the shell's process group
// looks like it should cover the rest, but does not: under bash *and* dash,
// with job control on or off, a backgrounded job always lands in a process
// group of its own while staying in the shell's session. `npm run dev &` then
// outlives the tab and keeps holding its port. Only the session sweep catches
// it.
//
// pty.StartWithSize sets Setsid, so this session contains this tab's processes
// and nothing else, and a session leader's id equals its pid on both Linux and
// macOS — which is why one pkill call serves both without parsing anything. If
// Setsid had failed the sid would not equal the pid, no process would match,
// and the sweep would do nothing; it cannot reach outside the tab.
//
// This matters more here than it would elsewhere: proc.KillTreeOnExit is a
// no-op without job objects, so off Windows nothing sweeps up the stragglers
// later the way the job object does at app exit.
//
// ponytail: needs pkill on PATH (procps on Linux, /usr/bin/pkill on macOS —
// standard on both). Without it this degrades to the process-group kill below,
// which is what the code did before. Read /proc on Linux and parse `ps -o
// sess=` on macOS if that ever proves not good enough.
func (p *unixPTY) Close() error {
	err := p.File.Close()
	if p.cmd == nil || p.cmd.Process == nil {
		return err
	}
	pid := p.cmd.Process.Pid

	// proc-detached: this IS the sweeper; it cannot be torn down by the thing it implements
	sweep := exec.Command("pkill", "-KILL", "-s", strconv.Itoa(pid))
	proc.HideConsole(sweep) // no-op here; keeps the "every exec site" rule exception-free
	_ = sweep.Run()

	// The shell's own group: what actually kills the foreground job, and the
	// whole of the cleanup on a machine with no pkill.
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = p.cmd.Wait() // reap, or the shell lingers as a zombie
	return err
}

// startPTY opens a pty running shellPath, sized and rooted where the caller
// asked. See terminal_windows.go for the ConPTY half.
func startPTY(shellPath string, cols, rows int, dir string) (ptySession, error) {
	// proc-detached: the PTY owns this shell and kills its whole session (see killSession above)
	cmd := exec.Command(shellPath)
	proc.HideConsole(cmd) // no-op here; keeps the "every exec site" rule exception-free
	cmd.Dir = dir
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, err
	}
	return &unixPTY{File: f, cmd: cmd}, nil
}

// shellCandidates offers the user's own login shell first and named fallbacks
// after it. $SHELL is what the terminal emulators on this machine already open
// and what the user's rc files are written for — guessing bash when they run
// fish hands them a shell with none of their aliases or prompt.
//
// TerminalShells drops whatever is not installed and collapses the duplicate
// that $SHELL almost always creates with one of the fallbacks below.
func shellCandidates() []ShellProfile {
	out := []ShellProfile{}
	if s := os.Getenv("SHELL"); s != "" {
		out = append(out, ShellProfile{Name: filepath.Base(s) + " (default)", Path: s})
	}
	return append(out,
		ShellProfile{Name: "Bash", Path: "bash"},
		ShellProfile{Name: "Zsh", Path: "zsh"},
		ShellProfile{Name: "Fish", Path: "fish"},
		ShellProfile{Name: "sh", Path: "sh"},
	)
}
