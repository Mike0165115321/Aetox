package skill

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/audit"
	"github.com/Mike0165115321/Aetox/internal/proc"
	"github.com/Mike0165115321/Aetox/internal/rtk"
)

type shellSkill struct {
	root string
}

// shellOutputCap bounds what one command may buffer in RAM. limitLines only
// trims after the command exits, so an unbounded buffer lets a runaway
// producer (`yes`, a looping log tail, a chatty build) grow to gigabytes for
// the full tool timeout and take the desktop app down with it.
const shellOutputCap = 1 << 20 // 1 MiB

// cappedWriter keeps the first shellOutputCap bytes and drops the rest — the
// head is what the model needs and limitLines trims it further anyway.
// No mutex: os/exec reuses a single pipe and copy goroutine when Stdout and
// Stderr hold the same interface value, which is how Execute wires it.
type cappedWriter struct {
	buf     bytes.Buffer
	dropped bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	room := shellOutputCap - w.buf.Len()
	if room <= 0 {
		w.dropped = true
		return len(p), nil
	}
	if len(p) > room {
		w.buf.Write(p[:room])
		w.dropped = true
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
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

	cmd := proc.ShellCommand(ctx, execLine)
	cmd.Dir = workDir
	proc.HideConsole(cmd)
	proc.KillOnCancel(cmd)
	buffer := &cappedWriter{}
	cmd.Stdout = buffer
	cmd.Stderr = buffer

	err = cmd.Run()
	out := strings.TrimSpace(buffer.buf.String())
	truncatedOutput, truncated := limitLines(out, defaultToolOutputLineLimit)
	truncated = truncated || buffer.dropped
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
