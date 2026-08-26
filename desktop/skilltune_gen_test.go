package main

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

type fakeDrafter struct {
	op, before, body, reason string
	calls                    int
	sawEvidence              string
}

func (f *fakeDrafter) Draft(_ context.Context, _, _, evidence string) (string, string, string, string, error) {
	f.calls++
	f.sawEvidence = evidence
	return f.op, f.before, f.body, f.reason, nil
}

type pendingRow struct {
	scope, op, before, body, source, evidence string
}

func pendingByKind(t *testing.T, a *App, kind string) []pendingRow {
	t.Helper()
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	rows, err := db.Query(
		`SELECT scope, op, before, body, source, evidence FROM pending_changes
		  WHERE kind = ? AND state = ? ORDER BY id`, kind, statePending)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []pendingRow
	for rows.Next() {
		var r pendingRow
		if err := rows.Scan(&r.scope, &r.op, &r.before, &r.body, &r.source, &r.evidence); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != (error)(nil) && err != sql.ErrNoRows {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// The whole stage-three pipeline without an LLM: three bad ratings flag a skill,
// the drafter (faked) proposes an edit, and it lands as a skill-kind proposal in
// the approval queue — grounded in the job rows, sourced to the optimizer, and
// nothing applied until a human approves.
func TestGeneratorQueuesADraftedSkillEdit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	a := newJobApp(t)
	for i := int64(1); i <= 3; i++ {
		skillJob(t, a, i, "aetox-slides", outcomeBad)
	}

	f := &fakeDrafter{op: "add", body: "OPTIMIZER-DRAFTED-LINE", reason: "เพราะโดน 👎 ซ้ำ"}
	a.generateSkillRefinements(context.Background(), f)

	rows := pendingByKind(t, a, kindSkill)
	if len(rows) != 1 {
		t.Fatalf("want one skill proposal, got %d", len(rows))
	}
	r := rows[0]
	if r.scope != "aetox-slides" || r.body != "OPTIMIZER-DRAFTED-LINE" {
		t.Errorf("proposal = %+v, want the drafted edit on aetox-slides", r)
	}
	if r.source != "optimizer" {
		t.Errorf("source = %q — the self-optimize loop must read distinct from the model deciding mid-task", r.source)
	}
	if !strings.HasPrefix(r.evidence, "jobs:") {
		t.Errorf("evidence should name the job rows it was drawn from: %q", r.evidence)
	}
	if !strings.Contains(f.sawEvidence, "ถาม:") {
		t.Errorf("the drafter was handed no grounded evidence: %q", f.sawEvidence)
	}

	// A second pass must not re-call the model or duplicate: the skill already
	// has a proposal waiting.
	a.generateSkillRefinements(context.Background(), f)
	if f.calls != 1 {
		t.Errorf("the model was called %d times; a skill with a pending proposal must not be re-drafted", f.calls)
	}
	if rows := pendingByKind(t, a, kindSkill); len(rows) != 1 {
		t.Errorf("a duplicate proposal was queued: %d rows", len(rows))
	}
}

// A drafter that returns an empty body is the "not the skill's fault" answer —
// nothing is queued.
func TestGeneratorQueuesNothingWhenTheDrafterDeclines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	a := newJobApp(t)
	for i := int64(1); i <= 3; i++ {
		skillJob(t, a, i, "aetox-slides", outcomeBad)
	}
	a.generateSkillRefinements(context.Background(), &fakeDrafter{op: "add", body: ""})
	if rows := pendingByKind(t, a, kindSkill); len(rows) != 0 {
		t.Fatalf("an empty draft was queued anyway: %+v", rows)
	}
}
