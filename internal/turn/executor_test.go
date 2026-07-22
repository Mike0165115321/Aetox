package turn

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

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
		result, err := executor.Execute(context.Background(), input, intent, nil, nil)
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
	result, err := executor.Execute(context.Background(), input, intent, nil, nil)
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
	}, nil)
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
	if _, err := executor.Execute(context.Background(), input, intent, nil, nil); err != nil {
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
	if _, err := executor.Execute(context.Background(), input, intent, nil, nil); err != nil {
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
	result, err := executor.Execute(context.Background(), "git status", intent, nil, nil)
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
	_, err := executor.Execute(context.Background(), "list directory internal", intent, nil, nil)
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

func (a *toolAwareAgent) RespondStream(_ context.Context, _ string, _ func(string) error, _ TurnOptions) (string, bool, error) {
	return a.summaryReply, false, nil
}

func (a *toolAwareAgent) RespondWithTools(
	_ context.Context,
	_ []model.ToolDefinition,
	_ string,
	_ func(context.Context, model.ToolCall) (string, error),
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

// writeToolCallAgent models a tool-capable model that decides on its own to
// call `write` — the only remaining route from natural language to a tool.
type writeToolCallAgent struct{}

func (a *writeToolCallAgent) Respond(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "done", nil
}

func (a *writeToolCallAgent) RespondStream(_ context.Context, _ string, _ func(string) error, _ TurnOptions) (string, bool, error) {
	return "done", false, nil
}

func (a *writeToolCallAgent) RespondWithTools(
	ctx context.Context,
	_ []model.ToolDefinition,
	_ string,
	exec func(context.Context, model.ToolCall) (string, error),
	_ TurnOptions,
) (string, bool, error) {
	_, err := exec(ctx, model.ToolCall{
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

func (a *successfulToolCallAgent) RespondStream(_ context.Context, _ string, _ func(string) error, _ TurnOptions) (string, bool, error) {
	return "done", false, nil
}

func (a *successfulToolCallAgent) RespondWithTools(
	ctx context.Context,
	_ []model.ToolDefinition,
	_ string,
	exec func(context.Context, model.ToolCall) (string, error),
	_ TurnOptions,
) (string, bool, error) {
	_, err := exec(ctx, model.ToolCall{
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
