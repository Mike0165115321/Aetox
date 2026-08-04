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
	// After the assignment, never before — it replaces SysProcAttr wholesale.
	// Done here rather than left to callers so the shell path cannot flash a
	// console window even if a new caller forgets.
	HideConsole(cmd)
	return cmd
}

// ShellName is what the model is told it is writing for. It lives next to the
// invocation it describes so the two cannot drift: a tool description that
// named the wrong shell would be worse than one that named none.
func ShellName() string { return "cmd.exe" }
