package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/audit"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/proc"
	"github.com/Mike0165115321/Aetox/internal/rtk"
)

type shellSkill struct {
	root string
	// shells is the registry of running commands. It used to be shared with two
	// sibling tools; now the three of them are three actions on this one
	// (packed.go), and the sharing is between this struct's own branches.
	shells *backgroundShells
	// actions this caller may use, nil for all of them. Set only by Narrow,
	// which is how a profile's `tools:` line reaches a tool that cannot see a
	// profile — internal/skill must never import the package that knows what an
	// agent is.
	actions []string
}

// shell resolves the backend once for one use. Once, deliberately: a turn that
// read the setting twice could guard a command line as bash and then run it in
// cmd, and the gap between those two reads is exactly where the containment
// check stops describing the thing that executes.
//
// Looked up by root rather than held in a field of its own. This type carried
// its own `backend func()` until §126.5 — set from the same option that records
// the workspace's shell, one call apart — and the answer being in two places is
// how the file tools came to believe a path meant something the shell did not.
// The lookup is still per use, so the picker still takes effect on the next
// command rather than the next restart.
func (s *shellSkill) shell() proc.Backend {
	if s == nil {
		return proc.Native()
	}
	return sandboxShellFor(s.root)
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
// allowedActions reports which of the three this caller may use.
//
// Narrow is the only thing that ever sets the field, so an unnarrowed tool —
// every desk session, every test, every host with no profiles at all — answers
// the full set through the same path rather than through a special case.
func (s *shellSkill) allowedActions() []string {
	p := packs["shell"]
	if s == nil || len(s.actions) == 0 {
		return append([]string(nil), p.actions...)
	}
	return s.actions
}

func (s *shellSkill) Actions() []string { return packs["shell"].permissions() }

// Narrow hands back a shell that offers only the named actions. A copy rather
// than a mutation: the parent registry is shared by every session in the
// process, and narrowing in place would let one agent's `tools:` line quietly
// take an action away from everyone else.
func (s *shellSkill) Narrow(named []string) Skill {
	narrowed := *s
	narrowed.actions = packs["shell"].narrow(named)
	return &narrowed
}

func (s *shellSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	// Written per action and assembled from the permitted set, so the block
	// never advertises something this caller would be refused — the same rule
	// the browser pack follows, and the reason a narrowed tool costs the model
	// nothing to read.
	lines := map[string]string{
		"run":    "`run` (command) — run it and wait. This is the default: a call with no action runs a command.",
		"output": "`output` (shell_id) — read what a background command has printed since you last read it, and whether it is still running. Output is consumed: each call returns only what is new. Prefer wait_for over polling in a loop.",
		"kill":   "`kill` (shell_id) — stop a background command and everything it started. Kill a dev server when you are done with it rather than leaving it holding a port.",
		"list":   "`list` — every background command still remembered, with its handle and state. The way back when a handle is no longer in your context.",
	}
	var actions strings.Builder
	for _, a := range allowed {
		actions.WriteString(lines[a] + "\n")
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        allowed,
				"description": "What to do. Omit it to run a command.",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "action=run: the command line, as it would be typed in a terminal.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "action=run: what this is for, in a few words, in the user's language. They read it in the timeline.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "action=run: default 60, ceiling 600.",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "action=run: return a handle immediately instead of waiting.",
			},
			"shell_id": map[string]any{
				"type":        "string",
				"description": "action=output/kill: the handle run returned.",
			},
			"filter": map[string]any{
				"type":        "string",
				"description": "action=output: keep only lines matching this regex.",
			},
			"wait_for": map[string]any{
				"type":        "string",
				"description": "action=output: block until new output matches this regex, or \"exit\" for the command finishing.",
			},
			"wait_timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "action=output: default 60, ceiling 600.",
			},
		},
		// `command` cannot be required here the way it was when this tool did
		// one thing — a flat schema cannot say "required unless the action is
		// output". It is checked in run() instead, which is where the refusal
		// can name the action that was missing it.
		"required":             []string{},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	// Resolved once for the whole description, per this type's own rule
	// (shell()): a Name from one read and a Note from another could describe
	// two different shells.
	backend := s.shell()
	syntax := "Commands are run by " + backend.Name() + " on this machine, so write them in that shell's syntax. "
	// The one constraint the name cannot carry — today, 5.1's missing `&&`.
	// A single fact is not the case list the comment below refuses: the model
	// knows the shell it is named, except where the name lies to its habits.
	if note := backend.Note(); note != "" {
		syntax += note + " "
	}
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "shell",
			// Naming the interpreter is not trivia. Without it a model writes
			// for the shell its training data is mostly made of, which is a
			// POSIX one — so on Windows the first command spent a round dying
			// on `ls -la` before the second tried `dir`. The name comes from
			// the backend, beside the invocation it describes, so the
			// description cannot end up naming a shell the tool does not run.
			//
			// Read here rather than baked in at build time, because the shell
			// is now the user's choice: a description still naming the native
			// shell after they switched the workspace to WSL would send every
			// command of the next turn into the wrong dialect, and the model
			// has no way to find out it was misled except by failing.
			//
			// The name and the backend's one-line Note, and nothing else: a
			// table of equivalents here would be a case list, and the model
			// already knows the shell it is told it is writing for.
			// Which shell, and nothing else, survives the split into signature and
			// guidance (guidance.go). It is not judgment: it is a fact about how to
			// write THIS call, it changes the moment the user points the workspace at
			// a distro, and a model told the wrong one writes the wrong dialect on
			// every command of the turn with no way to find out except by failing.
			// Guidance sent once could not correct a switch made after it.
			Description: "Run commands in the working folder — tests, builds, linters, package managers, anything the terminal can do. Actions:\n" +
				actions.String() + "\n" + syntax,
			Parameters: payload,
		},
	}
}

func (s *shellSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	p := packs["shell"]
	action := p.action(args)
	if action == "" {
		raw, _ := args["action"].(string)
		err := fmt.Errorf("unknown shell action %q — this session may use: %s",
			strings.TrimSpace(raw), strings.Join(s.allowedActions(), ", "))
		return newToolOutput("shell", "shell", "", time.Now(), false, err), err
	}
	// Refused here as well as left out of the enum, because a description is
	// guidance and a gate is a gate. A model that names an action it was never
	// offered has guessed, and a guess that runs is worse than one that is told
	// no.
	if !slices.Contains(s.allowedActions(), action) {
		err := fmt.Errorf("shell %s is not available here — this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
		return newToolOutput("shell", "shell", "", time.Now(), false, err), err
	}

	switch action {
	case "output":
		return (&shellOutputSkill{shells: s.shells}).ExecuteTool(ctx, args)
	case "kill":
		return (&shellKillSkill{shells: s.shells}).ExecuteTool(ctx, args)
	case "list":
		start := time.Now()
		if s.shells == nil {
			return newToolOutput("shell", "shell list", "ไม่มีคำสั่งเบื้องหลังในบิลด์นี้", start, false, nil), nil
		}
		lines := s.shells.snapshot()
		if len(lines) == 0 {
			return newToolOutput("shell", "shell list", "ไม่มีคำสั่งเบื้องหลังที่กำลังรันหรือค้างอยู่", start, false, nil), nil
		}
		return newToolOutput("shell", "shell list", strings.Join(lines, "\n"), start, false, nil), nil
	}

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
	backend := s.shell()
	// The same gate the foreground path runs, for the same reason: a background
	// command is not a smaller command, and it is the one that keeps running
	// after the turn that started it has ended.
	if err := guardCommandPaths(s.root, commandLine, gateFor(backend)); err != nil {
		return newToolOutput("shell", command, "", start, false, err), err
	}
	// Not rewritten through rtk: its whole purpose is compacting the output of
	// a command that finishes, and these do not.
	job, err := s.shells.start(backend, workDir, commandLine)
	if err != nil {
		return newToolOutput("shell", command, "", start, false, err), err
	}
	_ = audit.WriteShell(audit.ShellEntry{
		Command: commandLine + "  (background)",
		WorkDir: workDir,
		Success: true,
	})
	content := fmt.Sprintf("started in the background as %s — read it with action=output shell_id=%q, stop it with action=kill shell_id=%q",
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

	// Resolved once and used for the guard, the rewrite decision and the
	// invocation alike, so those three cannot disagree about which shell this
	// command belongs to.
	backend := s.shell()
	gate := gateFor(backend)

	// Every path this command names has to be inside the workspace, checked
	// through the same resolveSandboxPath every file tool answers to
	// (shell_sandbox.go). Before this, shell was the one tool that could walk
	// out of a focused project, and it could do it without a prompt.
	if err := guardCommandPaths(s.root, commandLine, gate); err != nil {
		return newToolOutput("shell", "shell "+commandLine, "", start, false, err), err
	}

	// Optional token-savings pass (ARCHITECTURE.md §13): if rtk has an
	// equivalent for this exact command, run that instead — same side effects
	// (rtk actually runs the real command), compacted output. Approval and
	// the audit log below still see the original commandLine; only what
	// actually executes changes.
	execLine := commandLine
	// Only for the shell rtk was resolved against. rtk is a Windows binary this
	// process found on the Windows PATH (internal/rtk/install.go), and the
	// rewrite it hands back is an invocation of that binary — sent into a WSL
	// distro it names a program that is not there, so every rewritten command
	// would fail on a path the user never wrote and cannot see. Skipping it
	// costs tokens, which is the whole point of rtk; running it costs the
	// command.
	if proc.IsNative(backend) {
		if rewritten, ok := rtk.Rewrite(ctx, commandLine); ok {
			execLine = rewritten
		}
	}
	// The rewrite is what actually runs, so it is what actually has to be
	// contained. rtk maps a command to an equivalent of its own; checking only
	// the line the model wrote would trust that mapping to never introduce a
	// path, which is a promise no rewrite table should be asked to keep.
	if execLine != commandLine {
		if err := guardCommandPaths(s.root, execLine, gate); err != nil {
			return newToolOutput("shell", "shell "+commandLine, "", start, false, err), err
		}
	}

	cmd := backend.Command(ctx, execLine, workDir)
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
		// A failure says where it failed, and that is not decoration.
		//
		// `wsl: command not found` is a true sentence about one program, and on
		// 2026-08-17 an agent read it as a true sentence about the machine: it
		// told the user there was no WSL here and no way to reach their D:
		// drive, minutes after reading files in that distro. Nothing in the
		// answer said which shell had spoken, so the largest possible reading of
		// the absence was also the only one available.
		//
		// It costs one clause on the path that already failed, and it is the
		// same rule searchBaseExists holds for a search: an absence must be
		// reported at the size it actually is.
		result.Stderr = err.Error() + " (in " + backend.Name() + ")"
		return result, err
	}
	return result, nil
}
