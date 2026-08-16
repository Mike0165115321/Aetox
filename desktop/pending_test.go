package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/learned"
)

func proposal(scope, op, before, body string) learned.Proposal {
	return learned.Proposal{Kind: "memory", Scope: scope, Op: op, Before: before, Body: body, Reason: "เห็นซ้ำสองรอบ"}
}

// Nothing an agent proposes may take effect on its own. This is the guarantee
// the design rests on, so it is checked against the memory file rather than
// against the queue.
func TestAProposalDoesNothingUntilApproved(t *testing.T) {
	a := newJobApp(t)

	if _, err := a.proposeLearned(proposal(learned.MainScope, learned.OpAdd, "", "เครื่องนี้ไม่มี Excel")); err != nil {
		t.Fatalf("propose: %v", err)
	}
	if got := learned.Read(learned.MainScope); got != "" {
		t.Fatalf("memory changed before approval: %q", got)
	}
	pending := a.ListPendingChanges()
	if len(pending) != 1 || a.PendingLearnedCount() != 1 {
		t.Fatalf("want one proposal waiting, got %d", len(pending))
	}
	if pending[0].Reason == "" || pending[0].Target == "" {
		t.Errorf("review needs the reasoning and the file it would land on: %+v", pending[0])
	}

	if err := a.ApprovePendingChange(pending[0].ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got := learned.Read(learned.MainScope); !strings.Contains(got, "Excel") {
		t.Fatalf("approval did not reach the file, got %q", got)
	}
	if a.PendingLearnedCount() != 0 {
		t.Error("an approved proposal should leave the queue")
	}
}

// The row is the audit trail. Deleting it on approval would leave a line in a
// memory file with nothing anywhere saying where it came from.
func TestDecidedProposalsAreKeptAsTheRecord(t *testing.T) {
	a := newJobApp(t)
	keep, _ := a.proposeLearned(proposal(learned.MainScope, learned.OpAdd, "", "จำอันนี้"))
	drop, _ := a.proposeLearned(proposal(learned.MainScope, learned.OpAdd, "", "ไม่ต้องจำอันนี้"))

	if err := a.ApprovePendingChange(keep.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := a.RejectPendingChange(drop.ID); err != nil {
		t.Fatalf("reject: %v", err)
	}

	decided := a.ListDecidedChanges(10)
	if len(decided) != 2 {
		t.Fatalf("want both decisions on the record, got %d", len(decided))
	}
	states := map[string]string{}
	for _, c := range decided {
		states[c.Body] = c.State
		if c.DecidedAt == "" {
			t.Errorf("a decided proposal should carry when it was decided: %+v", c)
		}
	}
	if states["จำอันนี้"] != stateApproved || states["ไม่ต้องจำอันนี้"] != stateRejected {
		t.Errorf("states not recorded as decided: %v", states)
	}
	if got := learned.Read(learned.MainScope); strings.Contains(got, "ไม่ต้องจำ") {
		t.Error("a rejected proposal must not reach the file")
	}
}

// The agent cannot see the queue — approved memory only reaches it next
// session — so proposing the same thing twice in one conversation is what a
// model that does not know it already asked will do.
func TestTheSameProposalTwiceIsNotQueuedTwice(t *testing.T) {
	a := newJobApp(t)
	first, err := a.proposeLearned(proposal(learned.MainScope, learned.OpAdd, "", "ซ้ำ"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	second, err := a.proposeLearned(proposal(learned.MainScope, learned.OpAdd, "", "ซ้ำ"))
	if err != nil {
		t.Fatalf("propose again: %v", err)
	}
	if !second.Duplicate || second.ID != first.ID {
		t.Errorf("want the same row reported as a duplicate, got %+v", second)
	}
	if n := a.PendingLearnedCount(); n != 1 {
		t.Errorf("want one row, got %d", n)
	}
}

// The card in the chat reads its own row by id, and it has to read it in every
// state. A card that could only see pending rows would go blank the moment the
// user answered it, and a session reopened after a decision made in Settings
// would sit there asking a question that has already been answered.
func TestOneProposalCanBeReadBackInWhateverStateItIsIn(t *testing.T) {
	a := newJobApp(t)
	p, err := a.proposeLearned(proposal(learned.MainScope, learned.OpAdd, "", "เครื่องนี้ไม่มี Excel"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}

	waiting := a.PendingChangeByID(p.ID)
	if waiting.ID != p.ID || waiting.State != statePending {
		t.Fatalf("want the row as proposed, got %+v", waiting)
	}
	if waiting.Body == "" || waiting.Reason == "" {
		t.Errorf("the card cannot be judged without the line and the reasoning: %+v", waiting)
	}

	if err := a.ApprovePendingChange(p.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if decided := a.PendingChangeByID(p.ID); decided.State != stateApproved {
		t.Errorf("state = %q, want the card to know it was already decided", decided.State)
	}

	// Nothing to draw rather than an empty card offering a decision on a row
	// that is not there.
	if missing := a.PendingChangeByID(p.ID + 999); missing.ID != 0 {
		t.Errorf("a row that does not exist came back as %+v", missing)
	}
}

// Deciding something twice is a stale UI clicking an old button, and it must
// not apply the change a second time.
func TestADecisionCannotBeMadeTwice(t *testing.T) {
	a := newJobApp(t)
	p, _ := a.proposeLearned(proposal(learned.MainScope, learned.OpAdd, "", "ครั้งเดียว"))
	if err := a.ApprovePendingChange(p.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := a.ApprovePendingChange(p.ID); err == nil {
		t.Error("approving twice should be refused")
	}
	if err := a.RejectPendingChange(p.ID); err == nil {
		t.Error("rejecting something already approved should be refused")
	}
	if got := learned.Read(learned.MainScope); strings.Count(got, "ครั้งเดียว") != 1 {
		t.Errorf("the line should appear once, got %q", got)
	}
}

// A change that cannot be applied leaves the queue untouched: a row marked
// approved whose change never landed is a lie in the one place built to be
// trusted.
func TestAFailedApplyDoesNotMarkTheProposalApproved(t *testing.T) {
	a := newJobApp(t)
	p, _ := a.proposeLearned(proposal(learned.MainScope, learned.OpReplace, "บรรทัดที่ไม่มีอยู่", "ใหม่"))
	if err := a.ApprovePendingChange(p.ID); err == nil {
		t.Fatal("replacing a line that does not exist should fail")
	}
	if n := a.PendingLearnedCount(); n != 1 {
		t.Errorf("the proposal should still be waiting, count = %d", n)
	}
}

// Scope is carried through to the file, so a delegate's memory lands in the
// delegate's file and nowhere near the main agent's prompt.
func TestScopeDecidesWhichFileAnApprovalReaches(t *testing.T) {
	a := newJobApp(t)
	p, _ := a.proposeLearned(proposal("explore", learned.OpAdd, "", "fixture อยู่ใน testdata/"))
	if err := a.ApprovePendingChange(p.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got := learned.Read("explore"); !strings.Contains(got, "testdata") {
		t.Errorf("delegate memory not written: %q", got)
	}
	if got := learned.Read(learned.MainScope); got != "" {
		t.Errorf("a delegate's memory must not land in the main agent's: %q", got)
	}
}

// Off means the door is shut, not that proposals pile up unseen.
func TestLearningOffRefusesProposals(t *testing.T) {
	a := newJobApp(t)
	pref, _, _ := config.LoadModelPreference()
	pref.LearningDisabled = true
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatalf("save preference: %v", err)
	}
	if _, err := a.proposeLearned(proposal(learned.MainScope, learned.OpAdd, "", "x")); err == nil {
		t.Fatal("with learning off, proposing should fail")
	}
	if n := a.PendingLearnedCount(); n != 0 {
		t.Errorf("nothing should have been queued, got %d", n)
	}
}

// The memory tool has to be told which project this is, and the answer must be
// the project being opened rather than the one being left.
//
// Found by running the real app (16 ส.ค.): the very first proposal ever made in
// a project was filed against "aetox" — the unfocused home folder the session
// had started in — while the status bar said ALM-X-IMPACT-Tennis. applyConfig
// builds the workbench tools at the top and assigns a.cfg at the bottom, and
// focusProject sets projectFocused before either, so "focused" was true while
// the root was still the previous one. Every scope Aetox has is only as good as
// the thing that decides it, and this decided it one project late.
func TestTheMemoryToolIsBoundToTheProjectBeingOpened(t *testing.T) {
	a := newJobApp(t)
	// The state applyConfig is in mid-switch: the flag already moved, a.cfg has
	// not, and the new root is only in the config being applied.
	a.projectFocused = true
	a.cfg.SandboxRoot = filepath.Join(t.TempDir(), "the-project-being-left")
	incoming := filepath.Join(t.TempDir(), "the-project-being-opened")

	var tool *learned.MemoryTool
	for _, s := range a.workbenchSkills(incoming) {
		if m, ok := s.(*learned.MemoryTool); ok {
			tool = m
		}
	}
	if tool == nil {
		t.Fatal("the workbench offers no memory tool")
	}
	if tool.Project != incoming {
		t.Errorf("memory is bound to %q, want the project being opened %q", tool.Project, incoming)
	}

	// And an unfocused session has no project at all — the home folder it is
	// rooted at is not one, however real a folder it is.
	a.projectFocused = false
	for _, s := range a.workbenchSkills(incoming) {
		if m, ok := s.(*learned.MemoryTool); ok && m.Project != "" {
			t.Errorf("an unfocused session was offered project %q", m.Project)
		}
	}
}
