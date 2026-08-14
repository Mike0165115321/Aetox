// Package subagent turns a markdown file into a sub-agent profile: who it is
// (the body), which tools it may be handed (Tools), which are denied outright
// (Deny), and how long its loop may run (Steps). ARCHITECTURE.md §44.
//
// A sub-agent is what the main agent hands work to. **There is no profile for
// the main agent** — it is one assistant with one identity, configured by the
// identity layer (internal/prompt, §11) and nothing else. Profiles here answer
// "who am I delegating to", never "who am I talking to" (§44.0).
//
// Profiles come in two kinds, and **the file's home is its kind** (owner's
// call, 2026-08-05 — เอเจน/ซับเอเจน):
//
//   - **Agents** — the team the user can see: they take jobs over the counter
//     and hold direct chats (§85). Home: profiles/agents (bundled) and
//     <DataRoot>/agents (user). A file here that names no desk sits in the
//     office; naming one is allowed and means that desk. This is the
//     extensible kind: hiring is dropping one more file.
//   - **Sub-agents** — the assistant's own hands, never chatted with, and
//     **part of the system** (owner's call, 2026-08-06: ผูกซับเอเจนกับระบบ
//     เพิ่มหรือแก้ไขไม่ได้): the bundled three in profiles/subagents are the
//     whole set. A user file in <DataRoot>/subagents is not read — it is
//     reported as a Conflict so it never vanishes silently — and the write
//     door (Save) refuses. What the user extends is the team, never the hands.
//
// That rule is decided in this file's resolver and nowhere else. The bindings
// and both pages ask the loaded Profile; the day one of them re-derives the
// kind from a path or a field is the day two answers exist again.
//
// Within one home, same shape as prompt presets (§35): bundled files are
// compiled in so a fresh install works with nothing to download, and a user
// file with the same name **in the same home** wins — editing a shipped
// profile is copying it out and changing it, never fighting the app. Across
// homes a name has one owner (memory, jobs and chat history all key on the
// bare name), so the same name in the other home is a conflict, not a shadow.
//
// A profile carries no execution logic. It is data that three existing knobs
// read: cognitive.AgentConfig (prompt, model, MaxToolCalls),
// safety.PermissionConfig (Deny), and the skill registry handed to the
// sub-agent (Tools).
package subagent

import (
	"embed"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/learned"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// The whole tree rather than the two file patterns it used to name. An agent is
// a package (config.AgentHome), and a package that could only ship its
// definition would make every shipped worker's knowledge a thing the user has
// to download — while a user's own worker keeps its skills in a folder beside
// its AGENT.md. Two layouts for one kind of thing is the debt; this is one.
//
//go:embed profiles
var bundledProfiles embed.FS

// The bundled halves of the two homes. Their names never reach a user; the
// on-disk halves are AgentsDir and Dir.
//
// The two shapes are the two kinds. An agent is a **folder** — one worker, one
// package, room for the memory it accumulates and the servers it will bring
// (config.AgentHome) — and the shipped three are laid out exactly like one the
// user writes, so "copy it out and change it" stays a copy rather than a
// translation. A helper is one flat file, because the helpers are part of the
// system and there is nothing per-helper to package: the bundled set is the
// whole set.
const (
	bundledAgentDir  = "profiles/agents"
	bundledHelperDir = "profiles/subagents"
)

// StepsUnlimited is the value a removed ceiling is carried as. Distinct from 0,
// which is what an absent or unreadable `steps:` gives and means "whatever the
// default is" rather than anything itself.
//
// The loop it feeds already understands it: cognitive.Agent treats
// MaxToolCalls <= 0 as unbounded, which is exactly how the main agent runs. So
// this is not a second mode — it is the one that already existed.
const StepsUnlimited = -1

// defaultSteps is the ceiling a sub-agent gets when its profile names none:
// there is not one.
//
// It was 24 until the owner removed it ("ปลดล็อคเพดานทุกซับเอเจนเลยครับ เป็น
// ค่าเริ่มต้นได้เลย แบบไม่ต้องมีเพดาน ผมสั่ง", 15 ส.ค. — §110). What the number
// was protecting turned out not to be the user: a delegate that reached it did
// not stop spending, it stopped *mid-job* and handed the parent a failure to
// split and start again — the same work, paid for twice, plus the wait.
//
// The brakes that actually end a runaway loop were never the counter and are
// all still here: the doom-loop guard (cognitive.DoomLoopStopPrefix) stops a
// delegate repeating itself with no progress, Stop reaches every delegate
// through Delegations.StopAll, and every call still passes the same approval
// layer and the same permission rules its parent's calls do.
//
// A profile that wants a ceiling writes `steps: 40` and gets exactly that.
const defaultSteps = StepsUnlimited

// stepsUnlimitedKeyword is what the frontmatter carries, rather than a bare -1:
// the file is something the user reads and edits by hand.
const stepsUnlimitedKeyword = "unlimited"

// forcedDenials are refused to every sub-agent whatever its profile says:
// `task`/`task_result`/`task_answer` because depth 1 is enforced by absence
// rather than a counter — every half has to go, or a delegate could collect work
// it was never allowed to start, or answer a question meant for the main agent;
// `help` because its listing belongs to the parent's registry; `ask_user` /
// `todo_write` because no human is attached to a sub-agent's loop — a question
// nobody can see would just burn the tool deadline.
//
// A delegate that needs a decision asks the main agent instead, with `ask_main`
// (ask.go) — which is not listed here because it is never in the parent's
// registry to filter out; it is injected into each child's own.
var forcedDenials = []string{"task", "task_result", "task_answer", "help", "ask_user", "todo_write"}

// Profile is one sub-agent definition. JSON tags are for the settings page,
// which renders exactly these fields as its row badges.
type Profile struct {
	Name        string   `json:"name"`        // the file's basename; also how `task` selects it
	Description string   `json:"description"` // shown in the settings row
	Model       string   `json:"model,omitempty"`
	Tools       []string `json:"tools,omitempty"` // empty = whatever the registry has
	Deny        []string `json:"deny,omitempty"`
	Steps       int      `json:"steps,omitempty"`
	// Desk makes this profile a *chair* rather than a delegate (COMPANY.md §4):
	// it names the desk the job runs at, and that desk's manifest becomes the
	// ceiling on everything below — so a chair that writes `tools: shell` into
	// itself simply does not get shell, by structure rather than by discipline.
	//
	// Empty is the ordinary sub-agent, which runs under the ceiling of whichever
	// desk called it. The difference is who the work belongs to: a delegate is
	// the caller doing its own work in a second context, while a chair is
	// another desk's job handed over the counter, and only the result comes back
	// (ARCHITECTURE.md §84).
	Desk string `json:"desk,omitempty"`
	// Icon is the mark this agent wears on the roster, by name from the app's
	// own icon set. Empty is the normal case and not a gap: the roster derives
	// one from what the agent actually produces, so a profile written before
	// this field existed — and one written by someone who does not care — still
	// arrives with a face. Set it to choose a different one.
	//
	// A name rather than an image: the file is a .md the user edits by hand,
	// and a path to a picture would be a second thing to keep alive beside it.
	Icon string `json:"icon,omitempty"`
	// Needs are the outside things this agent cannot do its job without —
	// "connection:<id>" for an external account, "mcp:<server>" for a tool
	// server. See needs.go for the rule that makes this safe: a need is a
	// declaration and never a grant, so a file naming something that does not
	// exist produces a sentence on screen rather than a silently empty agent.
	Needs  []string `json:"needs,omitempty"`
	Prompt string   `json:"prompt"`
	Path        string   `json:"path,omitempty"` // on-disk path; "" for a bundled profile
	Builtin     bool     `json:"builtin"`
	// Overrides marks a user file that shadows a bundled profile of the same
	// name. The settings page needs it because deleting one is a **revert** — the
	// bundled profile comes back — not a removal, and a delete button that lies
	// about that is how a user loses a capability they meant to reset.
	Overrides bool `json:"overrides,omitempty"`
	// Invalid is why this file cannot run, in the user's language, or "" for a
	// healthy profile. A sub-agent-home file that claims a desk is the case: a
	// contradiction between where a file sits and what it says is shown with
	// its reason, because reinterpreting it quietly — either way — would be
	// this package deciding something the user wrote down differently.
	Invalid string `json:"invalid,omitempty"`
}

// Dir returns <DataRoot>/subagents — the sub-agents' home (not created here).
func Dir() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "subagents"), nil
}

// AgentsDir returns <DataRoot>/agents — the agents' home (not created here),
// holding one folder per worker. config owns the layout inside it because
// internal/learned needs the same paths and cannot import this package.
func AgentsDir() (string, error) {
	return config.AgentsRoot()
}

// entry is one resolved profile with the raw text it came from. Everything the
// package answers — List, Load, ReadRaw, Chairs, Delegates, Conflicts — is a
// projection of resolve()'s output, which is what makes it impossible for two
// of those answers to disagree about a name.
type entry struct {
	Profile
	raw string
}

// Conflict is a user file whose name already belongs to the other home. It is
// not runnable and not in List — the owner keeps the name — but it must not
// vanish either: a file the user can see on disk and cannot explain missing is
// the debt this package exists to refuse.
type Conflict struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// applyHomeRules is the one place the home-decides-kind rule (see the package
// doc) turns into fields.
func applyHomeRules(p *Profile, agentHome bool) {
	if agentHome {
		if p.Desk == "" {
			p.Desk = mode.Office
		}
		return
	}
	if p.Desk != "" {
		p.Invalid = "ไฟล์นี้อยู่ในบ้านของซับเอเจน แต่ประกาศ desk: " + p.Desk +
			" — ซับเอเจนไม่มีโต๊ะ ถ้าตั้งใจให้เป็นเอเจน ย้ายไฟล์ไปโฟลเดอร์ agents"
		p.Desk = ""
	}
}

// resolve reads all four sources in ownership order — bundled agents, bundled
// sub-agents, the user's agents, the user's sub-agents — and settles every
// name once. Within a home a user file shadows a bundled one; across homes the
// first home to hold a name owns it and the other file is a Conflict.
func resolve() ([]entry, []Conflict) {
	// Before the first read, from whichever entry point got here first — see
	// config.MigrateAgentHomes. Cheap after the first call.
	config.MigrateAgentHomes()

	type source struct {
		agentHome bool
		bundled   string
		userFiles func() []string
	}
	sources := []source{
		{agentHome: true, bundled: bundledAgentDir, userFiles: userAgentFiles},
		{agentHome: false, bundled: bundledHelperDir, userFiles: userHelperFiles},
	}

	byName := map[string]int{} // name → index in entries
	homeOf := map[string]bool{}
	var entries []entry
	var conflicts []Conflict

	place := func(name, raw, path string, agentHome, builtin bool) {
		p := parse(name, raw)
		applyHomeRules(&p, agentHome)
		p.Path = path
		p.Builtin = builtin
		if i, taken := byName[name]; taken {
			if homeOf[name] != agentHome {
				home := "ซับเอเจน"
				if homeOf[name] {
					home = "เอเจน"
				}
				conflicts = append(conflicts, Conflict{
					Name: name, Path: path,
					Reason: "ชื่อ " + name + " เป็นของ" + home + "อยู่แล้ว — ความจำและประวัติงานผูกกับชื่อ ชื่อหนึ่งจึงเป็นได้อย่างเดียว ตั้งชื่อไฟล์นี้ใหม่",
				})
				return
			}
			// Same home: the user file shadows the bundled one.
			p.Overrides = entries[i].Builtin
			p.Builtin = false
			entries[i] = entry{Profile: p, raw: raw}
			return
		}
		byName[name] = len(entries)
		homeOf[name] = agentHome
		entries = append(entries, entry{Profile: p, raw: raw})
	}

	for _, src := range sources {
		for _, name := range bundledNames(src.bundled, src.agentHome) {
			raw, err := bundledProfiles.ReadFile(bundledPath(src.bundled, name, src.agentHome))
			if err != nil {
				continue
			}
			place(name, string(raw), "", src.agentHome, true)
		}
	}
	for _, src := range sources {
		for _, path := range src.userFiles() {
			name := userProfileName(path, src.agentHome)
			// The sub-agents' home is closed: the helpers are part of the system
			// and the bundled set is the whole set. A file the user put there is
			// never read as a profile — but it is on their disk, so it is
			// reported rather than silently dead (the same promise Conflict has
			// always made).
			if !src.agentHome {
				conflicts = append(conflicts, Conflict{
					Name: name, Path: path,
					Reason: "ซับเอเจนฝังมากับระบบ เพิ่มหรือแก้ไขไม่ได้ — ไฟล์นี้จึงไม่ถูกอ่าน ถ้าตั้งใจสร้างคนทำงานของคุณเอง สร้างเป็นเอเจนที่หน้าทีมเอเจน แล้วลบไฟล์นี้ทิ้งได้",
				})
				continue
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			place(name, string(raw), path, src.agentHome, false)
		}
	}

	// Bundled names (shadowed or not) first, then the user's own — each group
	// alphabetical across both homes, which is the order List has always kept.
	group := func(e entry) int {
		if e.Builtin || e.Overrides {
			return 0
		}
		return 1
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if gi, gj := group(entries[i]), group(entries[j]); gi != gj {
			return gi < gj
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, conflicts
}

// List reports every profile with a name of its own — bundled (and their
// shadows) first, then the user's, each group alphabetical. Sick files are
// included, carrying their reason; conflict losers are not (their name is
// somebody else's) and are reported by Conflicts instead.
func List() []Profile {
	entries, _ := resolve()
	out := make([]Profile, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Profile)
	}
	return out
}

// Conflicts reports the user files whose names already belong to the other
// home, for the settings page to show — nowhere else needs them, because they
// never run.
func Conflicts() []Conflict {
	_, conflicts := resolve()
	return conflicts
}

// Chairs returns the runnable profiles that sit at the named desk — the
// office's roster (COMPANY.md §4). Hiring is dropping one more file in the
// agents home, so this reads the folders every time rather than caching a list
// that a new file would not be in.
func Chairs(desk string) []Profile {
	desk = strings.ToLower(strings.TrimSpace(desk))
	if desk == "" {
		return nil
	}
	var out []Profile
	for _, p := range List() {
		if p.Desk == desk && p.Invalid == "" {
			out = append(out, p)
		}
	}
	return out
}

// Delegates returns the runnable sub-agents — the assistant's own hands, the
// ones with no desk. The settings page's roster since the split; Chairs is the
// office page's.
func Delegates() []Profile {
	var out []Profile
	for _, p := range List() {
		if p.Desk == "" && p.Invalid == "" {
			out = append(out, p)
		}
	}
	return out
}

// The two piles a delegation can land in, named for the UI (COMPANY.md §4:
// เอเจน / ซับเอเจน). The kind is decided by which home the file lives
// in — never by a word in its description — same rule as the `task` schema's
// grouping (task.go, agentChoice).
const (
	// Whose work it is — never what the work comes back as. See agentChoice for
	// what promising a return type in the schema cost.
	KindAgent  = "agent"  // has a desk: a colleague who takes a whole job
	KindHelper = "helper" // no desk: the assistant's own hands in a second context
)

// KindOf answers which pile the named worker belongs to, applying the same
// default `task` applies when the model names nobody. It exists so the engine
// can stamp the kind onto the events the UI counts — a frontend that guessed
// from names would be a second place answering a question this package already
// answers. Empty when no such profile can run: the row falls back to the
// generic label rather than claiming a kind.
func KindOf(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultProfile
	}
	p, ok := Load(name)
	if !ok {
		return ""
	}
	if p.Desk != "" {
		return KindAgent
	}
	return KindHelper
}

// Load returns the runnable profile named name. ok is false when nothing by
// that name exists — or when the file exists and cannot run, because handing a
// sick profile to a task loop would execute a contradiction the settings page
// is busy explaining.
func Load(name string) (Profile, bool) {
	name = strings.TrimSpace(name)
	// The name arrives from a tool call the model wrote and is about to be
	// compared against filenames — a trust boundary, not a formatting rule.
	if name == "" || !validName(name) {
		return Profile{}, false
	}
	entries, _ := resolve()
	for _, e := range entries {
		if e.Name == name && e.Invalid == "" {
			return e.Profile, true
		}
	}
	return Profile{}, false
}

// rawFor is ReadRaw's half of the single resolution: the owner's text, ok
// false when the name owns nothing.
func rawFor(name string) (string, bool) {
	entries, _ := resolve()
	for _, e := range entries {
		if e.Name == name {
			return e.raw, true
		}
	}
	return "", false
}

// PromptFor is the system prompt an agent actually runs with: its profile,
// plus whatever it has learned in its own scope and had approved.
//
// An agent's memory is folded here and nowhere else. The main agent's prompt
// never carries it (internal/prompt reads only the main scope), which is what
// keeps the main context flat no matter how much the delegates accumulate —
// the structural difference from a single agent that grows one prompt forever.
//
// Exported since §85: a chair's *direct chat* mounts exactly the prompt its
// delegate runs would — one fold, two doors, and they must never drift.
//
// A profile with nothing learned yet gets exactly its old prompt, byte for
// byte: the common case must not pay for the feature, and prefix caching keys
// on the leading bytes.
// Unmet needs are folded here too, and here is the only place they could be:
// an agent is reached from two doors — a delegated run and a direct chat — and
// a check that lived at one of them would leave the other able to run an agent
// that cannot work, without it knowing why. Appended last, after memory, so an
// agent with nothing missing still gets its old prompt byte for byte and keeps
// its prefix cache.
func PromptFor(p Profile) string {
	prompt := p.Prompt
	if memory := learned.Read(p.Name); memory != "" {
		prompt += "\n\n---\n# What you have learned doing this job before\n" + memory + "\n"
	}
	return prompt + needsNotice(UnmetNeeds(p))
}

// MaxToolCalls is what cognitive.AgentConfig gets for this sub-agent.
//
// Three cases, and the middle one is the reason this is not a bare max():
// a positive value is the ceiling the profile asked for and is honoured exactly,
// while 0 (absent or unparseable) and a negative both land on defaultSteps,
// which is no ceiling. Keeping the two apart still matters — a profile that
// writes a number means it, and must not inherit a default that changes
// underneath it.
func (p Profile) MaxToolCalls() int {
	if p.Steps < 0 {
		return StepsUnlimited
	}
	if p.Steps > 0 {
		return p.Steps
	}
	return defaultSteps
}

// DenyRules turns Deny into permission rules to append to the session's own, so
// a profile can forbid a tool without a second enforcement mechanism. Pattern
// "*" is explicit for readability — safety treats an empty pattern the same way.
func (p Profile) DenyRules() []safety.PermissionRule {
	out := make([]safety.PermissionRule, 0, len(p.Deny))
	for _, tool := range p.Deny {
		out = append(out, safety.PermissionRule{Tool: tool, Pattern: "*", Action: safety.PermissionDeny})
	}
	return out
}

// AllowsTool reports whether this sub-agent may be *handed* the named tool.
// Distinct from the permission layer on purpose: a denied tool is still sent to
// the model on every round of the loop and only blocked at execution, so
// trimming the list here is a token saving, while Deny is the safety gate. Both
// apply — this one first.
func (p Profile) AllowsTool(name string) bool {
	if !p.Permits(name) {
		return false
	}
	if len(p.Tools) == 0 {
		return true
	}
	return slices.Contains(p.Tools, strings.ToLower(strings.TrimSpace(name)))
}

// Permits is AllowsTool minus the allowlist: what this profile has not
// *forbidden*, whether or not its `tools:` line happens to name it.
//
// The distinction exists for MCP servers the user pointed at this agent in
// Settings (config.MCPServersForAgent). A profile's `tools:` is a list of
// built-in names its author chose, written before any server existed — and MCP
// tool names cannot be known in advance, since they arrive from the server on
// connect as `notion_search`, `notion_query` and so on. Filtering those through
// the allowlist would mean the toggle in Settings did nothing until the user
// also hand-edited AGENT.md with names they have no way to look up, which is a
// switch that lies.
//
// The denials still apply, both of them. `deny:` is the profile refusing
// something outright and outranks any grant (§44.0), and forcedDenials is what
// no delegate may hold at all. What is skipped is only the *allowlist* — the
// question "did the author think to list this", which a server added months
// later cannot have an answer to.
func (p Profile) Permits(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if slices.Contains(forcedDenials, name) {
		return false
	}
	return !slices.Contains(p.Deny, name)
}

// WantsToBeAsked reports whether this profile is willing to put a question to a
// human, for the one caller that has one attached: a chair chat (§85).
//
// AllowsTool cannot answer this. It runs through Permits, which applies
// forcedDenials — and `ask_user` is on that list for a reason that only holds
// while nobody is watching. Asked there, every profile says no, which is the
// right answer for a delegate and the wrong one for an agent being spoken to.
//
// Only the profile's own `deny:` is read. `tools:` is deliberately not: that
// list is what its author decided this agent may *touch*, and putting a
// question to the person already sitting in the conversation is not a reach
// into anything. A doc writer whose `tools:` names three writers would
// otherwise have to list ask_user to be allowed to speak.
func (p Profile) WantsToBeAsked() bool {
	return !slices.Contains(p.Deny, "ask_user")
}

// parse reads one profile file. The filename is the name, always: a `name:` key
// in the frontmatter that disagreed with the file it lives in is the exact
// product-vs-code split §35 was written about.
func parse(name, raw string) Profile {
	fields, body, err := skill.ParseFrontmatter(raw)
	if err != nil {
		// Unterminated frontmatter — treat the whole file as prompt rather than
		// dropping a profile the user can see on disk and can't explain missing.
		return Profile{Name: name, Prompt: strings.TrimSpace(raw)}
	}
	steps := parseSteps(fields["steps"])
	return Profile{
		Name:        name,
		Description: fields["description"],
		Model:       strings.TrimSpace(fields["model"]),
		Tools:       splitList(fields["tools"]),
		Deny:        splitList(fields["deny"]),
		Steps:       steps,
		Desk:        strings.ToLower(strings.TrimSpace(fields["desk"])),
		Icon:        strings.TrimSpace(fields["icon"]),
		Needs:       splitList(fields["needs"]),
		Prompt:      body,
	}
}

// parseSteps reads the `steps:` field. A number is a ceiling; the word
// "unlimited" says out loud what leaving the field out now also does; anything
// else — blank, a typo, a negative number someone hand-wrote — returns 0, which
// MaxToolCalls reads as "use the default".
//
// The keyword survives the default changing because it is what a reader of the
// file sees. "steps: unlimited" answers the question a blank line leaves open,
// and a profile copied out to another machine keeps saying what it meant.
func parseSteps(value string) int {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == stepsUnlimitedKeyword {
		return StepsUnlimited
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// splitList reads a comma-separated frontmatter value into lowercased tool
// names, so "grep, glob, Read" and "grep,glob,read" mean the same thing.
func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.ToLower(strings.TrimSpace(part)); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// validName rejects anything that would escape the profile directory or fail to
// be a filename.
func validName(name string) bool {
	return name != "." && name != ".." &&
		!strings.ContainsAny(name, `\/:*?"<>|`) &&
		!strings.ContainsAny(name, " \t\n")
}

// bundledNames lists one bundled home, reading whichever shape that home uses:
// a folder per agent, a file per helper. The name is the folder's or the
// file's — never anything inside the file, same rule as parse().
func bundledNames(dir string, agentHome bool) []string {
	entries, err := bundledProfiles.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if agentHome {
			if e.IsDir() {
				names = append(names, e.Name())
			}
			continue
		}
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		}
	}
	sort.Strings(names)
	return names
}

// userProfileName reads the name back off a path the readers above produced:
// an agent is named by its folder, a helper by its file. Both keep the rule
// that a name comes from where the file sits and never from inside it.
func userProfileName(path string, agentHome bool) string {
	if agentHome {
		return filepath.Base(filepath.Dir(path))
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// bundledPath is where a bundled profile's text sits, per that home's shape.
func bundledPath(dir, name string, agentHome bool) string {
	if agentHome {
		return dir + "/" + name + "/" + config.AgentDefinitionFile
	}
	return dir + "/" + name + ".md"
}

// userAgentFiles lists the AGENT.md of every folder in the user's agents home.
//
// A folder without one is skipped and *not* reported: that is a worker whose
// definition is bundled and which has only accumulated memory — the shipped
// three once they have learned something, and the helpers, whose scope lands
// here too. It is the normal state of a working install, not a stray file.
func userAgentFiles() []string {
	dir, err := AgentsDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // no folder yet is the normal state, not an error
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), config.AgentDefinitionFile)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

// userHelperFiles lists what the user put in the sub-agents' home. That home is
// closed, so everything here is reported as a Conflict rather than read — but
// it still has to be *found*, or a file on their disk dies silently.
func userHelperFiles() []string {
	dir, err := Dir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files
}
