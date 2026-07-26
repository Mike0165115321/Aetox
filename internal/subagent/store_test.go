package subagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

func TestSaveShadowsBundledAndDeleteReverts(t *testing.T) {
	isolate(t)

	raw, ok := ReadRaw("explore")
	if !ok || !strings.Contains(raw, "tools:") {
		t.Fatalf("ReadRaw(explore) = %q, %v", raw, ok)
	}
	if err := Save("explore", "---\ndescription: ของผม\n---\nMine.\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if p, _ := Load("explore"); p.Prompt != "Mine." || p.Builtin {
		t.Fatalf("user file not in effect: %+v", p)
	}

	// Deleting the shadow is the revert — the bundled profile comes back with its
	// tool list intact rather than the name disappearing from the list.
	if err := Delete("explore"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	p, ok := Load("explore")
	if !ok || !p.Builtin || len(p.Tools) != 4 {
		t.Fatalf("bundled explore did not come back: %+v ok=%v", p, ok)
	}
	if err := Delete("explore"); err != nil {
		t.Fatalf("second Delete should be a no-op, got %v", err)
	}
}

func TestSaveRejectsBadNames(t *testing.T) {
	isolate(t)
	for _, name := range []string{"", "   ", "..", "../evil", `sub\evil`, "has space", strings.Repeat("x", 41)} {
		if err := Save(name, "body"); err == nil {
			t.Errorf("Save(%q) was accepted", name)
		}
	}
	if err := Save("ok-name", "   "); err == nil {
		t.Error("empty body was accepted")
	}
}

// SetModel must edit one line and leave every other key — including ones this
// package does not read — exactly as written.
func TestSetModelEditsOneLine(t *testing.T) {
	dir := isolate(t)

	if err := SetModel("explore", "deepseek-v4-flash"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "explore.md"))
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
	p, _ := Load("explore")
	if p.Model != "deepseek-v4-flash" || len(p.Tools) != 4 {
		t.Fatalf("other fields damaged: %+v", p)
	}
	if !strings.Contains(p.Prompt, "file-search specialist") {
		t.Errorf("prompt body damaged: %q", p.Prompt)
	}

	// Empty model = inherit: the line goes away instead of being set to "".
	if err := SetModel("explore", ""); err != nil {
		t.Fatalf("SetModel(clear): %v", err)
	}
	if p, _ := Load("explore"); p.Model != "" {
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

// The registry a sub-agent is handed is a copy holding only what it may use —
// and never `task`, which is what makes depth 1 structural.
func TestFilterRegistry(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})

	explore, _ := Load("explore")
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

	general, _ := Load("general")
	generalRegistry := FilterRegistry(parent, general)
	for _, name := range forcedDenials {
		if _, ok := generalRegistry.Get(name); ok {
			t.Errorf("%q reached a sub-agent's registry", name)
		}
	}
	if _, ok := generalRegistry.Get("shell"); !ok {
		t.Error("general lost shell, which it is supposed to inherit")
	}
	// Even inheriting everything, the copy is never the parent itself — a
	// sub-agent must not be able to mutate the session's registry.
	if generalRegistry == parent {
		t.Error("FilterRegistry handed back the parent registry")
	}

	denied := Profile{Deny: []string{"write", "edit"}}
	deniedRegistry := FilterRegistry(parent, denied)
	for _, name := range denied.Deny {
		if _, ok := deniedRegistry.Get(name); ok {
			t.Errorf("a denied tool %q was handed over anyway", name)
		}
	}

	if FilterRegistry(nil, general) != nil {
		t.Error("nil parent should stay nil")
	}
}
