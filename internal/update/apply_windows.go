//go:build windows

package update

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf16"

	"github.com/Mike0165115321/Aetox/internal/proc"
)

// The two endings both run through a tiny hidden PowerShell that first waits
// for THIS process to exit. That ordering is the whole point: the new instance
// must never overlap the old one on the session database, and an installer
// must never race the app it is replacing. PowerShell because it is on every
// Windows, and -EncodedCommand because a path quoted through cmd.exe's parsing
// rules is a bug generator this package refuses to own.

// relaunchAfterExit starts exe once the current process is gone — the portable
// channel's ending, where the exe on disk is already the new build.
func relaunchAfterExit(exe string) error {
	return runHiddenPS(fmt.Sprintf(
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
	return runHiddenPS(fmt.Sprintf(
		"Wait-Process -Id %d -ErrorAction SilentlyContinue; "+
			"try { Start-Process -Wait -FilePath '%s' -ArgumentList '/S' } catch {}; "+
			"Start-Process -FilePath '%s'",
		os.Getpid(), psq(installer), psq(exe)))
}

// psq escapes one string for a single-quoted PowerShell literal.
func psq(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func runHiddenPS(script string) error {
	// proc-detached: the relaunch has to survive this process exiting; that is its whole job
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-WindowStyle", "Hidden", "-EncodedCommand", encodePS(script))
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

// encodePS is PowerShell's -EncodedCommand format: UTF-16LE, base64.
func encodePS(script string) string {
	codes := utf16.Encode([]rune(script))
	b := make([]byte, 0, len(codes)*2)
	for _, c := range codes {
		b = append(b, byte(c), byte(c>>8))
	}
	return base64.StdEncoding.EncodeToString(b)
}
