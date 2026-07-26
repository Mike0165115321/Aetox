package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentprofile "github.com/Mike0165115321/Aetox/internal/agent"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// newAgentTestApp builds an App on an isolated data root, bootstrapped like the
// real one. noop is the provider so nothing reaches the network.
func newAgentTestApp(t *testing.T) *App {
	t.Helper()
	base := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", filepath.Join(base, "data"))
	t.Setenv("APPDATA", base)
	t.Setenv("LOCALAPPDATA", base)
	a := &App{
		cfg:   config.Config{ModelProvider: "noop", ModelName: "aetox-grid", SandboxRoot: t.TempDir()},
		dbDir: t.TempDir(),
	}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(a.cfg)
	if a.agent == nil || a.registry == nil {
		t.Fatal("engine did not bootstrap")
	}
	return a
}

// toolNames is what the agent can actually reach, read through the same two
// bindings the Tools panel and the Settings page use (the split between them is
// builtin-vs-user-added, not what is available — see skillsFrom).
func toolNames(a *App) map[string]bool {
	out := map[string]bool{}
	for _, list := range [][]SkillInfo{a.ListBuiltinSkills(), a.ListSkills()} {
		for _, s := range list {
			out[strings.ToLower(s.Name)] = true
		}
	}
	return out
}

// A fresh install talks to `build` and keeps every tool. Two agents ship, and
// the sub-agent layer is not among the choices.
func TestDefaultAgentIsBuildWithAllTools(t *testing.T) {
	a := newAgentTestApp(t)
	if agentprofile.DefaultName != "build" {
		t.Fatalf("DefaultName = %q, want build", agentprofile.DefaultName)
	}
	if got := a.ActiveAgent(); got != "build" {
		t.Fatalf("ActiveAgent() = %q, want build", got)
	}
	tools := toolNames(a)
	for _, name := range []string{"write", "edit", "shell", "read", "grep"} {
		if !tools[name] {
			t.Errorf("build lost %q", name)
		}
	}
	if got := a.PrimaryAgents(); len(got) != 2 {
		t.Fatalf("PrimaryAgents() = %v, want the two agents only", got)
	}
	if got := len(a.ListSubagentProfiles()); got != 2 {
		t.Fatalf("ListSubagentProfiles() = %d, want 2", got)
	}
	// The two listings are separate: neither may contain the other's entries.
	for _, p := range a.ListAgentProfiles() {
		if p.IsSubagent() {
			t.Errorf("ListAgentProfiles returned sub-agent %q", p.Name)
		}
	}
	for _, p := range a.ListSubagentProfiles() {
		if !p.IsSubagent() {
			t.Errorf("ListSubagentProfiles returned agent %q", p.Name)
		}
	}
}

// Switching to plan must actually take a tool away from the running agent —
// this is the whole wiring under test: preference → applyConfig →
// FilterRegistry → the registry the Tools panel and the dispatcher both read.
func TestSwitchingToPlanRemovesMutatingTools(t *testing.T) {
	a := newAgentTestApp(t)

	if _, err := a.SetActiveAgent("plan"); err != nil {
		t.Fatalf("SetActiveAgent(plan): %v", err)
	}
	if got := a.ActiveAgent(); got != "plan" {
		t.Fatalf("ActiveAgent() = %q after switch", got)
	}
	tools := toolNames(a)
	for _, name := range []string{"write", "edit", "delete", "apply_patch", "shell", "plugin_install"} {
		if tools[name] {
			t.Errorf("plan was handed %q", name)
		}
	}
	for _, name := range []string{"read", "grep", "glob", "list"} {
		if !tools[name] {
			t.Errorf("plan lost %q, but planning has to read", name)
		}
	}

	// And back: switching away restores them, so the filter is per-bootstrap
	// state and not a one-way door.
	if _, err := a.SetActiveAgent("build"); err != nil {
		t.Fatalf("SetActiveAgent(build): %v", err)
	}
	if !toolNames(a)["write"] {
		t.Error("switching back to build did not restore write")
	}
}

// A sub-agent profile is spawn-only; offering it as the main agent would give
// the user a session that cannot do anything.
func TestSubagentProfileCannotBeSelected(t *testing.T) {
	a := newAgentTestApp(t)
	if _, err := a.SetActiveAgent("explore"); err == nil {
		t.Fatal("SetActiveAgent(explore) was accepted")
	}
	if _, err := a.SetActiveAgent("no-such-agent"); err == nil {
		t.Fatal("SetActiveAgent(unknown) was accepted")
	}
	if got := a.ActiveAgent(); got != agentprofile.DefaultName {
		t.Fatalf("a refused switch changed the active agent to %q", got)
	}
}

// The choice has to survive a restart, and a later model switch must not wipe
// it (persistModelPreference does a load-modify-save over the same file).
func TestActiveAgentPersistsAcrossModelSwitch(t *testing.T) {
	a := newAgentTestApp(t)
	if _, err := a.SetActiveAgent("plan"); err != nil {
		t.Fatalf("SetActiveAgent: %v", err)
	}
	persistModelPreference(a.cfg)

	pref, ok, err := config.LoadModelPreference()
	if err != nil || !ok {
		t.Fatalf("LoadModelPreference: ok=%v err=%v", ok, err)
	}
	if pref.AgentName != "plan" {
		t.Fatalf("preference file has agent %q, want plan", pref.AgentName)
	}
	// A fresh resolve (what the next launch does) must come back on code.
	if got := resolveConfig(config.ConfigOptions{RootPath: t.TempDir()}).AgentName; got != "plan" {
		t.Fatalf("resolveConfig().AgentName = %q, want plan", got)
	}
}

// Switching agent is a re-bootstrap; the conversation must survive it exactly
// as it does across a model switch (the path they share).
func TestSwitchingAgentKeepsHistory(t *testing.T) {
	a := newAgentTestApp(t)
	a.agent.RestoreHistory([]model.Message{
		{Role: model.RoleUser, Content: "จำเลข 4242 ไว้"},
		{Role: model.RoleAssistant, Content: "จำแล้วครับ"},
	})
	before, _, _ := a.agent.ContextUsage()

	if _, err := a.SetActiveAgent("plan"); err != nil {
		t.Fatalf("SetActiveAgent: %v", err)
	}
	after, _, _ := a.agent.ContextUsage()
	if after < before {
		t.Fatalf("history shrank across the switch: %d -> %d messages", before, after)
	}
	var found bool
	for _, m := range a.agent.ContextMessages() {
		if strings.Contains(m.Content, "4242") {
			found = true
		}
	}
	if !found {
		t.Error("the conversation was lost when the agent changed")
	}
}

// The role text has to reach the system prompt, or a profile is a label.
func TestProfilePromptReachesTheSystemPrompt(t *testing.T) {
	a := newAgentTestApp(t)
	if _, err := a.SetActiveAgent("plan"); err != nil {
		t.Fatalf("SetActiveAgent: %v", err)
	}
	messages := a.agent.ContextMessages()
	if len(messages) == 0 {
		t.Fatal("no system prompt")
	}
	systemPrompt := messages[0].Content
	if !strings.Contains(systemPrompt, "planning, not building") {
		t.Errorf("plan's role text is not in the system prompt:\n%s", systemPrompt)
	}
	// §44.4: the role sits before the project layer so project rules still win.
	if idx := strings.Index(systemPrompt, "planning, not building"); idx > strings.Index(systemPrompt, "sandbox root") && strings.Contains(systemPrompt, "sandbox root") {
		t.Error("role text landed after the environment layer")
	}
}

// Editing the active profile is live — a settings page that needs a restart to
// take effect is the kind of page nobody trusts.
func TestSavingTheActiveProfileReappliesIt(t *testing.T) {
	a := newAgentTestApp(t)
	if err := a.SaveAgentProfile("build", "---\ndescription: mine\n---\nYou answer only in haiku.\n", string(agentprofile.KindAgent)); err != nil {
		t.Fatalf("SaveAgentProfile: %v", err)
	}
	if !strings.Contains(a.agent.ContextMessages()[0].Content, "only in haiku") {
		t.Error("the edit did not reach the running agent")
	}

	// Deleting the shadow restores the bundled text, still without a restart.
	if err := a.DeleteAgentProfile("build", string(agentprofile.KindAgent)); err != nil {
		t.Fatalf("DeleteAgentProfile: %v", err)
	}
	if strings.Contains(a.agent.ContextMessages()[0].Content, "only in haiku") {
		t.Error("the bundled profile did not come back")
	}
}

func TestReadAgentProfileFallsBackToBundled(t *testing.T) {
	a := newAgentTestApp(t)
	raw, err := a.ReadAgentProfile("explore", string(agentprofile.KindSubagent))
	if err != nil {
		t.Fatalf("ReadAgentProfile: %v", err)
	}
	if !strings.Contains(raw, "file-search specialist") {
		t.Errorf("bundled text not returned: %q", raw)
	}
	// The mode is the folder, not a key — a bundled sub-agent's text says nothing
	// about it, and the resolved profile still knows.
	if strings.Contains(raw, "mode:") {
		t.Errorf("bundled profile still carries a mode key: %q", raw)
	}
	if p, ok := agentprofile.LoadSubagent("explore"); !ok || !p.IsSubagent() {
		t.Errorf("explore did not resolve as a sub-agent: %+v ok=%v", p, ok)
	}
	if _, err := a.ReadAgentProfile("nope", string(agentprofile.KindAgent)); err == nil {
		t.Error("unknown profile returned no error")
	}

	// The model dropdown writes a shadow rather than a second override store —
	// into the sub-agent folder, because that is where explore lives.
	if err := a.SetAgentProfileModel("explore", string(agentprofile.KindSubagent), "aetox-grid"); err != nil {
		t.Fatalf("SetAgentProfileModel: %v", err)
	}
	dir, _ := agentprofile.SubagentDir()
	if _, err := os.Stat(filepath.Join(dir, "explore.md")); err != nil {
		t.Fatalf("no shadow file written: %v", err)
	}
	for _, p := range a.ListAgentProfiles() {
		if p.Name == "explore" && p.Model != "aetox-grid" {
			t.Errorf("model pin not listed: %+v", p)
		}
	}
}
