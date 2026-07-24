package proc

import (
	"context"
	"os/exec"
	"syscall"
)

// ShellCommand builds the cmd.exe invocation for an agent-supplied command
// line.
//
// It sets SysProcAttr.CmdLine instead of passing the line as an argument
// because cmd.exe does not follow the C runtime's argv convention that
// os/exec's escaping assumes. Handed `exec.Command("cmd", "/C", line)`, Go
// escapes every embedded quote as \" — so `git commit -m "msg"` reached git
// as `-m \"msg\"`, `python -c "..."` broke, and `echo "hello world"` printed
// \"hello world\". Every quoted shell command the agent ran on Windows was
// silently corrupted (found 2026-07-25 while testing the process-tree kill).
//
// /S is the documented cmd.exe rule that makes this exact: strip the first
// and last quote of the line, take everything between them literally.
func ShellCommand(ctx context.Context, line string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /S /C "` + line + `"`}
	return cmd
}
