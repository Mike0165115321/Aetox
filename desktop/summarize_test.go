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
// is the reader — three same-shaped failures become one card at the door.
func TestRepeatedFailuresBecomeOneProposal(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	for i, detail := range []string{"AlphaVar in cmd one", "BetaVar in cmd two", "GammaVar in cmd three"} {
		failShell(a, "c"+string(rune('1'+i)), detail)
	}
	a.recordJobs(1, "เช็คเวอร์ชัน", "ไม่สำเร็จ", mark, time.Second)

	pending := a.ListPendingChanges()
	if len(pending) != 1 {
		t.Fatalf("want one card from three same-shaped failures, got %d", len(pending))
	}
	got := pending[0]
	if got.Kind != "memory" || got.Scope != learned.MainScope {
		t.Errorf("card = kind %q scope %q, want a main-scope memory", got.Kind, got.Scope)
	}
	if got.Source != "summarizer" {
		t.Errorf("source = %q — the system noticing a pattern must not read as the model deciding", got.Source)
	}
	if !strings.Contains(got.Body, "shell") || !strings.Contains(got.Body, "builds a path while it runs") {
		t.Errorf("body should quote the repeating part of the refusal: %q", got.Body)
	}
	if strings.Contains(got.Body, "AlphaVar") {
		t.Errorf("the variable detail is what distinguishes occurrences; it must not reach the lesson: %q", got.Body)
	}
	if !strings.HasPrefix(got.Evidence, "tool_runs:") {
		t.Errorf("evidence should name the rows it was drawn from: %q", got.Evidence)
	}
	if !strings.Contains(got.Reason, "3") {
		t.Errorf("reason should say how often: %q", got.Reason)
	}

	// Nothing reached the memory file: the summarizer proposes, never applies.
	if learned.Read(learned.MainScope) != "" {
		t.Fatal("a summarizer proposal wrote memory without approval")
	}
}

// Two occurrences are a coincidence, not a pattern — no card.
func TestRareFailuresStayOffTheDoor(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	failShell(a, "c1", "OneVar in x")
	failShell(a, "c2", "TwoVar in y")
	a.recordToolRun(turn.ToolRun{Ref: "c3", Name: "read", OK: false, Error: "no such file"})
	a.recordJobs(1, "งาน", "ตอบ", mark, time.Second)

	if pending := a.ListPendingChanges(); len(pending) != 0 {
		t.Fatalf("two repeats proposed a lesson: %+v", pending)
	}
}

// The pass runs after every turn over the whole store, so the same cluster is
// seen again and again. One card, however many times it is seen — and a card
// the user rejected stays rejected instead of coming back every turn.
func TestAClusterIsProposedOnceAndADecisionStands(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	for i, detail := range []string{"A in x", "B in y", "C in z"} {
		failShell(a, "c"+string(rune('1'+i)), detail)
	}
	a.recordJobs(1, "งาน", "ตอบ", mark, time.Second)

	mark = a.maxToolRunID()
	failShell(a, "c9", "D in w")
	a.recordJobs(2, "งานถัดไป", "ตอบ", mark, time.Second)

	pending := a.ListPendingChanges()
	if len(pending) != 1 {
		t.Fatalf("the growing cluster should still be one card, got %d", len(pending))
	}

	if err := a.RejectPendingChange(pending[0].ID); err != nil {
		t.Fatalf("reject: %v", err)
	}
	mark = a.maxToolRunID()
	failShell(a, "c10", "E in v")
	a.recordJobs(3, "อีกงาน", "ตอบ", mark, time.Second)

	if pending := a.ListPendingChanges(); len(pending) != 0 {
		t.Fatalf("a rejected lesson was proposed again: %+v", pending)
	}
}

// A delegate's failures are the delegate's lesson (ARCHITECTURE §44): the card
// targets that agent's file, not the main agent's prompt.
func TestDelegateFailuresLandInTheDelegatesScope(t *testing.T) {
	a := newJobApp(t)
	mark := a.maxToolRunID()
	a.recordToolRun(turn.ToolRun{Ref: "t1", Name: "task", Args: `{"agent":"explore"}`, Output: "x", OK: true})
	for i, ref := range []string{"k1", "k2", "k3"} {
		a.recordToolRun(turn.ToolRun{Ref: ref, Parent: "t1", Agent: "explore", Name: "grep",
			OK: false, Error: `pattern error near "token ` + string(rune('a'+i)) + `"`})
	}
	a.recordJobs(1, "หาให้หน่อย", "ไม่เจอ", mark, time.Second)

	pending := a.ListPendingChanges()
	if len(pending) != 1 {
		t.Fatalf("want the delegate's cluster as one card, got %d", len(pending))
	}
	if pending[0].Scope != "explore" {
		t.Errorf("scope = %q, want the delegate that failed", pending[0].Scope)
	}
}
