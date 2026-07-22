package skill

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/audit"
	"github.com/Mike0165115321/Aetox/internal/rtk"
)

type shellSkill struct {
	root string
}

func (*shellSkill) Name() string { return "shell" }

func (*shellSkill) Description() string {
	return "รันคำสั่ง shell ในโฟลเดอร์ sandbox root"
}

func (s *shellSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("shell skill unavailable")
		return newToolOutput("shell", "shell", "", start, false, err), err
	}
	args := stringSlice(input["args"])
	if len(args) == 0 {
		return newToolOutput("shell", "shell", "", start, false, errors.New("usage: shell <command>")), errors.New("usage: shell <command>")
	}
	commandLine := strings.Join(args, " ")

	workDir, err := resolveSandboxPath(s.root, ".")
	if err != nil {
		return newToolOutput("shell", "shell "+commandLine, "", start, false, err), err
	}

	// Optional token-savings pass (ARCHITECTURE.md §13): if rtk has an
	// equivalent for this exact command, run that instead — same side effects
	// (rtk actually runs the real command), compacted output. Approval and
	// the audit log below still see the original commandLine; only what
	// actually executes changes.
	execLine := commandLine
	if rewritten, ok := rtk.Rewrite(commandLine); ok {
		execLine = rewritten
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", execLine)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", execLine)
	}
	cmd.Dir = workDir
	buffer := &bytes.Buffer{}
	cmd.Stdout = buffer
	cmd.Stderr = buffer

	err = cmd.Run()
	out := strings.TrimSpace(buffer.String())
	truncatedOutput, truncated := limitLines(out, defaultToolOutputLineLimit)
	command := "shell " + commandLine
	result := newToolOutput("shell", command, truncatedOutput, start, truncated, err)

	auditEntry := audit.ShellEntry{
		Command:    commandLine,
		WorkDir:    workDir,
		Success:    err == nil && !errors.Is(ctx.Err(), context.Canceled),
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		auditEntry.Error = err.Error()
	}
	_ = audit.WriteShell(auditEntry)

	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return result, ctx.Err()
		}
		if out == "" {
			result.RawOutput = "(command failed)"
			result.Content = result.RawOutput
		}
		result.Stderr = err.Error()
		return result, err
	}
	return result, nil
}
