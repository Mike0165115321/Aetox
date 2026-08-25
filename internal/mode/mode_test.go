package mode

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// The three desks §83 ships must actually be there, parsed, and complete
// enough to put on a picker card: a mode with no description has nothing to
// show, and one with no prompt gives the session no direction — the owner's
// stated reason the feature exists.
func TestBundledDesksShipComplete(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // an empty user dir; bundled only

	byName := map[string]Mode{}
	for _, m := range List() {
		byName[m.Name] = m
	}
	for _, name := range []string{"assistant", "coding", "specialized"} {
		m, ok := byName[name]
		if !ok {
			t.Fatalf("bundled mode %q missing from List()", name)
		}
		if !m.Builtin {
			t.Errorf("%s: not marked Builtin", name)
		}
		if m.Description == "" {
			t.Errorf("%s: no description — the picker card would be blank", name)
		}
		if m.Prompt == "" {
			t.Errorf("%s: no prompt — the desk gives no direction", name)
		}
		if len(m.Categories) == 0 {
			t.Errorf("%s: no categories — an empty list means the full desk, which no bundled mode is", name)
		}
	}
}

// A category name in a manifest that category.go does not know matches no
// tool, ever — a typo would silently empty part of a desk. This is the same
// tripwire TestEveryToolHasACategory is on the other side of: there, every
// tool must land in a group; here, every group a mode names must exist.
func TestBundledCategoriesExist(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	for _, m := range List() {
		if !m.Builtin {
			continue
		}
		for _, c := range m.Categories {
			if !slices.Contains(skill.CategoryOrder, c) {
				t.Errorf("%s names category %q, which category.go does not have", m.Name, c)
			}
		}
	}
}

// The desks differ where COMPANY.md §2 says they differ. Spot checks, one per
// boundary that matters: coding carries no deliverables, assistant carries
// everything except the developer tools, specialized reads files but cannot
// edit them.
func TestDesksCarryWhatTheyClaim(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	load := func(name string) *Mode {
		m, ok := Load(name)
		if !ok || m == nil {
			t.Fatalf("Load(%q) failed", name)
		}
		return m
	}
	assistant, coding, specialized := load("assistant"), load("coding"), load("specialized")

	cases := []struct {
		mode *Mode
		tool string
		want bool
	}{
		{assistant, "memory", true},     // agent category
		{assistant, "web_search", true}, // web
		// The Office writers are the office's, not this desk's — a deliverable is
		// delegated to the chair whose job it is (§84's star), and leaving the
		// tools here meant the assistant did the work itself whenever the job
		// looked small enough, which is a decision by mood rather than by rule.
		{assistant, "doc_write", false},
		{assistant, "sheet_write", false},
		{assistant, "read", true},         // files
		{assistant, "edit", true},         // files — sorting out this machine is this desk's work
		{assistant, "shell", true},        // shell: COMPANY.md §2 — safety is the gate, not a missing tool
		{assistant, "diagnostics", false}, // no code category: developer tools are the coding desk's

		{coding, "read", true},        // files
		{coding, "shell", true},       // shell
		{coding, "diagnostics", true}, // code
		{coding, "doc_write", false},  // no deliverables
		{coding, "image_ocr", false},  // no media

		// The writers left this desk too (owner's call, 2026-08-06). The office
		// still makes documents — its agents do, under `chairs:` — but the
		// assistant sitting here hands the job over rather than doing it. See
		// TestChairsAreInTheRoomButNotOnTheDesk for the other half.
		{specialized, "doc_write", false},
		{specialized, "sheet_write", false},
		{specialized, "pdf_read", true}, // media
		{specialized, "read", true},     // explicit
		{specialized, "write", true},    // explicit
		{specialized, "edit", false},    // files category is not on this desk
		{specialized, "shell", false},
		{specialized, "symbol", false}, // no code category
	}
	for _, c := range cases {
		if got := c.mode.AllowsTool(c.tool); got != c.want {
			t.Errorf("%s.AllowsTool(%q) = %v, want %v", c.mode.Name, c.tool, got, c.want)
		}
	}
}

// A nil Mode is every session from before modes existed, and it must behave
// exactly as those sessions always did: everything on the desk, every server
// attached. Load("") is that nil, and it is an answer, not a miss.
// Showing the user a file is not one desk's privilege. Whatever a session is
// for, the person at it has to be able to see what was produced — and the desk
// that produces the most (specialized: decks, documents, workbooks) is the one
// that lost these when they were first filed under shell.
func TestEveryDeskCanPutSomethingOnTheDesk(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	for _, name := range []string{"assistant", "coding", "specialized"} {
		m, ok := Load(name)
		if !ok || m == nil {
			t.Fatalf("Load(%q) failed", name)
		}
		for _, tool := range []string{"desk_open", "desk_list"} {
			if !m.AllowsTool(tool) {
				t.Errorf("%s desk does not carry %s", name, tool)
			}
		}
	}

	// desk_terminal is the exception and must stay one: it starts a shell, and
	// the specialized desk carries no shell on purpose.
	specialized, _ := Load("specialized")
	if specialized.AllowsTool("desk_terminal") {
		t.Error("specialized desk carries desk_terminal, but it carries no shell")
	}
}

// Counting is not one desk's privilege either, and the trap here is subtler
// than the desk_open one: calc runs a script, so `code` is the category it
// reads like it belongs to — and `code` is the developer group that assistant
// and specialized refuse on purpose. Filed there, the desk whose whole job is
// answering a person's question would have been the one desk that has to do
// arithmetic in its head.
func TestEveryDeskCanWorkANumberOut(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	for _, name := range []string{"assistant", "coding", "specialized"} {
		m, ok := Load(name)
		if !ok || m == nil {
			t.Fatalf("Load(%q) failed", name)
		}
		if !m.AllowsTool("calc") {
			t.Errorf("%s desk cannot count: it does not carry calc", name)
		}
	}
}

func TestNilModeIsTheFullDesk(t *testing.T) {
	m, ok := Load("")
	if !ok {
		t.Fatal("Load(\"\") must be ok — the empty mode is the legacy full desk")
	}
	if m != nil {
		t.Fatalf("Load(\"\") returned %+v, want nil", m)
	}
	if !m.AllowsTool("anything") || !m.AllowsTool("shell") {
		t.Error("nil mode must allow every tool")
	}
	if !m.AllowsServer("any-server") {
		t.Error("nil mode must allow every server")
	}
}

// MCP stays default-closed, and a manifest can no longer open it: ownership
// moved onto the server itself (owner's call, 2026-08-06 — see Mode.MCP), so a
// desk carries exactly the servers whose `for:` names it, and nothing else.
//
// The `mcp:` line is deliberately not read. Leaving it half-working would be
// two places answering "which servers are on this desk", and the one that
// looks authoritative — the manifest the user is reading — would be the one
// with no effect.
func TestMCPIsResolvedFromTheServerNotTheManifest(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // no servers configured

	none := parse("x", "---\ncategories: web\n---\nbody")
	if none.AllowsServer("github") {
		t.Error("a desk with no servers resolved to it attached one")
	}
	if claimed := parse("x", "---\ncategories: web\nmcp: github, Notion\n---\nbody"); claimed.AllowsServer("github") {
		t.Error("a manifest attached a server by naming it — ownership is the server's, and two answers is one too many")
	}

	// The resolved field is what AllowsServer reads, case-insensitively.
	some := &Mode{Name: "x", MCP: []string{"github", "notion"}}
	if !some.AllowsServer("github") || !some.AllowsServer("Notion") {
		t.Error("a resolved server was not attached (names must match case-insensitively)")
	}
	if some.AllowsServer("linear") {
		t.Error("an unresolved server was attached")
	}
	if some.AllowsServer("") {
		t.Error("the empty server name matched")
	}
}

// deny: wins over both an explicit tools: entry and a category — it is the
// §44.0 read-only future ("the same assistant with its hands tied") and must
// beat every grant, or tying one hand quietly frees the other.
func TestDenyWinsOverEverything(t *testing.T) {
	m := parse("x", "---\ncategories: files\ntools: shell\ndeny: shell, write\n---\nbody")
	if m.AllowsTool("shell") {
		t.Error("deny lost to an explicit tools: entry")
	}
	if m.AllowsTool("write") {
		t.Error("deny lost to a category grant")
	}
	if !m.AllowsTool("read") {
		t.Error("deny removed more than it named")
	}
}

// Empty categories and tools mean the full desk — the sub-agent precedent —
// so `deny: shell` alone is a valid manifest meaning "everything minus one".
func TestEmptyListsMeanFullDesk(t *testing.T) {
	m := parse("x", "---\ndeny: shell\n---\nbody")
	if !m.AllowsTool("read") || !m.AllowsTool("doc_write") {
		t.Error("empty categories+tools should mean everything")
	}
	if m.AllowsTool("shell") {
		t.Error("the one denied tool survived")
	}
	if m.AllowsServer("github") {
		t.Error("the full desk still attaches no servers unless mcp: names them")
	}
}

// Dispatch is the one door between desks and it is default-closed: ผู้ช่วย
// declares the office, โค้ด declares nothing and so talks to no one, and the
// office calls no one back (COMPANY.md §3 — the star has one center).
func TestDispatchIsDefaultClosedAndOneWay(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	load := func(name string) *Mode {
		m, ok := Load(name)
		if !ok || m == nil {
			t.Fatalf("Load(%q) failed", name)
		}
		return m
	}
	if !load("assistant").AllowsDispatch(Office) {
		t.Error("ผู้ช่วย cannot hand work to the office — the one dispatch that exists")
	}
	if load("coding").AllowsDispatch(Office) {
		t.Error("โค้ด can hand work to the office; the three left-side desks talk to no one")
	}
	if load("assistant").AllowsDispatch("coding") {
		t.Error("a desk dispatched to one it never named")
	}
	if load(Office).AllowsDispatch("assistant") {
		t.Error("the office hands work back out — a leaf that calls is not a leaf")
	}
	if load("assistant").AllowsDispatch("") {
		t.Error("the empty desk name matched a dispatch rule")
	}

	// The legacy full desk could always reach every profile, and still can.
	var full *Mode
	if !full.AllowsDispatch(Office) {
		t.Error("a nil mode refused a dispatch it never used to")
	}
}

// Carries is where a *registered* tool is judged, and the MCP branch is the
// §83 trap: CategoryOf answers `agent` for every unknown name, so judging an
// MCP tool by AllowsTool would attach every installed server to every desk
// that carries the agent group — which is all of them.
func TestCarriesJudgesEachSourceItsOwnWay(t *testing.T) {
	m := parse("x", "---\ncategories: agent\n---\nbody")
	m.MCP = []string{"notion"} // resolved from the server's `for:`, not the manifest

	if !m.Carries("notion_search", skill.SourceMCP) {
		t.Error("a tool from the named server is not carried")
	}
	if m.Carries("linear_issue", skill.SourceMCP) {
		t.Error("a tool from an unnamed server was carried — the category fallback decided it")
	}
	// The same name, if it were a built-in, would be carried by the agent group.
	// That contrast is the whole point of splitting the branches.
	if !m.Carries("linear_issue", skill.SourceBuiltin) {
		t.Error("the agent category stopped covering unknown built-in names")
	}
	// A skill is knowledge, not capability: every desk keeps every skill, so the
	// user's /skill-name works wherever they type it.
	if !m.Carries("some-installed-skill", skill.SourceSkill) {
		t.Error("a desk dropped an installed skill")
	}
	var full *Mode
	if !full.Carries("anything", skill.SourceMCP) || !full.Carries("anything", skill.SourceBuiltin) {
		t.Error("the legacy full desk stopped carrying something")
	}
}

// A user file with a bundled mode's name replaces it — editing a shipped mode
// is copying it out, never fighting the app — and List() must say so, because
// deleting that file is a revert, not a removal.
func TestUserFileShadowsBundled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	dir := filepath.Join(root, "modes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "---\ndescription: ของฉัน\ncategories: web\n---\nmy coding desk"
	if err := os.WriteFile(filepath.Join(dir, "coding.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	m, ok := Load("coding")
	if !ok || m == nil {
		t.Fatal("Load(coding) failed")
	}
	if m.Prompt != "my coding desk" {
		t.Fatalf("user file did not win: prompt %q", m.Prompt)
	}
	if !m.Overrides {
		t.Error("shadowing file not marked Overrides")
	}
	seen := 0
	for _, x := range List() {
		if x.Name == "coding" {
			seen++
			if x.Prompt != "my coding desk" {
				t.Error("List() kept the bundled mode despite the shadow")
			}
		}
	}
	if seen != 1 {
		t.Fatalf("coding appears %d times in List(), want 1", seen)
	}
}

// The name is about to be joined onto a path; anything that could step out of
// the modes directory is refused before the filesystem sees it.
func TestLoadRefusesPathShapedNames(t *testing.T) {
	for _, name := range []string{"..", "a/b", `a\b`, "a b", "con:aux"} {
		if _, ok := Load(name); ok {
			t.Errorf("Load(%q) accepted a path-shaped name", name)
		}
	}
	if _, ok := Load("no-such-mode-anywhere"); ok {
		t.Error("an unknown mode name reported ok")
	}
}

// `chairs:` exists because one list was answering two questions (owner's call,
// 2026-08-06). Taking the writers off the specialized desk — so the assistant
// hands the job to an agent instead of doing it itself — used to take them away
// from the agents as well, because the manifest that describes the desk is also
// the ceiling FilterRegistry measures a chair against. The office would have
// lost the ability to make the one thing it exists to make.
//
// What this pins is the split itself, from both sides, plus the two boundaries
// that keep it from becoming a hole: deny still wins, and the widening is for
// chairs only — an ordinary delegate is answered by Carries and never sees it.
func TestChairsAreInTheRoomButNotOnTheDesk(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	specialized, ok := Load("specialized")
	if !ok {
		t.Fatal("specialized desk missing")
	}

	for _, tool := range []string{"doc_write", "sheet_write"} {
		if specialized.Carries(tool, skill.SourceBuiltin) {
			t.Errorf("%s is still on the desk — the assistant must hand the job to an agent", tool)
		}
		if !specialized.CarriesForChair(tool, skill.SourceBuiltin) {
			t.Errorf("%s is out of the agents' reach too — the office cannot make what it exists to make", tool)
		}
	}

	// Not a back door. `chairs:` says "in the room", never "above the rules".
	denied := &Mode{Name: "t", Tools: []string{"read"}, Chairs: []string{"doc_write"}, Deny: []string{"doc_write"}}
	if denied.CarriesForChair("doc_write", skill.SourceBuiltin) {
		t.Error("deny lost to chairs — a tool the desk removed outright must be gone for everyone")
	}

	// Shell moved into the room on 2026-08-10 (owner: the automation agent is
	// full-rank — it starts its own engine and works a real workstation), and
	// the split is the same one the writers live under: in the room for the
	// agents whose profiles ask, still not on the assistant's desk.
	if specialized.Carries("shell", skill.SourceBuiltin) {
		t.Error("shell reached the office assistant — the room's grant leaked onto the desk")
	}
	if !specialized.CarriesForChair("shell", skill.SourceBuiltin) {
		t.Error("shell is out of the agents' reach — the full-rank worker cannot start its own engine")
	}

	// And still not a way around the ceiling for ordinary work: a tool neither
	// the desk nor its chairs name stays unreachable to everyone here. `git`
	// was this example until 2026-08-19, when the whole of files and shell moved
	// into the room (owner: the agents were walled off from work their own
	// briefs told them to do). `symbol` is the unnamed one now — the code group
	// is what this desk still refuses.
	if specialized.Carries("symbol", skill.SourceBuiltin) || specialized.CarriesForChair("symbol", skill.SourceBuiltin) {
		t.Error("chairs widened the ceiling past what the desk names")
	}
}

// ---------------------------------------------------------------------------
// Connections — an external account is placed on desks the same way an MCP
// server is, and the desk that does not hold it does not see its tools.
// ---------------------------------------------------------------------------

// Nothing is taken away from anyone by connections.json arriving: an install
// that has never opened the page keeps every tool it had.
func TestUnplacedConnectionLeavesEveryDeskAsItWas(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	// Two desks, two tools, because the desks already differ by category:
	// github_search is `code` and only the coding desk ever had it, while
	// plugin_install is `agent` and the assistant desk carries it too.
	coding, _ := Load("coding")
	assistant, _ := Load("assistant")
	if coding == nil || assistant == nil {
		t.Fatal("bundled desks missing")
	}
	if !coding.Carries("github_search", skill.SourceBuiltin) {
		t.Fatal("coding lost github_search with nothing configured")
	}
	if !assistant.Carries("plugin_install", skill.SourceBuiltin) {
		t.Fatal("assistant lost plugin_install with nothing configured")
	}
}

func TestDeskWithoutTheConnectionCannotSeeItsTools(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	if err := config.SetConnectionTargets("github", []string{"coding"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}

	coding, _ := Load("coding")
	assistant, _ := Load("assistant")
	if coding == nil || assistant == nil {
		t.Fatal("bundled desks missing")
	}
	if !coding.Carries("github_search", skill.SourceBuiltin) {
		t.Fatal("the desk the account was placed on cannot see its tools")
	}
	// plugin_install, not github_search: the assistant desk never had the
	// second one (it is `code`), so it would prove nothing about placement.
	if assistant.Carries("plugin_install", skill.SourceBuiltin) {
		t.Fatal("a desk the account was kept off still carries plugin_install")
	}
	// Tools that belong to no connection are untouched by any of this.
	if !assistant.Carries("read", skill.SourceBuiltin) {
		t.Fatal("an ordinary tool was gated by a connection it has nothing to do with")
	}
}

// `chairs:` widens what an agent may hold, not what the user attached. A
// connection switched off is off for the room, the same as Deny.
func TestConnectionCannotComeBackThroughChairs(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	if err := config.SetConnectionTargets("github", []string{"coding"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}
	assistant, _ := Load("assistant")
	if assistant == nil {
		t.Fatal("assistant desk missing")
	}
	assistant.Chairs = append(assistant.Chairs, "plugin_install")

	if assistant.CarriesForChair("plugin_install", skill.SourceBuiltin) {
		t.Fatal("a chair reached an account the desk was kept off")
	}
}

// The pre-modes full desk carried everything and still does.
func TestFullDeskCarriesEveryConnection(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	if err := config.SetConnectionTargets("github", []string{"coding"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}
	var full *Mode
	if !full.Carries("github_search", skill.SourceBuiltin) {
		t.Fatal("the legacy full desk lost a connection's tools")
	}
}

// A manifest is compiled into the binary; a switch on the settings page is not.
// So a manifest may describe the desk — what work this is, what is on it, what
// belongs elsewhere — and may never name a tool whose existence the user
// decides at runtime.
//
// `task` is the one such tool today. assistant.md and specialized.md both spelt
// out the handover in prose, and with delegation off — the shipped default —
// the model read "hand the job over with `task`", called it, and got
// `tool "task" is not exposed to agent` (2026-08-20). The route lives in
// internal/prompt now, where it can be gated on whether the tool was built.
//
// The name is what this can check. The same mistake without the name is not
// catchable here and has to be caught in review: coding.md said "delegate
// searches and side-quests" the same day, which is the same instruction with
// the tool left unnamed. What replaced it is the half that holds either way —
// the main thread is for the change itself — and the judgment about WHEN to
// delegate lives in the tool's own description, which only exists when the tool
// does.
func TestNoManifestNamesATooltheUserCanSwitchOff(t *testing.T) {
	entries, err := bundledModes.ReadDir("modes")
	if err != nil {
		t.Fatalf("read bundled modes: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no bundled manifests to check")
	}
	for _, e := range entries {
		body, err := bundledModes.ReadFile("modes/" + e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(body), "`task`") {
			t.Errorf("%s names `task`, which does not exist when the user has delegation switched off — say it in internal/prompt, where the gate is", e.Name())
		}
	}
}

// A desk manifest briefs the model in prose, and the model paraphrases prose to
// the user. So a tool id written into one is an identifier handed to somebody
// who has never seen it and cannot type it — which is the §116 line crossed
// from the inside.
//
// It happened: `desk_open` sat in two manifest bodies, and under คู่คิด — where
// no tool definitions are sent and the body is the whole inventory — it was one
// of exactly three things the assistant could name when the owner asked what it
// could do, next to a window showing four labelled buttons (22 ส.ค.).
//
// Frontmatter is exempt and has to be: `tools:`, `deny:` and `chairs:` are
// configuration, read by AllowsTool and never shown to anybody. This guards the
// body, which is the half that gets repeated.
func TestNoDeskManifestSpellsAToolIdInItsBody(t *testing.T) {
	// Ids a body has reached for, or would plausibly reach for next. Not the
	// whole registry: a list that had to be complete would be a list that goes
	// stale, and what this is really holding is the habit.
	ids := []string{
		"desk_open", "desk_list", "desk_close", "desk_terminal",
		"edits", "web_fetch", "web_search", "image_ocr", "video_ocr",
		"audio_transcribe", "pdf_read", "todo_write", "session_search",
	}
	// Bundled only. A desk the user wrote is theirs, and failing this repo's
	// tests over the wording in someone's own file would be this rule enforcing
	// itself somewhere it has no standing.
	for _, m := range List() {
		if !m.Builtin {
			continue
		}
		body := m.Direction()
		for _, id := range ids {
			if strings.Contains(body, id) {
				t.Errorf("desk %q names the tool %q in its body — say the act instead, "+
					"the user never sees a tool id", m.Name, id)
			}
		}
	}
}
