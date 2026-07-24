//go:build !windows

package proc

// KillTreeOnExit is a no-op off Windows — see job_windows.go for why it
// exists. Unix front ends can rely on process groups when they need this.
func KillTreeOnExit() bool { return false }
