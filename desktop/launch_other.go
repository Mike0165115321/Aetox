//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/proc"
)

// launchDetached starts the user's own command and returns without waiting.
//
// The Windows build opens a console window so a server that refuses to start
// can say why. There is no portable equivalent — which terminal emulator a
// Linux desktop has is not something this can assume — so here the process is
// started headless and detached, and a failure shows up as the address never
// answering rather than as a readable line.
//
// Stated rather than papered over: this is the weaker half of the feature on
// this platform, and the honest fix when somebody needs it is to capture the
// output into the app, not to guess at `x-terminal-emulator`.
func launchDetached(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("ไม่มีคำสั่งให้รัน")
	}
	// proc-show-window: launching a GUI program — the same exception the Windows
	// file records, and here it is the weaker half: there is no terminal to show,
	// so the marker is really saying "this one is deliberately not hidden and
	// deliberately not visible either". See the comment above.
	//
	// Through a shell for the same reason as on Windows: the field holds a
	// command line a person could type, not a program and an argv.
	cmd := exec.Command("/bin/sh", "-c", command)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// launchLogged starts the user's command headless with its output going to
// logPath — see the Windows file for the incident that shaped it. On this
// platform it also quietly repairs the weakness launchDetached admits to
// above: the output now has somewhere to be read from.
func launchLogged(command, logPath string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("ไม่มีคำสั่งให้รัน")
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	proc.HideConsole(cmd)
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	logFile.Close()
	return cmd.Process.Release()
}

// tailCommand is what the desk terminal runs to watch a launchLogged server
// boot.
func tailCommand(logPath string) string {
	return "tail -n 100 -f '" + logPath + "'"
}
