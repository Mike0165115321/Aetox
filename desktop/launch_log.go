package main

import (
	"fmt"
	"os"
	"time"
)

// openBootLog opens the file a launchLogged server writes its output to, ready
// for this boot's first line.
//
// Deliberately not per-platform. It began as five lines inside the Windows
// launcher and the Unix twin kept its own `O_TRUNC` open — same function, same
// job, one of them carrying a fix the other had never heard of, and the test
// that names the fix is written once for both. A behaviour two platforms are
// supposed to share is one piece of code or it is two behaviours.
//
// Appended, never truncated, and that is a bug fix rather than a preference.
//
// The file is being followed while it is being written — the desk terminal runs
// `Get-Content -Wait` (or `tail -f`) on it, which holds a byte offset.
// Truncating to zero under a follower leaves that offset past the end of the
// file, and the follower does not recover: it spins and re-dumps, and the pane
// it is drawn in stutters hard enough to be the first thing anyone notices. Two
// starts a minute apart were enough to do it (2026-08-12).
//
// So a restart writes a marked line and carries on below the old boot. Growth is
// bounded by rotation rather than by truncation: past the cap the file is
// renamed aside, which leaves any existing follower attached to the old file
// reading a log that has stopped — stale, which costs a view, instead of wedged,
// which costs the machine.
func openBootLog(logPath string) (*os.File, error) {
	rotateIfLarge(logPath)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	// Where this boot begins, so a reader of an appended log can tell the run it
	// is watching from the one before it.
	fmt.Fprintf(logFile, "\n=== %s: starting ===\n", time.Now().Format("2006-01-02 15:04:05"))
	return logFile, nil
}

// rotateIfLarge keeps an appended log from growing without end, by renaming
// rather than emptying — see openBootLog for why nothing here may truncate a
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
