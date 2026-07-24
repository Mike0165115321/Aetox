package proc

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// HideConsole stops a console-subsystem child (git, cmd, npx, ffmpeg, ...)
// from materialising its own console window. The production desktop build is
// a GUI-subsystem exe — children can't inherit a console that doesn't exist,
// so Windows creates one per child, and Windows 11's "default terminal
// application" hosts it in a visible Windows Terminal window that pops over
// the app. Every exec.Cmd the engine spawns must pass through here.
func HideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
