package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// newSubagentTestApp builds an App on an isolated data root, bootstrapped like
// the real one. noop is the provider so nothing reaches the network.
func newSubagentTestApp(t *testing.T) *App {
	t.Helper()
	base := isolateUserDirs(t)
	t.Setenv("AETOX_DATA_ROOT", filepath.Join(base, "data"))
	a := &App{
		// DelegateOn is asked for, because delegation ships off (owner, 18 ส.ค.).
		// Every test in this file is about delegation, so every one of them
		// wants the capability that a fresh install does not have.
		cfg:   config.Config{ModelProvider: "noop", ModelName: "aetox-grid", SandboxRoot: t.TempDir(), DelegateOn: true},
		dbDir: t.TempDir(),
	}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(a.cur(), a.cfg)
	if a.cur().agent == nil || a.cur().registry == nil {
		t.Fatal("engine did not bootstrap")
	}
	return a
}

// The main agent is the assistant, full stop: bootstrapping hands it every tool
// and its system prompt comes from the identity layer, not from a profile (§44.0).
func TestMainAgentIsNotConfiguredByAProfile(t *testing.T) {
	a := newSubagentTestApp(t)

	tools := map[string]bool{}
	for _, list := range [][]SkillInfo{a.ListTools(), a.ListSkills()} {
		for _, s := range list {
			tools[strings.ToLower(s.Name)] = true
		}
	}
	for _, name := range []string{"write", "edit", "shell", "read", "grep"} {
		if !tools[name] {
			t.Errorf("the main agent lost %q", name)
		}
	}

	messages := a.cur().agent.ContextMessages()
	if len(messages) == 0 {
		t.Fatal("no system prompt")
	}
	// The identity/environment layers are there; no profile role text is.
	if !strings.Contains(messages[0].Content, "You are Aetox") {
		t.Errorf("identity layer missing from the system prompt:\n%s", messages[0].Content)
	}
	if strings.Contains(messages[0].Content, "planning, not building") {
		t.Error("a profile role text reached the system prompt")
	}
}

// The sub-agent profiles are managed as files; the bindings are the settings
// page's whole contract with them.
func TestSubagentProfileBindings(t *testing.T) {
	a := newSubagentTestApp(t)

	// Nine bundled: three delegates and six chairs — the office's five (doc,
	// sheet, deck, automation, research) plus github at the coding desk. The
	// settings page lists both kinds; a chair is a sub-agent profile managed
	// exactly like the others, and what makes it a chair is one line of its own
	// frontmatter, not a separate store.
	list := a.ListSubagentProfiles()
	if len(list) != 9 {
		t.Fatalf("ListSubagentProfiles() = %d, want 9 bundled", len(list))
	}

	raw, err := a.ReadSubagentProfile("explore")
	if err != nil {
		t.Fatalf("ReadSubagentProfile: %v", err)
	}
	if !strings.Contains(raw, "file-search specialist") {
		t.Errorf("bundled text not returned: %q", raw)
	}
	if _, err := a.ReadSubagentProfile("nope"); err == nil {
		t.Error("unknown profile returned no error")
	}

	// The helpers are part of the system (owner's call, 2026-08-06): both write
	// doors refuse, and the bundled profile stays exactly as shipped.
	if err := a.SaveSubagentProfile("explore", "---\ndescription: mine\n---\nMine.\n"); err == nil {
		t.Fatal("SaveSubagentProfile wrote over a system helper")
	}
	if err := a.SetSubagentModel("explore", "aetox-grid"); err == nil {
		t.Fatal("SetSubagentModel pinned a model onto a system helper")
	}
	if p, _ := subagent.Load("explore"); !p.Builtin || p.Model != "" {
		t.Fatalf("a refused write still took effect: %+v", p)
	}
	dir, _ := subagent.Dir()
	if _, err := os.Stat(filepath.Join(dir, "explore.md")); !os.IsNotExist(err) {
		t.Fatal("a refused write still left a file in the helpers' home")
	}

	// An agent still takes both: the team is the extensible kind.
	if err := a.SaveAgentProfile("doc", "---\ndescription: mine\n---\nMine.\n"); err != nil {
		t.Fatalf("SaveAgentProfile: %v", err)
	}
	if p, _ := subagent.Load("doc"); p.Prompt != "Mine." || !p.Overrides {
		t.Fatalf("the agent shadow did not take effect: %+v", p)
	}
	if err := a.DeleteSubagentProfile("doc"); err != nil {
		t.Fatalf("DeleteSubagentProfile: %v", err)
	}
	if p, _ := subagent.Load("doc"); !p.Builtin {
		t.Error("deleting the agent's shadow did not restore the bundled profile")
	}
}

// The main agent must actually be handed `task`, or none of the sub-agent work
// is reachable from a chat turn. Bootstrapped the real way, on Aetox's own model.
func TestTaskToolIsRegisteredForTheMainAgent(t *testing.T) {
	a := newSubagentTestApp(t)

	if _, ok := a.cur().registry.Get("task"); !ok {
		t.Fatal("task is not in the main agent's registry — the model can never delegate")
	}
	if src, _ := a.cur().registry.SourceOf("task"); src != skill.SourceBuiltin {
		t.Errorf("task registered as %q, want builtin — it ships with the engine", src)
	}
	// It reaches the model as a tool definition naming the profiles it can pick.
	var def *model.ToolDefinition
	for _, d := range skill.NewDispatcher(a.cur().registry).ToolDefinitions() {
		if d.Function.Name == "task" {
			d := d
			def = &d
		}
	}
	if def == nil {
		t.Fatal("task has no tool definition, so no model can call it")
	}
	if !strings.Contains(string(def.Function.Parameters), "explore") {
		t.Errorf("the task schema does not offer the bundled profiles: %s", def.Function.Parameters)
	}

	// And a delegate never gets it back: depth 1, structurally.
	profile, ok := subagent.Load("explore")
	if !ok {
		t.Fatal("explore profile missing")
	}
	if _, ok := subagent.FilterRegistry(a.cur().registry, profile, nil).Get("task"); ok {
		t.Error("a sub-agent was handed task — it could spawn its own children")
	}
}
