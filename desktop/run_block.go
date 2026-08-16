package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/runlang"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// RunBlockResult is what the chat's Run button gets back: the same fields the
// agent's own receipt is built from, shaped for a UI that shows them under
// the code block.
type RunBlockResult struct {
	Output     string `json:"output"`
	Success    bool   `json:"success"`
	DurationMs int64  `json:"durationMs"`
	// Lines is how many lines Output has. Counted here rather than in the
	// panel because the panel would be splitting a 32KB string to draw one
	// number, on every redraw of a message that is redrawn while the next
	// answer streams in underneath it.
	Lines int `json:"lines"`
	// Truncated says what is on screen is not all of it — either the shell
	// skill clipped the output for the model, or maxRunBlockOutput clipped it
	// for the webview. Without it the panel scrolls to a bottom that is not the
	// bottom and says nothing about the difference, which is the one way a
	// result can lie (owner, 16 ส.ค.).
	Truncated bool `json:"truncated"`
}

// maxRunBlockOutput caps what one click sends back to the chat UI. The shell
// skill already truncates for the model; this is the belt for the UI side —
// a command that prints a build log must not freeze the webview rendering it.
const maxRunBlockOutput = 32 << 10

// RunnableLanguages tells the chat which fence tags may carry a Run button on
// THIS machine, as tag → kind ("shell" or "script").
//
// The chat asks once, at startup, instead of holding its own list. That is the
// whole shape of the fix: the renderer used to carry a hardcoded set that had
// to be kept in step with the engine's table by hand, and neither of them ever
// asked whether the interpreter existed — so a machine with no Python was
// offered a Run button that came back with a Microsoft Store advert. A button
// that cannot do anything is never drawn now, and adding a language is one
// entry in internal/runlang rather than an edit in two languages.
//
// A map rather than a struct because that is the shape the question has: the
// renderer looks a tag up per fence, and every fence it draws asks.
func (a *App) RunnableLanguages() map[string]string {
	offered := runlang.Runnable()
	out := make(map[string]string, len(offered))
	for tag, kind := range offered {
		out[tag] = string(kind)
	}
	return out
}

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
	return a.runThroughShell(command)
}

// runThroughShell is the one door in the note above. Every Run button in the
// chat ends here, and here is the only place in this file that touches the
// shell skill — so a second kind of runnable block cannot quietly acquire a
// second set of rules.
func (a *App) runThroughShell(command string) (RunBlockResult, error) {
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
	// Truncated before the clip is the skill's own verdict — it clips for the
	// model long before this cap is reached — so both are asked, not just the
	// one that happens to fire here.
	truncated := out.Truncated
	if len(output) > maxRunBlockOutput {
		output = output[:maxRunBlockOutput] + "\n… (truncated)"
		truncated = true
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
		Lines:      countLines(output),
		Truncated:  truncated,
	}, nil
}

// countLines counts what the panel would show, which is not the same as
// counting '\n': a command whose output has no trailing newline still printed
// its last line, and one that printed nothing printed no lines at all.
func countLines(output string) int {
	trimmed := strings.TrimRight(output, "\n")
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "\n") + 1
}
