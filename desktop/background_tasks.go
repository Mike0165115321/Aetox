package main

import (
	"time"

	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// The tray over background delegations (§105). A delegate outlives the turn
// that started it, and the turn's timeline is a snapshot the moment the turn
// ends — so without this, work that is genuinely still running is invisible,
// which on screen is indistinguishable from work that died. The owner spotted
// it within a minute of running the build: "ถ้ามันทำงาน ควรจะไม่นิ่งแบบนี้".
//
// Read from the engine's Delegations register, never assembled from tool
// events, for the reason §105 wrote down in advance: the `task` tool call
// completes the instant the handle is returned — the moment the work starts —
// so a UI watching events sees every delegation as finished from birth. The
// register is the one place that knows running from done.

// BackgroundTask is one delegation as the tray shows it. A thin rename of
// subagent.TaskInfo rather than that type re-exported, so the wire shape the
// frontend depends on cannot change as a side effect of an engine refactor.
type BackgroundTask struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	Label string `json:"label"`
	// StartedAt is RFC3339; the frontend runs the clock from it so the row
	// ticks without polling Go every second.
	StartedAt string `json:"startedAt"`
	ToolCalls int    `json:"toolCalls"`
	// State: "running" | "waiting" (parked on a question) | "done" | "failed".
	State string `json:"state"`
	// ElapsedMs is the finished delegation's real duration, 0 while it runs —
	// the clock is still going then, and the frontend runs it from StartedAt.
	// Without this the card falls back to the `task` call's own duration, which
	// is how long the *spawn* took, and freezes there for the rest of the job.
	ElapsedMs int64 `json:"elapsedMs,omitempty"`
	// Question is the text a waiting delegate is stuck on, "" otherwise.
	Question string `json:"question,omitempty"`
	// Collected: a finished result somebody has already redeemed. The tray
	// shows the row struck through / hides it — the work is in the chat now.
	Collected bool `json:"collected"`
}

// BackgroundTasks lists this session's delegations, newest first. Bound to the
// frontend; polled while the tray has anything to show.
func (a *App) BackgroundTasks() []BackgroundTask {
	if a.delegations == nil {
		return []BackgroundTask{} // never nil: §34
	}
	infos := a.delegations.Snapshot()
	out := make([]BackgroundTask, 0, len(infos))
	for _, t := range infos {
		out = append(out, BackgroundTask{
			ID: t.ID, Agent: t.Profile, Label: t.Label,
			StartedAt: t.Started.Format(time.RFC3339),
			ToolCalls: t.ToolCalls,
			State:     taskState(t),
			ElapsedMs: t.ElapsedMs,
			Question:  t.Question,
			Collected: t.Collected,
		})
	}
	return out
}

func taskState(t subagent.TaskInfo) string {
	switch {
	case t.Waiting:
		return "waiting"
	case t.Running:
		return "running"
	case t.OK:
		return "done"
	default:
		return "failed"
	}
}
