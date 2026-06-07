package skill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type gitSkill struct {
	root string
}

func (*gitSkill) Name() string { return "git" }

func (*gitSkill) Description() string {
	return "run safe git read-only commands (status, log, branch, diff, show)"
}

func (s *gitSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("git skill unavailable")
		return newToolOutput("git", "git", "", start, false, err), err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := exec.LookPath("git"); err != nil {
		err = errors.New("git command not found in PATH")
		return newToolOutput("git", "git", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: git <status|log|branch|diff|show> [args]")
		return newToolOutput("git", "git", "", start, false, err), err
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	if _, ok := allowedGitReadActions[action]; !ok {
		err := fmt.Errorf("unsupported git action: %s", action)
		return newToolOutput("git", "git "+strings.Join(args, " "), "", start, false, err), err
	}

	actionArgs := args[1:]
	if err := validateGitReadArgs(action, actionArgs); err != nil {
		return newToolOutput("git", "git "+strings.Join(args, " "), "", start, false, err), err
	}

	root, err := resolveSafeWorkspace(s.root)
	if err != nil {
		return newToolOutput("git", "git "+strings.Join(args, " "), "", start, false, err), err
	}

	if err := ensureGitRepo(ctx, root); err != nil {
		return newToolOutput("git", "git "+strings.Join(args, " "), "", start, false, err), err
	}

	command := append([]string{action}, actionArgs...)
	output, err := executeCommand(ctx, "git", root, command...)
	output = strings.TrimSpace(output)
	if output == "" {
		output = "(no output)"
	}
	output, truncated := limitLines(output, defaultToolOutputLineLimit)
	commandText := "git " + strings.Join(args, " ")
	result := newToolOutput("git", commandText, output, start, truncated, err)

	if err != nil {
		return result, err
	}

	return result, nil
}

var allowedGitReadActions = map[string]struct{}{
	"status": {},
	"log":    {},
	"branch": {},
	"diff":   {},
	"show":   {},
}

func validateGitReadArgs(action string, args []string) error {
	for _, rawArg := range args {
		arg := strings.TrimSpace(rawArg)
		if arg == "" {
			continue
		}
		lower := strings.ToLower(arg)

		switch {
		case lower == "--git-dir" || lower == "--work-tree" || strings.HasPrefix(lower, "--git-dir=") || strings.HasPrefix(lower, "--work-tree="):
			return fmt.Errorf("unsafe git option blocked: %s", arg)
		case lower == "-c" || strings.HasPrefix(lower, "-c"):
			return fmt.Errorf("unsafe git option blocked: %s", arg)
		case lower == "-C" || strings.HasPrefix(lower, "-C"):
			return fmt.Errorf("unsafe git option blocked: %s", arg)
		}

		if action != "status" && strings.Contains(lower, "/../") {
			return fmt.Errorf("unsupported path argument blocked: %s", arg)
		}
	}
	return nil
}

func resolveSafeWorkspace(root string) (string, error) {
	workspace := strings.TrimSpace(root)
	if workspace == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("cannot detect current directory: %w", err)
		}
		workspace = cwd
	}

	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("invalid workspace path: %w", err)
	}

	stats, err := os.Stat(absWorkspace)
	if err != nil {
		return "", fmt.Errorf("workspace not available: %w", err)
	}
	if !stats.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", absWorkspace)
	}

	return absWorkspace, nil
}

func ensureGitRepo(ctx context.Context, workspace string) error {
	output, err := executeCommand(ctx, "git", workspace, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return errors.New(strings.TrimSpace(output))
	}
	if strings.TrimSpace(output) != "true" {
		return errors.New("not inside a git repository")
	}
	return nil
}

func executeCommand(ctx context.Context, name, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	buffer := &bytes.Buffer{}
	cmd.Stdout = buffer
	cmd.Stderr = buffer
	err := cmd.Run()
	out := strings.TrimSpace(buffer.String())
	if err != nil {
		if out == "" {
			out = "(command failed)"
		}
		return out, err
	}
	return out, nil
}
