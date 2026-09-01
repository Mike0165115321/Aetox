package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/subagent"
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
		rows[i].Icon = chairIcon(rows[i])
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
	return a.revealProfileHome(subagent.Dir)
}

// OpenAgentsFolder is the agents' half of the same contract — the office
// page's hiring door. Since the homes split, which folder a file lands in is
// which kind it is, so the two pages must each open their own.
func (a *App) OpenAgentsFolder() error {
	return a.revealProfileHome(subagent.AgentsDir)
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
			Name: s.Name, Description: s.Description, Bundled: s.Bundled,
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
	return a.revealProfileHome(func() (string, error) { return config.AgentSkillsPath(name) })
}

func (a *App) revealProfileHome(home func() (string, error)) error {
	dir, err := home()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// One implementation, in speech.go — the three copies of this switch had
	// all inherited the same window-hiding bug.
	return a.revealInFileManager(dir)
}

// AgentGate is why a teammate is behind a lock, and what the one button on it
// should do. One call rather than three, because the veil, its sentence and its
// button have to agree, and three separate questions is how they stop agreeing.
type AgentGate struct {
	Blocked bool `json:"blocked"`
	// Kind is what would fix it:
	//   "install"    — Aetox fetches it (Capability names what to fetch)
	//   "connect"    — the user's own account or a service; the settings page owns it
	//   "incomplete" — the tool is there and cannot work; Missing names what it wants
	Kind       string   `json:"kind,omitempty"`
	Capability string   `json:"capability,omitempty"`
	Missing    []string `json:"missing"`
}

// AgentGate decides whether an agent is offered as a working teammate.
//
// **`needs:` means needs.** It was briefly read as "would be improved by", on
// the reasoning that `research` still has web_search without firecrawl and so
// would be locked for nothing. The owner refused that reading on the ground
// that answers the question properly (30 ส.ค.): *"บังคับให้มี firecrawl ครับ
// ไม่งั้นมันจะต่างอะไรจากเมน"* — a research agent whose only tools are the ones
// the assistant already carries is the assistant with a different prompt, and
// offering it as a specialist is the lie, not locking it.
//
// So the fix for an over-locked agent is never a cleverer rule here. It is the
// agent's own file: an entry that is genuinely optional does not belong on the
// `needs:` line, and belongs in the prose instead. `github` lost `mcp:github`
// that way — it reads repositories through its own tool without it.
//
// **And a met requirement is not a working tool**, which is the second half and
// the one that was missed. kinocut connects, answers a handshake, registers all
// its tools — and then refuses every encode because there is no ffmpeg on the
// machine. An agent in that state is not "working with a warning", it is an
// agent that will fail on the job it was hired for, and the owner said so in as
// many words: *"เครื่องมือไม่ครบ กูบอกให้ใช้งานไม่ได้ไง"*. So the tool's own
// verdict counts as much as whether it is there at all.
//
// This does not change what `needs:` grants, which is still nothing (needs.go).
// Declaring and granting stay separate; what is new is that an unmet
// requirement, or a tool that says it cannot work, hides the door rather than
// only warning behind it.
func (a *App) AgentGate(name string) AgentGate {
	gate := AgentGate{Missing: []string{}} // never nil: §34
	p, ok := subagent.Load(name)
	if !ok {
		return gate
	}

	for _, req := range subagent.Requirements(p) {
		if req.Met {
			continue
		}
		gate.Blocked = true
		gate.Kind = "connect"
		// Alternatives are the user's choice between products ("n8n or
		// Windmill") and never ours to resolve, so only a single-option entry
		// naming something Aetox itself fetches becomes a download.
		if len(req.Options) == 1 && req.Options[0].Kind == subagent.NeedMCP {
			if c := a.CapabilityForServer(req.Options[0].ID); c != "" {
				gate.Kind = "install"
				gate.Capability = c
			}
		}
		for _, opt := range req.Options {
			gate.Missing = append(gate.Missing, opt.Label)
		}
		return gate
	}

	// Everything declared is present. Now ask the tools themselves.
	for _, req := range subagent.Requirements(p) {
		for _, opt := range req.Options {
			if opt.Kind != subagent.NeedMCP || !strings.EqualFold(opt.ID, VideoEditorServer) {
				continue
			}
			if short := a.VideoToolingStatus().MissingRequired; len(short) > 0 {
				// Still "incomplete" rather than "install", because the sentence
				// on the veil is a different one — the editor is here and cannot
				// work, which is not the same news as it being absent. The
				// capability is carried anyway, so the button opens Aetox's own
				// install report rather than a download page: everything named
				// here is something this app fetches.
				return AgentGate{
					Blocked:    true,
					Kind:       "incomplete",
					Capability: VideoEditorCapability,
					Missing:    short,
				}
			}
		}
	}

	// And the renderer, for the agent that makes videos.
	//
	// **This is the hole that opened the day `needs: mcp:kinocut` came off that
	// agent.** Everything above judges an agent by what its profile declares,
	// and declaring is how an MCP server gets named — but the scene renderer is
	// not a server, it is a program this app installs, so nothing in the profile
	// mentions it and the card sat unveiled on a machine that could not render a
	// single frame.
	//
	// Read off the agent's own skills rather than off its name: the folder that
	// teaches an agent to build a scene is the thing that means it will render
	// one, and renaming the agent cannot quietly turn the check off.
	//
	// It read the agent's `tools:` line until 31 ส.ค., which was the right
	// answer while agents differed by what they carried. They stopped: every
	// agent holds the same kit now and differs by its skills and the servers
	// pointed at it, so `video_render` in a tool list would veil every card on
	// a machine with no renderer — including the six agents that never render
	// anything.
	if profileNeedsSceneRenderer(p) && sceneRendererMissing() {
		// **`enable`, and nothing named.** The first version of this returned
		// the parts — "ตัวเรนเดอร์ฉาก, เบราว์เซอร์" — on the reasoning that a
		// veil saying only "something is missing" sends the reader off to find
		// out which. That reasoning is right for a tool somebody chose and wrong
		// here: nobody asked for a scene renderer or a headless browser, they
		// asked to make a video, and listing our own parts makes them read a
		// bill of materials to answer a question they did not have. Owner,
		// 30 ส.ค.: *"ทำไมไม่ขึ้นว่าติดตั้งตัวแยกย่อยแบบนี้ บอกว่าเปิดใช้งาน
		// เอเจนสร้างวิดีโอ"*. The readiness panel is where the parts belong, and
		// it lists every one of them with its size.
		//
		// Pointed at the make group alone, and deliberately not at the shared
		// ffmpeg as well: this button installs exactly one capability, so gating
		// on something it does not fetch would leave the veil up after a
		// successful press and send somebody round the same loop twice.
		return AgentGate{
			Blocked:    true,
			Kind:       "enable",
			Capability: VideoMakeCapability,
			Missing:    []string{},
		}
	}
	return gate
}

// sceneTemplatesSkill is the folder an agent carries when building scenes is
// its job. Named here because this is the only place that reads it as a
// signal; the skill itself is ordinary knowledge and does not know it is one.
const sceneTemplatesSkill = "video-templates"

// profileNeedsSceneRenderer reports whether building scenes is what this agent
// is for, by the knowledge it carries in its own folder.
func profileNeedsSceneRenderer(p subagent.Profile) bool {
	own, _ := subagent.OwnSkills(p.Name)
	for _, s := range own {
		if strings.EqualFold(strings.TrimSpace(s.Name), sceneTemplatesSkill) {
			return true
		}
	}
	return false
}

// sceneRendererMissing reports whether either half of the renderer is absent.
//
// A boolean rather than the list of names it used to return: the veil asks one
// question — can this agent work — and the parts belong on the readiness panel,
// which names every one of them with its size. See the gate above for the whole
// argument.
func sceneRendererMissing() bool {
	node, _ := hyperframesParts()
	return node == "" || findSceneBrowser() == ""
}

// AgentBlocked is AgentGate's first field, for callers that only draw the veil.
func (a *App) AgentBlocked(name string) bool { return a.AgentGate(name).Blocked }
