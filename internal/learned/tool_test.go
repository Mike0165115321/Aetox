package learned

import (
	"context"
	"path/filepath"
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

// The id of the queued row rides back on the output. Without it the chat can
// say only that something called "memory" ran: what it wants to remember, and
// the decision it is waiting for, then live on a Settings page the user has no
// reason to be looking at.
func TestTheReceiptCarriesTheQueuedProposal(t *testing.T) {
	isolate(t)
	rec := &recorder{}
	tool := &MemoryTool{Scope: MainScope, Proposer: rec}

	out, err := tool.ExecuteTool(context.Background(), map[string]any{"text": "เครื่องนี้ไม่มี Excel ติดตั้ง"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if out.ProposalID != 1 {
		t.Errorf("ProposalID = %d, want the id the door handed back", out.ProposalID)
	}

	// A second attempt at the same line is answered with the row already
	// waiting — the card under this answer is about that proposal, not nothing.
	rec.duplicate = true
	dup, err := tool.ExecuteTool(context.Background(), map[string]any{"text": "เครื่องนี้ไม่มี Excel ติดตั้ง"})
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if dup.ProposalID == 0 {
		t.Error("a duplicate lost its id, so the answer that proposed it can show nothing")
	}

	// A refusal proposes nothing, so there is nothing to draw.
	bad, err := tool.ExecuteTool(context.Background(), map[string]any{"op": OpAdd})
	if err == nil {
		t.Fatal("add without text should be refused")
	}
	if bad.ProposalID != 0 {
		t.Errorf("a refused call reported proposal %d", bad.ProposalID)
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

// A session at a desk can write to two scopes through one tool, and which one
// it lands in is the model's call — only it knows whether a fact is about the
// user or about this kind of work (ARCHITECTURE.md §83).
func TestADeskSessionCanRememberInEitherScope(t *testing.T) {
	isolate(t)
	rec := &recorder{}
	tool := &MemoryTool{Scope: MainScope, Desk: "coding", Proposer: rec}

	if _, err := run(t, tool, map[string]any{"text": "ผู้ใช้ชอบคำตอบสั้น", "why": "บอกไว้"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"text": "repo นี้เทสด้วยสคริปต์", "where": "this-desk", "why": "เห็นตอนรัน"}); err != nil {
		t.Fatalf("add to desk: %v", err)
	}
	if len(rec.got) != 2 {
		t.Fatalf("want two proposals, got %d", len(rec.got))
	}
	if rec.got[0].Scope != MainScope {
		t.Errorf("the default scope is %q, want the shared file — a guess must land where it always did", rec.got[0].Scope)
	}
	if rec.got[1].Scope != ModeScope("coding") {
		t.Errorf("this-desk proposed into %q, want the desk's own scope", rec.got[1].Scope)
	}

	// Anything but the one word means the shared file, including a word the
	// model invented.
	if _, err := run(t, tool, map[string]any{"text": "อีกอัน", "where": "somewhere-else"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if rec.got[2].Scope != MainScope {
		t.Errorf("an unrecognised where landed in %q, want the shared file", rec.got[2].Scope)
	}
}

// The third destination. What makes it worth its own scope is that the desk
// cannot stand in for it: โต๊ะโค้ด is the same desk in every repository, so a
// decision kept there would arrive as advice in the next project.
func TestASessionInAProjectCanKeepADecisionWhereItIsTrue(t *testing.T) {
	isolate(t)
	root := filepath.Join(t.TempDir(), "Aetox")
	rec := &recorder{}
	tool := &MemoryTool{Scope: MainScope, Desk: "coding", Project: root, Proposer: rec}

	for _, args := range []map[string]any{
		{"text": "ผู้ใช้ชอบคำตอบสั้น"},
		{"text": "เจ้าของอ่านดิฟก่อนเสมอ", "where": "this-desk"},
		{"text": "ที่นี่ตกลงกันว่าใช้ PowerShell", "where": "this-project"},
		// A word the model invented, and the word for a destination this session
		// does have — both must land in the shared file rather than nowhere.
		{"text": "อีกอัน", "where": "this-folder"},
	} {
		if _, err := tool.ExecuteTool(context.Background(), args); err != nil {
			t.Fatalf("add %v: %v", args, err)
		}
	}
	if len(rec.got) != 4 {
		t.Fatalf("want four proposals, got %d", len(rec.got))
	}
	want := []string{MainScope, ModeScope("coding"), ProjectScope(root), MainScope}
	for i, w := range want {
		if rec.got[i].Scope != w {
			t.Errorf("proposal %d went to scope %q, want %q", i, rec.got[i].Scope, w)
		}
	}

	// The parameter offers only what this session actually has, and names the
	// project so "this-project" means something the model can check.
	def := tool.ToolDefinition()
	params := string(def.Function.Parameters)
	for _, want := range []string{"this-desk", "this-project", "Aetox"} {
		if !strings.Contains(params, want) {
			t.Errorf("the tool block does not offer %q:\n%s", want, params)
		}
	}
}

// A session with a project but no desk offers the project and not the desk, and
// the other way round. The tool block rides in every request, so an option that
// cannot be used is a bill with no benefit.
func TestTheWhereParameterOffersOnlyWhatThisSessionHas(t *testing.T) {
	isolate(t)
	root := filepath.Join(t.TempDir(), "Aetox")

	projectOnly := string((&MemoryTool{Scope: MainScope, Project: root}).ToolDefinition().Function.Parameters)
	if !strings.Contains(projectOnly, "this-project") || strings.Contains(projectOnly, "this-desk") {
		t.Errorf("a session with no desk was offered one:\n%s", projectOnly)
	}
	deskOnly := string((&MemoryTool{Scope: MainScope, Desk: "coding"}).ToolDefinition().Function.Parameters)
	if !strings.Contains(deskOnly, "this-desk") || strings.Contains(deskOnly, "this-project") {
		t.Errorf("a session with no project was offered one:\n%s", deskOnly)
	}
	bare := string((&MemoryTool{Scope: MainScope}).ToolDefinition().Function.Parameters)
	if strings.Contains(bare, "where") {
		t.Errorf("a session with one destination was sent a choice:\n%s", bare)
	}
}

// Every session that is not at a desk — a delegate, and everything from before
// desks existed — must send the tool block it always did. The definition is in
// every request, so a parameter nobody can use is a bill with no benefit.
func TestTheMemoryToolGainsNothingWithoutADesk(t *testing.T) {
	isolate(t)
	plain := (&MemoryTool{Scope: MainScope}).ToolDefinition()
	if strings.Contains(string(plain.Function.Parameters), "where") {
		t.Errorf("a session with no desk was sent the desk parameter: %s", plain.Function.Parameters)
	}
	desked := (&MemoryTool{Scope: MainScope, Desk: "coding"}).ToolDefinition()
	if !strings.Contains(string(desked.Function.Parameters), "where") {
		t.Errorf("a session at a desk cannot say which scope it means: %s", desked.Function.Parameters)
	}
	if !strings.Contains(string(desked.Function.Parameters), "coding") {
		t.Error("the parameter does not name the desk, so the model cannot tell what this-desk means")
	}
}
