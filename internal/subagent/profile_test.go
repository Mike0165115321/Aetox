package subagent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/safety"
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

// Two sub-agents ship. The count is asserted because "one more profile, it's
// free" is how a bundled set becomes a menu nobody reads.
func TestBundledProfilesAreUsable(t *testing.T) {
	isolate(t)
	got := List()
	want := []string{"explore", "general"}
	if len(got) != len(want) {
		t.Fatalf("List() = %d profiles, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p.Name != want[i] {
			t.Errorf("List()[%d] = %q, want %q (alphabetical)", i, p.Name, want[i])
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
		// Every sub-agent is capped: nobody is watching its loop.
		if p.MaxToolCalls() <= 0 {
			t.Errorf("%s: MaxToolCalls = %d, want a cap", p.Name, p.MaxToolCalls())
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

// general has no tools: line at all, so it inherits the registry — the forced
// denials are the only thing standing between it and spawning its own children.
func TestGeneralInheritsToolsButNotTask(t *testing.T) {
	isolate(t)
	p, ok := Load("general")
	if !ok {
		t.Fatal("general profile missing")
	}
	if len(p.Tools) != 0 {
		t.Fatalf("general tools = %v, want empty (inherit)", p.Tools)
	}
	if !p.AllowsTool("shell") || !p.AllowsTool("write") {
		t.Error("general should inherit the mutating tools")
	}
	for _, tool := range forcedDenials {
		if p.AllowsTool(tool) {
			t.Errorf("general allows %q", tool)
		}
	}
}

func TestMaxToolCalls(t *testing.T) {
	if got := (Profile{}).MaxToolCalls(); got != defaultSteps {
		t.Errorf("MaxToolCalls = %d, want the default cap %d", got, defaultSteps)
	}
	if got := (Profile{Steps: 3}).MaxToolCalls(); got != 3 {
		t.Errorf("steps override = %d, want 3", got)
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

func TestUserFileShadowsBundled(t *testing.T) {
	dir := isolate(t)
	body := "---\ndescription: ของผมเอง\nmodel: deepseek-v4\nsteps: 5\n---\nBe mine.\n"
	if err := os.WriteFile(filepath.Join(dir, "explore.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p, ok := Load("explore")
	if !ok {
		t.Fatal("Load(explore) failed")
	}
	if p.Prompt != "Be mine." || p.Model != "deepseek-v4" || p.Steps != 5 {
		t.Fatalf("user file not used: %+v", p)
	}
	if p.Builtin || p.Path == "" {
		t.Errorf("Builtin=%v Path=%q, want false and a real path", p.Builtin, p.Path)
	}
	// The settings page groups by source, so a shadow has to declare itself: it
	// belongs under "yours", and deleting it reverts rather than removes.
	if !p.Overrides {
		t.Error("a user file shadowing a bundled profile did not set Overrides")
	}

	// Shadowing replaces, it does not duplicate.
	var seen int
	for _, listed := range List() {
		if listed.Name == "explore" {
			seen++
			if listed.Builtin {
				t.Error("List() returned the bundled explore, not the user's")
			}
		}
	}
	if seen != 1 {
		t.Fatalf("explore appears %d times in List()", seen)
	}
}

func TestUserProfileAddsToTheList(t *testing.T) {
	dir := isolate(t)
	if err := os.WriteFile(filepath.Join(dir, "backend.md"),
		[]byte("---\ndescription: API/DB\ntools: read, grep\n---\nBackend work.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p, ok := Load("backend")
	if !ok {
		t.Fatal("user profile not found")
	}
	if !slices.Equal(p.Tools, []string{"read", "grep"}) {
		t.Fatalf("parsed wrong: %+v", p)
	}
	// A name of its own overrides nothing.
	if p.Overrides {
		t.Error("a user-only profile claims to override a bundled one")
	}
	if got := len(List()); got != 3 {
		t.Fatalf("List() = %d, want 3 (2 bundled + 1 user)", got)
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
