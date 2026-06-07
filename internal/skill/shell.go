package skill

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"
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

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", commandLine)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", commandLine)
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
