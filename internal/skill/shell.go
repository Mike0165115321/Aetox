package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/audit"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/proc"
	"github.com/Mike0165115321/Aetox/internal/rtk"
)

type shellSkill struct {
	root string
	// shells is shared with shell_output and shell_kill — one registry of
	// running commands, three tools onto it.
	shells *backgroundShells
}

func (*shellSkill) Name() string { return "shell" }

func (*shellSkill) Description() string {
	return "รันคำสั่ง shell ในโฟลเดอร์ sandbox root"
}

// ToolDefinition hands shell to the model.
//
// ADR 0001 deferred this: phase 1 exposed only `time` and `list`, and shell was
// to stay "available only through explicit user command paths" until narrower
// tools proved sufficient. They did — read/grep/glob/edit/apply_patch/
// diagnostics all shipped (§15, §21) — but the gate was never revisited, so the
// agent could edit code and never once run it. That is the whole Verify half of
// Explore→Read→Edit→Verify: no tests, no build, no linter, no package install.
// Two docs had already drifted into describing the world where the model has
// it (§22's amendment, §44's "shell stays"), and after the tools/skills split
// Settings lists shell on the Tools page — so the UI claimed a capability the
// model was never sent.
//
// Nothing here weakens the gate that actually guards this: internal/turn still
// runs safety.AssessCommand on every call, and shell is the one skill with its
// own high-risk branch (EffectExecuteShell, plus isShellHighRisk on the command
// word). Under the default `ask` mode every call is still approved by a human.
func (*shellSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command line to run, exactly as it would be typed in a terminal.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "What this command is for, in a few words and in the user's language, e.g. \"run the speech tests\". This is the line the user reads in the timeline, so write it for them, not for yourself.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "How long to wait before reporting back that the command is still running (it keeps running either way). Defaults to 60 and may not exceed 600. Raise it for a full test suite or a large build; leave it alone otherwise. Ignored when run_in_background is set.",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Start the command and return immediately with a handle instead of waiting. This is the only way to run something that does not exit on its own — a dev server, a watch build, a log tail. Read it with shell_output and end it with shell_kill.",
			},
		},
		"required":             []string{"command"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "shell",
			Description: "Run a command in the working folder — tests, builds, linters, package managers, anything the terminal can do. " +
				"Use it to verify your own work after editing. " +
				"Paths in the command follow the same rule the file tools do: they must be inside the folders this session may use, " +
				"and a command naming anything outside is refused before it runs. " +
				"A path the command assembles as it runs cannot be checked, so write paths out literally. " +
				"Prefer read/grep/glob/list for looking at files: they are faster and their output is shaped for you. " +
				"After 60 seconds (or timeout_seconds) it reports back as still running rather than being killed — call it again with the same command to look in on it. " +
				"A command that never exits on its own — a dev server, a watch mode, a REPL — must set run_in_background, or it will run out the clock and be killed.",
			Parameters: payload,
		},
	}
}

func (s *shellSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	command, _ := args["command"].(string)
	if strings.TrimSpace(command) == "" {
		err := errors.New("command is required")
		return newToolOutput("shell", "shell", "", time.Now(), false, err), err
	}
	if background, _ := args["run_in_background"].(bool); background {
		return s.startBackground(command)
	}
	// One element, not strings.Fields: Execute joins with " ", and splitting
	// here would collapse the runs of whitespace inside a quoted argument.
	return s.Execute(ctx, Input{"args": []string{command}})
}

// startBackground launches a command that is not expected to end and hands back
// the handle instead of the output.
//
// The audit entry is written here rather than on completion, because for a
// command that runs until the app closes there is no completion to write one
// at — and a shell command that never appears in the audit log is exactly the
// kind of gap the log exists to close.
func (s *shellSkill) startBackground(commandLine string) (Output, error) {
	start := time.Now()
	command := "shell " + commandLine
	if s.shells == nil {
		err := errors.New("background commands are not available in this build")
		return newToolOutput("shell", command, "", start, false, err), err
	}
	workDir, err := resolveSandboxPath(s.root, ".")
	if err != nil {
		return newToolOutput("shell", command, "", start, false, err), err
	}
	// The same gate the foreground path runs, for the same reason: a background
	// command is not a smaller command, and it is the one that keeps running
	// after the turn that started it has ended.
	if err := guardCommandPaths(s.root, commandLine); err != nil {
		return newToolOutput("shell", command, "", start, false, err), err
	}
	// Not rewritten through rtk: its whole purpose is compacting the output of
	// a command that finishes, and these do not.
	job, err := s.shells.start(workDir, commandLine)
	if err != nil {
		return newToolOutput("shell", command, "", start, false, err), err
	}
	_ = audit.WriteShell(audit.ShellEntry{
		Command: commandLine + "  (background)",
		WorkDir: workDir,
		Success: true,
	})
	content := fmt.Sprintf("started in the background as %s — read it with shell_output(%q), stop it with shell_kill(%q)",
		job.id, job.id, job.id)
	return newToolOutput("shell", command, content, start, false, nil), nil
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

	// Every path this command names has to be inside the workspace, checked
	// through the same resolveSandboxPath every file tool answers to
	// (shell_sandbox.go). Before this, shell was the one tool that could walk
	// out of a focused project, and it could do it without a prompt.
	if err := guardCommandPaths(s.root, commandLine); err != nil {
		return newToolOutput("shell", "shell "+commandLine, "", start, false, err), err
	}

	// Optional token-savings pass (ARCHITECTURE.md §13): if rtk has an
	// equivalent for this exact command, run that instead — same side effects
	// (rtk actually runs the real command), compacted output. Approval and
	// the audit log below still see the original commandLine; only what
	// actually executes changes.
	execLine := commandLine
	if rewritten, ok := rtk.Rewrite(ctx, commandLine); ok {
		execLine = rewritten
	}
	// The rewrite is what actually runs, so it is what actually has to be
	// contained. rtk maps a command to an equivalent of its own; checking only
	// the line the model wrote would trust that mapping to never introduce a
	// path, which is a promise no rewrite table should be asked to keep.
	if execLine != commandLine {
		if err := guardCommandPaths(s.root, execLine); err != nil {
			return newToolOutput("shell", "shell "+commandLine, "", start, false, err), err
		}
	}

	cmd := proc.ShellCommand(ctx, execLine)
	cmd.Dir = workDir
	proc.HideConsole(cmd)
	proc.KillOnCancel(cmd)
	buffer := &cappedWriter{}
	cmd.Stdout = buffer
	cmd.Stderr = buffer

	err = cmd.Run()
	// rtk advertises itself on every invocation; the model has no use for it.
	out := rtk.StripBanner(strings.TrimSpace(buffer.buf.String()))
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
