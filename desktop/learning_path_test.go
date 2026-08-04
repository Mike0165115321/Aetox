package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/learned"
	"github.com/Mike0165115321/Aetox/internal/safety"
)

// Whole-path tests for the learning floor, on Aetox's own model (ARCHITECTURE
// §45): a real turn through cognitive.Agent → turn.Executor → the registry,
// recorded by the same code a live provider would trigger.
//
// The rest of the learning tests build their rows by hand, which proves the
// bookkeeping and nothing about the wiring — every one of them would still pass
// if `recordJobs` were never called from a turn at all, or if `memory` were
// never registered. These are the ones that would not.
//
// `aetox-tools:test` with `memory` in the brief is the same thing a person
// switches to in the app to check this by hand, so a green test and a hand
// check are looking at one behaviour.

func bootLearningApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	// ctx is what Wails hands the app at startup; runTurn derives the turn's
	// cancellable context from it. `emit` is the test seam every event goes
	// through — the Wails runtime rejects any context but its own, so a turn
	// that pushes an event would take the process down without it.
	a := &App{
		ctx:       context.Background(),
		emit:      func(string, ...any) {},
		dbDir:     t.TempDir(),
		sessionID: newSessionID(),
	}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})
	return a
}

// One turn, end to end: the model calls a tool, the tool runs through the real
// dispatcher, and what the store holds afterwards is a job with the shape of
// the work in it.
func TestALiveTurnRecordsTheWorkItDid(t *testing.T) {
	a := bootLearningApp(t)

	reply, err := a.SendMessage("memory: ทดสอบชั้นการเรียนรู้")
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if reply.MessageID == 0 {
		t.Fatal("a stored reply must come back with its row id, or it can never be rated")
	}

	jobs := readJobs(t, a)
	if len(jobs) != 1 {
		t.Fatalf("want one job for one turn, got %d", len(jobs))
	}
	job := jobs[0]
	if job.toolSeq != "memory" {
		t.Errorf("tool_seq = %q — the shape of the work is read off the real run, not declared", job.toolSeq)
	}
	if job.toolCount != 1 || job.failedTools != 0 {
		t.Errorf("counts = %d/%d, want one call and no failures", job.toolCount, job.failedTools)
	}
	if job.messageID != reply.MessageID {
		t.Errorf("job is filed under message %d but the reply is %d — the thumbs would address nothing",
			job.messageID, reply.MessageID)
	}
	if !strings.Contains(job.request, "ทดสอบ") {
		t.Errorf("the job should carry what was asked, got %q", job.request)
	}
	if job.outcome != outcomeUnknown {
		t.Errorf("a fresh job must not be scored, got %q", job.outcome)
	}
}

// The same turn, from the other end: the tool the model called was the real
// `memory`, it went through the real approval door, and nothing reached the
// memory file on the way.
func TestALiveTurnQueuesWhatItWantedToRememberAndWritesNothing(t *testing.T) {
	a := bootLearningApp(t)

	reply, err := a.SendMessage("memory: ทดสอบชั้นการเรียนรู้")
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	// The model reports what the tool answered. If that ever becomes "saved",
	// the guarantee has been broken somewhere below this line.
	if !strings.Contains(strings.ToLower(reply.Text), "approve") {
		t.Errorf("the reply should say the change is waiting for approval:\n%s", reply.Text)
	}

	pending := a.ListPendingChanges()
	if len(pending) != 1 {
		t.Fatalf("want one proposal queued by the turn, got %d", len(pending))
	}
	if pending[0].Scope != learned.MainScope {
		t.Errorf("the main agent's proposal must be in the main scope, got %q", pending[0].Scope)
	}
	if pending[0].Reason == "" {
		t.Error("the proposal reached the queue without its reasoning")
	}
	if got := learned.Read(learned.MainScope); got != "" {
		t.Fatalf("a turn wrote into memory without approval: %q", got)
	}

	// And approving it is what puts it on disk — through the same call the
	// button makes.
	if err := a.ApprovePendingChange(pending[0].ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got := learned.Read(learned.MainScope); !strings.Contains(got, "Excel") {
		t.Fatalf("approval did not reach the file, got %q", got)
	}
	_ = reply
}

// Rating the answer a real turn produced, addressed the way the UI addresses
// it — by the bubble, not by a job id the frontend never sees.
func TestRatingALiveTurnLandsOnItsJob(t *testing.T) {
	a := bootLearningApp(t)

	reply, err := a.SendMessage("memory: ทดสอบการให้คะแนน")
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	a.RateTurn(reply.MessageID, outcomeGood)

	jobs := readJobs(t, a)
	if len(jobs) != 1 || jobs[0].outcome != outcomeGood || jobs[0].outcomeSource != "thumb" {
		t.Fatalf("the rating did not reach the turn's job: %+v", jobs)
	}
	if a.TurnRating(reply.MessageID) != outcomeGood {
		t.Error("a reopened session would show no thumb")
	}
}

// Switching learning off has to stop the whole thing at the source — not hide
// it. A turn still answers; it just leaves nothing behind.
func TestALiveTurnRecordsNothingWhenLearningIsOff(t *testing.T) {
	a := bootLearningApp(t)
	pref, _, _ := config.LoadModelPreference()
	pref.LearningDisabled = true
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatalf("save preference: %v", err)
	}

	reply, err := a.SendMessage("memory: ทดสอบตอนปิดสวิตช์")
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if strings.TrimSpace(reply.Text) == "" {
		t.Error("the turn should still answer with learning off")
	}
	if jobs := readJobs(t, a); len(jobs) != 0 {
		t.Errorf("want no job rows with learning off, got %d", len(jobs))
	}
	if n := a.PendingLearnedCount(); n != 0 {
		t.Errorf("want nothing queued with learning off, got %d", n)
	}
}

// A delegate's proposal belongs to the delegate. This is the claim the flat
// context rests on, and it is the one failure that would look identical from
// the outside — a proposal appears either way — so it is checked through a real
// delegation rather than by calling the scoped tool directly.
func TestADelegatesProposalIsFiledUnderTheDelegate(t *testing.T) {
	a := bootLearningApp(t)

	// "general" picks the profile: it is the delegate with no tool allowlist,
	// so it is handed a `memory` bound to its own name.
	if _, err := a.SendMessage("subagent general memory: ให้ลูกจำอะไรสักอย่าง"); err != nil {
		t.Fatalf("turn failed: %v", err)
	}

	pending := a.ListPendingChanges()
	if len(pending) == 0 {
		t.Fatal("the delegation queued nothing — the delegate never reached its memory tool")
	}
	for _, c := range pending {
		if c.Scope == learned.MainScope {
			t.Fatalf("a delegate's proposal was filed against the main agent: %+v", c)
		}
		if c.Scope != "general" {
			t.Errorf("proposal scope = %q, want the profile that ran", c.Scope)
		}
		if !strings.Contains(filepath.ToSlash(c.Target), "memory/agents/") {
			t.Errorf("a delegate's memory must land in its own file, got %q", c.Target)
		}
	}
	if got := learned.Read(learned.MainScope); got != "" {
		t.Errorf("the delegation touched the main agent's memory: %q", got)
	}
}

// The other half of the same rule: a profile that was never given `memory`
// cannot propose one. `explore` is read-only search with an explicit tool
// allowlist, and a tool it did not ask for arriving anyway would be the
// allowlist quietly meaning nothing.
func TestAReadOnlyDelegateIsNeverHandedMemory(t *testing.T) {
	a := bootLearningApp(t)

	// No "general" in the brief, so `task` spawns explore — its allowlist is
	// grep/glob/list/read.
	if _, err := a.SendMessage("subagent memory: ลองให้ explore จำ"); err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if n := a.PendingLearnedCount(); n != 0 {
		t.Fatalf("a profile with no memory in its allowlist proposed %d change(s)", n)
	}
}
