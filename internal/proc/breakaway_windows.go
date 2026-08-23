package proc

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// Breakaway makes cmd start OUTSIDE the KillTreeOnExit job — for the one kind
// of child whose whole purpose begins at the moment this process exits. Today
// that is the update waiter: it must watch the app die, run the installer, and
// bring the new build up. Without this flag it joins the job like every other
// child and KILL_ON_JOB_CLOSE shoots it in the same instant it was waiting for
// — the update silently never installs, and "restart to update" restarts into
// the same build (found 2026-08-23, v1.5.0 offered v1.5.2 and came back v1.5.0).
//
// Works only because KillTreeOnExit's job sets JOB_OBJECT_LIMIT_BREAKAWAY_OK:
// the job opens the door, and only a child that asks walks through it. In an
// environment whose own outer job forbids breakaway, the spawn fails outright
// (CreateProcess, ERROR_ACCESS_DENIED) rather than silently staying inside —
// callers get a loud error instead of a waiter that cannot do its job.
func Breakaway(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_BREAKAWAY_FROM_JOB
}
