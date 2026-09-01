package subagent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/mode"
	"github.com/Mikedev115/Aetox/internal/safety"
)

// isolate points DataRoot at a fresh temp dir so a developer's real profiles can
// neither leak into these tests nor be written by them.
func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	dir := filepath.Join(root, "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	return dir
}

// Eleven profiles ship, in two kinds, and the split is the thing being asserted:
// four delegates any desk may run, and seven agents that sit in the office
// (COMPANY.md §4). The counts are pinned because "one more profile, it's free"
// is how a bundled set becomes a menu nobody reads — so a number that moves here
// should be a hire somebody decided on, never a file that arrived.
//
// **No bundled agent writes `desk:`, and none should.** A file in the agents
// home is given the office by applyHomeRules, which is the whole rule an author
// needs to know: drop the folder in, it is one of the team. Writing the field
// can only ever get it wrong — and wrong is silent, because a profile at any
// other desk still parses, still validates, still appears in List(), and
// disappears from the roster, the chat page's picker and every door a user
// could walk through. That happened once, to the github agent, on the day it
// was written. This assertion is what notices.
func TestBundledProfilesAreUsable(t *testing.T) {
	isolate(t)
	got := List()
	want := []string{"automation", "doc", "editor", "explore", "general", "github", "research", "reviewer", "sheet", "tester", "video"}
	if len(got) != len(want) {
		t.Fatalf("List() = %d profiles, want %d", len(got), len(want))
	}
	chairs := map[string]bool{
		"automation": true, "doc": true, "editor": true, "github": true, "research": true,
		"sheet": true, "video": true,
	}
	for i, p := range got {
		if p.Name != want[i] {
			t.Errorf("List()[%d] = %q, want %q (alphabetical)", i, p.Name, want[i])
		}
		if wantDesk := chairs[p.Name]; wantDesk != (p.Desk != "") {
			t.Errorf("%s: Desk=%q, want an agent=%v", p.Name, p.Desk, wantDesk)
		}
		if chairs[p.Name] && p.Desk != mode.Office {
			t.Errorf("%s: sits at desk %q, want %q — an agent anywhere else is unreachable from every door",
				p.Name, p.Desk, mode.Office)
		}
		if !p.Builtin || p.Path != "" || p.Overrides {
			t.Errorf("%s: Builtin=%v Path=%q Overrides=%v", p.Name, p.Builtin, p.Path, p.Overrides)
		}
		// No description = invisible in the settings row; no prompt = a nameless
		// delegate with no brief.
		if p.Description == "" {
			t.Errorf("%s: no description", p.Name)
		}
		if len(p.Prompt) < 40 {
			t.Errorf("%s: prompt is %d chars, expected a real brief", p.Name, len(p.Prompt))
		}
		// A worker's ceiling is whatever its own file says, and nothing else: a
		// number means that number, and no `steps:` line means no ceiling
		// (§110's default).
		//
		// This deliberately does NOT require the shipped files to be free of
		// `steps:`. It said that for a day, and that was a rule nobody asked
		// for: the owner's instruction was to make *unlimited the default*
		// ("ปลดล็อคเพดานทุกซับเอเจนเลยครับ เป็นค่าเริ่มต้นได้เลย", §110), and the
		// change that carried it out also stripped the eight numbers that were
		// written down on purpose and then locked the door behind them here.
		// Removing a ceiling and forbidding one are different decisions, and
		// only the first was made — §110 says so in its own words one paragraph
		// later: "steps: 40 still means exactly 40 for anyone who wants one."
		//
		// So this asserts the mechanism is honest, and leaves the number to
		// whoever is responsible for the worker (2026-08-16).
		switch {
		case p.Steps > 0 && p.MaxToolCalls() != p.Steps:
			t.Errorf("%s: writes steps: %d but runs with a cap of %d", p.Name, p.Steps, p.MaxToolCalls())
		case p.Steps <= 0 && p.MaxToolCalls() > 0:
			t.Errorf("%s: names no ceiling but runs under one (%d) — the default is no ceiling",
				p.Name, p.MaxToolCalls())
		}
	}
}

func TestExploreIsReadOnlyAndCannotRecurse(t *testing.T) {
	isolate(t)
	p, ok := Load("explore")
	if !ok {
		t.Fatal("explore profile missing")
	}
	if !slices.Equal(p.Tools, []string{"grep", "glob", "list", "read"}) {
		t.Fatalf("explore tools = %v", p.Tools)
	}
	for _, tool := range []string{"write", "shell", "edit", "web_fetch"} {
		if p.AllowsTool(tool) {
			t.Errorf("explore allows %q, which is not in its tool list", tool)
		}
	}
	for _, tool := range forcedDenials {
		if p.AllowsTool(tool) {
			t.Errorf("explore allows %q, which every sub-agent is refused", tool)
		}
	}
}

// general still inherits the registry — a profile named "general" that had to be
// told about each new tool would go stale the moment one is added, and an
// allowlist here was tried and cut: it silently took web_search away from a
// delegate asked to research something. What it denies is the short list with a
// reason that is not "fewer tokens": nobody is watching this loop.
//
// `shell` stays, so this is not a wall — a delegate can still reach the disk. It
// removes the two the model would otherwise reach for *by name*.
func TestGeneralInheritsToolsButNotTheUnattendedOnes(t *testing.T) {
	isolate(t)
	p, ok := Load("general")
	if !ok {
		t.Fatal("general profile missing")
	}
	// Inherits: whatever the registry has, including tools added after this test.
	for _, tool := range []string{"read", "edit", "write", "shell", "grep", "web_search"} {
		if !p.AllowsTool(tool) {
			t.Errorf("general cannot use %q — it is the catch-all delegate and should inherit the registry", tool)
		}
	}
	// plugin_install changes what Aetox itself can do; delete is one-shot and
	// irreversible. Both with no human attached to the loop.
	for _, tool := range []string{"plugin_install", "delete"} {
		if p.AllowsTool(tool) {
			t.Errorf("general is handed %q, which no unattended delegate should reach for", tool)
		}
	}
	// Deny is the safety gate, not just a token filter — it has to reach the
	// permission layer too, or a discovered skill by the same name walks through.
	rules := p.DenyRules()
	if len(rules) != 2 {
		t.Errorf("general deny rules = %d, want 2 (plugin_install, delete)", len(rules))
	}
	for _, tool := range forcedDenials {
		if p.AllowsTool(tool) {
			t.Errorf("general allows %q", tool)
		}
	}
}

// WantsToBeAsked is what the chair-chat path asks instead of AllowsTool, and
// the two must disagree — that disagreement is the whole point. AllowsTool
// answers for a delegate, where forcedDenials is right because nobody is
// watching; this answers for an agent someone is talking to.
func TestWantsToBeAskedSplitsFromTheDelegateAnswer(t *testing.T) {
	// A profile whose `tools:` never mentions ask_user still gets to ask: the
	// allowlist is what it may touch, and a question touches nothing.
	narrow := Profile{Name: "doc", Tools: []string{"doc_write", "read"}}
	if narrow.AllowsTool("ask_user") {
		t.Error("AllowsTool must keep saying no — that answer is for the delegate path")
	}
	if !narrow.WantsToBeAsked() {
		t.Error("a chair with a narrow tools: line still has to be able to ask the person it is talking to")
	}
	// An author who writes deny: ask_user means it, and outranks the grant.
	silent := Profile{Name: "quiet", Deny: []string{"ask_user"}}
	if silent.WantsToBeAsked() {
		t.Error("deny: ask_user must still silence the chair — the profile refusing outright outranks the grant")
	}
}

func TestMaxToolCalls(t *testing.T) {
	// cognitive.Agent reads <= 0 as unbounded, so the default has to arrive as a
	// value it reads that way rather than as a very large number pretending to
	// be infinity.
	if got := (Profile{}).MaxToolCalls(); got > 0 {
		t.Errorf("MaxToolCalls = %d, want the default, which is no ceiling", got)
	}
	if got := (Profile{Steps: 3}).MaxToolCalls(); got != 3 {
		t.Errorf("steps override = %d, want 3", got)
	}
	if got := (Profile{Steps: StepsUnlimited}).MaxToolCalls(); got > 0 {
		t.Errorf("unlimited steps = %d, want a value the agent loop reads as no ceiling", got)
	}
}

// A ceiling is only ever the number a profile wrote. A blank field, a typo or a
// hand-written negative all land on the default instead of inventing one, so a
// mistyped `steps:` cannot quietly cap a delegate at some other value.
func TestParseStepsOnlyTakesACeilingFromANumber(t *testing.T) {
	unbounds := []string{"unlimited", "  Unlimited  ", "UNLIMITED"}
	for _, in := range unbounds {
		if got := parseSteps(in); got != StepsUnlimited {
			t.Errorf("parseSteps(%q) = %d, want StepsUnlimited", in, got)
		}
	}

	defaults := []string{"", "   ", "none", "-1", "-40", "12abc", "infinity"}
	for _, in := range defaults {
		if got := parseSteps(in); got != 0 {
			t.Errorf("parseSteps(%q) = %d, want 0 so MaxToolCalls falls back to the default", in, got)
		}
		if got := (Profile{Steps: parseSteps(in)}).MaxToolCalls(); got > 0 {
			t.Errorf("parseSteps(%q) reached MaxToolCalls as %d, want the default, which is no ceiling", in, got)
		}
	}

	if got := parseSteps("8"); got != 8 {
		t.Errorf("parseSteps(\"8\") = %d, want 8", got)
	}
}

// The frontmatter is what the user edits by hand, so the keyword has to survive
// a real file rather than only the field parser.
func TestParseUnlimitedFromFrontmatter(t *testing.T) {
	p := parse("runner", "---\ndescription: long job\nsteps: unlimited\n---\nYou run until done.")
	if p.Steps != StepsUnlimited {
		t.Fatalf("Steps = %d, want StepsUnlimited", p.Steps)
	}
	if got := p.MaxToolCalls(); got > 0 {
		t.Errorf("MaxToolCalls = %d, want no ceiling", got)
	}
}

// A profile may deny tools as well as list them, and the permission layer has to
// agree with AllowsTool — the token filter is not the safety gate, so both hold.
func TestDenyRulesReachThePermissionLayer(t *testing.T) {
	p := Profile{Deny: []string{"shell", "write"}}
	for _, tool := range p.Deny {
		if p.AllowsTool(tool) {
			t.Errorf("AllowsTool(%q) = true despite deny", tool)
		}
	}
	rules := p.DenyRules()
	if len(rules) != 2 {
		t.Fatalf("DenyRules() = %d rules, want 2", len(rules))
	}
	cfg := safety.PermissionConfig{Rules: rules}
	if action, matched := cfg.Resolve("shell", []string{"rm -rf /"}); !matched || action != safety.PermissionDeny {
		t.Fatalf("Resolve(shell) = (%q, %v), want deny", action, matched)
	}
}

// The helpers are part of the system (owner's call, 2026-08-06): a user file
// named after a bundled one does NOT shadow it — the bundled profile stays
// authoritative, and the file is reported so it never vanishes silently.
// Shadowing still exists for agents, in their own home (homes_test covers it).
func TestHelperShadowIsIgnoredBundledWins(t *testing.T) {
	dir := isolate(t)
	body := "---\ndescription: ของผมเอง\nmodel: deepseek-v4\nsteps: 5\n---\nBe mine.\n"
	if err := os.WriteFile(filepath.Join(dir, "explore.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p, ok := Load("explore")
	if !ok {
		t.Fatal("Load(explore) failed")
	}
	if !p.Builtin || p.Overrides || p.Model == "deepseek-v4" || p.Prompt == "Be mine." {
		t.Fatalf("the shadow took effect on a system helper: %+v", p)
	}
	if len(p.Tools) != 4 {
		t.Fatalf("bundled explore damaged: %+v", p)
	}
	var seen int
	for _, listed := range List() {
		if listed.Name == "explore" {
			seen++
			if !listed.Builtin {
				t.Error("List() returned the user's explore, not the bundled one")
			}
		}
	}
	if seen != 1 {
		t.Fatalf("explore appears %d times in List()", seen)
	}
	if _, ok := findConflict(Conflicts(), "explore"); !ok {
		t.Fatal("the ignored shadow is not reported — it just vanished")
	}
}

// Nor can a user file add a NEW delegate: the bundled files are the whole set.
func TestHelperHomeCannotAddADelegate(t *testing.T) {
	dir := isolate(t)
	if err := os.WriteFile(filepath.Join(dir, "backend.md"),
		[]byte("---\ndescription: API/DB\ntools: read, grep\n---\nBackend work.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, ok := Load("backend"); ok {
		t.Fatal("a helper-home user file loaded as a delegate")
	}
	if got := len(List()); got != 11 {
		t.Fatalf("List() = %d, want the 11 bundled only", got)
	}
	if c, ok := findConflict(Conflicts(), "backend"); !ok || c.Reason == "" {
		t.Fatal("the locked-out file is not reported with a reason")
	}
}

// The name reaches Load from a tool call the model wrote, so it is a
// path-traversal boundary rather than a formatting concern.
func TestLoadRejectsPathEscapes(t *testing.T) {
	isolate(t)
	for _, name := range []string{"", "..", "../explore", `..\explore`, "sub/explore", "a b"} {
		if _, ok := Load(name); ok {
			t.Errorf("Load(%q) succeeded", name)
		}
	}
}

// Bad input must not drop a profile the user can see sitting on disk.
func TestBrokenFilesStillLoad(t *testing.T) {
	if p := parse("x", "no frontmatter here"); p.Prompt != "no frontmatter here" {
		t.Errorf("bare markdown parsed as %+v", p)
	}
	if p := parse("x", "---\ndescription: unterminated\nbody"); p.Prompt == "" {
		t.Error("broken frontmatter dropped the whole file instead of keeping it as prompt")
	}
}

// `deny:` is enforced at execution rather than by trimming the list the model
// sees: the profile still carries the tool and is refused when it reaches for
// it. `general` is the bundled profile that uses the mechanism — it loops
// through a list of work and must not install a plugin or delete anything while
// doing it.
//
// The `plan` helper guarded this until 2026-08-20, when the owner removed that
// profile: it was never reached for in practice. The mechanism outlived its
// first subject, which is why this test moved rather than went.
func TestADenyListIsEnforcedAtExecution(t *testing.T) {
	isolate(t)
	p, ok := Load("general")
	if !ok {
		t.Fatal("general profile missing")
	}
	cfg := safety.PermissionConfig{Rules: p.DenyRules()}
	for _, tool := range []string{"plugin_install", "delete"} {
		action, matched := cfg.Resolve(tool, nil)
		if !matched || action != safety.PermissionDeny {
			t.Errorf("Resolve(%q) = (%q, %v), want deny — the profile named it and it was let through", tool, action, matched)
		}
	}
	// Doing the work is the whole job, so nothing it needs may be caught by the
	// same net.
	for _, tool := range []string{"read", "grep", "glob", "write", "edit", "shell"} {
		if action, matched := cfg.Resolve(tool, nil); matched && action == safety.PermissionDeny {
			t.Errorf("Resolve(%q) = deny — the delegate cannot do the work it was given", tool)
		}
	}
}

// KindOf is what the engine stamps onto a `task` call's events, so the UI can
// count เอเจน and ซับเอเจน apart (the chip used to read "ซับเอเจน 1 ตัว" on a
// turn where the doc agent made the file). The kind comes from which home the
// file lives in — the same rule the `task` schema's grouping follows — and the
// default an unnamed delegation gets is the same one `task` itself applies.
func TestKindOfSplitsTheTwoPiles(t *testing.T) {
	isolate(t)
	cases := map[string]string{
		"doc":      KindAgent, // a chair in the office
		"sheet":    KindAgent,
		"explore":  KindHelper, // the assistant's own hands
		"general":  KindHelper,
		"research": KindAgent,
		"":         KindHelper, // unnamed → the default profile, which is explore
		"nobody":   "",         // not runnable → no kind claimed
	}
	for name, want := range cases {
		if got := KindOf(name); got != want {
			t.Errorf("KindOf(%q) = %q, want %q", name, got, want)
		}
	}
}

// The field is a footgun with no upside: the home already decides, and the only
// thing writing it can do is take a worker off the roster. Bundled files are
// the examples people copy, so none of them may carry it.
func TestNoBundledAgentWritesItsOwnDesk(t *testing.T) {
	isolate(t)
	for _, p := range List() {
		if p.Desk == "" {
			continue // a helper; it has no home to be given one by
		}
		raw, ok := ReadRaw(p.Name)
		if !ok {
			t.Fatalf("%s: cannot read its own file back", p.Name)
		}
		if strings.Contains(raw, "\ndesk:") {
			t.Errorf("%s writes desk: in its frontmatter — the agents home already gives it the office, and writing it can only ever get it wrong", p.Name)
		}
	}
}

// The half of a description before the dash is what the tool block carries on
// every request (task.go's ForClause), and for the two helpers that exist to be
// reached for at a MOMENT it has to name that moment.
//
// Measured, not argued: while those halves read "รีวิวโค้ด" and "รันเทสต์" —
// job titles — three long sessions with both on the roster called `task` zero
// times. A title says what somebody does and nothing about when to call them.
func TestTheCheckingHelpersNameTheirMoment(t *testing.T) {
	isolate(t)
	for _, name := range []string{"reviewer", "tester"} {
		p, ok := Load(name)
		if !ok {
			t.Fatalf("Load(%s) found nothing", name)
		}
		clause := ForClause(p.Description)
		if !strings.Contains(clause, "ก่อนบอกว่างานเสร็จ") {
			t.Errorf("%s: the block carries %q, which names no moment to reach for it", name, clause)
		}
	}
}
