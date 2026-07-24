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
