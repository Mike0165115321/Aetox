package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"aetox-cli/internal/contracts"
)

type ShellRunTool struct {
	sandboxRoot string
	maxBytes    int
}

func NewShellRunTool(root string) *ShellRunTool {
	return &ShellRunTool{
		sandboxRoot: root,
		maxBytes:    20_000,
	}
}

func (s *ShellRunTool) Name() string {
	return "shell"
}

func (s *ShellRunTool) Description() string {
	return "Runs shell commands inside sandbox directory"
}

func (s *ShellRunTool) Actions() []string {
	return []string{"run"}
}

func (s *ShellRunTool) Risk(action string, _ map[string]any) contracts.RiskLevel {
	if action == "run" {
		return contracts.RiskHigh
	}
	return contracts.RiskHigh
}

func (s *ShellRunTool) Execute(ctx context.Context, action string, params map[string]any) (contracts.StepResult, error) {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "run":
		return s.execRun(ctx, params)
	default:
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      "unsupported action: " + action,
			Artifacts:  map[string]string{},
			Confidence: 0.0,
		}, fmt.Errorf("unsupported action: %s", action)
	}
}

func (s *ShellRunTool) execRun(ctx context.Context, params map[string]any) (contracts.StepResult, error) {
	command := strings.TrimSpace(strParam(params, "command", ""))
	if command == "" {
		command = strings.TrimSpace(strParam(params, "cmd", ""))
	}
	if command == "" {
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      "missing command",
			Confidence: 0.0,
		}, fmt.Errorf("missing command")
	}
	cwd := strings.TrimSpace(strParam(params, "cwd", s.sandboxRoot))
	if cwd == "" {
		cwd = s.sandboxRoot
	}
	targetDir, err := resolveSandboxPath(s.sandboxRoot, cwd)
	if err != nil {
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      err.Error(),
			Confidence: 0.0,
		}, err
	}

	shell, shellArg := shellCommand(command)
	cmd := exec.CommandContext(ctx, shell, shellArg...)
	cmd.Dir = targetDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	errText := stderr.String()

	if len(output) > s.maxBytes {
		output = output[:s.maxBytes] + "\n[truncated]"
	}
	if errText != "" && len(errText) > s.maxBytes {
		errText = errText[:s.maxBytes] + "\n[truncated]"
	}

	if err != nil {
		if output == "" {
			output = strings.TrimSpace(errText)
		}
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Output:     output,
			Error:      strings.TrimSpace(err.Error()),
			Artifacts:  map[string]string{"command": command, "cwd": targetDir},
			Confidence: 0.3,
		}, err
	}

	output = strings.TrimSpace(output)
	if output == "" {
		output = "(empty output)"
	}

	if errText != "" {
		output = strings.TrimSpace(output + "\n" + errText)
	}

	return contracts.StepResult{
		Status:     contracts.StatusSuccess,
		Output:     output,
		Artifacts:  map[string]string{"command": command, "cwd": targetDir},
		Confidence: 1.0,
		MemoryUpdates: map[string]any{
			"last_command": command,
		},
	}, nil
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}
