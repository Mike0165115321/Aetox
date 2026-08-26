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

	// The `where` words reach a delegate's tool as ordinary arguments — a
	// model can always send a parameter nobody offered — and every one of
	// them must still land in the delegate's own file. `everywhere` resolves
	// to t.Scope, which for a delegate IS its own scope; `this-project` needs
	// a Project no delegate is ever given.
	for _, where := range []string{"everywhere", "this-project", "this-desk"} {
		if _, err := run(t, tool, map[string]any{"text": "x " + where, "where": where}); err != nil {
			t.Fatalf("add with where=%s: %v", where, err)
		}
	}
	for i, p := range rec.got[1:] {
		if p.Scope != "explore" {
			t.Errorf("where=%q wrote a delegate's line into scope %q — memory crossed an agent boundary", []string{"everywhere", "this-project", "this-desk"}[i], p.Scope)
		}
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

// The same door as the room check above, and the reported bug that opened it
// (18 ส.ค.): the agent proposed "ผู้ใช้เป็นนักพัฒนาระบบ", the user corrected it
// in the next message, and the agent revised the line it had just proposed.
// Nothing had approved that line, so the revision named a line no file held —
// and queued anyway, as a card whose อนุมัติ button could only ever error. The
// user's one way out was ไม่เอา on a fact they had asked for.
func TestARevisionOfSomethingUnrememberedIsRefused(t *testing.T) {
	isolate(t)
	rec := &recorder{}
	tool := &MemoryTool{Scope: MainScope, Proposer: rec}

	msg, err := run(t, tool, map[string]any{
		"op": OpReplace, "old": "นักพัฒนาระบบ", "text": "ผู้ใช้เป็นนักพัฒนา Aetox", "why": "ผู้ใช้แก้ให้"})
	if err == nil {
		t.Fatal("a replace of a line nothing remembers should be refused")
	}
	// Told which of the two things is true, because it changes the next move:
	// the line is not there, so adding it is the op, not revising it.
	if !strings.Contains(msg, "add") {
		t.Errorf("the refusal should point at the op that would work, got %q", msg)
	}
	if _, err := run(t, tool, map[string]any{"op": OpRemove, "old": "นักพัฒนาระบบ"}); err == nil {
		t.Error("removing a line nothing remembers should be refused")
	}
	if len(rec.got) != 0 {
		t.Fatalf("an unappliable proposal reached the queue: %+v", rec.got)
	}

	// And once the line is actually in memory, the same revision goes through.
	if err := Apply(MainScope, OpAdd, "", "ผู้ใช้เป็นนักพัฒนาระบบ (system developer)"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := run(t, tool, map[string]any{
		"op": OpReplace, "old": "นักพัฒนาระบบ", "text": "ผู้ใช้เป็นนักพัฒนา Aetox", "why": "ผู้ใช้แก้ให้"}); err != nil {
		t.Fatalf("replace of a remembered line: %v", err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("the appliable revision did not queue: %+v", rec.got)
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
	// The other half of the principle, and the half that was missing until a
	// user pointed at a chat where they had said who they were and what they
	// had built, and nothing was proposed. The bar is whether a line stays true
	// and matters, never where it came from — and the user's own sentence about
	// themselves is not a claim awaiting corroboration, it is the source.
	for _, phrase := range []string{"tell you about themselves", "already the evidence"} {
		if !strings.Contains(desc, phrase) {
			t.Errorf("description should not make the user's own words wait for evidence, missing %q:\n%s", phrase, desc)
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

// The desk's memory architecture decides where an unsaid `where` lands (§184):
// a shared-first session writes the file every session reads, and the explicit
// words still reach both destinations. An invented word means the default,
// never nowhere.
func TestASharedFirstSessionDefaultsToTheSharedFile(t *testing.T) {
	isolate(t)
	root := filepath.Join(t.TempDir(), "Aetox")
	rec := &recorder{}
	tool := &MemoryTool{Scope: MainScope, Project: root, Proposer: rec}

	for _, args := range []map[string]any{
		{"text": "ผู้ใช้ชอบคำตอบสั้น", "why": "บอกไว้"},
		{"text": "ที่นี่ตกลงกันว่าใช้ PowerShell", "where": "this-project"},
		{"text": "เครื่องนี้ไม่มี Excel", "where": "everywhere"},
		// A word the model invented must land where an unsaid word would have,
		// rather than nowhere.
		{"text": "อีกอัน", "where": "this-folder"},
	} {
		if _, err := tool.ExecuteTool(context.Background(), args); err != nil {
			t.Fatalf("add %v: %v", args, err)
		}
	}
	if len(rec.got) != 4 {
		t.Fatalf("want four proposals, got %d", len(rec.got))
	}
	want := []string{MainScope, ProjectScope(root), MainScope, MainScope}
	for i, w := range want {
		if rec.got[i].Scope != w {
			t.Errorf("proposal %d went to scope %q, want %q", i, rec.got[i].Scope, w)
		}
	}
}

// A project-first session — โต๊ะโค้ด with a repository open — lands an unsaid
// `where` in the project's own file, because its work is settling things and a
// decision carried into the next repository arrives as advice (§116). The
// shared file stays one explicit word away, because "this machine has no
// Excel" is still true everywhere and still discovered while coding.
func TestAProjectFirstSessionDefaultsToTheProjectsOwnFile(t *testing.T) {
	isolate(t)
	root := filepath.Join(t.TempDir(), "Aetox")
	rec := &recorder{}
	tool := &MemoryTool{Scope: MainScope, Project: root, ProjectFirst: true, Proposer: rec}

	for _, args := range []map[string]any{
		{"text": "ที่นี่ตกลงกันว่า package นี้ถือ retry", "why": "ตัดสินใจกันในงานนี้"},
		{"text": "เครื่องนี้ไม่มี Excel", "where": "everywhere"},
		// An invented word means the desk's default — here, the project.
		{"text": "อีกอัน", "where": "this-folder"},
	} {
		if _, err := tool.ExecuteTool(context.Background(), args); err != nil {
			t.Fatalf("add %v: %v", args, err)
		}
	}
	if len(rec.got) != 3 {
		t.Fatalf("want three proposals, got %d", len(rec.got))
	}
	want := []string{ProjectScope(root), MainScope, ProjectScope(root)}
	for i, w := range want {
		if rec.got[i].Scope != w {
			t.Errorf("proposal %d went to scope %q, want %q", i, rec.got[i].Scope, w)
		}
	}

	// The parameter names the project so "this-project" means something the
	// model can check, and says which destination the unsaid word means.
	params := string(tool.ToolDefinition().Function.Parameters)
	for _, want := range []string{"this-project (default)", "Aetox"} {
		if !strings.Contains(params, want) {
			t.Errorf("the tool block does not say %q:\n%s", want, params)
		}
	}
}

// A project-first desk with no project focused has nowhere narrower to write:
// the rule goes inert and the session behaves exactly like the pre-desks one —
// shared destination, no `where` parameter at all. The alternative was a junk
// drawer every unfocused session shared.
func TestProjectFirstWithoutAProjectFallsBackToShared(t *testing.T) {
	isolate(t)
	rec := &recorder{}
	tool := &MemoryTool{Scope: MainScope, ProjectFirst: true, Proposer: rec}

	if _, err := run(t, tool, map[string]any{"text": "ผู้ใช้ชอบคำตอบสั้น"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if rec.got[0].Scope != MainScope {
		t.Errorf("an unfocused project-first session proposed into %q, want the shared file", rec.got[0].Scope)
	}
	if strings.Contains(string(tool.ToolDefinition().Function.Parameters), "where") {
		t.Errorf("a session with one destination was sent a choice:\n%s", tool.ToolDefinition().Function.Parameters)
	}
}

// The `where` parameter exists only when a project is focused, and never
// offers a desk: which file an unqualified line lands in is the desk's own
// architecture, not a destination the model picks (§184). The tool block
// rides in every request, so an option nobody can use is a bill with no
// benefit.
func TestTheWhereParameterOffersOnlyWhatThisSessionHas(t *testing.T) {
	isolate(t)
	root := filepath.Join(t.TempDir(), "Aetox")

	focused := string((&MemoryTool{Scope: MainScope, Project: root}).ToolDefinition().Function.Parameters)
	if !strings.Contains(focused, "this-project") {
		t.Errorf("a focused session was not offered its project:\n%s", focused)
	}
	if strings.Contains(focused, "this-desk") {
		t.Errorf("a desk is an architecture, not a destination — it must not be offered:\n%s", focused)
	}
	bare := string((&MemoryTool{Scope: MainScope}).ToolDefinition().Function.Parameters)
	if strings.Contains(bare, "where") {
		t.Errorf("a session with one destination was sent a choice:\n%s", bare)
	}
}
