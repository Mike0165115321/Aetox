package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// askUserSkill lets the model pause mid-turn and ask the user to pick between
// concrete options (the Claude Code AskUserQuestion pattern). The tool blocks
// until the user clicks an option in the chat UI or the turn is canceled.
// One question in flight at a time — the tool loop is sequential anyway.
type askUserSkill struct{ app *App }

func (*askUserSkill) Name() string { return "ask_user" }

func (*askUserSkill) Description() string {
	return "ถามผู้ใช้ให้เลือกตัวเลือก แล้วรอคำตอบก่อนทำงานต่อ"
}

func (*askUserSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "The question to ask, in the user's language. Ask only when genuinely blocked on a decision that is the user's to make.",
			},
			"options": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "2-4 short, mutually exclusive choices. The user can also type a free-text answer.",
			},
		},
		"required":             []string{"question", "options"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "ask_user",
			Description: "Ask the user to choose between options and wait for the answer. Use when blocked on a decision only the user can make; never for questions you can resolve yourself.",
			Parameters:  payload,
		},
	}
}

func (s *askUserSkill) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	question, _ := args["question"].(string)
	var options []string
	if raw, ok := args["options"].([]any); ok {
		for _, o := range raw {
			if v, ok := o.(string); ok && strings.TrimSpace(v) != "" {
				options = append(options, strings.TrimSpace(v))
			}
		}
	}
	return s.ask(ctx, strings.TrimSpace(question), options)
}

func (s *askUserSkill) Execute(ctx context.Context, input skill.Input) (skill.Output, error) {
	question, _ := input["question"].(string)
	return s.ask(ctx, strings.TrimSpace(question), nil)
}

func (s *askUserSkill) ask(ctx context.Context, question string, options []string) (skill.Output, error) {
	start := time.Now()
	fail := func(err error) (skill.Output, error) {
		return skill.Output{
			Name: "ask_user", Command: "ask_user", Success: false,
			Content: err.Error(), RawOutput: err.Error(), Stderr: err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
		}, err
	}
	if question == "" {
		return fail(errors.New("question is required"))
	}
	if len(options) < 2 {
		return fail(errors.New("provide at least 2 options"))
	}

	answerCh, err := s.app.beginUserQuestion(question, options)
	if err != nil {
		return fail(err)
	}
	defer s.app.endUserQuestion()

	select {
	case answer := <-answerCh:
		receipt := fmt.Sprintf("user chose: %s", answer)
		return skill.Output{
			Name: "ask_user", Command: "ask_user " + question, Success: true,
			Content: receipt, RawOutput: receipt,
			DurationMs: time.Since(start).Milliseconds(),
		}, nil
	case <-ctx.Done():
		return fail(ctx.Err())
	}
}

// beginUserQuestion registers the single in-flight question and pushes it to
// the chat UI. Fails loudly if one is already pending.
func (a *App) beginUserQuestion(question string, options []string) (chan string, error) {
	a.askMu.Lock()
	defer a.askMu.Unlock()
	if a.askCh != nil {
		return nil, errors.New("another question is already awaiting the user")
	}
	a.askCh = make(chan string, 1)
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "ask:user", map[string]any{
			"question": question,
			"options":  options,
		})
	}
	return a.askCh, nil
}

func (a *App) endUserQuestion() {
	a.askMu.Lock()
	defer a.askMu.Unlock()
	a.askCh = nil
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "ask:done", nil)
	}
}

// AnswerUserQuestion delivers the user's choice to the blocked ask_user tool
// call. No-op when nothing is pending (e.g. a stale click after cancel).
func (a *App) AnswerUserQuestion(answer string) {
	a.askMu.Lock()
	defer a.askMu.Unlock()
	if a.askCh == nil {
		return
	}
	select {
	case a.askCh <- strings.TrimSpace(answer):
	default: // already answered
	}
}

// todoWriteSkill is the Claude Code TodoWrite pattern: on long multi-step
// work the model maintains a task checklist the user can watch. Each call
// replaces the whole list; state lives in the frontend only.
// ponytail: not persisted with the session — store it in SessionMessage land
// if surviving a reload ever matters.
type todoWriteSkill struct{ app *App }

func (*todoWriteSkill) Name() string { return "todo_write" }

func (*todoWriteSkill) Description() string {
	return "อัปเดตรายการสิ่งที่ต้องทำของงานปัจจุบันให้ผู้ใช้เห็น"
}

func (*todoWriteSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type":        "array",
				"description": "The full task list (replaces the previous list). Keep one item in_progress at a time; mark items completed as soon as they are done.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{"type": "string", "description": "Short task description in the user's language"},
						"status":  map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
					},
					"required":             []string{"content", "status"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"todos"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "todo_write",
			Description: "Maintain a visible task checklist for large or long-running work: call it when starting multi-step work, and again whenever a step's status changes. Sends the FULL list each call.",
			Parameters:  payload,
		},
	}
}

func (s *todoWriteSkill) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	start := time.Now()
	raw, _ := args["todos"].([]any)
	type todoItem struct {
		Content string `json:"content"`
		Status  string `json:"status"`
	}
	items := make([]todoItem, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		content, _ := m["content"].(string)
		status, _ := m["status"].(string)
		content = strings.TrimSpace(content)
		switch status {
		case "pending", "in_progress", "completed":
		default:
			status = "pending"
		}
		if content == "" {
			continue
		}
		items = append(items, todoItem{Content: content, Status: status})
	}
	if s.app.ctx != nil {
		wailsruntime.EventsEmit(s.app.ctx, "todo:update", items)
	}
	doneCount := 0
	for _, it := range items {
		if it.Status == "completed" {
			doneCount++
		}
	}
	receipt := fmt.Sprintf("todo list updated: %d items, %d completed", len(items), doneCount)
	return skill.Output{
		Name: "todo_write", Command: "todo_write", Success: true,
		Content: receipt, RawOutput: receipt,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

func (s *todoWriteSkill) Execute(ctx context.Context, input skill.Input) (skill.Output, error) {
	return s.ExecuteTool(ctx, map[string]any{"todos": input["todos"]})
}
