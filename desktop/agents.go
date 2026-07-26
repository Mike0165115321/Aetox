package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	agentprofile "github.com/Mike0165115321/Aetox/internal/agent"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

// Bindings for the Settings → เอเจน page (ARCHITECTURE.md §44). Thin on purpose:
// every rule about what a profile is lives in internal/agent, so the page cannot
// invent a second definition of one.
//
// Agents and sub-agents get **separate bindings**, not one list with a field to
// filter on — the frontend should not be able to put a sub-agent where the
// session's agent is chosen, and a list it never receives cannot be misused.

// ListAgentProfiles reports the agents the user can talk to.
func (a *App) ListAgentProfiles() []agentprofile.Profile {
	return jsonSlice(agentprofile.List())
}

// ListSubagentProfiles reports the profiles an agent can hand work to.
func (a *App) ListSubagentProfiles() []agentprofile.Profile {
	return jsonSlice(agentprofile.ListSubagents())
}

// ActiveAgent is the profile the main agent is currently running as. Read off the
// live config rather than the preference file so it reports what the engine
// actually bootstrapped with, not what was last written to disk.
func (a *App) ActiveAgent() string {
	if name := strings.TrimSpace(a.cfg.AgentName); name != "" {
		return name
	}
	return agentprofile.DefaultName
}

// PrimaryAgents lists the selectable agent names, for the composer's picker.
func (a *App) PrimaryAgents() []string {
	names := []string{}
	for _, p := range agentprofile.List() {
		names = append(names, p.Name)
	}
	return names
}

// SetActiveAgent switches which profile the main agent runs as. It goes through
// applyConfig — the same path a model switch takes — so the conversation is
// preserved (RestoreHistory) and the new prompt, tool set, step cap and denials
// all take effect at once. A sub-agent name simply does not resolve here, because
// Load never searches that layer.
//
// Returns the fresh ModelInfo like every other switch binding, so the composer
// chip updates from the same struct instead of a second round-trip.
func (a *App) SetActiveAgent(name string) (ModelInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = agentprofile.DefaultName
	}
	if _, ok := agentprofile.Load(name); !ok {
		return a.GetModelInfo(), fmt.Errorf("ไม่พบเอเจนชื่อ %s (ซับเอเจนใช้เป็นเอเจนหลักไม่ได้)", name)
	}
	if name == a.ActiveAgent() {
		return a.GetModelInfo(), nil // never re-bootstrap for no reason
	}
	cfg := a.cfg
	cfg.AgentName = name
	a.applyConfig(cfg)
	if err := a.persistAgentName(name); err != nil {
		return a.GetModelInfo(), err
	}
	return a.GetModelInfo(), nil
}

// persistAgentName stores the choice next to the other preferences. Separate from
// persistModelPreference's load-modify-save (which leaves this field alone) for
// the same reason SetUserName is: a model switch must not overwrite it, and it
// must survive one.
func (a *App) persistAgentName(name string) error {
	pref, ok, err := config.LoadModelPreference()
	if err != nil {
		return err
	}
	if !ok {
		pref = config.ModelPreference{}
	}
	pref.AgentName = strings.TrimSpace(name)
	return config.SaveModelPreference(pref)
}

// ReadAgentProfile returns the raw markdown behind a profile — the user's file if
// there is one, otherwise the bundled original, which is exactly what makes
// "edit a built-in" mean "copy it out and change it". kind picks the layer.
func (a *App) ReadAgentProfile(name, kind string) (string, error) {
	raw, ok := agentprofile.ReadRaw(name, agentprofile.Kind(kind))
	if !ok {
		return "", fmt.Errorf("ไม่พบเอเจนชื่อ %s", name)
	}
	return raw, nil
}

// SaveAgentProfile writes a user profile. kind ("agent"/"subagent") picks the
// directory, which is the only record of which layer it is in. Saving under a
// bundled profile's name creates the shadow. If it is the agent currently in use,
// the engine is re-bootstrapped so the edit is live instead of waiting for a
// restart.
func (a *App) SaveAgentProfile(name, body, kind string) error {
	if err := agentprofile.Save(name, body, agentprofile.Kind(kind)); err != nil {
		return err
	}
	a.reapplyIfActive(name, kind)
	return nil
}

// DeleteAgentProfile removes a user profile from its layer. Deleting a shadow
// restores the bundled profile it was hiding.
func (a *App) DeleteAgentProfile(name, kind string) error {
	if err := agentprofile.Delete(name, agentprofile.Kind(kind)); err != nil {
		return err
	}
	a.reapplyIfActive(name, kind)
	return nil
}

// SetAgentProfileModel pins one profile to a model, or clears the pin when
// modelName is empty ("inherit whatever is selected"). Implemented as a one-line
// frontmatter edit saved as a user file — no second override store.
func (a *App) SetAgentProfileModel(name, kind, modelName string) error {
	if err := agentprofile.SetModel(name, agentprofile.Kind(kind), modelName); err != nil {
		return err
	}
	a.reapplyIfActive(name, kind)
	return nil
}

// reapplyIfActive re-bootstraps only when the edited profile is the agent
// currently running, so editing a sub-agent (or another agent) costs nothing. A
// sub-agent can never be the active one, whatever its name — which is why the
// kind is part of the check rather than the name alone.
func (a *App) reapplyIfActive(name, kind string) {
	if agentprofile.Kind(kind) == agentprofile.KindSubagent {
		return
	}
	if strings.EqualFold(strings.TrimSpace(name), a.ActiveAgent()) {
		a.applyConfig(a.cfg)
	}
}

// OpenAgentsFolder creates both profile directories if needed and reveals the
// data root, so adding a profile is "drop a .md file in the right folder" — same
// contract as the prompts folder, and the reason neither has to exist at install
// time. Both are created so the split is discoverable from the file manager
// rather than from documentation.
func (a *App) OpenAgentsFolder() error {
	dir, err := agentprofile.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if sub, err := agentprofile.SubagentDir(); err == nil {
		_ = os.MkdirAll(sub, 0o755)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	proc.HideConsole(cmd)
	return cmd.Start()
}

// OpenSubagentsFolder is the same for the other layer — two folders, two buttons,
// because a user who is told the layers are separate should not have to navigate
// to find that out.
func (a *App) OpenSubagentsFolder() error {
	dir, err := agentprofile.SubagentDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	proc.HideConsole(cmd)
	return cmd.Start()
}
