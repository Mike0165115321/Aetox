//go:build !windows

package proc

import "os/exec"

// Breakaway is a no-op off Windows — see breakaway_windows.go for why it
// exists. Unix has no job-object equivalent to escape; a child meant to
// outlive the app just needs its own session/group at the spawn site.
func Breakaway(*exec.Cmd) {}
