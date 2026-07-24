//go:build !windows

package proc

import "os/exec"

// HideConsole is a no-op off Windows — see hide_windows.go for why it exists.
func HideConsole(*exec.Cmd) {}
