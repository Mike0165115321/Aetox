package agent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/safety"
)

// isolate points DataRoot at a fresh temp dir so a developer's real profiles can
// neither leak into these tests nor be written by them. Returns both layers'
// directories — two siblings, because the directory is the only thing that
// records which layer a profile is in.
func isolate(t *testing.T) (agents, subagents string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	agents, subagents = filepath.Join(root, "agents"), filepath.Join(root, "subagents")
	for _, dir := range []string{agents, subagents} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return agents, subagents
}

// Two agents and two sub-agents ship (§44.3, cut from five on the owner's call).
// The count is asserted because "one more profile, it's free" is how a bundled
// set becomes a menu nobody reads.
func TestBundledProfilesAreUsable(t *testing.T) {
	isolate(t)
	for _, tc := range []struct {
		kind  Kind
		names []string
		list  func() []Profile
	}{
		{KindAgent, []string{"build", "plan"}, List},
		{KindSubagent, []string{"explore", "general"}, ListSubagents},
	} {
		got := tc.list()
		if len(got) != len(tc.names) {
			t.Fatalf("%s: got %d profiles, want %d", tc.kind, len(got), len(tc.names))
		}
		for i, p := range got {
			if p.Name != tc.names[i] {
				t.Errorf("%s[%d] = %q, want %q (alphabetical)", tc.kind, i, p.Name, tc.names[i])
			}
			if p.Kind != tc.kind {
				t.Errorf("%s: %s has kind %q", tc.kind, p.Name, p.Kind)
			}
			if !p.Builtin || p.Path != "" {
				t.Errorf("%s: Builtin=%v Path=%q, want true and empty", p.Name, p.Builtin, p.Path)
			}
			// No description = invisible in the settings row; no prompt = the
			// default agent wearing a different name.
			if p.Description == "" {
				t.Errorf("%s: no description", p.Name)
			}
			if len(p.Prompt) < 40 {
				t.Errorf("%s: prompt is %d chars, expected a real role", p.Name, len(p.Prompt))
			}
		}
	}
}

// The layers do not leak into each other. This is the whole point of the split:
// a sub-agent must not be reachable where the session's agent is chosen.
func TestLayersDoNotLeak(t *testing.T) {
	isolate(t)
	if _, ok := Load("explore"); ok {
		t.Error("Load found a sub-agent — it must only search agents")
	}
	if _, ok := Load("general"); ok {
		t.Error("Load found a sub-agent — it must only search agents")
	}
	if _, ok := LoadSubagent("build"); ok {
		t.Error("LoadSubagent found an agent — the task tool must not spawn the session's agent")
	}
	if _, ok := LoadSubagent("plan"); ok {
		t.Error("LoadSubagent found an agent")
	}
	for _, p := range List() {
		if p.IsSubagent() {
			t.Errorf("List returned sub-agent %q", p.Name)
		}
	}
	for _, p := range ListSubagents() {
		if !p.IsSubagent() {
			t.Errorf("ListSubagents returned agent %q", p.Name)
		}
	}
}

func TestDefaultNameResolves(t *testing.T) {
	isolate(t)
	p, ok := Load("")
	if !ok || p.Name != DefaultName {
		t.Fatalf("Load(\"\") = %q, %v; want %q", p.Name, ok, DefaultName)
	}
	if p := LoadOrDefault("nope-not-a-profile"); p.Name != DefaultName {
		t.Fatalf("LoadOrDefault(unknown).Name = %q, want %q", p.Name, DefaultName)
	}
	// A sub-agent name is "unknown" as far as the session is concerned, so it
	// falls back rather than resolving into the other layer.
	if p := LoadOrDefault("explore"); p.Name != DefaultName {
		t.Fatalf("LoadOrDefault(explore).Name = %q, want %q", p.Name, DefaultName)
	}
}

// plan is the whole "a mode is a permission ruleset, not a code path" argument
// (§44.3). If a mutator ever falls off this list, plan silently becomes build.
func TestPlanDeniesEveryMutator(t *testing.T) {
	isolate(t)
	p, ok := Load("plan")
	if !ok {
		t.Fatal("plan profile missing")
	}
	for _, tool := range []string{"write", "edit", "delete", "apply_patch", "shell", "plugin_install"} {
		if p.AllowsTool(tool) {
			t.Errorf("plan allows %q", tool)
		}
	}
	for _, tool := range []string{"read", "grep", "glob", "list", "diagnostics"} {
		if !p.AllowsTool(tool) {
			t.Errorf("plan denies %q, but planning needs to read", tool)
		}
	}
	rules := p.DenyRules()
	if len(rules) != len(p.Deny) {
		t.Fatalf("DenyRules() = %d rules for %d denials", len(rules), len(p.Deny))
	}
	for _, r := range rules {
		if r.Action != safety.PermissionDeny {
			t.Errorf("rule for %q has action %q, want deny", r.Tool, r.Action)
		}
	}
	// The permission layer must agree with AllowsTool — the token filter is not
	// the safety gate, so both have to hold.
	cfg := safety.PermissionConfig{Rules: rules}
	if action, matched := cfg.Resolve("write", []string{"main.go"}); !matched || action != safety.PermissionDeny {
		t.Fatalf("Resolve(write) = (%q, %v), want deny", action, matched)
	}
}

// build is the default, so it has to be able to do everything the three-profile
// set could: talk, read, and change files.
func TestBuildKeepsEveryTool(t *testing.T) {
	isolate(t)
	p, ok := Load("build")
	if !ok {
		t.Fatal("build profile missing")
	}
	if len(p.Tools) != 0 || len(p.Deny) != 0 {
		t.Fatalf("build should filter nothing: tools=%v deny=%v", p.Tools, p.Deny)
	}
	for _, tool := range []string{"write", "edit", "shell", "read", "grep", "web_search"} {
		if !p.AllowsTool(tool) {
			t.Errorf("build lost %q", tool)
		}
	}
	if p.MaxToolCalls() != 0 {
		t.Errorf("MaxToolCalls = %d, want 0 (unbounded, a human is watching)", p.MaxToolCalls())
	}
}

func TestExploreIsReadOnlyAndCannotRecurse(t *testing.T) {
	isolate(t)
	p, ok := LoadSubagent("explore")
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
	for _, tool := range forcedSubagentDenials {
		if p.AllowsTool(tool) {
			t.Errorf("explore allows %q, which every sub-agent is refused", tool)
		}
	}
}

// general has no tools: line at all, so it inherits the registry — the forced
// denials are the only thing standing between it and spawning its own children.
func TestGeneralInheritsToolsButNotTask(t *testing.T) {
	isolate(t)
	p, ok := LoadSubagent("general")
	if !ok {
		t.Fatal("general profile missing")
	}
	if len(p.Tools) != 0 {
		t.Fatalf("general tools = %v, want empty (inherit)", p.Tools)
	}
	if !p.AllowsTool("shell") || !p.AllowsTool("write") {
		t.Error("general should inherit the mutating tools")
	}
	for _, tool := range forcedSubagentDenials {
		if p.AllowsTool(tool) {
			t.Errorf("general allows %q", tool)
		}
	}
}

func TestMaxToolCalls(t *testing.T) {
	if got := (Profile{Kind: KindAgent}).MaxToolCalls(); got != 0 {
		t.Errorf("agent MaxToolCalls = %d, want 0 (unbounded, a human is watching)", got)
	}
	if got := (Profile{Kind: KindSubagent}).MaxToolCalls(); got != subagentSteps {
		t.Errorf("sub-agent MaxToolCalls = %d, want %d", got, subagentSteps)
	}
	if got := (Profile{Kind: KindSubagent, Steps: 3}).MaxToolCalls(); got != 3 {
		t.Errorf("steps override = %d, want 3", got)
	}
}

func TestUserFileShadowsBundled(t *testing.T) {
	agents, _ := isolate(t)
	body := "---\ndescription: ของผมเอง\nmodel: deepseek-v4\nsteps: 5\n---\nBe mine.\n"
	if err := os.WriteFile(filepath.Join(agents, "build.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p, ok := Load("build")
	if !ok {
		t.Fatal("Load(build) failed")
	}
	if p.Prompt != "Be mine." || p.Model != "deepseek-v4" || p.Steps != 5 {
		t.Fatalf("user file not used: %+v", p)
	}
	if p.Builtin || p.Path == "" {
		t.Errorf("Builtin=%v Path=%q, want false and a real path", p.Builtin, p.Path)
	}

	// Shadowing replaces, it does not duplicate.
	var seen int
	for _, listed := range List() {
		if listed.Name == "build" {
			seen++
			if listed.Builtin {
				t.Error("List() returned the bundled build, not the user's")
			}
		}
	}
	if seen != 1 {
		t.Fatalf("build appears %d times in List()", seen)
	}
}

// The directory decides the kind, and the same bytes in the other directory are
// the other kind. No key in the file has a say.
func TestTheDirectoryDecidesTheKind(t *testing.T) {
	agents, subagents := isolate(t)
	body := []byte("---\ndescription: API/DB\ntools: read, grep\n---\nBackend work.\n")
	if err := os.WriteFile(filepath.Join(subagents, "backend.md"), body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p, ok := LoadSubagent("backend")
	if !ok {
		t.Fatal("user sub-agent not found")
	}
	if !p.IsSubagent() || !slices.Equal(p.Tools, []string{"read", "grep"}) {
		t.Fatalf("parsed wrong: %+v", p)
	}
	if _, ok := Load("backend"); ok {
		t.Error("a user sub-agent showed up in the agent layer")
	}
	if got := len(ListSubagents()); got != 3 {
		t.Fatalf("ListSubagents() = %d, want 3 (2 bundled + 1 user)", got)
	}
	if got := len(List()); got != 2 {
		t.Fatalf("List() = %d, want 2 — a user sub-agent must not add to the agent list", got)
	}

	// A stray `kind:`/`mode:` key is not read at all, so it cannot contradict the
	// directory it sits in. That contradiction is the debt this split removed.
	if err := os.WriteFile(filepath.Join(agents, "frontend.md"),
		[]byte("---\ndescription: UI\nkind: subagent\nmode: subagent\n---\nFrontend work.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	q, ok := Load("frontend")
	if !ok {
		t.Fatal("frontend not found")
	}
	if q.IsSubagent() {
		t.Error("a key in the file overrode the directory — the second source of truth is back")
	}
	if q.MaxToolCalls() != 0 {
		t.Errorf("MaxToolCalls = %d, want 0: it is an agent because of its directory", q.MaxToolCalls())
	}
}

// Both layers may hold the same name and they stay two different profiles —
// there is no tie-break rule to get wrong because nothing ever searches both.
func TestSameNameInBothLayersStaysTwoProfiles(t *testing.T) {
	agents, subagents := isolate(t)
	if err := os.WriteFile(filepath.Join(agents, "dup.md"), []byte("---\ndescription: a\n---\nAgent.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subagents, "dup.md"), []byte("---\ndescription: s\n---\nSub.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if p, ok := Load("dup"); !ok || p.Prompt != "Agent." || p.IsSubagent() {
		t.Fatalf("Load(dup) = %+v, ok=%v", p, ok)
	}
	if p, ok := LoadSubagent("dup"); !ok || p.Prompt != "Sub." || !p.IsSubagent() {
		t.Fatalf("LoadSubagent(dup) = %+v, ok=%v", p, ok)
	}
}

// The selector reaches Load from a hand-editable preference file, so it is a
// path-traversal boundary rather than a formatting concern.
func TestLoadRejectsPathEscapes(t *testing.T) {
	isolate(t)
	for _, name := range []string{"..", "../build", `..\build`, "sub/build", "a b"} {
		if _, ok := Load(name); ok {
			t.Errorf("Load(%q) succeeded", name)
		}
		if _, ok := LoadSubagent(name); ok {
			t.Errorf("LoadSubagent(%q) succeeded", name)
		}
	}
}

// Bad input must not drop a profile the user can see sitting on disk.
func TestBrokenFilesStillLoad(t *testing.T) {
	if p := parse("x", "no frontmatter here", KindAgent); p.Kind != KindAgent || p.Prompt != "no frontmatter here" {
		t.Errorf("bare markdown parsed as %+v", p)
	}
	if p := parse("x", "---\ndescription: unterminated\nbody", KindSubagent); p.Prompt == "" {
		t.Error("broken frontmatter dropped the whole file instead of keeping it as prompt")
	} else if !p.IsSubagent() {
		t.Error("a broken file still belongs to the layer it was found in")
	}
	// An unrecognized kind never lands in the sub-agent layer: a typo must not be
	// able to make a profile spawn-only.
	if p := parse("x", "body", Kind("subagnet")); p.Kind != KindAgent {
		t.Errorf("kind = %q, want %q", p.Kind, KindAgent)
	}
}
