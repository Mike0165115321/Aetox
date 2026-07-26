package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

func TestSaveShadowsBundledAndDeleteReverts(t *testing.T) {
	isolate(t)

	raw, ok := ReadRaw("plan", KindAgent)
	if !ok || !strings.Contains(raw, "deny:") {
		t.Fatalf("ReadRaw(plan) = %q, %v", raw, ok)
	}
	if err := Save("plan", "---\ndescription: ของผม\n---\nMine.\n", KindAgent); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if p, _ := Load("plan"); p.Prompt != "Mine." || p.Builtin {
		t.Fatalf("user file not in effect: %+v", p)
	}

	// Deleting the shadow is the revert — the bundled profile comes back with its
	// denials intact rather than the name disappearing from the list.
	if err := Delete("plan", KindAgent); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	p, ok := Load("plan")
	if !ok || !p.Builtin || len(p.Deny) == 0 {
		t.Fatalf("bundled plan did not come back: %+v ok=%v", p, ok)
	}
	if err := Delete("plan", KindAgent); err != nil {
		t.Fatalf("second Delete should be a no-op, got %v", err)
	}
}

// Nothing crosses layers, not even a delete: removing a sub-agent's file must not
// be able to touch an agent of the same name.
func TestWritesStayInTheirLayer(t *testing.T) {
	agents, subagents := isolate(t)
	if err := Save("dup", "---\ndescription: a\n---\nAgent.\n", KindAgent); err != nil {
		t.Fatalf("Save agent: %v", err)
	}
	if err := Save("dup", "---\ndescription: s\n---\nSub.\n", KindSubagent); err != nil {
		t.Fatalf("Save sub-agent: %v", err)
	}
	if err := Delete("dup", KindSubagent); err != nil {
		t.Fatalf("Delete sub-agent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agents, "dup.md")); err != nil {
		t.Errorf("deleting the sub-agent removed the agent too: %v", err)
	}
	if _, err := os.Stat(filepath.Join(subagents, "dup.md")); err == nil {
		t.Error("the sub-agent file survived its own delete")
	}
	if raw, ok := ReadRaw("dup", KindAgent); !ok || !strings.Contains(raw, "Agent.") {
		t.Errorf("ReadRaw(agent) = %q, %v", raw, ok)
	}
}

func TestSaveRejectsBadNames(t *testing.T) {
	isolate(t)
	for _, name := range []string{"", "   ", "..", "../evil", `sub\evil`, "has space", strings.Repeat("x", 41)} {
		if err := Save(name, "body", KindAgent); err == nil {
			t.Errorf("Save(%q) was accepted", name)
		}
	}
	if err := Save("ok-name", "   ", KindAgent); err == nil {
		t.Error("empty body was accepted")
	}
}

// Save's kind argument is the only thing that records the layer, so it has to put
// the file in the right directory — and a typo must land in the agent layer,
// never in the spawn-only one.
func TestSaveWritesToTheDirectoryForItsKind(t *testing.T) {
	agents, subagents := isolate(t)
	if err := Save("backend", "---\ndescription: db\n---\nWork.\n", KindSubagent); err != nil {
		t.Fatalf("Save sub-agent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(subagents, "backend.md")); err != nil {
		t.Fatalf("sub-agent not written to the subagents dir: %v", err)
	}
	if p, ok := LoadSubagent("backend"); !ok || !p.IsSubagent() {
		t.Error("saved sub-agent did not load as one")
	}

	if err := Save("weird", "---\ndescription: x\n---\nWork.\n", Kind("subagnet")); err != nil {
		t.Fatalf("Save typo kind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agents, "weird.md")); err != nil {
		t.Fatalf("a typo'd kind did not fall back to the agent directory: %v", err)
	}
}

// SetModel must edit one line and leave every other key — including ones this
// package does not read — exactly as written.
func TestSetModelEditsOneLine(t *testing.T) {
	agents, subagents := isolate(t)

	if err := SetModel("explore", KindSubagent, "deepseek-v4-flash"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	// Pinning a model must not relocate the profile.
	if _, err := os.Stat(filepath.Join(agents, "explore.md")); err == nil {
		t.Error("SetModel wrote a sub-agent into the agent directory")
	}
	raw, err := os.ReadFile(filepath.Join(subagents, "explore.md"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "model: deepseek-v4-flash") {
		t.Errorf("model line missing:\n%s", text)
	}
	if strings.Count(text, "model:") != 1 {
		t.Errorf("model key appears %d times:\n%s", strings.Count(text, "model:"), text)
	}
	p, _ := LoadSubagent("explore")
	if p.Model != "deepseek-v4-flash" || !p.IsSubagent() || len(p.Tools) != 4 {
		t.Fatalf("other fields damaged: %+v", p)
	}
	if !strings.Contains(p.Prompt, "file-search specialist") {
		t.Errorf("prompt body damaged: %q", p.Prompt)
	}

	// Empty model = inherit: the line goes away instead of being set to "".
	if err := SetModel("explore", KindSubagent, ""); err != nil {
		t.Fatalf("SetModel(clear): %v", err)
	}
	if p, _ := LoadSubagent("explore"); p.Model != "" {
		t.Errorf("model = %q after clearing", p.Model)
	}
}

func TestSetFrontmatterFieldOnUnusualInput(t *testing.T) {
	// No frontmatter at all: one gets added, body preserved.
	got := setFrontmatterField("just a prompt\n", "model", "m1")
	if !strings.HasPrefix(got, "---\nmodel: m1\n---\n") || !strings.Contains(got, "just a prompt") {
		t.Errorf("bare body: %q", got)
	}
	// Nothing to do, nothing changed.
	if got := setFrontmatterField("just a prompt\n", "model", ""); got != "just a prompt\n" {
		t.Errorf("no-op changed the file: %q", got)
	}
	// Unterminated frontmatter is left alone rather than guessed at.
	broken := "---\ndescription: x\nbody"
	if got := setFrontmatterField(broken, "model", "m1"); got != broken {
		t.Errorf("edited a broken file: %q", got)
	}
}

func TestFilterRegistry(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})

	// No tool list, no denials, an agent: the same registry, not a copy.
	build, _ := Load("build")
	if FilterRegistry(parent, build) != parent {
		t.Error("a profile with nothing to filter should reuse the parent registry")
	}

	explore, _ := LoadSubagent("explore")
	child := FilterRegistry(parent, explore)
	if got := len(child.Names()); got != 4 {
		t.Fatalf("explore registry has %d tools, want 4: %v", got, child.Names())
	}
	for _, name := range explore.Tools {
		if _, ok := child.Get(name); !ok {
			t.Errorf("%q missing from the filtered registry", name)
		}
	}
	// Source survives the copy, or the Tools panel would relabel every builtin.
	if src, ok := child.SourceOf("read"); !ok || src != skill.SourceBuiltin {
		t.Errorf("SourceOf(read) = %v, %v", src, ok)
	}

	// A sub-agent cannot be handed task/help even with tools left unset — this is
	// what enforces depth 1.
	general, _ := LoadSubagent("general")
	generalRegistry := FilterRegistry(parent, general)
	for _, name := range forcedSubagentDenials {
		if _, ok := generalRegistry.Get(name); ok {
			t.Errorf("%q reached a sub-agent's registry", name)
		}
	}
	if _, ok := generalRegistry.Get("shell"); !ok {
		t.Error("general lost shell, which it is supposed to inherit")
	}

	plan, _ := Load("plan")
	planRegistry := FilterRegistry(parent, plan)
	for _, name := range plan.Deny {
		if _, ok := planRegistry.Get(name); ok {
			t.Errorf("plan was handed %q", name)
		}
	}
	if _, ok := planRegistry.Get("read"); !ok {
		t.Error("plan lost read")
	}

	if FilterRegistry(nil, plan) != nil {
		t.Error("nil parent should stay nil")
	}
}
