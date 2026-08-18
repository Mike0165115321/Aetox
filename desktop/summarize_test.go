package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/learned"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// failShell records one failed run whose error carries a variable part — the
// parenthesized detail differs on every occurrence, as the real refusals do.
func failShell(a *App, ref, detail string) {
	a.recordToolRun(turn.ToolRun{Ref: ref, Name: "shell",
		Args:  `{"command":"type $` + detail + `"}`,
		OK:    false,
		Error: "this command builds a path while it runs (the variable $" + detail + "), so it cannot be checked",
	})
}

// The floor was collecting and nothing read it back: 16 identical shell
// refusals sat in tool_runs while the same mistake was made a 17th time. This
// is the reader — three same-shaped failures become one row on the problems
// page.
func TestRepeatedFailuresBecomeOneIssue(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	for i, detail := range []string{"AlphaVar in cmd one", "BetaVar in cmd two", "GammaVar in cmd three"} {
		failShell(a, "c"+string(rune('1'+i)), detail)
	}
	a.recordJobs(1, "เช็คเวอร์ชัน", "ไม่สำเร็จ", mark, time.Second)

	issues := a.ListSystemIssues()
	if len(issues) != 1 {
		t.Fatalf("want one issue from three same-shaped failures, got %d", len(issues))
	}
	got := issues[0]
	if got.Kind != kindIssue || got.Scope != learned.MainScope {
		t.Errorf("row = kind %q scope %q, want a main-scope issue", got.Kind, got.Scope)
	}
	if got.Source != "summarizer" {
		t.Errorf("source = %q — the system noticing a pattern must not read as the model deciding", got.Source)
	}
	if !strings.Contains(got.Body, "shell") || !strings.Contains(got.Body, "builds a path while it runs") {
		t.Errorf("body should quote the repeating part of the failure: %q", got.Body)
	}
	if strings.Contains(got.Body, "AlphaVar") {
		t.Errorf("the variable detail is what distinguishes occurrences; it must not reach the body: %q", got.Body)
	}
	if !strings.HasPrefix(got.Evidence, "tool_runs:") {
		t.Errorf("evidence should name the rows it was drawn from: %q", got.Evidence)
	}
	if !strings.Contains(got.Reason, "3") {
		t.Errorf("reason should say how often: %q", got.Reason)
	}

	// Nothing reached the memory file: the summarizer reports, never applies.
	if learned.Read(learned.MainScope) != "" {
		t.Fatal("a summarizer row wrote memory without approval")
	}
}

// The split itself (2026-08-18), asserted from the side that was wrong for a
// year: whatever the summarizer finds, the queue the user reads for lessons
// must not move. Seventeen of the twenty-two cards that queue had ever held
// arrived this way, and one of them was a usable lesson.
func TestFailuresNeverReachTheLearningQueue(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	for i, detail := range []string{"AlphaVar in one", "BetaVar in two", "GammaVar in three"} {
		failShell(a, "c"+string(rune('1'+i)), detail)
	}
	a.recordJobs(1, "งาน", "ตอบ", mark, time.Second)

	if len(a.ListSystemIssues()) != 1 {
		t.Fatal("the cluster did not land on the problems page at all")
	}
	if pending := a.ListPendingChanges(); len(pending) != 0 {
		t.Errorf("a failure cluster was offered as something to remember: %+v", pending)
	}
	if n := a.PendingLearnedCount(); n != 0 {
		t.Errorf("the badge counted %d things to decide; a problem is not one of them", n)
	}
	if n := a.PendingIssueCount(); n != 1 {
		t.Errorf("the problems room counted %d, want 1", n)
	}
}

// Reporting is not approving, and the difference is the whole reason these are
// two queues: approving applies a change, reporting applies nothing at all.
func TestReportingAnIssueAppliesNothing(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	for i, detail := range []string{"A in x", "B in y", "C in z"} {
		failShell(a, "c"+string(rune('1'+i)), detail)
	}
	a.recordJobs(1, "งาน", "ตอบ", mark, time.Second)
	issue := a.ListSystemIssues()[0]

	if err := a.ApprovePendingChange(issue.ID); err == nil {
		t.Error("an issue was applied as if it were a lesson — the memory file is not its destination")
	}
	if err := a.MarkIssueReported(issue.ID); err != nil {
		t.Fatalf("report: %v", err)
	}
	if learned.Read(learned.MainScope) != "" {
		t.Error("reporting a problem wrote something into memory")
	}
	if got := a.PendingChangeByID(issue.ID).State; got != stateReported {
		t.Errorf("state = %q, want %q", got, stateReported)
	}
	if len(a.ListSystemIssues()) != 0 {
		t.Error("a reported problem is still waiting to be reported")
	}
	// Twice would file the same cluster twice; the second click is the same
	// click, and the row already said which way it went.
	if err := a.MarkIssueReported(issue.ID); err == nil {
		t.Error("the same problem was reported twice")
	}
}

// Two occurrences are a coincidence, not a pattern — nothing raised.
func TestRareFailuresStayOffTheDoor(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	failShell(a, "c1", "OneVar in x")
	failShell(a, "c2", "TwoVar in y")
	a.recordToolRun(turn.ToolRun{Ref: "c3", Name: "read", OK: false, Error: "no such file"})
	a.recordJobs(1, "งาน", "ตอบ", mark, time.Second)

	if issues := a.ListSystemIssues(); len(issues) != 0 {
		t.Fatalf("two repeats raised a problem: %+v", issues)
	}
}

// The pass runs after every turn over the whole store, so the same cluster is
// seen again and again. One row, however many times it is seen — and a problem
// the user waved off stays waved off instead of coming back every turn.
func TestAClusterIsRaisedOnceAndADecisionStands(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	for i, detail := range []string{"A in x", "B in y", "C in z"} {
		failShell(a, "c"+string(rune('1'+i)), detail)
	}
	a.recordJobs(1, "งาน", "ตอบ", mark, time.Second)

	mark = a.maxToolRunID()
	failShell(a, "c9", "D in w")
	a.recordJobs(2, "งานถัดไป", "ตอบ", mark, time.Second)

	issues := a.ListSystemIssues()
	if len(issues) != 1 {
		t.Fatalf("the growing cluster should still be one row, got %d", len(issues))
	}

	if err := a.RejectPendingChange(issues[0].ID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	mark = a.maxToolRunID()
	failShell(a, "c10", "E in v")
	a.recordJobs(3, "อีกงาน", "ตอบ", mark, time.Second)

	if issues := a.ListSystemIssues(); len(issues) != 0 {
		t.Fatalf("a dismissed problem was raised again: %+v", issues)
	}
}

// A program returning nonzero is not the tool failing. `go test` with failures,
// a grep with no match, a timeout firing — the tool did its job, and there is
// nothing about using it worth telling anyone. Before the run carried where its
// error came from, three of these became a memory line telling the agent to
// avoid whatever hits "exit status 1", which named no pattern and taught nothing.
func TestProgramExitCodesAreNotRaised(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	for i, ref := range []string{"e1", "e2", "e3", "e4"} {
		a.recordToolRun(turn.ToolRun{Ref: ref, Name: "shell",
			Args:      `{"command":"go test ./pkg` + string(rune('a'+i)) + `"}`,
			OK:        false,
			Error:     "exit status 1",
			ErrorKind: turn.ErrorFromProgram,
		})
	}
	a.recordJobs(1, "รันเทสต์", "ตก", mark, time.Second)

	if issues := a.ListSystemIssues(); len(issues) != 0 {
		t.Fatalf("a failing program was raised as a problem with the tool: %+v", issues)
	}
}

// A state report describes a machine at a moment — the n8n that was down
// tonight, the key that expired — and will be false tomorrow. Three of these
// reached the approval queue as permanent-memory cards ("เลี่ยงรูปแบบที่ชน
// เงื่อนไขนี้ตั้งแต่ครั้งแรก" about a server that simply was not running,
// 2026-08-12) before the rows carried where the failure came from.
func TestStateReportsAreNotRaised(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	for i, ref := range []string{"s1", "s2", "s3"} {
		a.recordToolRun(turn.ToolRun{Ref: ref, Name: "browser",
			Args:      `{"action":"open","url":"http://localhost:567` + string(rune('7'+i)) + `"}`,
			OK:        false,
			Error:     "page did not finish loading",
			ErrorKind: turn.ErrorFromWorld,
		})
	}
	a.recordJobs(1, "เปิดหน้าเว็บ", "ไม่ขึ้น", mark, time.Second)

	if issues := a.ListSystemIssues(); len(issues) != 0 {
		t.Fatalf("tonight's outage was raised as a problem to report: %+v", issues)
	}
}

// The report is the sentence, and this codebase's sentences are written to end
// with what to do instead — so a body cut at the grouping key's length loses
// the only half worth reading. The real one lost "…write the path out
// literally", which is exactly the part a developer opening the issue needs.
func TestTheReportKeepsTheWholeSentence(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	const remedy = "write the path out literally, or run the step that needs it as its own command"
	for i, ref := range []string{"r1", "r2", "r3"} {
		a.recordToolRun(turn.ToolRun{Ref: ref, Name: "shell",
			Args: `{"command":"type $VAR"}`,
			OK:   false,
			Error: "this command builds a path while it runs (the variable $V" + string(rune('a'+i)) +
				"), so it cannot be checked against the folders this session may use — " + remedy,
		})
	}
	a.recordJobs(1, "งาน", "ตอบ", mark, time.Second)

	issues := a.ListSystemIssues()
	if len(issues) != 1 {
		t.Fatalf("want one issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Body, remedy) {
		t.Errorf("the report dropped the half that says what happened:\n%q", issues[0].Body)
	}
	// The stranded space a removed (...) span leaves behind.
	if strings.Contains(issues[0].Body, " ,") {
		t.Errorf("orphaned punctuation from the dropped detail: %q", issues[0].Body)
	}
}

// One tool's three ways of failing must not take all three slots — that is how
// a single noisy tool comes to own the memory file.
func TestOneToolCannotFillTheQueue(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	ref := 0
	for _, shape := range []string{"first refusal shape", "second refusal shape", "third refusal shape"} {
		for i := 0; i < 3; i++ {
			ref++
			a.recordToolRun(turn.ToolRun{Ref: "n" + string(rune('a'+ref)), Name: "shell",
				OK:    false,
				Error: shape + ` (detail ` + string(rune('a'+i)) + `)`})
		}
	}
	a.recordJobs(1, "งาน", "ตอบ", mark, time.Second)

	issues := a.ListSystemIssues()
	if len(issues) != 1 {
		t.Fatalf("three clusters from one tool raised %d rows, want 1", len(issues))
	}
}

// A delegate's failures are the delegate's own (ARCHITECTURE §44): the row
// is filed under that agent, not under the main one.
func TestDelegateFailuresLandInTheDelegatesScope(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	a.recordToolRun(turn.ToolRun{Ref: "t1", Name: "task", Args: `{"agent":"explore"}`, Output: "x", OK: true})
	for i, ref := range []string{"k1", "k2", "k3"} {
		a.recordToolRun(turn.ToolRun{Ref: ref, Parent: "t1", Agent: "explore", Name: "grep",
			OK: false, Error: `pattern error near "token ` + string(rune('a'+i)) + `"`})
	}
	a.recordJobs(1, "หาให้หน่อย", "ไม่เจอ", mark, time.Second)

	issues := a.ListSystemIssues()
	if len(issues) != 1 {
		t.Fatalf("want the delegate's cluster as one row, got %d", len(issues))
	}
	if issues[0].Scope != "explore" {
		t.Errorf("scope = %q, want the delegate that failed", issues[0].Scope)
	}
}
