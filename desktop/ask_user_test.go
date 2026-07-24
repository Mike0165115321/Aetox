package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ask_user must reject calls that would render an unanswerable prompt.
func TestAskUserValidation(t *testing.T) {
	s := &askUserSkill{app: &App{}}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"options": []any{"a", "b"},
	}); err == nil || !strings.Contains(err.Error(), "question") {
		t.Fatalf("missing question must fail loudly, got %v", err)
	}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"question": "pick one",
		"options":  []any{"only"},
	}); err == nil || !strings.Contains(err.Error(), "2 options") {
		t.Fatalf("fewer than 2 options must fail loudly, got %v", err)
	}
}

// The full round-trip: the tool blocks until AnswerUserQuestion delivers the
// user's choice, then reports it to the model.
func TestAskUserAnswerRoundTrip(t *testing.T) {
	app := &App{}
	s := &askUserSkill{app: app}

	type result struct {
		content string
		err     error
	}
	done := make(chan result, 1)
	go func() {
		out, err := s.ExecuteTool(context.Background(), map[string]any{
			"question": "which one?",
			"options":  []any{"A", "B"},
		})
		done <- result{out.Content, err}
	}()

	// Wait until the question is registered, then answer as the user would.
	deadline := time.After(2 * time.Second)
	for {
		app.askMu.Lock()
		pending := app.askCh != nil
		app.askMu.Unlock()
		if pending {
			break
		}
		select {
		case <-deadline:
			t.Fatal("question was never registered")
		case <-time.After(5 * time.Millisecond):
		}
	}
	app.AnswerUserQuestion("B")

	r := <-done
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if !strings.Contains(r.content, "user chose: B") {
		t.Fatalf("answer must reach the model receipt, got %q", r.content)
	}
	// The slot must be free again for the next question.
	app.askMu.Lock()
	defer app.askMu.Unlock()
	if app.askCh != nil {
		t.Fatal("ask slot must be cleared after the answer")
	}
}

// Turn cancellation (Stop button) must unblock a waiting question.
func TestAskUserCancelUnblocks(t *testing.T) {
	app := &App{}
	s := &askUserSkill{app: app}
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := s.ExecuteTool(ctx, map[string]any{
			"question": "still there?",
			"options":  []any{"A", "B"},
		})
		errCh <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("canceled question must return an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not unblock the question")
	}
}

// Only one question may be in flight — a second concurrent ask fails loudly
// instead of silently queueing.
func TestAskUserSecondQuestionFailsWhilePending(t *testing.T) {
	app := &App{}
	if _, err := app.beginUserQuestion("first", []string{"a", "b"}); err != nil {
		t.Fatalf("first question must register: %v", err)
	}
	defer app.endUserQuestion()
	if _, err := app.beginUserQuestion("second", []string{"a", "b"}); err == nil {
		t.Fatal("second concurrent question must fail")
	}
}

// A stale answer (after cancel/completion) must be a no-op, not a panic.
func TestAnswerUserQuestionNoPendingIsNoop(t *testing.T) {
	app := &App{}
	app.AnswerUserQuestion("stale click") // must not panic
}

// todo_write sanitizes junk input and reports honest counts.
func TestTodoWriteSanitizesAndCounts(t *testing.T) {
	s := &todoWriteSkill{app: &App{}}
	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "task one", "status": "completed"},
			map[string]any{"content": "task two", "status": "in_progress"},
			map[string]any{"content": "task three", "status": "bogus-status"}, // → pending
			map[string]any{"content": "   ", "status": "pending"},             // dropped
			"not an object", // dropped
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatal("todo_write must succeed")
	}
	if !strings.Contains(out.Content, "3 items, 1 completed") {
		t.Fatalf("receipt must count sanitized items, got %q", out.Content)
	}
}

// An empty list is valid — it clears the checklist.
func TestTodoWriteEmptyListClears(t *testing.T) {
	s := &todoWriteSkill{app: &App{}}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"todos": []any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "0 items, 0 completed") {
		t.Fatalf("empty list receipt wrong: %q", out.Content)
	}
}
