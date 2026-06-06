package skill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type shellSkill struct{}

func (*shellSkill) Name() string { return "shell" }

func (*shellSkill) Description() string {
	return "run a local shell command"
}

func (*shellSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) == 0 {
		return Output{Name: "shell"}, errors.New("usage: shell <command>")
	}
	commandLine := strings.Join(args, " ")

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", commandLine)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", commandLine)
	}
	buffer := &bytes.Buffer{}
	cmd.Stdout = buffer
	cmd.Stderr = buffer

	err := cmd.Run()
	out := strings.TrimSpace(buffer.String())
	if out == "" && err == nil {
		out = "(no output)"
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return Output{Name: "shell"}, ctx.Err()
		}
		if out == "" {
			out = "(command failed)"
		}
		out = fmt.Sprintf("%s\nerror: %v", out, err)
	}

	return Output{
		Name:    "shell",
		Content: out,
	}, nil
}

