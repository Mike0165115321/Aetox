//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// Both launchers below pass -ExecutionPolicy RemoteSigned, and the value is
// the point.
//
// The field holds a command line the user could type, which includes the path
// of a .ps1 they wrote — and a script file IS subject to execution policy,
// where a -Command string is not. So the switch cannot simply be dropped: on a
// default Windows client (Restricted) their own start.ps1 would stop running.
//
// It used to say Bypass, which does the job and also writes the single most
// recognisable word in malicious-PowerShell detection onto our command line —
// Splunk, Elastic and Rapid7 all ship a rule that keys on it, and Aetox is an
// unsigned binary with no reputation that already spawns shells for a living.
// RemoteSigned buys the same thing for the case that exists (a local script
// they wrote, which carries no mark-of-the-web and runs unsigned) and refuses
// the case that should be refused anyway: an unsigned script downloaded from
// the internet.

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
	// proc-detached: a server the user started from Settings; its console is how they stop it
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoExit", "-ExecutionPolicy", "RemoteSigned", "-Command", command)
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
	// Appended under a live follower rather than truncated, and rotated by
	// rename — launch_log.go carries the incident that shaped both, and carries
	// them for the Unix twin at the same time.
	logFile, err := openBootLog(logPath)
	if err != nil {
		return err
	}
	// proc-detached: a server the user started from Settings; its console is how they stop it
	cmd := exec.Command("powershell.exe", "-NoLogo", "-ExecutionPolicy", "RemoteSigned", "-Command", command)
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
