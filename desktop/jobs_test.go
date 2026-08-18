package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

type storedJob struct {
	sessionID     string
	messageID     int64
	agent         string
	parentRef     string
	request       string
	answer        string
	toolSeq       string
	toolCount     int
	failedTools   int
	outcome       string
	outcomeSource string
}

func readJobs(t *testing.T, a *App) []storedJob {
	t.Helper()
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	rows, err := db.Query(`SELECT session_id, message_id, agent, parent_ref, request, answer,
		tool_seq, tool_count, failed_tools, outcome, outcome_source FROM jobs ORDER BY id`)
	if err != nil {
		t.Fatalf("query jobs: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []storedJob
	for rows.Next() {
		var j storedJob
		if err := rows.Scan(&j.sessionID, &j.messageID, &j.agent, &j.parentRef, &j.request,
			&j.answer, &j.toolSeq, &j.toolCount, &j.failedTools, &j.outcome, &j.outcomeSource); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("jobs: read %d row(s) and then failed, so the assertions below would be checking a short list: %v", len(out), err)
	}
	return out
}

// newJobApp isolates DataRoot as well as the database: learningEnabled reads
// the preference file, and a test that consulted the developer's real one would
// pass or fail depending on a switch in their settings.
func newJobApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := seed(&App{cfg: config.Config{}, dbDir: t.TempDir()}, &conversation{id: "20260804-120000.000"})
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	return a
}

// The shape of the work is the whole reason the table exists: without an
// ordered sequence there is nothing to match a later job against.
func TestRecordJobsCapturesTheToolSequence(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID(a.cur())
	for _, r := range []turn.ToolRun{
		{Ref: "c1", Name: "read", OK: true},
		{Ref: "c2", Name: "image_ocr", OK: true},
		{Ref: "c3", Name: "sheet_write", OK: false, Error: "no such folder"},
	} {
		a.recordToolRun(a.cur(), r)
	}
	a.recordJobs(a.cur(), 7, "อ่านสลิปนี้ให้หน่อย", "ยอด 1,250 บาท", mark, 3*time.Second)

	jobs := readJobs(t, a)
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	got := jobs[0]
	if got.toolSeq != "read>image_ocr>sheet_write" {
		t.Errorf("tool_seq = %q, want the calls in order", got.toolSeq)
	}
	if got.toolCount != 3 || got.failedTools != 1 {
		t.Errorf("counts = %d tools / %d failed, want 3/1", got.toolCount, got.failedTools)
	}
	if got.messageID != 7 || got.agent != "" {
		t.Errorf("main job should be message 7 with empty agent, got %d / %q", got.messageID, got.agent)
	}
	if got.outcome != outcomeUnknown {
		t.Errorf("a fresh job must not be scored: outcome = %q", got.outcome)
	}
}

// A delegate's job is reconstructed from rows that already exist, so scoring a
// sub-agent needs no second capture path.
func TestRecordJobsReconstructsDelegateWork(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID(a.cur())
	a.recordToolRun(a.cur(), turn.ToolRun{Ref: "t1", Name: "task",
		Args: `{"agent":"explore","prompt":"หาไฟล์ใบเสร็จทั้งหมด"}`, Output: "เจอ 12 ไฟล์", OK: true})
	a.recordToolRun(a.cur(), turn.ToolRun{Ref: "c1", Parent: "t1", Agent: "explore", Name: "glob", OK: true})
	a.recordToolRun(a.cur(), turn.ToolRun{Ref: "c2", Parent: "t1", Agent: "explore", Name: "grep", OK: true})
	a.recordJobs(a.cur(), 9, "หาใบเสร็จ", "เจอ 12 ไฟล์", mark, time.Second)

	jobs := readJobs(t, a)
	if len(jobs) != 2 {
		t.Fatalf("want a main job and a delegate job, got %d", len(jobs))
	}
	main, child := jobs[0], jobs[1]
	if main.toolSeq != "task" {
		t.Errorf("the main agent's sequence should show it delegated: %q", main.toolSeq)
	}
	if child.agent != "explore" {
		t.Errorf("delegate job scope = %q, want explore", child.agent)
	}
	if child.toolSeq != "glob>grep" {
		t.Errorf("delegate sequence = %q, want its own calls", child.toolSeq)
	}
	if child.parentRef != "t1" {
		t.Errorf("delegate job should name the task call that caused it, got %q", child.parentRef)
	}
	if !strings.Contains(child.request, "ใบเสร็จ") || child.answer != "เจอ 12 ไฟล์" {
		t.Errorf("delegate brief/result not carried: %q / %q", child.request, child.answer)
	}
	if child.messageID != 0 {
		t.Errorf("a delegate's job has no bubble to be rated under, got message %d", child.messageID)
	}
}

// The switch has to actually switch it off — a setting that only hides the
// feature while still recording would be worse than not having one.
func TestLearningOffRecordsNothing(t *testing.T) {
	a := newJobApp(t)
	pref, _, _ := config.LoadModelPreference()
	pref.LearningDisabled = true
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatalf("save preference: %v", err)
	}
	mark := a.maxToolRunID(a.cur())
	a.recordToolRun(a.cur(), turn.ToolRun{Ref: "c1", Name: "read", OK: true})
	a.recordJobs(a.cur(), 3, "q", "a", mark, time.Second)

	if jobs := readJobs(t, a); len(jobs) != 0 {
		t.Fatalf("learning is off; want no job rows, got %d", len(jobs))
	}
}

func TestRateTurnStoresAndClearsTheVerdict(t *testing.T) {
	a := newJobApp(t)
	a.recordJobs(a.cur(), 4, "q", "a", a.maxToolRunID(a.cur()), time.Second)

	a.RateTurn(4, outcomeGood)
	if got := a.TurnRating(4); got != outcomeGood {
		t.Fatalf("rating = %q, want good", got)
	}
	// Pressing the lit thumb again withdraws it.
	a.RateTurn(4, "")
	if got := a.TurnRating(4); got != outcomeUnknown {
		t.Fatalf("after clearing, rating = %q, want unknown", got)
	}
}

// Asking again is the negative signal that costs the user nothing to give.
func TestRedoMarksTheAttemptBadButNeverOverridesAThumb(t *testing.T) {
	a := newJobApp(t)
	a.recordJobs(a.cur(), 5, "q", "a", a.maxToolRunID(a.cur()), time.Second)
	a.markTurnRedone(5)
	if got := a.TurnRating(5); got != outcomeBad {
		t.Fatalf("after redo, rating = %q, want bad", got)
	}

	b := newJobApp(t)
	b.recordJobs(b.cur(), 6, "q", "a", b.maxToolRunID(b.cur()), time.Second)
	b.RateTurn(6, outcomeGood)
	b.markTurnRedone(6)
	if got := b.TurnRating(6); got != outcomeGood {
		t.Fatalf("a verdict the user gave must survive a regenerate, got %q", got)
	}
}

// A second attempt is a second row against the same bubble, and the rating
// belongs to the answer on screen.
func TestRatingAddressesTheNewestAttempt(t *testing.T) {
	a := newJobApp(t)
	a.recordJobs(a.cur(), 8, "q", "first try", a.maxToolRunID(a.cur()), time.Second)
	a.markTurnRedone(8)
	a.recordJobs(a.cur(), 8, "q", "second try", a.maxToolRunID(a.cur()), time.Second)

	a.RateTurn(8, outcomeGood)
	jobs := readJobs(t, a)
	if len(jobs) != 2 {
		t.Fatalf("both attempts should be kept, got %d rows", len(jobs))
	}
	if jobs[0].outcome != outcomeBad || jobs[0].outcomeSource != "redo" {
		t.Errorf("the attempt that was rejected should stay rejected: %q/%q", jobs[0].outcome, jobs[0].outcomeSource)
	}
	if jobs[1].outcome != outcomeGood {
		t.Errorf("the rating should land on the newest attempt, got %q", jobs[1].outcome)
	}
}

// Deleting a conversation has to mean it in every table that copied it.
func TestDeleteSessionTakesItsJobsWithIt(t *testing.T) {
	a := newJobApp(t)
	a.appendTurn(a.cur(), SessionMessage{Role: "user", Text: "q"}, SessionMessage{Role: "agent", Text: "a"})
	a.recordJobs(a.cur(), 2, "q", "a", a.maxToolRunID(a.cur()), time.Second)
	if len(readJobs(t, a)) == 0 {
		t.Fatal("setup: expected a job row")
	}
	if err := a.DeleteSession(a.cur().id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if jobs := readJobs(t, a); len(jobs) != 0 {
		t.Fatalf("want no jobs after deleting the session, got %d", len(jobs))
	}
}
