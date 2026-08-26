package main

import "testing"

func seedPendingRow(t *testing.T, a *App, kind, scope, op, body string) {
	t.Helper()
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO pending_changes(kind, scope, target, op, before, body, reason, evidence, source, state, created_at)
		 VALUES(?,?,'',?,'',?,'r','e','optimizer',?,'2026-08-26T00:00:00Z')`,
		kind, scope, op, body, statePending); err != nil {
		t.Fatalf("seed %s row: %v", kind, err)
	}
}

// The two queues are separate axes: a skill edit is reviewed in the skill-tuning
// room, a memory line in the learning room, and neither shows up where the other
// is decided. The counts stay separate for the same reason.
func TestSkillProposalsAreTheirOwnQueue(t *testing.T) {
	a := newJobApp(t)
	seedPendingRow(t, a, kindMemory, "main", "add", "a remembered fact")
	seedPendingRow(t, a, kindSkill, "aetox-slides", "add", "a skill edit")

	mem := a.ListPendingChanges()
	if len(mem) != 1 || mem[0].Kind != kindMemory {
		t.Fatalf("the learning queue should hold only the memory line, got %+v", mem)
	}
	sk := a.ListSkillProposals()
	if len(sk) != 1 || sk[0].Kind != kindSkill || sk[0].Scope != "aetox-slides" {
		t.Fatalf("the skill queue should hold only the skill edit, got %+v", sk)
	}
	if a.PendingLearnedCount() != 1 || a.PendingSkillTuneCount() != 1 {
		t.Errorf("counts crossed: learned=%d skill=%d, want 1 and 1",
			a.PendingLearnedCount(), a.PendingSkillTuneCount())
	}
}

// Off ships by default — drafting spends a model call — and the switch persists.
func TestSkillTuneAutoPrefRoundTrips(t *testing.T) {
	a := newJobApp(t)
	if a.SkillTuneAuto() {
		t.Error("auto-draft should ship off")
	}
	if err := a.SetSkillTuneAuto(true); err != nil {
		t.Fatalf("turn on: %v", err)
	}
	if !a.SkillTuneAuto() {
		t.Error("the switch did not turn on")
	}
	if err := a.SetSkillTuneAuto(false); err != nil {
		t.Fatalf("turn off: %v", err)
	}
	if a.SkillTuneAuto() {
		t.Error("the switch did not turn off")
	}
}

// The manual trigger refuses when learning is off rather than spending a model
// call on a feature the user switched away from.
func TestRunSkillTuneupNeedsLearning(t *testing.T) {
	a := newJobApp(t)
	if err := a.SetLearningEnabled(false); err != nil {
		t.Fatalf("disable learning: %v", err)
	}
	if _, err := a.RunSkillTuneup(); err == nil {
		t.Error("with learning off the manual trigger should refuse")
	}
}

// With nothing rated bad, no skill is flagged, so the drafter is never called
// (no model call) and nothing is queued — the safe common case.
func TestRunSkillTuneupIsQuietWithNoMisfires(t *testing.T) {
	a := newJobApp(t)
	n, err := a.RunSkillTuneup()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if n != 0 {
		t.Errorf("queued %d proposals from no misfires, want 0", n)
	}
	if got := len(a.ListSkillProposals()); got != 0 {
		t.Errorf("a proposal appeared from nothing: %d", got)
	}
}
