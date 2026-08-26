//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// The two endings both run through a tiny PowerShell that first waits for THIS
// process to exit. That ordering is the whole point: the new instance must
// never overlap the old one on the session database, and an installer must
// never race the app it is replacing. PowerShell because it is on every
// Windows.
//
// The script goes across as readable text. It used to go as -EncodedCommand,
// base64, on the theory that a path quoted through cmd.exe's parsing rules is
// a bug generator this package refuses to own. That theory was answering a
// problem it does not have: exec.Command never goes through cmd.exe, it builds
// the command line itself, and psq below leaves a script with no double quote
// in it for anything to trip over.
//
// What base64 did buy was a command line indistinguishable from a dropper's.
// Hidden-window PowerShell running an encoded payload is ASR rule
// 5beb7efe-fd9a-4556-801d-275e5ffc04cc, "Block execution of potentially
// obfuscated scripts" — the same family of shape that got our unsigned
// installer classified as Program:Win32/Wacapew.C!ml on 2026-08-20. And
// internal/skill/shell_sandbox.go already refuses -EncodedCommand from the
// agent, in those words: nothing that needs base64 to say what it runs is
// being written for a human to read. That applied to us too.
//
// The window is suppressed by proc.HideConsole (CREATE_NO_WINDOW) rather than
// by -WindowStyle Hidden, for the same reason. The flag is a word on a command
// line every detection rule in the industry reads; the creation flag is how a
// GUI-subsystem app has always started a console child it does not want drawn.

// relaunchAfterExit starts exe once the current process is gone — the portable
// channel's ending, where the exe on disk is already the new build.
func relaunchAfterExit(exe string) error {
	return runWaiter(fmt.Sprintf(
		"Wait-Process -Id %d -ErrorAction SilentlyContinue; Start-Process -FilePath '%s'",
		os.Getpid(), psq(exe)))
}

// handOffToInstaller runs the downloaded installer silently once the app has
// exited, then starts the app again — the same sequence as re-running the
// installer by hand, minus the hands. UAC still shows if the install scope
// needs it; a cancelled elevation falls through to relaunching the untouched
// old build, which is the correct failure.
func handOffToInstaller(installer string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return runWaiter(fmt.Sprintf(
		"Wait-Process -Id %d -ErrorAction SilentlyContinue; "+
			"try { Start-Process -Wait -FilePath '%s' -ArgumentList '/S' } catch {}; "+
			"Start-Process -FilePath '%s'",
		os.Getpid(), psq(installer), psq(exe)))
}

// psq escapes one string for a single-quoted PowerShell literal. Single quotes
// throughout is also what keeps the script free of double quotes, which is
// what lets it cross as one plain argument.
func psq(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func runWaiter(script string) error {
	// proc-detached: the relaunch has to survive this process exiting; that is its whole job
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	proc.HideConsole(cmd)
	// Breakaway is what makes the line above true, not just intended: every
	// child normally joins the app's KILL_ON_JOB_CLOSE job, and a waiter
	// inside the job is killed by the OS at the exact moment Wait-Process
	// would have returned — the installer never runs, the app never comes
	// back, and the whole thing looks like the update simply didn't happen.
	proc.Breakaway(cmd)
	// Start, never Run: this process is about to exit, and the waiter's whole
	// job begins at that moment.
	return cmd.Start()
}
