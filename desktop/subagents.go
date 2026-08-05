package main

import (
	"fmt"
	"os"

	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// Bindings for the Settings → ผู้ช่วยตัวแทน page (ARCHITECTURE.md §44). Thin on
// purpose: every rule about what a profile is lives in internal/subagent, so the
// page cannot invent a second definition of one.
//
// There is nothing here about the main agent. It is the assistant — one
// identity, configured by the identity files (§11) — and it is not chosen from a
// list (§44.0).

// ListSubagentProfiles reports the sub-agents the main agent can hand work to.
func (a *App) ListSubagentProfiles() []subagent.Profile {
	return jsonSlice(subagent.List())
}

// ReadSubagentProfile returns the raw markdown behind a profile — the user's file
// if there is one, otherwise the bundled original, which is what makes "edit a
// built-in" mean "copy it out and change it".
func (a *App) ReadSubagentProfile(name string) (string, error) {
	raw, ok := subagent.ReadRaw(name)
	if !ok {
		return "", fmt.Errorf("ไม่พบโปรไฟล์ชื่อ %s", name)
	}
	return raw, nil
}

// SaveSubagentProfile writes a user profile. Saving under a bundled profile's
// name creates the shadow. No re-bootstrap: a sub-agent's profile is read when it
// is spawned, so the next delegation already sees the edit.
//
// Routed by the name's owner: this binding still serves the settings page's
// editor, which today edits agents too, and an agent's edit belongs in the
// agents' home. The answer comes from the resolver (Load), not from a second
// reading of any rule here. Creating a brand-new agent is the team page's door
// (commit 2), never this one.
func (a *App) SaveSubagentProfile(name, body string) error {
	if p, ok := subagent.Load(name); ok && p.Desk != "" {
		return subagent.SaveAgent(name, body)
	}
	return subagent.Save(name, body)
}

// SaveAgentProfile is the team page's door: the file lands in the agents'
// home, which is what makes it an agent — the caller never writes the kind
// into the body, and the backend refuses a name the other kind owns.
func (a *App) SaveAgentProfile(name, body string) error {
	return subagent.SaveAgent(name, body)
}

// DeleteSubagentProfile removes a user profile. Deleting a shadow restores the
// bundled profile it was hiding.
func (a *App) DeleteSubagentProfile(name string) error {
	return subagent.Delete(name)
}

// SetSubagentModel pins one profile to a model, or clears the pin when modelName
// is empty ("inherit whatever is selected"). Implemented as a one-line
// frontmatter edit saved as a user file — no second override store.
func (a *App) SetSubagentModel(name, modelName string) error {
	return subagent.SetModel(name, modelName)
}

// OpenSubagentsFolder creates the sub-agents' home if needed and reveals it, so
// adding a profile is "drop a .md file here" — same contract as the prompts
// folder, and the reason neither has to exist at install time.
func (a *App) OpenSubagentsFolder() error {
	return revealProfileHome(subagent.Dir)
}

// OpenAgentsFolder is the agents' half of the same contract — the office
// page's hiring door. Since the homes split, which folder a file lands in is
// which kind it is, so the two pages must each open their own.
func (a *App) OpenAgentsFolder() error {
	return revealProfileHome(subagent.AgentsDir)
}

func revealProfileHome(home func() (string, error)) error {
	dir, err := home()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// One implementation, in speech.go — the three copies of this switch had
	// all inherited the same window-hiding bug.
	return openInFileManager(dir)
}
