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
// call, 2026-08-05 — ตัวแทน/ผู้ช่วยตัวแทน):
//
//   - **Agents** — the team the user can see: they take jobs over the counter
//     and hold direct chats (§85). Home: profiles/agents (bundled) and
//     <DataRoot>/agents (user). A file here that names no desk sits in the
//     office; naming one is allowed and means that desk.
//   - **Sub-agents** — the assistant's own hands, never chatted with. Home:
//     profiles/subagents (bundled) and <DataRoot>/subagents (user). A file
//     here that claims a desk is a contradiction and is refused loudly
//     (Invalid), never silently reinterpreted.
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

//go:embed profiles/agents/*.md profiles/subagents/*.md
var bundledProfiles embed.FS

// The bundled halves of the two homes. Their names never reach a user; the
// on-disk halves are AgentsDir and Dir.
const (
	bundledAgentDir  = "profiles/agents"
	bundledHelperDir = "profiles/subagents"
)

// defaultSteps caps a tool loop nobody is watching. The main agent runs
// unbounded on purpose (internal/cognitive/agent.go — the brakes are the
// approval layer and the Stop button, both of which need a human), and a
// sub-agent has neither, so it gets a number instead. One of the three things
// that make delegation cheaper rather than pricier (§44.7).
const defaultSteps = 24

// StepsUnlimited is what `steps: unlimited` parses to. Distinct from 0, which is
// what an absent or unreadable value gives and still means "use defaultSteps" —
// a profile has to say it wants no ceiling before it gets one.
//
// The loop it feeds already understands this: cognitive.Agent treats
// MaxToolCalls <= 0 as unbounded, which is exactly how the main agent runs. So
// this does not add a mode; it lets a sub-agent opt into the one that already
// exists, for the delegation that legitimately cannot be sized up front.
const StepsUnlimited = -1

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
	Desk   string `json:"desk,omitempty"`
	Prompt string `json:"prompt"`
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

// AgentsDir returns <DataRoot>/agents — the agents' home (not created here).
func AgentsDir() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "agents"), nil
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
		p.Invalid = "ไฟล์นี้อยู่ในบ้านของผู้ช่วยตัวแทน แต่ประกาศ desk: " + p.Desk +
			" — ผู้ช่วยตัวแทนไม่มีโต๊ะ ถ้าตั้งใจให้เป็นตัวแทน ย้ายไฟล์ไปโฟลเดอร์ agents"
		p.Desk = ""
	}
}

// resolve reads all four sources in ownership order — bundled agents, bundled
// sub-agents, the user's agents, the user's sub-agents — and settles every
// name once. Within a home a user file shadows a bundled one; across homes the
// first home to hold a name owns it and the other file is a Conflict.
func resolve() ([]entry, []Conflict) {
	type source struct {
		agentHome bool
		bundled   string
		userDir   func() (string, error)
	}
	sources := []source{
		{agentHome: true, bundled: bundledAgentDir, userDir: AgentsDir},
		{agentHome: false, bundled: bundledHelperDir, userDir: Dir},
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
				home := "ผู้ช่วยตัวแทน"
				if homeOf[name] {
					home = "ตัวแทน"
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
		for _, name := range bundledNames(src.bundled) {
			raw, err := bundledProfiles.ReadFile(src.bundled + "/" + name + ".md")
			if err != nil {
				continue
			}
			place(name, string(raw), "", src.agentHome, true)
		}
	}
	for _, src := range sources {
		for _, path := range userFiles(src.userDir) {
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
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
func PromptFor(p Profile) string {
	memory := learned.Read(p.Name)
	if memory == "" {
		return p.Prompt
	}
	return p.Prompt + "\n\n---\n# What you have learned doing this job before\n" + memory + "\n"
}

// MaxToolCalls is what cognitive.AgentConfig gets for this sub-agent.
//
// Three cases, and the middle one is the reason this is not a bare max():
// a negative value is a deliberate "no ceiling" and is passed through as such,
// while 0 is an absent or unparseable field and still falls back to the
// default. Collapsing them would turn a typo in the frontmatter into an
// unbounded loop nobody asked for.
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
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if slices.Contains(forcedDenials, name) {
		return false
	}
	if slices.Contains(p.Deny, name) {
		return false
	}
	if len(p.Tools) == 0 {
		return true
	}
	return slices.Contains(p.Tools, name)
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
		Prompt:      body,
	}
}

// parseSteps reads the `steps:` field. A number is a ceiling; the word
// "unlimited" removes it; anything else — blank, a typo, a negative number
// someone hand-wrote — returns 0, which MaxToolCalls reads as "use the
// default". Only the keyword can unbound a loop, so a mistyped ceiling fails
// closed rather than open.
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

func bundledNames(dir string) []string {
	entries, err := bundledProfiles.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		}
	}
	sort.Strings(names)
	return names
}

func userFiles(home func() (string, error)) []string {
	dir, err := home()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // no folder yet is the normal state, not an error
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
