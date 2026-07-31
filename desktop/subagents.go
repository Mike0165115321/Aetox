package main

import (
	"fmt"
	"os"

	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// Bindings for the Settings → ซับเอเจน page (ARCHITECTURE.md §44). Thin on
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
		return "", fmt.Errorf("ไม่พบซับเอเจนชื่อ %s", name)
	}
	return raw, nil
}

// SaveSubagentProfile writes a user profile. Saving under a bundled profile's
// name creates the shadow. No re-bootstrap: a sub-agent's profile is read when it
// is spawned, so the next delegation already sees the edit.
func (a *App) SaveSubagentProfile(name, body string) error {
	return subagent.Save(name, body)
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

// OpenSubagentsFolder creates <DataRoot>/subagents if needed and reveals it, so
// adding a profile is "drop a .md file here" — same contract as the prompts
// folder, and the reason neither has to exist at install time.
func (a *App) OpenSubagentsFolder() error {
	dir, err := subagent.Dir()
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
