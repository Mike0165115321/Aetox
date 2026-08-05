package mode

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/skill"
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
// boundary that matters: coding carries no deck writer, assistant carries
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
		{assistant, "memory", true},       // agent category
		{assistant, "web_search", true},   // web
		{assistant, "slides_write", true}, // deliverables
		{assistant, "read", true},         // files
		{assistant, "edit", true},         // files — sorting out this machine is this desk's work
		{assistant, "shell", true},        // shell: COMPANY.md §2 — safety is the gate, not a missing tool
		{assistant, "diagnostics", false}, // no code category: developer tools are the coding desk's

		{coding, "read", true},          // files
		{coding, "shell", true},         // shell
		{coding, "diagnostics", true},   // code
		{coding, "slides_write", false}, // no deliverables
		{coding, "image_ocr", false},    // no media

		{specialized, "doc_write", true}, // deliverables
		{specialized, "pdf_read", true},  // media
		{specialized, "read", true},      // explicit
		{specialized, "write", true},     // explicit
		{specialized, "edit", false},     // files category is not on this desk
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

// MCP is the one default-closed field: a manifest that says nothing about
// servers attaches none. Letting absence mean "all" would put every installed
// server on every desk, which is the pile §83 exists to take apart.
func TestMCPDefaultsClosed(t *testing.T) {
	none := parse("x", "---\ncategories: web\n---\nbody")
	if none.AllowsServer("github") {
		t.Error("a mode with no mcp: field attached a server")
	}
	some := parse("x", "---\ncategories: web\nmcp: github, Notion\n---\nbody")
	if !some.AllowsServer("github") || !some.AllowsServer("notion") {
		t.Error("a named server was not attached (names must match case-insensitively)")
	}
	if some.AllowsServer("linear") {
		t.Error("an unnamed server was attached")
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
	if !m.AllowsTool("read") || !m.AllowsTool("slides_write") {
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
	m := parse("x", "---\ncategories: agent\nmcp: notion\n---\nbody")

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
