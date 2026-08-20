package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// TaskChip is one piece of side work the agent noticed but did not do: a
// dead config it grepped past, a stale doc, a missing test. The agent flags
// it with suggest_task mid-turn; the user sees a chip and decides — one
// click starts it in a fresh session, or dismisses it. The current turn
// never stops for it.
//
// Held in memory for the app run, not in aetox.db: a chip is a suggestion,
// not a record. If it mattered enough to survive a restart the user would
// have clicked it; persisting declined suggestions forever would make the
// store a nag list.
type TaskChip struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Tldr      string `json:"tldr"`
	Prompt    string `json:"prompt"`
	CreatedAt string `json:"createdAt"`
}

type taskChips struct {
	mu    sync.Mutex
	next  int
	chips []TaskChip
}

func (t *taskChips) add(title, tldr, prompt string) TaskChip {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.next++
	chip := TaskChip{
		ID:        fmt.Sprintf("chip-%d", t.next),
		Title:     title,
		Tldr:      tldr,
		Prompt:    prompt,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	t.chips = append(t.chips, chip)
	return chip
}

func (t *taskChips) remove(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, c := range t.chips {
		if c.ID == id {
			t.chips = append(t.chips[:i], t.chips[i+1:]...)
			return true
		}
	}
	return false
}

func (t *taskChips) list() []TaskChip {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Never nil: §34, a nil slice crashes the frontend.
	out := make([]TaskChip, len(t.chips))
	copy(out, t.chips)
	return out
}

// ListTaskChips returns the pending suggestions, for the UI to render on
// mount — chips suggested while a different view was open must not be lost.
func (a *App) ListTaskChips() []TaskChip {
	return a.cur().taskChips.list()
}

// DismissTaskChip removes one suggestion. Also called by the start flow:
// starting a chip consumes it.
func (a *App) DismissTaskChip(id string) {
	// The chat on screen owns the chip the user is clicking: it is drawn in that
	// conversation's tray and nowhere else.
	if a.cur().taskChips.remove(id) {
		a.emitTaskChips(a.cur())
	}
}

// emitTaskChips pushes the current chip list to the UI. Not emitEvent: that
// helper assumes either the test seam or a live Wails ctx, and this event
// also fires from the tool-coverage harness where neither exists — a chip
// added with nobody listening is still added, just rendered on next mount.
func (a *App) emitTaskChips(conv *conversation) {
	// Stamped like every other agent event: the window draws the tray of the
	// chat it is showing, and a chip raised in a background conversation must
	// not appear under a conversation that never saw the work.
	payload := sessionEvent[[]TaskChip]{SessionID: conv.id, Data: conv.taskChips.list()}
	if a.emit != nil {
		a.emit("tasks:changed", payload)
		return
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "tasks:changed", payload)
	}
}

// suggestTaskSkill is the agent-facing half: a workbench tool the model
// calls when it notices work worth doing that is NOT what the user asked
// for. The bar in the description is deliberate — vague code-smell hunches
// as chips would teach the user to ignore the feature.
// conv is the chat that noticed the work. Same reason every other workbench
// tool carries one.
type suggestTaskSkill struct {
	app  *App
	conv *conversation
}

func (*suggestTaskSkill) Name() string { return "suggest_task" }

func (*suggestTaskSkill) Description() string {
	return "แขวนงานนอกขอบเขตที่เจอระหว่างทำงานไว้เป็นการ์ดให้ผู้ใช้กดทำทีหลัง โดยไม่หยุดงานตรงหน้า"
}

func (*suggestTaskSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Imperative action phrase under 60 chars, e.g. \"Fix stale README badge\". Shown on the chip.",
			},
			"prompt": map[string]any{
				"type": "string",
				"description": "The full instruction for the future session. Must stand alone: name the files, " +
					"the finding, and what to do — the new session has none of this conversation.",
			},
			"tldr": map[string]any{
				"type":        "string",
				"description": "1-2 plain sentences of what will be done and why, in the user's language. Shown as the chip's tooltip.",
			},
		},
		"required":             []string{"title", "prompt"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "suggest_task",
			Description: "Flag an out-of-scope issue you noticed for a separate later session — dead code, a stale " +
				"doc, missing coverage, a confirmed bug beside the actual task. A chip appears for the user; one " +
				"click starts it fresh, so the prompt must stand alone (file paths, the finding, what to do). " +
				"Your current work continues uninterrupted. Do NOT flag vague hunches, trivial fixes you can do " +
				"inline, or the very thing the user asked you to do.",
			Parameters: payload,
		},
	}
}

func (s *suggestTaskSkill) Execute(ctx context.Context, input skill.Input) (skill.Output, error) {
	args := map[string]any{}
	if input != nil {
		if raw, ok := input["args"].(string); ok && strings.TrimSpace(raw) != "" {
			args["title"] = strings.TrimSpace(raw)
			args["prompt"] = strings.TrimSpace(raw)
		}
	}
	return s.ExecuteTool(ctx, args)
}

func (s *suggestTaskSkill) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	start := time.Now()
	title, _ := args["title"].(string)
	prompt, _ := args["prompt"].(string)
	tldr, _ := args["tldr"].(string)
	title = strings.TrimSpace(title)
	prompt = strings.TrimSpace(prompt)
	tldr = strings.TrimSpace(tldr)

	fail := func(err error) (skill.Output, error) {
		return skill.Output{
			Name:       "suggest_task",
			Content:    err.Error(),
			Command:    "suggest_task",
			Stderr:     err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
		}, err
	}
	if title == "" || prompt == "" {
		return fail(fmt.Errorf("title and prompt are both required — the prompt must let a fresh session act without this conversation"))
	}
	// A one-line prompt is almost never self-contained; catch the lazy call
	// here, where a retry costs one round, instead of in a future session
	// that starts with no idea what file it is about.
	if len(prompt) < 40 {
		return fail(fmt.Errorf("prompt is too thin to stand alone — name the files, the finding, and what to do"))
	}

	chip := s.conv.taskChips.add(title, tldr, prompt)
	s.app.emitTaskChips(s.conv)

	return skill.Output{
		Name:       "suggest_task",
		Content:    fmt.Sprintf("Noted as %s (%q). The user sees a chip and decides — continue your current work; do not start this yourself.", chip.ID, title),
		Command:    "suggest_task " + title,
		Success:    true,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}
