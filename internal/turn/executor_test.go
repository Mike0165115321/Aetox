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

func TestExecute_InferredWriteForNonToolAgent(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &toolAwareAgent{
		supportsTools: false,
		summaryReply:  "executed (done). file written",
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	intent := command.Parse("create file example.md with content test content", command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), "create file example.md with content test content", intent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != TurnStatusDone {
		t.Fatalf("expected status done, got %s", result.Status)
	}

	target := filepath.Join(root, "example.md")
	raw, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("expected file to be created at %s: %v", target, readErr)
	}
	if strings.TrimSpace(string(raw)) != "test content" {
		t.Fatalf("unexpected file content: %q", string(raw))
	}
}

func TestExecute_PermissionDenyBlocksWithoutPrompting(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{root: root, t: t}
	agent := &toolAwareAgent{supportsTools: false, summaryReply: "blocked"}
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

	intent := command.Parse("create file example.md with content test content", command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), "create file example.md with content test content", intent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != TurnStatusBlocked {
		t.Fatalf("expected status blocked, got %s", result.Status)
	}
	if _, statErr := os.Stat(filepath.Join(root, "example.md")); statErr == nil {
		t.Fatalf("expected file to NOT be created under a deny permission rule")
	}
}

func TestExecute_PermissionAskOverridesFullAccess(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{root: root, t: t}
	agent := &toolAwareAgent{supportsTools: false, summaryReply: "blocked"}
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

	intent := command.Parse("create file example.md with content test content", command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), "create file example.md with content test content", intent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promptCalls != 1 {
		t.Fatalf("expected the ask rule to force exactly one prompt under full-access, got %d", promptCalls)
	}
	if result.Status != TurnStatusBlocked {
		t.Fatalf("expected status blocked, got %s", result.Status)
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

func TestExecute_FallsBackToInferredToolWhenToolCapableAgentSkipsTools(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &toolAwareAgent{
		supportsTools: true,
		summaryReply:  "executed (done). file written by fallback",
		// Simulate a chat-style response with no tool calls.
		withToolsReply: "ok done",
		withToolsUsed:  false,
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	intent := command.Parse("create file fallback.md with content from model", command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), "create file fallback.md with content from model", intent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != TurnStatusDone {
		t.Fatalf("expected status done, got %s", result.Status)
	}

	target := filepath.Join(root, "fallback.md")
	raw, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("expected fallback tool execution to create file, got %v", readErr)
	}
	if !strings.Contains(string(raw), "from model") {
		t.Fatalf("unexpected file content: %q", string(raw))
	}
}

func TestExecute_DoesNotFallbackToInferredToolWhenAgentToolSucceeds(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("fixture failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "note.md"), []byte("from model tool"), 0o644); err != nil {
		t.Fatalf("fixture failed: %v", err)
	}
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &successfulToolCallAgent{}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	intent := command.Parse("list directory internal", command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), "list directory internal", intent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != TurnStatusDone {
		t.Fatalf("expected status done, got %s", result.Status)
	}

	if dispatcher.toolExecutions != 1 {
		t.Fatalf("expected only model tool execution, got %d", dispatcher.toolExecutions)
	}
	if dispatcher.lastTool != "list" {
		t.Fatalf("expected list tool, got %q", dispatcher.lastTool)
	}
}

func TestExecute_FallsBackWhenAgentToolCallFails(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("fixture failed: %v", err)
	}
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &failedToolCallAgent{
		summaryReply: "executed (done). file written by fallback",
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	input := "list directory internal"
	intent := command.Parse(input, command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), input, intent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != TurnStatusDone {
		t.Fatalf("expected status done, got %s", result.Status)
	}
	if dispatcher.toolExecutions != 2 {
		t.Fatalf("expected failed model tool plus fallback read, got %d", dispatcher.toolExecutions)
	}
	if dispatcher.lastTool != "list" {
		t.Fatalf("expected fallback list tool, got %q", dispatcher.lastTool)
	}
}

func TestInferToolCandidates_TimeAndList(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	candidates := executor.inferToolCandidates("time and list directory internal")
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Name != "time" || candidates[1].Name != "list" {
		t.Fatalf("unexpected candidate order: %#v", candidates)
	}
	if got := candidates[1].Args["path"]; got != "internal" {
		t.Fatalf("expected list path internal, got %v", got)
	}
}

func TestInferToolCandidates_WriteWithTimeInFilename(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	candidates := executor.inferToolCandidates("create file safe_after_time.txt with content alpha")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "write" {
		t.Fatalf("expected write candidate, got %s", candidates[0].Name)
	}
	if candidates[0].MissingMessage != "" {
		t.Fatalf("unexpected missing message: %s", candidates[0].MissingMessage)
	}
	if path, ok := candidates[0].Args["content"]; !ok || path != "alpha" {
		t.Fatalf("unexpected content: %#v", path)
	}
}

func TestInferToolCandidates_WriteWithContentWordInFilename(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	candidates := executor.inferToolCandidates("create file safe_missingcontent.txt with content hello")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "write" {
		t.Fatalf("expected write candidate, got %s", candidates[0].Name)
	}
	content, ok := candidates[0].Args["content"].(string)
	if !ok || content != "hello" {
		t.Fatalf("unexpected content: %#v", candidates[0].Args["content"])
	}
}

func TestInferToolCandidates_WriteWithContentWordInFilenameWithoutContentBlocks(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	candidates := executor.inferToolCandidates("create file content_in_filename_no_content.txt")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "write" {
		t.Fatalf("expected write candidate, got %s", candidates[0].Name)
	}
	if strings.TrimSpace(candidates[0].MissingMessage) == "" {
		t.Fatalf("expected missing content message, got %#v", candidates[0])
	}
	if _, ok := candidates[0].Args["content"]; ok {
		t.Fatalf("filename content substring should not become content: %#v", candidates[0].Args)
	}
}

func TestInferToolCandidates_WriteWithTimeInContent(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	candidates := executor.inferToolCandidates("create file safe_after_time.txt with content alpha and time")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "write" {
		t.Fatalf("expected write candidate, got %s", candidates[0].Name)
	}
	content, ok := candidates[0].Args["content"].(string)
	if !ok || content != "alpha and time" {
		t.Fatalf("unexpected content: %#v", candidates[0].Args["content"])
	}
}

func TestInferToolCandidates_GitHubURLDefaultsToSummary(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	candidates := executor.inferToolCandidates("https://github.com/openai/codex")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "github_repo_summary" {
		t.Fatalf("expected github_repo_summary candidate, got %s", candidates[0].Name)
	}
	if got := candidates[0].Args["repo_url"]; got != "https://github.com/openai/codex" {
		t.Fatalf("unexpected repo_url: %#v", got)
	}
}

func TestInferToolCandidates_GitHubInstallAndSummary(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	candidates := executor.inferToolCandidates("install plugin from https://github.com/openai/codex and summarize it")
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Name != "plugin_install" || candidates[1].Name != "github_repo_summary" {
		t.Fatalf("unexpected candidate order: %#v", candidates)
	}
}

func TestInferToolCandidates_PluginInstallRequiresRepoURL(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	candidates := executor.inferToolCandidates("install plugin for me")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "plugin_install" {
		t.Fatalf("expected plugin_install candidate, got %s", candidates[0].Name)
	}
	if strings.TrimSpace(candidates[0].MissingMessage) == "" {
		t.Fatalf("expected missing message, got %#v", candidates[0])
	}
}

func TestExecute_InferredThaiGeneratedWriteCreatesFile(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &toolAwareAgent{
		supportsTools: false,
		summaryReply:  "done",
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	input := "write a self introduction script and create txt file for me"
	intent := command.Parse(input, command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), input, intent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != TurnStatusDone {
		t.Fatalf("expected done, got %s", result.Status)
	}
	target := filepath.Join(root, "self_introduction.txt")
	raw, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("expected generated file to be created: %v", readErr)
	}
	if !strings.Contains(string(raw), "Aetox") {
		t.Fatalf("expected generated content to mention Aetox, got %q", string(raw))
	}
}

func TestExecute_InferredThaiSelfIntroductionDefaultsToIntroMarkdown(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &toolAwareAgent{
		supportsTools: false,
		summaryReply:  "done",
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	input := "สร้างไฟล์แนะนำตัวเองหน่อย"
	intent := command.Parse(input, command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), input, intent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != TurnStatusDone {
		t.Fatalf("expected done, got %s", result.Status)
	}
	target := filepath.Join(root, "intro.md")
	raw, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("expected generated file to be created: %v", readErr)
	}
	if !strings.Contains(string(raw), "# สวัสดีครับ ผม Aetox") {
		t.Fatalf("expected markdown intro content, got %q", string(raw))
	}
	if dispatcher.lastTool != "write" {
		t.Fatalf("expected write tool call, got %q", dispatcher.lastTool)
	}
}

func TestExecute_InferredReadAndDelete(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "delete-me.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o644); err != nil {
		t.Fatalf("fixture failed: %v", err)
	}
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &toolAwareAgent{
		supportsTools: false,
		summaryReply:  "done",
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	readInput := "read file delete-me.txt"
	readIntent := command.Parse(readInput, command.ParseTokens, nil)
	readResult, err := executor.Execute(context.Background(), readInput, readIntent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if readResult.Status != TurnStatusDone {
		t.Fatalf("expected read done, got %s", readResult.Status)
	}
	if dispatcher.lastTool != "read" {
		t.Fatalf("expected read tool, got %q", dispatcher.lastTool)
	}

	deleteInput := "delete file delete-me.txt"
	deleteIntent := command.Parse(deleteInput, command.ParseTokens, nil)
	deleteResult, err := executor.Execute(context.Background(), deleteInput, deleteIntent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if deleteResult.Status != TurnStatusDone {
		t.Fatalf("expected delete done, got %s", deleteResult.Status)
	}
	if dispatcher.lastTool != "delete" {
		t.Fatalf("expected delete tool, got %q", dispatcher.lastTool)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, stat err=%v", err)
	}
}

func TestExecute_MixedIntents_TimeAndList(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "internal"), 0o755)
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &toolAwareAgent{
		supportsTools: false,
		summaryReply:  "done",
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	intent := command.Parse("time and list internal", command.ParseTokens, nil)
	result, err := executor.Execute(context.Background(), "time and list internal", intent, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != TurnStatusDone {
		t.Fatalf("expected status done, got %s", result.Status)
	}
	if len(dispatcher.executionHistory) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(dispatcher.executionHistory))
	}
	if dispatcher.executionHistory[0] != "time" || dispatcher.executionHistory[1] != "list" {
		t.Fatalf("expected execution order time->list, got %v", dispatcher.executionHistory)
	}
	if path, ok := dispatcher.lastArgs["path"]; !ok || strings.TrimSpace(strings.TrimSuffix(path, "/")) != "internal" {
		t.Fatalf("expected list path internal, got %v", dispatcher.lastArgs)
	}
}

func TestExecute_InferredListSentences(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("create fixture failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "note.md"), []byte("fixture"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &toolAwareAgent{
		supportsTools: false,
		summaryReply:  "done",
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	cases := []struct {
		name  string
		input string
	}{
		{name: "directory", input: "list directory internal"},
		{name: "path", input: "list path internal"},
		{name: "folder", input: "list folder internal"},
		{name: "mixed", input: "show me list internal files"},
		{name: "composite", input: "list internal and show logs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dispatcher.Reset()
			intent := command.Parse(tc.input, command.ParseTokens, nil)
			result, err := executor.Execute(context.Background(), tc.input, intent, nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Status != TurnStatusDone {
				t.Fatalf("expected done, got %s", result.Status)
			}
			if dispatcher.lastTool != "list" {
				t.Fatalf("expected list tool call, got %q", dispatcher.lastTool)
			}
			if path, ok := dispatcher.lastArgs["path"]; !ok || path != "internal" {
				t.Fatalf("expected list path internal, got %#v", dispatcher.lastArgs)
			}
		})
	}
}

func TestExecute_InferredWrite_RejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &toolAwareAgent{
		supportsTools: false,
		summaryReply:  "should not run",
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	tests := []string{
		`create file /tmp/x with content nope`,
		`create file ../x with content nope`,
		`create file C:\\temp\\x with content nope`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			dispatcher.Reset()
			intent := command.Parse(input, command.ParseTokens, nil)
			result, err := executor.Execute(context.Background(), input, intent, nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Status != TurnStatusBlocked && result.Status != TurnStatusError {
				t.Fatalf("expected blocked/error status, got %s", result.Status)
			}
			if !strings.Contains(strings.ToLower(result.Reply), "unsafe path") {
				t.Fatalf("expected unsafe path error, got %q", result.Reply)
			}
			if dispatcher.toolExecutions != 0 {
				t.Fatalf("unsafe path should not execute tools")
			}
		})
	}
}

func TestExecute_InferredWrite_QuotedPaths(t *testing.T) {
	root := t.TempDir()
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	agent := &toolAwareAgent{
		supportsTools: false,
		summaryReply:  "done",
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	cases := []string{
		`create file "quote one.md" with content first line`,
		`create file 'quote two.md' content: second line`,
		"create file `quote three.md` content=third line",
		"create file quoted.txt: plain split format",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			dispatcher.Reset()
			intent := command.Parse(input, command.ParseTokens, nil)
			_, err := executor.Execute(context.Background(), input, intent, nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if dispatcher.lastTool != "write" {
				t.Fatalf("expected write tool call, got %q", dispatcher.lastTool)
			}
		})
	}

	if _, err := os.Stat(filepath.Join(root, "quote one.md")); err != nil {
		t.Fatalf("expected quote one.md created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "quote two.md")); err != nil {
		t.Fatalf("expected quote two.md created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "quote three.md")); err != nil {
		t.Fatalf("expected quote three.md created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "quoted.txt")); err != nil {
		t.Fatalf("expected quoted.txt created: %v", err)
	}
}

func TestParseListCandidatePath_SupportedSentences(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	cases := map[string]string{
		"list directory internal":     "internal",
		"list path internal":          "internal",
		"list folder internal":        "internal",
		"show me list internal files": "internal",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			path, err := executor.parseListCandidatePath(input)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if path != want {
				t.Fatalf("expected %q, got %q", want, path)
			}
		})
	}
}

func TestInferToolCandidates_MixedIntentOrder(t *testing.T) {
	dispatcher := &toolDispatcher{
		root: t.TempDir(),
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Dispatcher: dispatcher,
	})

	cases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "semicolon separator",
			input:    "time;list internal",
			expected: []string{"time", "list"},
		},
		{
			name:     "then separator",
			input:    "time then list folder internal",
			expected: []string{"time", "list"},
		},
		{
			name:     "multiple separators",
			input:    "list directory internal and show logs and time",
			expected: []string{"list", "time"},
		},
		{
			name:     "inline mixed",
			input:    "time and list directory internal",
			expected: []string{"time", "list"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := executor.inferToolCandidates(tc.input)
			if len(got) != len(tc.expected) {
				t.Fatalf("expected %d candidates, got %d", len(tc.expected), len(got))
			}
			for i, expected := range tc.expected {
				if got[i].Name != expected {
					t.Fatalf("candidate[%d]: expected %q got %q", i, expected, got[i].Name)
				}
			}
		})
	}
}

func TestValidateInferredPath_SafetyMatrix(t *testing.T) {
	executor := NewExecutor(ExecutorOptions{})

	accept := []string{
		"file.txt",
		"a/b/c.txt",
		"nested/dir/file.md",
		"docs/readme",
	}
	for _, path := range accept {
		t.Run("accept_"+path, func(t *testing.T) {
			if err := executor.validateInferredPath(path); err != nil {
				t.Fatalf("expected accept for %q: got error %v", path, err)
			}
		})
	}

	reject := map[string]string{
		"/tmp/x":       "unsafe path",
		"../x":         "unsafe path",
		"..\\x":        "unsafe path",
		"C:\\tmp\\x":   "unsafe path",
		"tmp/../x":     "unsafe path",
		"tmp/..":       "unsafe path",
		"~/note.md":    "unsafe path",
		"note<>.md":    "unsafe path",
		"   ":          "unsafe path",
		"\x00evil.txt": "unsafe path",
		"file.txt/":    "unsafe path",
	}
	for path, want := range reject {
		t.Run("reject_"+strings.ReplaceAll(path, "\\", "/"), func(t *testing.T) {
			err := executor.validateInferredPath(path)
			if err == nil {
				t.Fatalf("expected reject for %q", path)
			}
			if !strings.Contains(strings.ToLower(err.Error()), want) {
				t.Fatalf("expected %q in error for %q, got %q", want, path, err.Error())
			}
		})
	}
}

func TestExecute_InferredIntent_SmokeMatrix(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "internal"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "internal", "seed.txt"), []byte("seed"), 0o644)
	agent := &toolAwareAgent{
		supportsTools: false,
	}
	dispatcher := &toolDispatcher{
		root: root,
		t:    t,
	}
	executor := NewExecutor(ExecutorOptions{
		Agent:      agent,
		Dispatcher: dispatcher,
	})

	cases := []struct {
		name             string
		input            string
		wantStatus       TurnStatus
		expectedSequence []string
		expectPath       string
		expectReply      string
	}{
		{name: "time only", input: "what is the time", wantStatus: TurnStatusDone, expectedSequence: []string{"time"}},
		{name: "time short", input: "time", wantStatus: TurnStatusDone, expectedSequence: []string{"time"}},
		{name: "time sentence", input: "please tell me time now", wantStatus: TurnStatusDone, expectedSequence: []string{"time"}},
		{name: "list root", input: "list", wantStatus: TurnStatusDone, expectedSequence: []string{"list"}},
		{name: "list internal", input: "list internal", wantStatus: TurnStatusDone, expectedSequence: []string{"list"}},
		{name: "list dir plain", input: "list directory internal", wantStatus: TurnStatusDone, expectedSequence: []string{"list"}},
		{name: "list folder plain", input: "list folder internal", wantStatus: TurnStatusDone, expectedSequence: []string{"list"}},
		{name: "list path phrase", input: "list path internal", wantStatus: TurnStatusDone, expectedSequence: []string{"list"}},
		{name: "list nested path", input: "list directory internal/a", wantStatus: TurnStatusError, expectedSequence: []string{"list"}},
		{name: "list files phrasing", input: "show me list internal files", wantStatus: TurnStatusDone, expectedSequence: []string{"list"}},
		{name: "list and logs phrase", input: "list internal and show logs", wantStatus: TurnStatusDone, expectedSequence: []string{"list"}},
		{name: "list semicolon phrase", input: "list internal; list folder internal", wantStatus: TurnStatusDone, expectedSequence: []string{"list", "list"}},
		{name: "list then phrase", input: "list folder internal then time", wantStatus: TurnStatusDone, expectedSequence: []string{"list", "time"}},
		{name: "time and list phrase", input: "time and list internal", wantStatus: TurnStatusDone, expectedSequence: []string{"time", "list"}},
		{name: "time then list phrase", input: "time then list directory internal", wantStatus: TurnStatusDone, expectedSequence: []string{"time", "list"}},
		{name: "time and list and write", input: "time and list directory internal and create file internal/t1.md with content hello", wantStatus: TurnStatusDone, expectedSequence: []string{"time", "list", "write"}, expectPath: "internal/t1.md"},
		{name: "write simple", input: "create file report.md with content one", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "report.md"},
		{name: "write quote", input: "create file \"report two.md\" with content one", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "report two.md"},
		{name: "write single-quote", input: "create file 'report three.md' content: one", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "report three.md"},
		{name: "write backtick", input: "create file `report four.md` content=one", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "report four.md"},
		{name: "write split", input: "create file report-five.md: one", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "report-five.md"},
		{name: "write nested", input: "create file internal/report-six.md with content one", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "internal/report-six.md"},
		{name: "write nested", input: "create file internal/report-seven.md with content line1 line2", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "internal/report-seven.md"},
		{name: "write with list keyword", input: "create file list-note.md with content list", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "list-note.md"},
		{name: "write with time in filename", input: "create file safe_after_time.txt with content alpha", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "safe_after_time.txt"},
		{name: "write with time in content", input: "create file safe_after_time.txt with content alpha and time", wantStatus: TurnStatusDone, expectedSequence: []string{"write"}, expectPath: "safe_after_time.txt"},
		{name: "mixed explicit phrase", input: "time and create file quote.md with content one", wantStatus: TurnStatusDone, expectedSequence: []string{"time", "write"}, expectPath: "quote.md"},
		{name: "mixed list then create", input: "list internal then create file internal/combined.md with content two", wantStatus: TurnStatusDone, expectedSequence: []string{"list", "write"}, expectPath: "internal/combined.md"},
		{name: "mixed create then time", input: "create file internal/combined2.md with content two then time", wantStatus: TurnStatusDone, expectedSequence: []string{"write", "time"}, expectPath: "internal/combined2.md"},
		{name: "blocked absolute", input: "create file /tmp/x with content no", wantStatus: TurnStatusBlocked, expectReply: "unsafe path"},
		{name: "blocked traversal", input: "create file ../x with content no", wantStatus: TurnStatusBlocked, expectReply: "unsafe path"},
		{name: "blocked windows", input: "create file C:\\tmp\\x with content no", wantStatus: TurnStatusBlocked, expectReply: "unsafe path"},
		{name: "blocked relative backtrack", input: "create file internal/../x with content no", wantStatus: TurnStatusBlocked, expectReply: "unsafe path"},
		{name: "blocked root segment", input: "create file tmp/.. with content no", wantStatus: TurnStatusBlocked, expectReply: "unsafe path"},
		{name: "blocked home", input: "create file ~/x with content no", wantStatus: TurnStatusBlocked, expectReply: "unsafe path"},
		{name: "blocked bad char", input: "create file note<>.md with content no", wantStatus: TurnStatusBlocked, expectReply: "unsafe path"},
		{name: "blocked empty path", input: "create file   with content no", wantStatus: TurnStatusBlocked, expectReply: "missing file path for write"},
		{name: "blocked missing content", input: "create file missing.md", wantStatus: TurnStatusBlocked, expectReply: "content required for write"},
		{name: "blocked missing name", input: "create file", wantStatus: TurnStatusBlocked},
		{name: "github summary", input: "https://github.com/openai/codex", wantStatus: TurnStatusDone, expectedSequence: []string{"github_repo_summary"}, expectReply: "GitHub repo"},
		{name: "github install and summary", input: "install plugin from https://github.com/openai/codex and summarize it", wantStatus: TurnStatusDone, expectedSequence: []string{"plugin_install", "github_repo_summary"}, expectReply: "plugin install unsupported"},
		{name: "plugin install missing repo", input: "install plugin for me", wantStatus: TurnStatusBlocked, expectReply: "GitHub repository URL"},
		{name: "conversation no intent", input: "hello this is just chat", wantStatus: TurnStatusDone},
		{name: "list then time plus logs", input: "list internal and time and show logs", wantStatus: TurnStatusDone, expectedSequence: []string{"list", "time"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dispatcher.Reset()
			intent := command.Parse(tc.input, command.ParseTokens, nil)
			result, err := executor.Execute(context.Background(), tc.input, intent, nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Status != tc.wantStatus {
				t.Fatalf("status: want %s got %s", tc.wantStatus, result.Status)
			}
			if tc.expectedSequence != nil {
				if strings.Join(dispatcher.executionHistory, ",") != strings.Join(tc.expectedSequence, ",") {
					t.Fatalf("execution sequence: want %v got %v", tc.expectedSequence, dispatcher.executionHistory)
				}
			}
			if tc.expectReply != "" && !strings.Contains(result.Reply, tc.expectReply) {
				t.Fatalf("reply: expected %q in %q", tc.expectReply, result.Reply)
			}
			if tc.expectPath != "" {
				if _, err := os.Stat(filepath.Join(root, tc.expectPath)); err != nil {
					t.Fatalf("expected file %q to exist: %v", tc.expectPath, err)
				}
			}
		})
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

type failedToolCallAgent struct {
	summaryReply string
}

func (a *failedToolCallAgent) Respond(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return a.summaryReply, nil
}

func (a *failedToolCallAgent) RespondStream(_ context.Context, _ string, _ func(string) error, _ TurnOptions) (string, bool, error) {
	return a.summaryReply, false, nil
}

func (a *failedToolCallAgent) RespondWithTools(
	ctx context.Context,
	_ []model.ToolDefinition,
	_ string,
	exec func(context.Context, model.ToolCall) (string, error),
	_ TurnOptions,
) (string, bool, error) {
	_, _ = exec(ctx, model.ToolCall{
		ID:   "bad_call_1",
		Type: "function",
		Function: model.FunctionCall{
			Name:      "write_file",
			Arguments: `{"path":"model-claimed.txt","content":"bad"}`,
		},
	})
	return "model claims the file was created", true, nil
}

func (a *failedToolCallAgent) SupportsToolCalling() bool {
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
