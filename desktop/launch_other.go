//go:build !windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
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
