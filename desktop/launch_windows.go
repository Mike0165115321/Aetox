//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
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
