//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
	"time"
)

// KillOnCancel makes context cancellation kill the whole child tree — see
// tree_windows.go for why the default single-process kill is not enough.
// Unix gets this for free with a process group: the child leads its own group
// and the negative-PID signal reaches every descendant.
func KillOnCancel(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.WaitDelay = 3 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// KillTree kills a tree now, for a caller that is not going through a context —
// see tree_windows.go for why a graceful protocol shutdown leaves one to kill.
//
// It signals the process group, so it only reaches descendants of a command
// that led one, which is what KillOnCancel's Setpgid arranges at spawn. A
// command started without it has no group of its own and this reaches only the
// process itself; pair the two.
//
// The pid > 1 guard is not defensive tidiness. kill(-1) signals every process
// the user can signal, so a zero pid from a command that never started would
// take the user's session down with it.
func KillTree(pid int) {
	if pid <= 1 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
