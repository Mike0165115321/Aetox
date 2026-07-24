package proc

import (
	"os/exec"
	"strconv"
	"time"
)

// KillOnCancel makes context cancellation (Stop button, tool timeout) kill the
// whole child tree instead of only the process we spawned. exec.CommandContext
// kills the direct child alone, so cancelling `cmd /C npm install` leaves npm,
// node and their children running as orphans for the rest of the session —
// §24's Job Object only reaps them when the app itself exits, which is far too
// late for a user who pressed Stop.
//
// WaitDelay matters just as much: Cmd.Run copies output through a pipe that a
// surviving grandchild still holds open, so without it Run blocks past the
// kill. Every long-running exec.Cmd should pass through here.
func KillOnCancel(cmd *exec.Cmd) {
	cmd.WaitDelay = 3 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// taskkill /T walks the parent-PID tree. Windows has no "kill process
		// group" primitive short of giving every command its own Job Object.
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		HideConsole(kill)
		return kill.Run()
	}
}
