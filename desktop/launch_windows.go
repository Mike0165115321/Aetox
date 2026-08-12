//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Mike0165115321/Aetox/internal/proc"
)

// launchDetached starts the user's own command in a window of its own and
// returns without waiting for it.
//
// A window rather than a hidden process, deliberately. This starts a SERVER —
// something that runs for hours and prints why it refused to start when it
// refuses. Hidden, its output goes nowhere, and a user whose n8n died on a port
// clash would see only Aetox's timeout with no way to find out more. The window
// is also how they stop it, which a background process would have taken away.
//
// Run through the shell rather than exec'd directly because what the field
// holds is a command line the user could type — `D:\lab\start.ps1`, or an npm
// script with flags — not a program and an argv this code could split correctly
// on its own.
func launchDetached(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("ไม่มีคำสั่งให้รัน")
	}
	// proc-show-window: launching a GUI program — read the rule in
	// internal/proc/coverage_test.go, which names this second kind of exception:
	// a server the user asked to start, whose console is the only place it can
	// say why it refused and the only way they can stop it. Hiding it would turn
	// a port clash into Aetox timing out with nothing to read.
	//
	// -NoExit keeps the window open after the command ends, which is what makes
	// a failure readable: without it a server that refuses to start closes its
	// own window before anybody can see the reason.
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoExit", "-ExecutionPolicy", "Bypass", "-Command", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: newConsole}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Released rather than waited on: the caller's job is done the moment the
	// window exists, and the server is expected to outlive this click.
	return cmd.Process.Release()
}

// newConsole is CREATE_NEW_CONSOLE. Written out rather than imported so the
// non-Windows file next door does not have to pretend it exists.
const newConsole = 0x00000010

// launchLogged starts the user's command hidden, with everything it prints
// going to logPath — the engine-start shape (engine_server.go).
//
// A third shape beside launchDetached's visible console, and the incident that
// forced it is worth keeping: on 2026-08-11 the engine was typed into a desk
// terminal's ConPTY so the user could watch it boot, and the boot froze — n8n
// alive, port never opened — because a console child's stdout is a pipe some
// part of the terminal plumbing has to keep draining, and the same command run
// outside that plumbing came up in nine seconds. A file blocks nobody, ever.
// The watching the desk terminal used to provide is now a tail of this file
// (tailCommand below): the server writes, the pane only reads, and a pane that
// wedges costs the user their view of the boot rather than the boot.
//
// Hidden is safe here where launchDetached documents it must not be, because
// the two reasons its window exists — reading why the server refused, and
// stopping it — have new answers: the refusal is in the log the tool reads
// back on failure, and the process dies with the app through the §24 job,
// which a detached console deliberately escapes.
func launchLogged(command, logPath string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("ไม่มีคำสั่งให้รัน")
	}
	// Appended, never truncated, and that is a bug fix rather than a preference.
	//
	// The file is being followed while it is being written — the desk terminal
	// runs `Get-Content -Wait` on it, which holds a byte offset. Truncating to
	// zero under a follower leaves that offset past the end of the file, and the
	// follower does not recover: it spins and re-dumps, and the pane it is drawn
	// in stutters hard enough to be the first thing anyone notices. Two starts a
	// minute apart were enough to do it (2026-08-12).
	//
	// So a restart writes a marked line and carries on below the old boot. Growth
	// is bounded by rotation rather than by truncation: past the cap the file is
	// renamed aside, which leaves any existing follower attached to the old file
	// reading a log that has stopped — stale, which costs a view, instead of
	// wedged, which costs the machine.
	rotateIfLarge(logPath)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	// Where this boot begins, so a reader of an appended log can tell the run it
	// is watching from the one before it.
	fmt.Fprintf(logFile, "\n=== %s: starting ===\n", time.Now().Format("2006-01-02 15:04:05"))
	cmd := exec.Command("powershell.exe", "-NoLogo", "-ExecutionPolicy", "Bypass", "-Command", command)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	proc.HideConsole(cmd)
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	// The file handle is the child's now; ours can close. Released rather than
	// waited on for launchDetached's reason: the server outlives this call.
	logFile.Close()
	return cmd.Process.Release()
}

// tailCommand is what the desk terminal runs to watch a launchLogged server
// boot: the log, live, from the top of this boot.
func tailCommand(logPath string) string {
	return "powershell -NoLogo -Command \"Get-Content -Path '" + logPath + "' -Wait -Tail 100\""
}

// rotateIfLarge keeps an appended log from growing without end, by renaming
// rather than emptying — see launchLogged for why nothing here may truncate a
// file something else is following.
//
// Best effort throughout: a log that cannot be measured, or cannot be renamed
// because a reader still holds it, is simply appended to. Growing is a problem
// worth solving later; refusing to start an engine over it is not.
func rotateIfLarge(logPath string) {
	const cap = 4 << 20 // 4 MB — hundreds of boots, and small enough to open by hand
	info, err := os.Stat(logPath)
	if err != nil || info.Size() < cap {
		return
	}
	_ = os.Remove(logPath + ".prev")
	_ = os.Rename(logPath, logPath+".prev")
}
