package mode

import (
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

// The zero value has to be the default, because three separate things rely on
// it saying so without translating: the sessions.stance column default, a
// caller that never sets bootstrap.Options.Stance, and every session written
// before the column existed.
func TestTheZeroStanceIsTheOneThatChangesNothing(t *testing.T) {
	var s Stance
	if s != StanceAct {
		t.Fatalf("the zero Stance must be StanceAct, got %q", s)
	}
	if s.String() != "" {
		t.Errorf("StanceAct must store as the empty string, got %q", s.String())
	}
	if !s.AllowsTool("write") {
		t.Error("ลงมือ must leave every tool on the desk — it is today's behaviour, unchanged")
	}
	if s.Direction() != "" {
		t.Error("ลงมือ must add nothing to the prompt: it changes nothing, so it has nothing to say")
	}
}

func TestConsultCarriesNoToolDefinitions(t *testing.T) {
	s := StanceConsult
	for _, name := range []string{"write", "edit", "shell", "browser", "calc", "task", "ask_user", "memory"} {
		if s.AllowsTool(name) {
			t.Errorf("คู่คิด must carry no tools; it kept %q", name)
		}
	}
	for _, source := range []skill.Source{skill.SourceBuiltin, skill.SourceWorkbench, skill.SourceMCP} {
		if s.Carries("anything", source) {
			t.Errorf("คู่คิด must carry nothing from source %v", source)
		}
	}
	if s.Direction() == "" {
		t.Error("a stance that changes the engine has to say so in the prompt")
	}
}

// The one thing no stance takes away. A skill contributes no tool definition to
// begin with (Dispatcher.ToolDefinitions skips SourceSkill), so withholding it
// would buy nothing — and it would break the `/skill-name` command the user
// typed on purpose. Same sentence Mode.Carries already holds.
func TestNoStanceTakesASkillAway(t *testing.T) {
	for _, s := range Stances() {
		if !s.Carries("tax-invoice", skill.SourceSkill) {
			t.Errorf("stance %q dropped a skill: a skill costs no tokens and /skill-name must work everywhere", s)
		}
	}
}

// A stance may only subtract. Nothing in the type can express "put this back",
// and this is the test that says so out loud — the property COMPANY.md §6.3
// rests on once the dial can move mid-session.
func TestEveryStanceIsASubsetOfTheDefault(t *testing.T) {
	names := []string{"write", "edit", "shell", "browser", "calc", "task", "diagnostics"}
	for _, s := range Stances() {
		for _, name := range names {
			if s.AllowsTool(name) && !StanceAct.AllowsTool(name) {
				t.Fatalf("stance %q handed back %q — a stance must never widen a desk", s, name)
			}
		}
	}
}

// A name from a later build — a session left in วางแผน and reopened after a
// downgrade — must come back as ลงมือ. Failing the other way opens a
// conversation that silently carries nothing and gives no reason why.
func TestAnUnknownStanceFallsBackToTheOneThatWithholdsNothing(t *testing.T) {
	for _, name := range []string{"goal", "ผิด", "CONSULT-ish", "  "} {
		if got := NormalizeStance(name); got != StanceAct {
			t.Errorf("NormalizeStance(%q) = %q, want ลงมือ", name, got)
		}
	}
	if got := NormalizeStance("  CONSULT "); got != StanceConsult {
		t.Errorf("NormalizeStance must trim and lowercase a real name, got %q", got)
	}
	if got := NormalizeStance("plan"); got != StancePlan {
		t.Errorf("วางแผน is implemented now and must survive normalization, got %q", got)
	}
}

// วางแผน is the stance that looks at everything and changes nothing, so the
// split has to hold in both directions: every reading tool kept, every writing,
// running or delegating one gone.
func TestPlanKeepsWhatOnlyLooksAndDropsWhatChanges(t *testing.T) {
	s := StancePlan
	for _, name := range []string{
		"read", "list", "glob", "grep", "web_search", "web_fetch",
		"pdf_read", "image_ocr", "diagnostics", "symbol", "github",
		"n8n_workflow_read", "todo_write", "ask_user", "calc", "skills_list",
	} {
		if !s.AllowsTool(name) {
			t.Errorf("วางแผน dropped %q — it only reads, and a plan written without looking is a guess", name)
		}
	}
	for _, name := range []string{
		"write", "edit", "apply_patch", "delete", "notebook_edit",
		"shell", "shell_kill", "git", "desk_terminal",
		"doc_write", "sheet_write", "slides_write",
		"n8n_workflow_create", "n8n_server_start", "windmill_flow_update",
		"task", "plugin_install", "memory", "browser",
	} {
		if s.AllowsTool(name) {
			t.Errorf("วางแผน kept %q — the whole promise of this stance is that it changes nothing", name)
		}
	}
}

// An allow-list, and this is the test that says why. A tool nobody has
// classified must be withheld here, so that adding one next month cannot
// quietly hand a writing tool to the mode that promises not to write.
func TestPlanWithholdsAToolNobodyHasClassified(t *testing.T) {
	if StancePlan.AllowsTool("some_tool_added_next_month") {
		t.Error("วางแผน must fail closed on an unknown tool — a deny-list would fail the other way, silently")
	}
}

// An MCP tool is withheld without being looked at: `jira_search` and
// `jira_create_issue` are the same kind of string to this package, so there is
// no honest way to tell them apart from here.
func TestPlanWithholdsMCPBecauseItCannotTellReadFromWrite(t *testing.T) {
	if StancePlan.Carries("jira_search", skill.SourceMCP) {
		t.Error("วางแผน must withhold MCP: a promise this code cannot keep is worse than a missing tool")
	}
	if !StanceAct.Carries("jira_search", skill.SourceMCP) {
		t.Error("ลงมือ must still carry MCP — it withholds nothing")
	}
}

// ลงมือ leads because it is the way back: a control you can enter and cannot
// leave is not a switch (§106.3).
func TestThePickerOpensWithTheWayBack(t *testing.T) {
	all := Stances()
	if len(all) < 2 {
		t.Fatalf("a switch needs somewhere to go and somewhere to return to, got %d", len(all))
	}
	if all[0] != StanceAct {
		t.Errorf("ลงมือ must be first in picker order, got %q", all[0])
	}
	// Stances() must hand back a copy — a caller that sorts or truncates the
	// result must not be able to edit the set every session is normalized
	// against.
	all[0] = "tampered"
	if Stances()[0] != StanceAct {
		t.Error("Stances() leaked its backing array; a caller can now rewrite the set")
	}
}

// §44.0 and COMPANY.md §6.1: a manifest says what the work is, never who the
// assistant is. A stance is a manifest with no file, and is held to the line
// the same way.
func TestAStanceDirectionDescribesTheWorkAndNotTheWorker(t *testing.T) {
	for _, s := range Stances() {
		d := strings.ToLower(s.Direction())
		if d == "" {
			continue
		}
		for _, forbidden := range []string{"you are a", "you are an", "act as", "your persona", "your role is"} {
			if strings.Contains(d, forbidden) {
				t.Errorf("stance %q says %q — that is identity, and identity has one home (§44.0)", s, forbidden)
			}
		}
	}
}
