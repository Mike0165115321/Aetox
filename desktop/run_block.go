package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

// RunBlockResult is what the chat's Run button gets back: the same fields the
// agent's own receipt is built from, shaped for a UI that shows them under
// the code block.
type RunBlockResult struct {
	Output     string `json:"output"`
	Success    bool   `json:"success"`
	DurationMs int64  `json:"durationMs"`
}

// maxRunBlockOutput caps what one click sends back to the chat UI. The shell
// skill already truncates for the model; this is the belt for the UI side —
// a command that prints a build log must not freeze the webview rendering it.
const maxRunBlockOutput = 32 << 10

// RunChatCommand executes a fenced command block the user clicked Run on.
//
// The click IS the consent: the command is fully visible in the block the
// user is looking at, exactly as if they had retyped it into the Workbench
// terminal — so there is no approval dialog on top. What the click does NOT
// bypass is the machinery: the command runs through the same shell skill as
// the agent's own calls — sandbox working directory, background-shells
// registry, RTK rewrite, shell audit log — never through a bare exec.Cmd,
// so there is exactly one way a command runs in Aetox, with one set of
// rules, whether the model called it or the user clicked it.
func (a *App) RunChatCommand(command string) (RunBlockResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return RunBlockResult{}, fmt.Errorf("empty command")
	}
	if a.registry == nil {
		return RunBlockResult{}, fmt.Errorf("engine is not ready yet")
	}
	s, ok := a.registry.Get("shell")
	if !ok {
		return RunBlockResult{}, fmt.Errorf("shell tool is not available")
	}
	tool, ok := s.(skill.Tool)
	if !ok {
		return RunBlockResult{}, fmt.Errorf("shell tool is not invokable")
	}

	// The shell skill's own timeout parameter governs the child process; this
	// context is the outer guard so a wedged call can never hang the click
	// forever. 90s > the skill's 60s default, so the skill's nicer "still
	// running" report wins over a blunt context cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	out, err := tool.ExecuteTool(ctx, map[string]any{"command": command})
	output := out.Content
	if len(output) > maxRunBlockOutput {
		output = output[:maxRunBlockOutput] + "\n… (truncated)"
	}
	// A failed command is a RESULT the user wants to read, not a Go error:
	// the exit status and stderr are the answer to "why didn't it work".
	// Only a failure to run at all (engine not ready, tool missing) errors.
	if err != nil && output == "" {
		output = err.Error()
	}
	return RunBlockResult{
		Output:     output,
		Success:    out.Success,
		DurationMs: out.DurationMs,
	}, nil
}
