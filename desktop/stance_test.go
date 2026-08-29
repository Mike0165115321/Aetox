package main

// What a stance does to a real engine (DECISIONS.md §106).
//
// internal/mode has its own unit tests and they prove that คู่คิด withholds
// what it says it withholds. None of that would catch the failure this file is
// about: a stance that is perfectly correct and never reaches the dispatcher
// the session runs on, or that reaches it and takes the conversation with it.
// So every assertion here goes through the app — the engine the window builds
// — and asks what the *model* would be sent.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/mode"
	"github.com/Mikedev115/Aetox/internal/model"
)

// The headline. คู่คิด means no tool definitions at all, and the measurement is
// the point of the stance rather than a side effect of it.
func TestConsultSendsTheModelNoToolsAtAll(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	if len(toolNames(a)) == 0 {
		t.Fatal("test is stale: the assistant desk sent no tools even at ลงมือ")
	}

	if _, err := a.SetStance(string(mode.StanceConsult)); err != nil {
		t.Fatalf("SetStance: %v", err)
	}
	if got := toolNames(a); len(got) != 0 {
		t.Errorf("คู่คิด must send no tool definitions, got %d: %v", len(got), got)
	}
}

// The whole difference between a stance and a desk, as a test. A desk is fixed
// for a session's life because switching it would put another desk's tools in
// this context; a stance only subtracts, so it moves in place — and the
// conversation has to survive the re-bootstrap that does it.
func TestSwitchingStanceKeepsTheConversation(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	a.cur().agent.RestoreHistory([]model.Message{
		{Role: "user", Content: "จำเลข 4127 ไว้นะ"},
		{Role: "assistant", Content: "จำแล้วครับ"},
	})
	before := len(a.cur().agent.ContextMessages())
	if before == 0 {
		t.Fatal("test is stale: RestoreHistory left the agent with no context")
	}

	if _, err := a.SetStance(string(mode.StanceConsult)); err != nil {
		t.Fatalf("SetStance: %v", err)
	}
	after := a.cur().agent.ContextMessages()
	if len(after) != before {
		t.Fatalf("the conversation must survive a stance switch: %d messages before, %d after", before, len(after))
	}
	var joined strings.Builder
	for _, m := range after {
		joined.WriteString(m.Content)
	}
	if !strings.Contains(joined.String(), "4127") {
		t.Error("the carried context lost what was said before the switch")
	}
}

// And back again. A control you can enter but not leave is not a switch
// (§106.3), so the return trip has to restore the desk exactly.
func TestReturningToTheDefaultRestoresTheDesk(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	full := toolNames(a)

	if _, err := a.SetStance(string(mode.StanceConsult)); err != nil {
		t.Fatalf("SetStance: %v", err)
	}
	if _, err := a.SetStance(string(mode.StanceAct)); err != nil {
		t.Fatalf("SetStance back: %v", err)
	}

	back := toolNames(a)
	if len(back) != len(full) {
		t.Fatalf("returning to ลงมือ must restore the desk: %d tools before, %d after", len(full), len(back))
	}
	for i := range full {
		if full[i] != back[i] {
			t.Fatalf("the desk came back different at %d: %q vs %q", i, full[i], back[i])
		}
	}
}

// A stance may only subtract. Nothing in the app should be able to hand back a
// tool the desk withheld — the property COMPANY.md §6.3 rests on once the dial
// can move inside a live session.
func TestAStanceNeverWidensTheDesk(t *testing.T) {
	// The specialized desk has no shell and no code tools; whatever stance it
	// is put into, it must not acquire any.
	a := bootDeskApp(t, mode.Office)
	for _, s := range mode.Stances() {
		if _, err := a.SetStance(s.String()); err != nil {
			t.Fatalf("SetStance(%q): %v", s, err)
		}
		for _, name := range toolNames(a) {
			if name == "shell" || name == "diagnostics" {
				t.Errorf("stance %q handed the office %q — a stance must never widen a desk", s, name)
			}
		}
	}
}

// The tools panel and the model must be shown one list, not two readings of
// the rules. This is the drift AttendedRegistry's own comment warns about,
// arriving one axis later: deskTools is a second place that answers "what does
// this session carry".
func TestTheToolsPanelAgreesWithTheModel(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	if _, err := a.SetStance(string(mode.StanceConsult)); err != nil {
		t.Fatalf("SetStance: %v", err)
	}
	if names := a.deskTools().Names(); len(names) != 0 {
		// Names() includes skills, which every stance keeps on purpose — so
		// this asserts on the tool definitions the panel would draw instead.
		for _, n := range names {
			if source, ok := a.cur().registry.SourceOf(n); ok && source == "skill" {
				continue
			}
			t.Errorf("the tools panel still lists %q while the model has been sent nothing", n)
		}
	}
}

// A new chat starts at ลงมือ. Coming back to a blank conversation that quietly
// carries no tools is the worst version of this feature: nothing works, and the
// screen has already stopped explaining why.
func TestANewSessionStartsAtTheDefaultStance(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	if _, err := a.SetStance(string(mode.StanceConsult)); err != nil {
		t.Fatalf("SetStance: %v", err)
	}
	a.startNewSession()
	if a.cur().stance != mode.StanceAct {
		t.Fatalf("a new chat must start at ลงมือ, got %q", a.cur().stance)
	}
	if len(toolNames(a)) == 0 {
		t.Error("the field was reset but the engine was not rebuilt — the new chat is still carrying nothing")
	}
}

// The picker's list comes from the engine, so the two cannot disagree about
// which stances exist.
func TestTheAppOffersExactlyTheStancesTheEngineImplements(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	got, want := a.Stances(), mode.Stances()
	if len(got) != len(want) {
		t.Fatalf("App.Stances() offered %d, engine implements %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i].String() {
			t.Errorf("stance %d: app says %q, engine says %q", i, got[i], want[i])
		}
	}
}

// A name this build does not implement comes back as ลงมือ rather than an
// error. A picker offering an unknown stance is a bug on the other side of the
// wire; refusing would leave the window showing a stance nothing is enforcing.
func TestAnUnknownStanceLandsOnTheOneThatWithholdsNothing(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	// "goal" is มุ่งเป้า, designed and declined (§106.10). It stands in for any
	// name a row could hold that this build does not implement — a stance
	// written by a later version, read back after a downgrade.
	got, err := a.SetStance("goal")
	if err != nil {
		t.Fatalf("an unknown stance must not be an error: %v", err)
	}
	if got != "" || a.cur().stance != mode.StanceAct {
		t.Errorf("unknown stance became %q/%q, want ลงมือ", got, a.cur().stance)
	}
	if len(toolNames(a)) == 0 {
		t.Error("falling back must withhold nothing — this session came back carrying no tools")
	}
}

// วางแผน through a real engine: the reading tools reach the model and the
// writing ones do not. internal/mode proves the split; this proves it is wired
// to the dispatcher the session actually runs on.
func TestPlanSendsTheModelReadersAndNoWriters(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	if _, err := a.SetStance(string(mode.StancePlan)); err != nil {
		t.Fatalf("SetStance: %v", err)
	}
	got := toolNames(a)
	if len(got) == 0 {
		t.Fatal("วางแผน must still carry the reading tools — a plan written without looking is a guess")
	}
	has := func(name string) bool {
		for _, n := range got {
			if n == name {
				return true
			}
		}
		return false
	}
	// `search` rather than `grep`: list, glob and grep are one packed entry now
	// (internal/skill/search_pack.go), and วางแผน keeps all three of its acts.
	for _, name := range []string{"read", "search", "web_fetch"} {
		if !has(name) {
			t.Errorf("วางแผน should have sent %q; got %v", name, got)
		}
	}
	// `change` rather than write/edit: those are acts of one packed entry now
	// (internal/skill/change_pack.go), and naming a tool that is no longer
	// registered would make this half of the assertion pass for free.
	for _, name := range []string{"change", "shell", "task", "doc_write"} {
		if has(name) {
			t.Errorf("วางแผน sent %q — this stance changes nothing", name)
		}
	}
}

// The prompt has to match the tools. A stance that withheld `write` used to be
// handed longform ("write it to a .md file yourself with write") and
// fileEditing — three paragraphs on using the tools it had just been refused.
func TestPlanIsNotTaughtToUseTheToolsItWithheld(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	if _, err := a.SetStance(string(mode.StancePlan)); err != nil {
		t.Fatalf("SetStance: %v", err)
	}
	// The system prompt is the context's first message (cognitive/agent.go).
	msgs := a.cur().agent.ContextMessages()
	if len(msgs) == 0 {
		t.Fatal("the rebuilt agent has no context at all")
	}
	p := msgs[0].Content
	for _, phrase := range []string{"edits", "write it to a .md file", "one shell script"} {
		if strings.Contains(p, phrase) {
			t.Errorf("วางแผน was told %q, and it has no such tool", phrase)
		}
	}
	// And it still gets the layers that are about looking, which it can do.
	if !strings.Contains(p, "skills_list") {
		t.Error("วางแผน lost capability() — it can still look things up")
	}
}

// วางแผน and a packed tool: the payoff of the per-action filter, proved on the
// session the app actually runs.
//
// Before it, `browser` was one name over twelve rights and this stance could
// only take it whole — so a plan could not open a page at all, and the seven
// actions of it that merely look went with the five that act. stance.go said so
// itself and left it undone.
func TestPlanCarriesTheBrowserWithItsReadingHalfOnly(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	if _, err := a.SetStance(string(mode.StancePlan)); err != nil {
		t.Fatalf("SetStance: %v", err)
	}

	var params []byte
	for _, d := range a.deskTools().ToolDefinitions() {
		if d.Function.Name == "browser" {
			params = d.Function.Parameters
		}
	}
	if params == nil {
		t.Fatal("วางแผน carries no browser at all — the narrowing is not wired to this session")
	}
	var schema struct {
		Properties struct {
			Action struct {
				Enum []string `json:"enum"`
			} `json:"action"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("browser parameters do not parse: %v", err)
	}
	offered := map[string]bool{}
	for _, a := range schema.Properties.Action.Enum {
		offered[a] = true
	}
	// The actions, not the permission names: this is what the model is asked to
	// choose between. `scroll` rides on browser_read (packed.go).
	for _, action := range []string{"open", "read", "scroll", "wait", "back", "console", "network"} {
		if !offered[action] {
			t.Errorf("วางแผน was not offered browser %q, which only looks; offered %v", action, schema.Properties.Action.Enum)
		}
	}
	for _, action := range []string{"click", "type", "tabs", "dialog", "capture"} {
		if offered[action] {
			t.Errorf("วางแผน was offered browser %q — this stance changes nothing", action)
		}
	}
}

// ...and the door agrees with the block. A narrowed definition that still
// executed the withheld action would be worse than no narrowing: the model can
// name an action it was never shown.
//
// The refusal is the TOOL's, not the dispatcher's, and that is the better half
// of the answer: the call is handled and comes back saying which actions this
// session may use, rather than coming back as an unknown name. A model told
// "browser click is not available here, this session may use open, read, ..."
// can do something with that; one told nothing tries again.
func TestPlanRefusesABrowserActionItWasNotOffered(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	if _, err := a.SetStance(string(mode.StancePlan)); err != nil {
		t.Fatalf("SetStance: %v", err)
	}
	_, handled, err := a.deskTools().ExecuteTool(context.Background(), "browser", map[string]any{
		"action": "click", "ref": "1",
	})
	if !handled {
		t.Fatal("the browser was not reached at all; วางแผน should carry its reading half")
	}
	if err == nil {
		t.Fatal("วางแผน ran browser click, which it was never offered")
	}
	if !strings.Contains(err.Error(), "not available here") {
		t.Errorf("refusal was %q, want it to name what this session may use instead", err)
	}
}
