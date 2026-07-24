//go:build !windows

package proc

import (
	"context"
	"os/exec"
)

// ShellCommand builds the shell invocation for an agent-supplied command
// line. Unix needs no special handling — execve takes an argv array, so the
// line reaches sh byte-for-byte. See shell_windows.go for why Windows does.
func ShellCommand(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", line)
}
