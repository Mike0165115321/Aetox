package main

import (
	"fmt"
	"os"

	"github.com/Mike0165115321/Aetox/internal/config"
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
	rows := jsonSlice(subagent.List())
	// Icon leaves here filled in, the same way the roster's does (office.go).
	// In a *list* the field means "the mark to draw", never "what the file
	// says" — the editor reads the file itself for its picker, so the one place
	// that has to know a blank means "choose for me" is chairIcon. Resolving it
	// twice, or in TypeScript, is how the same agent ends up wearing two faces
	// on two pages.
	for i := range rows {
		rows[i].Icon = chairIcon(rows[i], rows[i].Tools)
	}
	return rows
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

// AgentSkillInfo is one entry on an agent's own shelf, for the editor's สกิล
// panel. Not SkillInfo: that type carries a Source and a Category, which are
// the shared shelf's questions ("did I install this", "what is it for"). Here
// every entry has the same source by construction, and the only fact the reader
// needs beyond the description is whether it came in the box — because that is
// what decides whether their own file can replace it.
type AgentSkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Bundled     bool   `json:"bundled"`
}

// AgentSkills lists what one agent knows out of its own folder — the answer to
// "what can this one look up that the others cannot".
//
// Read through subagent.OwnSkills, the same call the dispatcher makes, so this
// page cannot show a shelf the running agent does not have. Errors are the
// scanner's own (a malformed SKILL.md) and are dropped here for the same reason
// the dispatcher logs and continues: one bad file must not blank the panel that
// would let the user find it.
func (a *App) AgentSkills(name string) []AgentSkillInfo {
	own, _ := subagent.OwnSkills(name)
	out := make([]AgentSkillInfo, 0, len(own)) // never nil: §34
	for _, s := range own {
		out = append(out, AgentSkillInfo{
			Name: s.Skill.Name(), Description: s.Skill.Description(), Bundled: s.Bundled,
		})
	}
	return out
}

// AgentNeeds reports every `needs:` entry an agent declared, with each way of
// satisfying it and where that one stands.
//
// The editor's "ต้องใช้" panel, and the first place this answer has ever been
// on screen: the engine has computed it since needs.go was written, but only
// ever folded it into the agent's own prompt (PromptFor), so an agent that
// could not work said so in conversation while the page you configure it on
// showed nothing.
//
// Requirements rather than UnmetNeeds. The automation agent needs "n8n or
// Windmill", and the unmet list is one line about whichever is nearest — which
// on screen read as "n8n is required" and hid both the alternative and the fact
// that either would do. What the page has to draw is the whole entry: what
// would satisfy it, and which of those is already on.
//
// A name with no profile returns nothing rather than an error: the editor asks
// this while the user is typing a new agent's name, and a red box on every
// keystroke of a name that does not exist yet is noise.
func (a *App) AgentNeeds(name string) []subagent.Requirement {
	p, ok := subagent.Load(name)
	if !ok {
		return []subagent.Requirement{} // never nil: §34
	}
	return subagent.Requirements(p)
}

// OpenAgentSkillsFolder reveals one agent's skills folder, creating it if this
// is the first time anyone has looked.
//
// Created on open, unlike config.AgentSkillsPath which deliberately does not:
// the folder is absent in the normal case, and "open the place where they go"
// is the one moment where an empty folder is the useful answer rather than a
// cost paid on every dispatch.
func (a *App) OpenAgentSkillsFolder(name string) error {
	return revealProfileHome(func() (string, error) { return config.AgentSkillsPath(name) })
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
