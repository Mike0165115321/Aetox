package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/skill"
)

// The chip lifecycle in one test: the agent flags work, the user sees it
// listed, starting-or-dismissing consumes it, and every change reaches the
// UI through the event seam.
func TestSuggestTaskLifecycle(t *testing.T) {
	var events int
	a := &App{emit: func(event string, _ ...any) {
		if event == "tasks:changed" {
			events++
		}
	}}
	tool := &suggestTaskSkill{app: a, conv: a.cur()}

	out, err := tool.ExecuteTool(context.Background(), map[string]any{
		"title":  "Fix stale badge",
		"prompt": "In README.md the CI badge points at a renamed workflow; update the URL to the current one.",
		"tldr":   "Dead badge in the README.",
	})
	if err != nil {
		t.Fatalf("suggest_task: %v", err)
	}
	// The receipt must steer the model back to its current work — a model
	// that starts doing the flagged task itself defeats the whole feature.
	if !strings.Contains(out.Content, "continue your current work") {
		t.Fatalf("receipt does not redirect the model: %q", out.Content)
	}

	chips := a.ListTaskChips()
	if len(chips) != 1 || chips[0].Title != "Fix stale badge" {
		t.Fatalf("ListTaskChips = %+v, want the one suggestion", chips)
	}
	if chips[0].Tldr != "Dead badge in the README." {
		t.Fatalf("tldr lost: %+v", chips[0])
	}

	a.DismissTaskChip(chips[0].ID)
	if got := a.ListTaskChips(); len(got) != 0 {
		t.Fatalf("chip still listed after dismiss: %+v", got)
	}
	if events != 2 {
		t.Fatalf("UI heard %d tasks:changed events, want 2 (suggest + dismiss)", events)
	}

	// Dismissing what is already gone must not emit a phantom update.
	a.DismissTaskChip("chip-999")
	if events != 2 {
		t.Fatalf("dismissing a missing chip emitted an event")
	}
}

// A thin prompt would start a future session that has no idea what it is
// about — reject it now, where the retry costs the model one round.
func TestSuggestTaskRejectsThinPrompt(t *testing.T) {
	a := &App{emit: func(string, ...any) {}}
	tool := &suggestTaskSkill{app: a, conv: a.cur()}

	if _, err := tool.ExecuteTool(context.Background(), map[string]any{
		"title":  "Fix it",
		"prompt": "fix the bug",
	}); err == nil {
		t.Fatalf("a 11-char prompt cannot stand alone and must be rejected")
	}
	if _, err := tool.ExecuteTool(context.Background(), map[string]any{
		"title": "No prompt at all",
	}); err == nil {
		t.Fatalf("missing prompt must be rejected")
	}
	if got := a.ListTaskChips(); len(got) != 0 {
		t.Fatalf("rejected suggestions must not leave chips behind: %+v", got)
	}
}

// ListTaskChips must never return nil — §34, a nil slice crashes the frontend.
func TestListTaskChipsEmptyIsNotNil(t *testing.T) {
	a := &App{}
	if a.ListTaskChips() == nil {
		t.Fatalf("empty chip list marshaled as null")
	}
}

// The Run button's whole contract: the visible command runs through the real
// shell skill — sandbox, audit, the works — and what comes back is the
// command's own output, success flag included, whether it passed or failed.
func TestRunChatCommandThroughRealShell(t *testing.T) {
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	a := seed(&App{}, &conversation{registry: registry})

	res, err := a.RunChatCommand("echo run-block-ok")
	if err != nil {
		t.Fatalf("RunChatCommand: %v", err)
	}
	if !res.Success || !strings.Contains(res.Output, "run-block-ok") {
		t.Fatalf("result = %+v, want the echo output with success", res)
	}

	// A failing command is a result to read, not a Go error.
	res, err = a.RunChatCommand("exit 7")
	if err != nil {
		t.Fatalf("a failing command must not surface as an error: %v", err)
	}
	if res.Success {
		t.Fatalf("exit 7 reported success")
	}
}

func TestRunChatCommandGuards(t *testing.T) {
	a := &App{}
	if _, err := a.RunChatCommand("echo hi"); err == nil {
		t.Fatalf("no registry must mean a clear 'engine not ready' error")
	}
	a.cur().registry = skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	if _, err := a.RunChatCommand("   "); err == nil {
		t.Fatalf("blank command must be rejected before reaching the shell")
	}
}
