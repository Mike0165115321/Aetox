package turn

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// ARCHITECTURE.md §17 regression: natural-language text must go to the model,
// never to a keyword-guessed tool. This exact phrasing used to be hijacked by
// the deleted regex layer into a direct `write` before the model saw it.
func TestExecute_ConversationTextNeverTriggersToolsDirectly(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{root: root, t: t}
	agent := &toolAwareAgent{supportsTools: false, summaryReply: "model reply"}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	for _, input := range []string{
		"create file example.md with content test content",
		"คุณทำอะไรได้อีก เอาเนื้อหาในเว็บมา ทำเป็นไฟล์ html ให้ผมได้ไหม",
	} {
		intent := command.Parse(input, command.ParseTokens, nil)
		result, err := executor.Execute(context.Background(), input, intent, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Reply != "model reply" {
			t.Fatalf("expected the model's own reply, got %q", result.Reply)
		}
		if dispatcher.toolExecutions != 0 {
			t.Fatalf("expected no direct tool execution for NL input %q, got %d", input, dispatcher.toolExecutions)
		}
	}
}

// §17: when a tool-capable model chooses to answer in plain text, that answer
// is final — nothing re-guesses it into a tool afterward.
func TestExecute_ModelTextAnswerIsFinalForToolCapableAgent(t *testing.T) {
	dispatcher := &toolDispatcher{root: t.TempDir(), t: t}
	agent := &toolAwareAgent{supportsTools: true, withToolsReply: "just an answer", withToolsUsed: false}
	executor := NewExecutor(ExecutorOptions{Agent: agent, Dispatcher: dispatcher})

	input := "create file example.md with content test content"
	intent := command.Parse(input, command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), input, intent, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reply != "just an answer" {
		t.Fatalf("expected the model's text answer to be final, got %q", result.Reply)
	}
	if dispatcher.toolExecutions != 0 {
		t.Fatalf("expected no tool execution after a plain text answer, got %d", dispatcher.toolExecutions)
	}
}

// The desktop streams replies via onChunk — a tool-loop turn must deliver its
// final text through the same callback (see desktop/app.go SendMessage).
func TestExecute_ToolLoopReplyReachesOnChunk(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("fixture failed: %v", err)
	}
	dispatcher := &toolDispatcher{root: root, t: t}
	executor := NewExecutor(ExecutorOptions{
		Agent:        &successfulToolCallAgent{},
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess,
	})

	var chunks []string
	input := "อ่านโฟลเดอร์ internal ให้หน่อย"
	intent := command.Parse(input, command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), input, intent, func(chunk string) {
		chunks = append(chunks, chunk)
	}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reply != "ok done via model tool" {
		t.Fatalf("unexpected reply: %q", result.Reply)
	}
	if len(chunks) != 1 || chunks[0] != result.Reply {
		t.Fatalf("expected the reply delivered once via onChunk, got %#v", chunks)
	}
	if dispatcher.toolExecutions != 1 {
		t.Fatalf("expected exactly one model-driven tool execution, got %d", dispatcher.toolExecutions)
	}
}

func TestExecute_PermissionDenyBlocksWithoutPrompting(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{root: root, t: t}
	agent := &writeToolCallAgent{}
	executor := NewExecutor(ExecutorOptions{
		Agent:        agent,
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess, // would otherwise never prompt
		Permissions: safety.PermissionConfig{Rules: []safety.PermissionRule{
			{Tool: "write", Action: safety.PermissionDeny},
		}},
		// No Approve func: if the deny rule failed to short-circuit, the
		// nil-safe approveOrDeny would auto-approve and the file would be
		// written, so this test can only pass via the permission gate.
	})

	input := "please write example.md for me"
	intent := command.Parse(input, command.ParseTokens, nil)
	if _, err := executor.Execute(context.Background(), input, intent, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dispatcher.toolExecutions != 0 {
		t.Fatalf("expected dispatcher.ExecuteTool to never run under a deny rule, got %d calls", dispatcher.toolExecutions)
	}
	if _, statErr := os.Stat(filepath.Join(root, "example.md")); statErr == nil {
		t.Fatalf("expected file to NOT be created under a deny permission rule")
	}
}

func TestExecute_PermissionAskOverridesFullAccess(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{root: root, t: t}
	agent := &writeToolCallAgent{}
	promptCalls := 0
	executor := NewExecutor(ExecutorOptions{
		Agent:        agent,
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess, // would otherwise never prompt
		Permissions: safety.PermissionConfig{Rules: []safety.PermissionRule{
			{Tool: "write", Action: safety.PermissionAsk},
		}},
		Approve: func(context.Context, string, string) (bool, error) {
			promptCalls++
			return false, nil
		},
	})

	input := "please write example.md for me"
	intent := command.Parse(input, command.ParseTokens, nil)
	if _, err := executor.Execute(context.Background(), input, intent, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promptCalls != 1 {
		t.Fatalf("expected the ask rule to force exactly one prompt under full-access, got %d", promptCalls)
	}
	if dispatcher.toolExecutions != 0 {
		t.Fatalf("expected dispatcher.ExecuteTool to never run after a denied prompt, got %d calls", dispatcher.toolExecutions)
	}
}

func TestExecute_PermissionDenyBlocksExplicitSkillCommandWithoutPrompting(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{root: root, t: t}
	agent := &toolAwareAgent{supportsTools: false, summaryReply: "n/a"}
	commandSet := command.BuildCommandSet([]string{"git"})
	executor := NewExecutor(ExecutorOptions{
		Agent:        agent,
		Dispatcher:   dispatcher,
		CommandSet:   commandSet,
		ApprovalMode: safety.ApprovalFullAccess, // would otherwise never prompt
		Permissions: safety.PermissionConfig{Rules: []safety.PermissionRule{
			{Tool: "git", Action: safety.PermissionDeny},
		}},
	})

	intent := command.Parse("git status", command.ParseTokens, commandSet)
	if intent.Kind != command.KindSkill {
		t.Fatalf("fixture invalid: expected KindSkill intent, got %v", intent.Kind)
	}
	result, err := executor.Execute(context.Background(), "git status", intent, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != TurnStatusBlocked {
		t.Fatalf("expected status blocked, got %s (reply: %q)", result.Status, result.Reply)
	}
}

func TestExecute_PermissionDenyBlocksModelDrivenToolCallWithoutExecutingDispatcher(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("fixture failed: %v", err)
	}
	dispatcher := &toolDispatcher{root: root, t: t}
	agent := &successfulToolCallAgent{}
	executor := NewExecutor(ExecutorOptions{
		Agent:        agent,
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess, // would otherwise never prompt
		Permissions: safety.PermissionConfig{Rules: []safety.PermissionRule{
			{Tool: "list", Action: safety.PermissionDeny},
		}},
	})

	intent := command.Parse("list directory internal", command.ParseTokens, nil)
	_, err := executor.Execute(context.Background(), "list directory internal", intent, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dispatcher.toolExecutions != 0 {
		t.Fatalf("expected dispatcher.ExecuteTool to never run under a deny rule, got %d calls", dispatcher.toolExecutions)
	}
}

type toolAwareAgent struct {
	supportsTools  bool
	summaryReply   string
	withToolsReply string
	withToolsUsed  bool
}

func (a *toolAwareAgent) Respond(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return a.summaryReply, nil
}

func (a *toolAwareAgent) RespondEphemeral(ctx context.Context, prompt string, opts TurnOptions) (string, error) {
	return a.Respond(ctx, prompt, opts)
}

func (a *toolAwareAgent) RespondStream(_ context.Context, _ string, _ func(string) error, _ func(string) error, _ TurnOptions) (string, bool, error) {
	return a.summaryReply, false, nil
}

func (a *toolAwareAgent) RespondWithTools(
	_ context.Context,
	_ []model.ToolDefinition,
	_ string,
	_ func(context.Context, model.ToolCall) (string, []model.Image, error),
	_ func(string) error,
	_ TurnOptions,
) (string, bool, error) {
	if a.withToolsReply == "" {
		a.withToolsReply = "ok"
	}
	return a.withToolsReply, a.withToolsUsed, nil
}

func (a *toolAwareAgent) SupportsToolCalling() bool {
	return a.supportsTools
}

// Truncated tool-call JSON (max_tokens cut it mid-content) must fail loudly
// with a message that names truncation — and must never reach the dispatcher.
// The old salvage path here mangled the path into ":" and doom-looped the model.
func TestExecuteToolCallWithOutcome_TruncatedArgsFailLoudly(t *testing.T) {
	dispatcher := &toolDispatcher{root: t.TempDir(), t: t}
	executor := NewExecutor(ExecutorOptions{
		Agent:        &toolAwareAgent{supportsTools: true},
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess,
	})

	_, _, success, err := executor.executeToolCallWithOutcome(context.Background(), model.ToolCall{
		ID:   "call_trunc",
		Type: "function",
		Function: model.FunctionCall{
			Name:      "write",
			Arguments: `{"path": "landing.html", "content": "<!DOCTYPE html>\n<html lang=\"th`,
		},
	})
	if err == nil {
		t.Fatal("expected an error for truncated JSON arguments")
	}
	if success {
		t.Fatal("truncated call must not be reported as success")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error must explain truncation so the model can adapt, got %q", err.Error())
	}
	if dispatcher.toolExecutions != 0 {
		t.Fatalf("dispatcher must not run on unparseable args, ran %d times", dispatcher.toolExecutions)
	}
}

// slowDispatcher hangs in ExecuteTool until released, ignoring ctx — like the
// FS-walking skills do.
type slowDispatcher struct {
	toolDispatcher
	release chan struct{}
	runs    atomic.Int64
}

func (d *slowDispatcher) ExecuteTool(_ context.Context, _ string, _ map[string]any) (skill.Output, bool, error) {
	d.runs.Add(1)
	<-d.release
	return skill.Output{Content: "the real answer", Success: true}, true, nil
}

// A tool over its deadline is parked, not abandoned: the model is told to call
// it again with the same arguments, and that second call collects the result
// instead of starting the work over. The bug this pins cost a 90-second
// transcription twice — abandoned at 60s, restarted from zero, still no answer.
func TestExecuteTool_SlowToolIsParkedAndTheSameCallCollectsIt(t *testing.T) {
	saved := toolExecutionTimeout
	toolExecutionTimeout = 50 * time.Millisecond
	defer func() { toolExecutionTimeout = saved }()

	dispatcher := &slowDispatcher{toolDispatcher: toolDispatcher{root: t.TempDir(), t: t}, release: make(chan struct{})}
	executor := NewExecutor(ExecutorOptions{
		Agent:        &toolAwareAgent{supportsTools: true},
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess,
	})

	output, handled, err := executor.executeTool(context.Background(), "grep", map[string]any{"pattern": "x"})
	if !handled || err != nil {
		t.Fatalf("an overrunning tool is a status, not a failure: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(output.Content, "STILL RUNNING") {
		t.Fatalf("model must be told the call is still running, got %q", output.Content)
	}
	if !output.Success {
		t.Error("still-running is not a failed row in the user's timeline")
	}

	close(dispatcher.release) // the parked call finishes while the model is elsewhere

	toolExecutionTimeout = 2 * time.Second // the check-up collects rather than timing out again
	output, handled, err = executor.executeTool(context.Background(), "grep", map[string]any{"pattern": "x"})
	if !handled || err != nil {
		t.Fatalf("collecting a finished call: handled=%v err=%v", handled, err)
	}
	if output.Content != "the real answer" {
		t.Fatalf("check-up must hand back the real result, got %q", output.Content)
	}
	if runs := dispatcher.runs.Load(); runs != 1 {
		t.Fatalf("the check-up must not run the work again: %d runs", runs)
	}
}

// The Settings dropdown has to reach the turn that is already running: what
// makes anyone switch to full access is a prompt sitting on screen right now.
func TestExecuteTool_ApprovalModeChangeReachesARunningTurn(t *testing.T) {
	asked := 0
	executor := NewExecutor(ExecutorOptions{
		Agent:        &toolAwareAgent{supportsTools: true},
		Dispatcher:   &toolDispatcher{root: t.TempDir(), t: t},
		ApprovalMode: safety.ApprovalAsk,
		Approve: func(context.Context, string, string) (bool, error) {
			asked++
			return true, nil
		},
	})

	if _, _, err := executor.executeTool(context.Background(), "write", map[string]any{"path": "a.txt", "content": "hi"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if asked != 1 {
		t.Fatalf("ask mode must prompt: %d prompts", asked)
	}

	executor.SetApprovalMode(safety.ApprovalFullAccess)
	if _, _, err := executor.executeTool(context.Background(), "write", map[string]any{"path": "b.txt", "content": "hi"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if asked != 1 {
		t.Fatalf("full access must stop the prompts at once, got %d", asked)
	}
}

func TestToolCallDeadline(t *testing.T) {
	saved := toolExecutionTimeout
	toolExecutionTimeout = 60 * time.Second
	defer func() { toolExecutionTimeout = saved }()

	cases := []struct {
		name string
		tool string
		args map[string]any
		want time.Duration
	}{
		{"no argument keeps the default", "shell", map[string]any{"command": "go build ./..."}, 60 * time.Second},
		{"a test suite may ask for longer", "shell", map[string]any{"timeout_seconds": float64(300)}, 300 * time.Second},
		{"an int survives a non-JSON caller", "shell", map[string]any{"timeout_seconds": 300}, 300 * time.Second},
		{"shorter is honored too", "shell", map[string]any{"timeout_seconds": float64(5)}, 5 * time.Second},
		{"the ceiling holds", "shell", map[string]any{"timeout_seconds": float64(99999)}, maxToolExecutionTimeout},
		{"nonsense falls back rather than failing the call", "shell", map[string]any{"timeout_seconds": "soon"}, 60 * time.Second},
		{"zero is not a deadline", "shell", map[string]any{"timeout_seconds": float64(0)}, 60 * time.Second},
		{"negative is not a deadline", "shell", map[string]any{"timeout_seconds": float64(-9)}, 60 * time.Second},
		// Only shell knows how long its own work takes; nothing else gets to
		// extend the guard by naming a field.
		{"another tool cannot buy time", "grep", map[string]any{"timeout_seconds": float64(600)}, 60 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolCallDeadline(tc.tool, tc.args); got != tc.want {
				t.Errorf("toolCallDeadline(%q, %v) = %s, want %s", tc.tool, tc.args, got, tc.want)
			}
		})
	}
}

// The deadline is read where it is enforced, not just where it is computed.
func TestExecuteTool_ShellTimeoutOutlivesTheDefault(t *testing.T) {
	saved := toolExecutionTimeout
	toolExecutionTimeout = 50 * time.Millisecond
	defer func() { toolExecutionTimeout = saved }()

	dispatcher := &slowDispatcher{toolDispatcher: toolDispatcher{root: t.TempDir(), t: t}, release: make(chan struct{})}
	executor := NewExecutor(ExecutorOptions{
		Agent:        &toolAwareAgent{supportsTools: true},
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess,
	})

	// Finishes long after the 50ms default, well inside the second it asked for.
	go func() {
		time.Sleep(200 * time.Millisecond)
		close(dispatcher.release)
	}()
	output, handled, err := executor.executeTool(context.Background(), "shell",
		map[string]any{"command": "go test ./...", "timeout_seconds": float64(1)})
	if !handled {
		t.Fatal("shell must be handled")
	}
	if err != nil {
		t.Fatalf("a command inside its own timeout must not be abandoned: %v", err)
	}
	if !output.Success {
		t.Error("Success = false, want the command's own result")
	}
}

// Interactive tools (ask_user) wait on a human — the slow-tool guard must let
// them run past the timeout instead of abandoning them.
func TestExecuteTool_InteractiveToolExemptFromTimeout(t *testing.T) {
	saved := toolExecutionTimeout
	toolExecutionTimeout = 50 * time.Millisecond
	defer func() { toolExecutionTimeout = saved }()

	dispatcher := &slowDispatcher{toolDispatcher: toolDispatcher{root: t.TempDir(), t: t}, release: make(chan struct{})}
	executor := NewExecutor(ExecutorOptions{
		Agent:        &toolAwareAgent{supportsTools: true},
		Dispatcher:   dispatcher,
		ApprovalMode: safety.ApprovalFullAccess,
	})

	// Release ("the user answers") well after the timeout would have fired.
	go func() {
		time.Sleep(200 * time.Millisecond)
		close(dispatcher.release)
	}()
	output, handled, err := executor.executeTool(context.Background(), "ask_user", map[string]any{"question": "q"})
	if !handled {
		t.Fatal("interactive tool must be handled")
	}
	if err != nil {
		t.Fatalf("interactive tool must not be timed out: %v", err)
	}
	if !output.Success {
		t.Fatal("expected the released tool result, not an abandonment")
	}
}

// writeToolCallAgent models a tool-capable model that decides on its own to
// call `write` — the only remaining route from natural language to a tool.
type writeToolCallAgent struct{}

func (a *writeToolCallAgent) Respond(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "done", nil
}

func (a *writeToolCallAgent) RespondEphemeral(ctx context.Context, prompt string, opts TurnOptions) (string, error) {
	return a.Respond(ctx, prompt, opts)
}

func (a *writeToolCallAgent) RespondStream(_ context.Context, _ string, _ func(string) error, _ func(string) error, _ TurnOptions) (string, bool, error) {
	return "done", false, nil
}

func (a *writeToolCallAgent) RespondWithTools(
	ctx context.Context,
	_ []model.ToolDefinition,
	_ string,
	exec func(context.Context, model.ToolCall) (string, []model.Image, error),
	_ func(string) error,
	_ TurnOptions,
) (string, bool, error) {
	_, _, err := exec(ctx, model.ToolCall{
		ID:   "write_call_1",
		Type: "function",
		Function: model.FunctionCall{
			Name:      "write",
			Arguments: `{"path":"example.md","content":"test content"}`,
		},
	})
	if err != nil {
		return "", true, err
	}
	return "wrote the file", true, nil
}

func (a *writeToolCallAgent) SupportsToolCalling() bool {
	return true
}

type successfulToolCallAgent struct{}

func (a *successfulToolCallAgent) Respond(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "done", nil
}

func (a *successfulToolCallAgent) RespondEphemeral(ctx context.Context, prompt string, opts TurnOptions) (string, error) {
	return a.Respond(ctx, prompt, opts)
}

func (a *successfulToolCallAgent) RespondStream(_ context.Context, _ string, _ func(string) error, _ func(string) error, _ TurnOptions) (string, bool, error) {
	return "done", false, nil
}

func (a *successfulToolCallAgent) RespondWithTools(
	ctx context.Context,
	_ []model.ToolDefinition,
	_ string,
	exec func(context.Context, model.ToolCall) (string, []model.Image, error),
	_ func(string) error,
	_ TurnOptions,
) (string, bool, error) {
	_, _, err := exec(ctx, model.ToolCall{
		ID:   "good_call_1",
		Type: "function",
		Function: model.FunctionCall{
			Name:      "list",
			Arguments: `{"path":"internal"}`,
		},
	})
	if err != nil {
		return "", true, err
	}
	return "ok done via model tool", true, nil
}

func (a *successfulToolCallAgent) SupportsToolCalling() bool {
	return true
}

type toolDispatcher struct {
	root string
	t    *testing.T
	// toolExecutions counts ExecuteTool invocations; used by tests that verify fallback behavior.
	toolExecutions int
	// lastTool tracks the most recent tool invocation.
	lastTool string
	// lastArgs tracks the most recent tool arguments.
	lastArgs map[string]string
	// executionHistory tracks tool invocation order.
	executionHistory []string
}

func (d *toolDispatcher) Reset() {
	d.toolExecutions = 0
	d.lastTool = ""
	d.lastArgs = nil
	d.executionHistory = nil
}

func (d *toolDispatcher) Execute(_ context.Context, _ string) (skill.Output, bool, error) {
	return skill.Output{}, false, nil
}

func (d *toolDispatcher) ToolDefinitions() []model.ToolDefinition {
	return []model.ToolDefinition{
		{
			Type: "function",
			Function: model.ToolFunction{
				Name:       "time",
				Parameters: []byte(`{"type":"object"}`),
			},
		},
		{
			Type: "function",
			Function: model.ToolFunction{
				Name:       "write",
				Parameters: []byte(`{"type":"object"}`),
			},
		},
		{
			Type: "function",
			Function: model.ToolFunction{
				Name:       "list",
				Parameters: []byte(`{"type":"object"}`),
			},
		},
		{
			Type: "function",
			Function: model.ToolFunction{
				Name:       "read",
				Parameters: []byte(`{"type":"object"}`),
			},
		},
		{
			Type: "function",
			Function: model.ToolFunction{
				Name:       "delete",
				Parameters: []byte(`{"type":"object"}`),
			},
		},
		{
			Type: "function",
			Function: model.ToolFunction{
				Name:       "github_repo_summary",
				Parameters: []byte(`{"type":"object"}`),
			},
		},
		{
			Type: "function",
			Function: model.ToolFunction{
				Name:       "plugin_install",
				Parameters: []byte(`{"type":"object"}`),
			},
		},
	}
}

func (d *toolDispatcher) ExecuteTool(_ context.Context, name string, args map[string]any) (skill.Output, bool, error) {
	d.executionHistory = append(d.executionHistory, name)
	d.toolExecutions++
	tool := strings.ToLower(strings.TrimSpace(name))
	d.lastTool = tool
	d.lastArgs = map[string]string{}
	for key, value := range args {
		if value == nil {
			continue
		}
		d.lastArgs[key] = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(key, ""), " "))
		if valueStr, ok := value.(string); ok {
			d.lastArgs[key] = strings.TrimSpace(valueStr)
		}
	}

	switch tool {
	case "write":
		path := ""
		content := ""
		if rawPath, ok := args["path"].(string); ok {
			path = strings.TrimSpace(rawPath)
		}
		if contentValue, ok := args["content"].(string); ok {
			content = contentValue
		}
		if path == "" {
			if d.t != nil {
				d.t.Fatalf("expected path argument")
			}
			return skill.Output{}, true, nil
		}
		target := filepath.Join(d.root, path)
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return skill.Output{}, true, err
		}
		return skill.Output{
			Name:      "write",
			Command:   "write " + path,
			Content:   "written",
			Success:   true,
			RawOutput: "written",
		}, true, nil
	case "list":
		path := "."
		if rawPath, ok := args["path"].(string); ok {
			path = strings.TrimSpace(rawPath)
			if path == "" {
				path = "."
			}
		}
		target := filepath.Join(d.root, path)
		entries, err := os.ReadDir(target)
		if err != nil {
			return skill.Output{
				Name:      "list",
				Command:   "list " + path,
				Success:   false,
				Stderr:    err.Error(),
				RawOutput: "",
			}, true, err
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		return skill.Output{
			Name:       "list",
			Command:    "list " + path,
			Content:    strings.Join(names, "\n"),
			RawOutput:  strings.Join(names, "\n"),
			Success:    true,
			DurationMs: 0,
		}, true, nil
	case "read":
		path := ""
		if rawPath, ok := args["path"].(string); ok {
			path = strings.TrimSpace(rawPath)
		}
		if path == "" {
			if d.t != nil {
				d.t.Fatalf("expected path argument")
			}
			return skill.Output{}, true, nil
		}
		raw, err := os.ReadFile(filepath.Join(d.root, path))
		if err != nil {
			return skill.Output{
				Name:    "read",
				Command: "read " + path,
				Success: false,
				Stderr:  err.Error(),
			}, true, err
		}
		return skill.Output{
			Name:      "read",
			Command:   "read " + path,
			Content:   string(raw),
			RawOutput: string(raw),
			Success:   true,
		}, true, nil
	case "delete":
		path := ""
		if rawPath, ok := args["path"].(string); ok {
			path = strings.TrimSpace(rawPath)
		}
		if path == "" {
			if d.t != nil {
				d.t.Fatalf("expected path argument")
			}
			return skill.Output{}, true, nil
		}
		if err := os.Remove(filepath.Join(d.root, path)); err != nil {
			return skill.Output{
				Name:    "delete",
				Command: "delete " + path,
				Success: false,
				Stderr:  err.Error(),
			}, true, err
		}
		return skill.Output{
			Name:      "delete",
			Command:   "delete " + path,
			Content:   "deleted",
			RawOutput: "deleted",
			Success:   true,
		}, true, nil
	case "time":
		return skill.Output{
			Name:      "time",
			Command:   "time",
			Content:   "12:34:56",
			RawOutput: "12:34:56",
			Success:   true,
		}, true, nil
	case "github_repo_summary":
		repoURL := ""
		if rawURL, ok := args["repo_url"].(string); ok {
			repoURL = strings.TrimSpace(rawURL)
		}
		return skill.Output{
			Name:      "github_repo_summary",
			Command:   "github_repo_summary " + repoURL,
			Content:   "GitHub repo: " + repoURL,
			RawOutput: "GitHub repo: " + repoURL,
			Success:   true,
		}, true, nil
	case "plugin_install":
		repoURL := ""
		if rawURL, ok := args["repo_url"].(string); ok {
			repoURL = strings.TrimSpace(rawURL)
		}
		return skill.Output{
			Name:      "plugin_install",
			Command:   "plugin_install " + repoURL,
			Content:   "plugin install unsupported: " + repoURL,
			RawOutput: "plugin install unsupported: " + repoURL,
			Success:   true,
		}, true, nil
	default:
		return skill.Output{}, false, nil
	}
}

func TestToolCallToArgs(t *testing.T) {
	cases := []struct {
		name string
		tool string
		args map[string]any
		want []string
	}{
		{"write path+content", "write", map[string]any{"path": "a.txt", "content": "x"}, []string{"a.txt", "x"}},
		{"read path", "read", map[string]any{"path": "a.txt"}, []string{"a.txt"}},
		{"delete path", "delete", map[string]any{"path": "a.txt"}, []string{"a.txt"}},
		{"edit path only, match strings excluded", "edit", map[string]any{"path": "a.txt", "old_string": "old", "new_string": "new"}, []string{"a.txt"}},
		{"grep pattern+path", "grep", map[string]any{"pattern": "needle", "path": "sub"}, []string{"needle", "sub"}},
		{"grep pattern only", "grep", map[string]any{"pattern": "needle"}, []string{"needle"}},
		{"grep empty args", "grep", map[string]any{}, []string{}},
		{"unknown tool", "mystery", map[string]any{"path": "a.txt"}, nil},
	}
	for _, tc := range cases {
		got := toolCallToArgs(tc.tool, tc.args)
		if len(got) != len(tc.want) {
			t.Errorf("%s: toolCallToArgs(%q) = %v, want %v", tc.name, tc.tool, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: toolCallToArgs(%q)[%d] = %q, want %q", tc.name, tc.tool, i, got[i], tc.want[i])
			}
		}
	}
}

// The permission rules a user writes (e.g. {Tool:"edit", Pattern:"docs/*"})
// only work if toolCallToArgs feeds the path into Resolve — this pins the
// whole chain for the two new coding-loop tools.
func TestPermissionRulesMatchEditAndGrepArgs(t *testing.T) {
	cfg := safety.PermissionConfig{Rules: []safety.PermissionRule{
		{Tool: "edit", Pattern: "docs/*", Action: safety.PermissionAllow},
		{Tool: "grep", Pattern: "*", Action: safety.PermissionAllow},
	}}

	editArgs := toolCallToArgs("edit", map[string]any{"path": "docs/x.md", "old_string": "a", "new_string": "b"})
	if action, matched := cfg.Resolve("edit", editArgs); !matched || action != safety.PermissionAllow {
		t.Errorf("edit docs/x.md: Resolve = (%q, %v), want (allow, true)", action, matched)
	}
	editArgs = toolCallToArgs("edit", map[string]any{"path": "src/x.go", "old_string": "a", "new_string": "b"})
	if _, matched := cfg.Resolve("edit", editArgs); matched {
		t.Error("edit src/x.go should not match docs/* rule")
	}

	grepArgs := toolCallToArgs("grep", map[string]any{"pattern": "needle"})
	if action, matched := cfg.Resolve("grep", grepArgs); !matched || action != safety.PermissionAllow {
		t.Errorf("grep: Resolve = (%q, %v), want (allow, true)", action, matched)
	}
}

// narratingAgent scripts one round of narration + a tool call + a final reply,
// streaming a little reasoning first — the shape §59's timeline interleaving
// consumes.
// delegatingAgent hands one job to the doc chair and finishes — the smallest
// turn that opens a delegation.
type delegatingAgent struct{}

func (*delegatingAgent) Respond(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "", nil
}

func (*delegatingAgent) RespondEphemeral(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "", nil
}

func (*delegatingAgent) RespondStream(_ context.Context, _ string, _ func(string) error, _ func(string) error, _ TurnOptions) (string, bool, error) {
	return "", false, nil
}

func (a *delegatingAgent) RespondWithTools(
	ctx context.Context,
	_ []model.ToolDefinition,
	_ string,
	exec func(context.Context, model.ToolCall) (string, []model.Image, error),
	_ func(string) error,
	_ TurnOptions,
) (string, bool, error) {
	_, _, _ = exec(ctx, model.ToolCall{
		ID:   "task_call_1",
		Type: "function",
		Function: model.FunctionCall{
			Name:      "task",
			Arguments: `{"description":"ทำรายงานสรุป","prompt":"เขียนรายงานสรุปการประชุมทีมเป็น .docx","agent":"doc"}`,
		},
	})
	return "ส่งงานให้ doc แล้วครับ", true, nil
}

func (*delegatingAgent) SupportsToolCalling() bool { return true }

// taskDispatcher answers only `task`, successfully — the delegation mechanics
// themselves are internal/subagent's tests, not this one's.
type taskDispatcher struct{}

func (*taskDispatcher) Execute(_ context.Context, _ string) (skill.Output, bool, error) {
	return skill.Output{}, false, nil
}

func (*taskDispatcher) ToolDefinitions() []model.ToolDefinition {
	return []model.ToolDefinition{{
		Type:     "function",
		Function: model.ToolFunction{Name: "task", Parameters: []byte(`{"type":"object"}`)},
	}}
}

func (*taskDispatcher) ExecuteTool(_ context.Context, name string, _ map[string]any) (skill.Output, bool, error) {
	return skill.Output{Name: name, Content: "started", Success: true}, true, nil
}

// A `task` call's live event and its written-down part both say which pile the
// worker is in — the kind the injected DelegateKind resolves, never a guess
// from the name. The UI counts ตัวแทน and ซับเอเจน apart on both paths: the
// chip on a live turn reads the event, and a reopened session reads only the
// parts, where a row that forgot its kind would fall back into the helper pile.
func TestExecute_DelegationCarriesAgentKind(t *testing.T) {
	var events []ToolEvent
	executor := NewExecutor(ExecutorOptions{
		Agent:        &delegatingAgent{},
		Dispatcher:   &taskDispatcher{},
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: func(ev ToolEvent) { events = append(events, ev) },
		DelegateKind: func(agent string) string {
			if agent == "doc" {
				return "agent"
			}
			return "helper"
		},
	})

	input := "ทำรายงานสรุปการประชุมให้หน่อย"
	intent := command.Parse(input, command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), input, intent, nil, func(string) {}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var call *ToolEvent
	for i := range events {
		if events[i].Action == "call" {
			call = &events[i]
		}
	}
	if call == nil {
		t.Fatal("no call event reached the timeline")
	}
	if call.Agent != "doc" || call.AgentKind != "agent" {
		t.Errorf("call event Agent=%q AgentKind=%q, want doc/agent", call.Agent, call.AgentKind)
	}
	if call.Brief == "" {
		t.Error("call event lost the brief")
	}

	var part *ToolPart
	for _, p := range result.Parts {
		if p.Kind == PartTool && p.Tool != nil {
			part = p.Tool
		}
	}
	if part == nil {
		t.Fatal("no tool part was written down")
	}
	if part.Agent != "doc" || part.AgentKind != "agent" {
		t.Errorf("stored part Agent=%q AgentKind=%q, want doc/agent — a reopened session would miscount the pile", part.Agent, part.AgentKind)
	}
	if part.Brief == "" {
		t.Error("stored part lost the brief")
	}
}

type narratingAgent struct{}

func (*narratingAgent) Respond(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "", nil
}

func (*narratingAgent) RespondEphemeral(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "", nil
}

func (*narratingAgent) RespondStream(_ context.Context, _ string, _ func(string) error, _ func(string) error, _ TurnOptions) (string, bool, error) {
	return "", false, nil
}

func (a *narratingAgent) RespondWithTools(
	ctx context.Context,
	_ []model.ToolDefinition,
	_ string,
	exec func(context.Context, model.ToolCall) (string, []model.Image, error),
	onReasoning func(string) error,
	opts TurnOptions,
) (string, bool, error) {
	if onReasoning != nil {
		_ = onReasoning("hmm, the folder first")
	}
	if opts.OnRound != nil {
		opts.OnRound(RoundEvent{Text: "กำลังอ่านโฟลเดอร์ก่อน"})
	}
	_, _, _ = exec(ctx, model.ToolCall{
		ID:   "narrate_call_1",
		Type: "function",
		Function: model.FunctionCall{
			Name:      "list",
			Arguments: `{"path":"internal"}`,
		},
	})
	if opts.OnRound != nil {
		opts.OnRound(RoundEvent{Text: "เสร็จแล้ว", Final: true})
	}
	return "เสร็จแล้ว", true, nil
}

func (*narratingAgent) SupportsToolCalling() bool { return true }

// §59: a round's thinking and the narration the model wrote alongside its tool
// calls reach the timeline as events of their own, in the order they happened —
// thinking, then the note, then the calls it announced. The final reply must
// NOT also arrive as a note: the reply bubble already shows it.
func TestExecute_NarrationAndThinkingReachTheTimeline(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("fixture failed: %v", err)
	}
	var events []ToolEvent
	executor := NewExecutor(ExecutorOptions{
		Agent:        &narratingAgent{},
		Dispatcher:   &toolDispatcher{root: root, t: t},
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: func(ev ToolEvent) { events = append(events, ev) },
	})

	input := "อ่านโฟลเดอร์ internal ให้หน่อย"
	intent := command.Parse(input, command.ParseTokens, nil)
	if _, err := executor.Execute(context.Background(), input, intent, nil, func(string) {}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var actions []string
	for _, ev := range events {
		actions = append(actions, ev.Action)
	}
	want := []string{"thinking", "note", "call", "result"}
	if len(actions) != len(want) {
		t.Fatalf("event actions = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("event actions = %v, want %v", actions, want)
		}
	}
	if events[0].Secs < 1 {
		t.Errorf("thinking Secs = %d, want at least 1", events[0].Secs)
	}
	if events[1].Text != "กำลังอ่านโฟลเดอร์ก่อน" {
		t.Errorf("note Text = %q, want the round's narration", events[1].Text)
	}
	for _, ev := range events {
		if ev.Action == "note" && ev.Text == "เสร็จแล้ว" {
			t.Error("the final reply must not double as a note")
		}
	}
}

// Full access says "รับทุกอย่างโดยไม่ถาม" on the card the user clicks, and it
// has to be true. It was not: bootstrap attaches an "ask" rule per MCP server
// so tools never auto-run before anyone has decided anything, a matched rule
// skipped the mode check entirely, and every single MCP call opened a dialog
// under the mode whose whole point is not opening dialogs (2026-08-06).
//
// The distinction is who wrote the rule. The app's opening position yields to
// the mode the user then chose; a rule the user wrote themselves does not.
func TestAnAppDefaultAskYieldsToFullAccessButAUserAskDoesNot(t *testing.T) {
	assessment := safety.Assessment{SkillName: "notion_search", Risk: safety.RiskLow}

	appDefault := &Executor{
		permissions: safety.PermissionConfig{Rules: []safety.PermissionRule{
			{Tool: "notion_*", Pattern: "*", Action: safety.PermissionAsk, Default: true},
		}},
		approve: func(context.Context, string, string) (bool, error) {
			t.Error("full access prompted on an app-generated ask")
			return false, nil
		},
	}
	appDefault.SetApprovalMode(safety.ApprovalFullAccess)
	if ok, err := appDefault.resolveApproval(context.Background(), "notion_search", nil, "notion_search", assessment); err != nil || !ok {
		t.Fatalf("full access refused an MCP call: ok=%v err=%v", ok, err)
	}

	// The user's own rule is their decision and still wins.
	asked := false
	userWritten := &Executor{
		permissions: safety.PermissionConfig{Rules: []safety.PermissionRule{
			{Tool: "notion_*", Pattern: "*", Action: safety.PermissionAsk},
		}},
		approve: func(context.Context, string, string) (bool, error) { asked = true; return true, nil },
	}
	userWritten.SetApprovalMode(safety.ApprovalFullAccess)
	if _, err := userWritten.resolveApproval(context.Background(), "notion_search", nil, "notion_search", assessment); err != nil {
		t.Fatalf("resolveApproval: %v", err)
	}
	if !asked {
		t.Error("a rule the user wrote was skipped — writing it is how they asked to be asked")
	}

	// A default ask still gates the modes that gate: it is only outranked, not
	// discarded.
	gated := false
	underAsk := &Executor{
		permissions: safety.PermissionConfig{Rules: []safety.PermissionRule{
			{Tool: "notion_*", Pattern: "*", Action: safety.PermissionAsk, Default: true},
		}},
		approve: func(context.Context, string, string) (bool, error) { gated = true; return true, nil },
	}
	underAsk.SetApprovalMode(safety.ApprovalAsk)
	if _, err := underAsk.resolveApproval(context.Background(), "notion_search", nil, "notion_search",
		safety.Assessment{SkillName: "notion_search", Effects: []safety.Effect{safety.EffectUseNetwork}}); err != nil {
		t.Fatalf("resolveApproval: %v", err)
	}
	if !gated {
		t.Error("ask mode stopped prompting for MCP entirely")
	}
}
