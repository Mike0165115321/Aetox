package main

import (
	"time"

	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/subagent"
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
	// Model is what this delegate ran on, and Tokens what it spent. Both are
	// per-delegate answers the session's own stats cannot give: a delegate's
	// tokens land in the user's total untouched, so the total knows how much was
	// spent and nothing knew by whom.
	Model  string `json:"model,omitempty"`
	Tokens int    `json:"tokens"`
	// Run and Phase place this row inside a declared job, both empty for a
	// delegate started on its own. The tray groups on Run and orders by the
	// phases the run declared, never by the phases it has seen — a phase nobody
	// has put work in yet is exactly the row worth drawing.
	Run   string `json:"run,omitempty"`
	Phase string `json:"phase,omitempty"`
	// TokensIn and TokensOut split Tokens into what this delegate's model read
	// and what it wrote, updated live as its rounds come back. The tray draws
	// both because they fail differently and the brake is a different decision
	// for each: input climbing is a transcript being re-sent every round,
	// output climbing is a model that will not stop writing.
	//
	// CachedIn is the share of TokensIn the provider served from its prompt
	// cache. CacheReported is whether it accounts for that at all, and the UI
	// must draw nothing rather than a zero when it does not: a local runtime
	// reporting no cache is not a runtime reporting a 0% hit.
	TokensIn      int  `json:"tokensIn"`
	TokensOut     int  `json:"tokensOut"`
	CachedIn      int  `json:"cachedIn"`
	CacheReported bool `json:"cacheReported"`
	// State: "running" | "waiting" (parked on a question) | "done" | "failed"
	// | "stopped" (the user ended it).
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
	if a.cur().delegations == nil {
		return []BackgroundTask{} // never nil: §34
	}
	infos := a.cur().delegations.Snapshot()
	out := make([]BackgroundTask, 0, len(infos))
	for _, t := range infos {
		out = append(out, BackgroundTask{
			ID: t.ID, Agent: t.Profile, Label: t.Label,
			StartedAt: t.Started.Format(time.RFC3339),
			ToolCalls: t.ToolCalls,
			Model:     t.Model,
			Tokens:    t.Tokens,

			TokensIn:      t.TokensIn,
			TokensOut:     t.TokensOut,
			CachedIn:      t.CachedIn,
			CacheReported: t.CacheReported,
			Run:           t.Run,
			Phase:         t.Phase,
			State:         taskState(t),
			ElapsedMs:     t.ElapsedMs,
			Question:      t.Question,
			Collected:     t.Collected,
		})
	}
	return out
}

// BackgroundRun is one declared job as the tray draws it: the group above the
// rows, and the phases it promised in the order it promised them.
//
// A thin rename of subagent.RunInfo for the same reason BackgroundTask is one of
// TaskInfo — the wire shape the frontend draws must not change as a side effect
// of an engine refactor.
type BackgroundRun struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Brief     string            `json:"brief,omitempty"`
	StartedAt string            `json:"startedAt"`
	Running   bool              `json:"running"`
	Tokens    int               `json:"tokens"`
	Phases    []BackgroundPhase `json:"phases"`
}

// BackgroundPhase is one declared stage. Planned is what the plan said and 0
// when it did not say, which the card draws as a bare count rather than "n of m"
// — a denominator nobody promised is one the user would hold the work to.
type BackgroundPhase struct {
	Title   string `json:"title"`
	Planned int    `json:"planned"`
	Done    int    `json:"done"`
	Failed  int    `json:"failed"`
	Running int    `json:"running"`
	Waiting int    `json:"waiting"`
	Tokens  int    `json:"tokens"`
}

// BackgroundRuns lists the declared jobs, newest first. Bound to the frontend
// and polled alongside BackgroundTasks.
func (a *App) BackgroundRuns() []BackgroundRun {
	if a.cur().delegations == nil {
		return []BackgroundRun{} // never nil: §34
	}
	infos := a.cur().delegations.Runs()
	out := make([]BackgroundRun, 0, len(infos))
	for _, r := range infos {
		run := BackgroundRun{
			ID: r.ID, Name: r.Name, Brief: r.Brief,
			StartedAt: r.Started.Format(time.RFC3339),
			Running:   r.Running, Tokens: r.Tokens,
		}
		for _, p := range r.Phases {
			run.Phases = append(run.Phases, BackgroundPhase{
				Title: p.Title, Planned: p.Planned, Done: p.Done, Failed: p.Failed,
				Running: p.Running, Waiting: p.Waiting, Tokens: p.Tokens,
			})
		}
		out = append(out, run)
	}
	return out
}

// taskState flattens the register's flags into the one word the tray draws.
//
// Stopped comes first, ahead of Running, and that order is the whole point of
// the field. A delegate is marked stopped the moment the user asks for it and
// keeps running until its goroutine unwinds, which can be a tool call away; a
// row that reported "running" through that window would answer the click with
// a spinner, and the user would press it again. Ahead of the outcome for the
// same reason it exists at all: work somebody stopped is not work that failed.
func taskState(t subagent.TaskInfo) string {
	switch {
	case t.Stopped:
		return "stopped"
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

// StopBackgroundTask ends one running delegation, and reports whether there was
// anything left to end.
//
// The tray's per-row brake. Until this existed the only door to a running
// delegate was CancelTurn, which the composer only offers while a turn is live
// โ€” and a delegate deliberately outlives the turn that started it
// (internal/subagent/runner.go). So the ordinary case, an agent dispatched and
// the turn finished, left up to four sub-agents looping with no button on
// screen that could reach them. The card in the tray is on screen for exactly
// as long as that work is unclaimed, which makes it the right place for it.
//
// False is not an error. The tray polls every two seconds, so a row can finish
// between the paint and the click, and the user got what they wanted anyway.
func (a *App) StopBackgroundTask(id string) bool {
	if a.cur().delegations == nil {
		return false
	}
	stopped := a.cur().delegations.Stop(id)
	debuglog.Msg("stop: sub-agent %s stopped=%v", id, stopped)
	return stopped
}

// StopBackgroundRun ends every delegate inside one declared job and reports how
// many it found. A run is one piece of work to the person watching it, however
// many workers it spread across, so it needs a brake shaped like the job.
func (a *App) StopBackgroundRun(runID string) int {
	if a.cur().delegations == nil {
		return 0
	}
	stopped := a.cur().delegations.StopRun(runID)
	debuglog.Msg("stop: run %s ended %d sub-agent(s)", runID, stopped)
	return stopped
}
