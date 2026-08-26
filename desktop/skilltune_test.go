package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/turn"
)

// skillJob runs one skill as a tool inside a turn, records the job, and applies
// the human's verdict — the same path a real reply takes: the skill's call lands
// in tool_runs, recordJobs folds it into the job's tool_seq, and RateTurn /
// markTurnRedone score it. verdict is "good", "bad", "redo", or "" (unrated).
func skillJob(t *testing.T, a *App, msgID int64, skillName, verdict string) {
	t.Helper()
	mark := a.maxToolRunID(a.cur())
	a.recordToolRun(a.cur(), turn.ToolRun{Ref: fmt.Sprintf("r%d", msgID), Name: skillName, OK: true})
	a.recordJobs(a.cur(), msgID, "งาน", "ตอบ", mark, time.Second)
	switch verdict {
	case outcomeGood, outcomeBad:
		a.RateTurn(msgID, verdict)
	case "redo":
		a.markTurnRedone(msgID)
	}
}

func misfires(t *testing.T, a *App, skills ...string) []skillMisfire {
	t.Helper()
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	set := map[string]bool{}
	for _, s := range skills {
		set[s] = true
	}
	return detectSkillMisfires(db, set, skillMisfireMinBad)
}

// jobs.outcome is the one signal the floor recorded and never read back. This is
// the reader: a skill the human marked bad three separate times is a candidate
// for a second look — the same threshold the summarizer uses for failures.
func TestASkillWithEnoughBadRatingsIsFlagged(t *testing.T) {
	a := newJobApp(t)
	for i := int64(1); i <= 3; i++ {
		skillJob(t, a, i, "aetox-slides", outcomeBad)
	}

	got := misfires(t, a, "aetox-slides")
	if len(got) != 1 {
		t.Fatalf("want one flagged skill, got %d: %+v", len(got), got)
	}
	if got[0].skill != "aetox-slides" || got[0].bad != 3 {
		t.Errorf("flagged %q with %d bad, want aetox-slides with 3", got[0].skill, got[0].bad)
	}
}

// Two is a coincidence, not a pattern — nothing flagged, exactly as the
// summarizer leaves two failures alone.
func TestTwoBadRatingsAreNotAPattern(t *testing.T) {
	a := newJobApp(t)
	skillJob(t, a, 1, "aetox-slides", outcomeBad)
	skillJob(t, a, 2, "aetox-slides", outcomeBad)

	if got := misfires(t, a, "aetox-slides"); len(got) != 0 {
		t.Fatalf("two bad ratings flagged a skill: %+v", got)
	}
}

// 👍 is the baseline, not a misfire: a skill only ever rated good is working, and
// the good count rides along on a flagged one as the frame a later stage checks
// an edit against.
func TestGoodRatingsAreABaselineNotAMisfire(t *testing.T) {
	a := newJobApp(t)
	for i := int64(1); i <= 4; i++ {
		skillJob(t, a, i, "aetox-slides", outcomeGood)
	}
	if got := misfires(t, a, "aetox-slides"); len(got) != 0 {
		t.Fatalf("a well-rated skill was flagged: %+v", got)
	}

	b := newJobApp(t)
	for i := int64(1); i <= 3; i++ {
		skillJob(t, b, i, "aetox-slides", outcomeBad)
	}
	skillJob(t, b, 4, "aetox-slides", outcomeGood)
	skillJob(t, b, 5, "aetox-slides", outcomeGood)
	got := misfires(t, b, "aetox-slides")
	if len(got) != 1 || got[0].bad != 3 || got[0].good != 2 {
		t.Fatalf("want 3 bad / 2 good carried on the flag, got %+v", got)
	}
}

// Only a skill can be refined, so only a skill is blamed. A bad turn whose work
// was a plain tool names no candidate — there is nothing to rewrite.
func TestOnlySkillsAreBlamedNotPlainTools(t *testing.T) {
	a := newJobApp(t)
	for i := int64(1); i <= 3; i++ {
		skillJob(t, a, i, "shell", outcomeBad)
	}
	// The skill set does not contain "shell", so the reader attributes nothing.
	if got := misfires(t, a, "aetox-slides"); len(got) != 0 {
		t.Fatalf("a plain tool was flagged as a misfiring skill: %+v", got)
	}
}

// "Answer again" is the negative signal nobody has to type. markTurnRedone
// writes outcome=bad (source=redo), so a skill regenerated away from three times
// is a candidate exactly as three thumbs-down would be.
func TestRedoCountsAsBad(t *testing.T) {
	a := newJobApp(t)
	for i := int64(1); i <= 3; i++ {
		skillJob(t, a, i, "aetox-slides", "redo")
	}
	got := misfires(t, a, "aetox-slides")
	if len(got) != 1 || got[0].bad != 3 {
		t.Fatalf("three redos should flag the skill with 3 bad, got %+v", got)
	}
}

// The worst offender leads: given two flagged skills, the higher bad-rate sorts
// first so a later pass takes the strongest candidate.
func TestTheWorstRateLeads(t *testing.T) {
	a := newJobApp(t)
	// aetox-slides: 3 bad, 0 good → rate 1.0
	for i := int64(1); i <= 3; i++ {
		skillJob(t, a, i, "aetox-slides", outcomeBad)
	}
	// aetox-ui-design: 3 bad, 5 good → lower rate
	for i := int64(10); i <= 12; i++ {
		skillJob(t, a, i, "aetox-ui-design", outcomeBad)
	}
	for i := int64(13); i <= 17; i++ {
		skillJob(t, a, i, "aetox-ui-design", outcomeGood)
	}

	got := misfires(t, a, "aetox-slides", "aetox-ui-design")
	if len(got) != 2 {
		t.Fatalf("want both flagged, got %d: %+v", len(got), got)
	}
	if got[0].skill != "aetox-slides" {
		t.Errorf("worst rate should lead; got %q first", got[0].skill)
	}
}
