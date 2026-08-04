package learned

import (
	"context"
	"strings"
	"testing"
)

type recorder struct {
	got       []Proposal
	duplicate bool
}

func (r *recorder) Propose(p Proposal) (Result, error) {
	r.got = append(r.got, p)
	return Result{ID: int64(len(r.got)), Duplicate: r.duplicate}, nil
}

func run(t *testing.T, tool *MemoryTool, args map[string]any) (string, error) {
	t.Helper()
	out, err := tool.ExecuteTool(context.Background(), args)
	return out.Content, err
}

// The tool proposes; it never writes. This is the guarantee the whole approval
// design rests on, so it is asserted against the disk rather than against the
// proposal list.
func TestTheToolNeverTouchesTheDisk(t *testing.T) {
	isolate(t)
	rec := &recorder{}
	tool := &MemoryTool{Scope: MainScope, Proposer: rec}

	if _, err := run(t, tool, map[string]any{"text": "เครื่องนี้ไม่มี Excel ติดตั้ง", "why": "ลองเปิดแล้วไม่มี"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("want one proposal, got %d", len(rec.got))
	}
	if got := Read(MainScope); got != "" {
		t.Fatalf("nothing may reach memory before approval, found %q", got)
	}
	if rec.got[0].Op != OpAdd || rec.got[0].Reason == "" {
		t.Errorf("proposal lost its op or its reasoning: %+v", rec.got[0])
	}
}

// Scope is set by construction, not by an argument — a delegate has no way to
// name a scope other than its own.
func TestScopeIsNotSomethingTheModelCanChoose(t *testing.T) {
	isolate(t)
	rec := &recorder{}
	tool := &MemoryTool{Scope: "explore", Proposer: rec}

	if _, err := run(t, tool, map[string]any{"text": "x", "scope": "main"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if rec.got[0].Scope != "explore" {
		t.Fatalf("scope = %q; a delegate must only ever write its own", rec.got[0].Scope)
	}
}

func TestEachOpDemandsWhatItNeeds(t *testing.T) {
	isolate(t)
	tool := &MemoryTool{Scope: MainScope, Proposer: &recorder{}}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"add without text", map[string]any{"op": OpAdd}},
		{"replace without old", map[string]any{"op": OpReplace, "text": "x"}},
		{"replace without text", map[string]any{"op": OpReplace, "old": "x"}},
		{"remove without old", map[string]any{"op": OpRemove}},
		{"an op that does not exist", map[string]any{"op": "forget-everything", "text": "x"}},
	} {
		if _, err := run(t, tool, tc.args); err == nil {
			t.Errorf("%s should be refused", tc.name)
		}
	}
}

// Refused at the moment the agent writes it, not at approval: a queue full of
// proposals that cannot be applied looks like progress to the user and teaches
// the agent nothing.
func TestAFullScopeRefusesTheProposalNotTheApproval(t *testing.T) {
	isolate(t)
	line := strings.Repeat("ก", 500)
	for i := 0; i < 40; i++ {
		if err := Apply(MainScope, OpAdd, "", line+string(rune('a'+i))); err != nil {
			break
		}
	}
	rec := &recorder{}
	tool := &MemoryTool{Scope: MainScope, Proposer: rec}
	msg, err := run(t, tool, map[string]any{"text": line})
	if err == nil {
		t.Fatal("a full scope should refuse an add")
	}
	if !strings.Contains(msg, "full") {
		t.Errorf("the refusal should say why, got %q", msg)
	}
	if len(rec.got) != 0 {
		t.Errorf("nothing should have been queued, got %d", len(rec.got))
	}
}

// The model is told the truth about what happened: queued, already queued, or
// unavailable. Reporting success for all three is how a model ends up
// proposing the same line every turn forever.
func TestTheAnswerDistinguishesQueuedFromAlreadyQueued(t *testing.T) {
	isolate(t)
	rec := &recorder{duplicate: true}
	tool := &MemoryTool{Scope: MainScope, Proposer: rec}
	msg, err := run(t, tool, map[string]any{"text": "x"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(strings.ToLower(msg), "already") {
		t.Errorf("a duplicate should say so, got %q", msg)
	}

	none := &MemoryTool{Scope: MainScope}
	if _, err := run(t, none, map[string]any{"text": "x"}); err == nil {
		t.Error("with no approval door the tool must fail rather than pretend")
	}
}

// The description has to teach what belongs in memory as a principle. A list
// of forbidden topics answers the failures someone remembered and nothing
// else; what generalises is that a kept line is paid for on every future
// request, so it has to still be true and still change what the agent does.
func TestTheDescriptionTeachesTheCostNotACaseList(t *testing.T) {
	def := (&MemoryTool{}).ToolDefinition()
	desc := def.Function.Description
	for _, phrase := range []string{"still be true", "costs context on every request"} {
		if !strings.Contains(desc, phrase) {
			t.Errorf("description should state the principle %q:\n%s", phrase, desc)
		}
	}
	// A word-trigger in a tool description is routing that beats any prompt
	// principle — the tool must describe capability, never claim a phrase.
	for _, trigger := range []string{"whenever the user says", "if the user asks you to remember"} {
		if strings.Contains(strings.ToLower(desc), trigger) {
			t.Errorf("description claims a phrase instead of stating capability: %q", trigger)
		}
	}
}
