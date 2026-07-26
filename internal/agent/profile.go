// Package agent turns a markdown file into an agent profile: who the agent is
// (the body), which tools it may be handed (Tools), which are denied outright
// (Deny), and how long its loop may run (Steps). ARCHITECTURE.md §44.
//
// # Two layers, kept apart on purpose
//
// An **agent** is what the user talks to. A **sub-agent** is what an agent hands
// work to. They are separate directories, separate listings and separate
// lookups — Load never reaches into the sub-agent layer and ListSubagents never
// returns an agent — so a sub-agent cannot become the session, and neither list
// can silently grow the other's entries (owner, twice: *"แยก เอเจนกับซับเอเจนดิ
// อนาคตจะเป็นหนี้ในระบบนะ"* … *"แยกชั้นกับซับเอเจนให้เด็ดขาด"*).
//
// **The directory is the only thing that records which layer a profile is in.**
// There is deliberately no `kind:`/`mode:` frontmatter key: two places saying the
// same thing is a place they can disagree, and then someone has to invent a
// tie-break rule.
//
//	profiles/agents/*.md      <DataRoot>/agents/*.md       → Kind == KindAgent
//	profiles/subagents/*.md   <DataRoot>/subagents/*.md    → Kind == KindSubagent
//
// Both layers work the same two ways as prompt presets (§35): bundled copies are
// compiled in so a fresh install has working profiles with nothing to download,
// and a user file of the same name in the matching directory **wins** — editing a
// shipped profile is copying it out and changing it, never fighting the app.
//
// A profile carries no execution logic. It is data that three existing knobs
// read: cognitive.AgentConfig (prompt, model, MaxToolCalls),
// safety.PermissionConfig (Deny), and the skill registry handed to the agent
// (Tools).
package agent

import (
	"embed"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

//go:embed profiles/agents/*.md profiles/subagents/*.md
var bundledProfiles embed.FS

// Kind is which layer a profile belongs to. A typed string rather than a bare
// one so a caller cannot pass "primary", "sub", or a typo and have it mean
// something.
type Kind string

const (
	// KindAgent is talked to directly by the user.
	KindAgent Kind = "agent"
	// KindSubagent is only ever spawned by an agent, never selectable as the
	// session's own agent.
	KindSubagent Kind = "subagent"
)

// DefaultName is the agent a fresh install talks to. An empty or unknown
// selection resolves here rather than failing: a deleted profile file must not
// be able to stop the app from starting.
const DefaultName = "build"

// subagentSteps caps a tool loop nobody is watching. An agent runs unbounded on
// purpose (internal/cognitive/agent.go — the brakes are the approval layer and
// the Stop button, both of which need a human), and a sub-agent has neither, so
// it gets a number instead. This is one of the three things that make delegation
// cheaper rather than pricier (§44.7).
const subagentSteps = 24

// forcedSubagentDenials are refused to every sub-agent whatever its profile
// says: `task` because depth 1 is enforced by absence rather than a counter,
// `help` because its listing belongs to the parent's registry, and `ask_user` /
// `todo_write` because no human is attached to a sub-agent's loop — a question
// nobody can see would just burn the tool deadline.
var forcedSubagentDenials = []string{"task", "help", "ask_user", "todo_write"}

// Profile is one agent or sub-agent definition. JSON tags are for the settings
// page, which renders exactly these fields as its row badges.
type Profile struct {
	Name        string   `json:"name"`        // the file's basename; also the selector
	Description string   `json:"description"` // shown in the settings row and the palette
	Kind        Kind     `json:"kind"`        // from the directory, never from the file
	Model       string   `json:"model,omitempty"`
	Tools       []string `json:"tools,omitempty"` // empty = whatever the registry has
	Deny        []string `json:"deny,omitempty"`
	Steps       int      `json:"steps,omitempty"`
	Prompt      string   `json:"prompt"`
	Path        string   `json:"path,omitempty"` // on-disk path; "" for a bundled profile
	Builtin     bool     `json:"builtin"`
}

// Dir returns <DataRoot>/agents — the agent layer (not created here).
func Dir() (string, error) { return userDir(KindAgent) }

// SubagentDir returns <DataRoot>/subagents. A **sibling** of Dir, not a child:
// nesting it made the agent listing have to remember to skip a subdirectory,
// which is the kind of "remember to" that eventually gets forgotten.
func SubagentDir() (string, error) { return userDir(KindSubagent) }

func userDir(kind Kind) (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, dirName(kind)), nil
}

// dirName is the single place a Kind becomes a folder name, for both the
// embedded copies and the user's directories.
func dirName(kind Kind) string {
	if kind == KindSubagent {
		return "subagents"
	}
	return "agents"
}

func embedDir(kind Kind) string { return "profiles/" + dirName(kind) }

// List reports every agent the user can talk to — bundled first, then theirs,
// each alphabetical. Sub-agents are not here; that is ListSubagents.
func List() []Profile { return list(KindAgent) }

// ListSubagents reports every profile an agent can hand work to.
func ListSubagents() []Profile { return list(KindSubagent) }

// Load returns the agent named name. Sub-agents are NOT searched: a name that
// only exists in that layer does not resolve here, which is what stops a
// read-only searcher from becoming the whole session. An empty name means
// DefaultName; ok is false when no agent by that name exists.
func Load(name string) (Profile, bool) {
	if strings.TrimSpace(name) == "" {
		name = DefaultName
	}
	return load(name, KindAgent)
}

// LoadSubagent returns the sub-agent named name. Agents are NOT searched — the
// task tool must not be able to spawn the session's own agent by name.
func LoadSubagent(name string) (Profile, bool) { return load(name, KindSubagent) }

// LoadOrDefault is Load with the fallback every caller would otherwise write: an
// unknown or unreadable agent resolves to the default rather than leaving the
// app with no agent at all.
func LoadOrDefault(name string) Profile {
	if p, ok := Load(name); ok {
		return p
	}
	p, _ := Load(DefaultName)
	return p
}

func list(kind Kind) []Profile {
	byName := map[string]Profile{}
	var order []string

	for _, name := range bundledNames(kind) {
		raw, err := bundledProfiles.ReadFile(embedDir(kind) + "/" + name + ".md")
		if err != nil {
			continue
		}
		p := parse(name, string(raw), kind)
		p.Builtin = true
		byName[name] = p
		order = append(order, name)
	}

	for _, path := range userFiles(kind) {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if _, shadowed := byName[name]; !shadowed {
			order = append(order, name)
		}
		p := parse(name, string(raw), kind)
		p.Path = path
		byName[name] = p
	}

	out := make([]Profile, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

func load(name string, kind Kind) (Profile, bool) {
	name = strings.TrimSpace(name)
	// The name arrives from a preference file the user can hand-edit (or from a
	// tool call) and is about to be joined onto a path — a trust boundary, not a
	// formatting rule.
	if name == "" || !validName(name) {
		return Profile{}, false
	}
	if dir, err := userDir(kind); err == nil {
		path := filepath.Join(dir, name+".md")
		if raw, err := os.ReadFile(path); err == nil {
			p := parse(name, string(raw), kind)
			p.Path = path
			return p, true
		}
	}
	if raw, err := bundledProfiles.ReadFile(embedDir(kind) + "/" + name + ".md"); err == nil {
		p := parse(name, string(raw), kind)
		p.Builtin = true
		return p, true
	}
	return Profile{}, false
}

// IsSubagent reports whether this profile may only run under the task tool.
func (p Profile) IsSubagent() bool { return p.Kind == KindSubagent }

// MaxToolCalls is what cognitive.AgentConfig gets: 0 means unbounded.
func (p Profile) MaxToolCalls() int {
	if p.Steps > 0 {
		return p.Steps
	}
	if p.IsSubagent() {
		return subagentSteps
	}
	return 0
}

// DenyRules turns Deny into permission rules to append to the user's own, so a
// profile can forbid a tool without a second enforcement mechanism. Pattern "*"
// is explicit for readability — safety treats an empty pattern the same way.
func (p Profile) DenyRules() []safety.PermissionRule {
	out := make([]safety.PermissionRule, 0, len(p.Deny))
	for _, tool := range p.Deny {
		out = append(out, safety.PermissionRule{Tool: tool, Pattern: "*", Action: safety.PermissionDeny})
	}
	return out
}

// AllowsTool reports whether this profile's agent may be *handed* the named
// tool. Distinct from the permission layer on purpose: a denied tool is still
// sent to the model on every round of the loop and only blocked at execution,
// so trimming the list here is a token saving, while Deny is the safety gate.
// Both apply — this one first.
func (p Profile) AllowsTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if p.IsSubagent() && slices.Contains(forcedSubagentDenials, name) {
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

// parse reads one profile file. Two things are decided by where the file is, not
// by what it says: the name (its filename) and the kind (its directory). A
// frontmatter key that could disagree with the file it lives in is the exact
// product-vs-code split §35 was written about — so `name:` and `kind:`/`mode:`
// are not read at all, and the settings page shows every profile under the
// layer its folder puts it in, which makes a misplaced file visible at a glance.
func parse(name, raw string, kind Kind) Profile {
	if kind != KindSubagent {
		kind = KindAgent
	}
	fields, body, err := skill.ParseFrontmatter(raw)
	if err != nil {
		// Unterminated frontmatter — treat the whole file as prompt rather than
		// dropping a profile the user can see on disk and can't explain missing.
		return Profile{Name: name, Kind: kind, Prompt: strings.TrimSpace(raw)}
	}
	steps, _ := strconv.Atoi(strings.TrimSpace(fields["steps"]))
	return Profile{
		Name:        name,
		Description: fields["description"],
		Kind:        kind,
		Model:       strings.TrimSpace(fields["model"]),
		Tools:       splitList(fields["tools"]),
		Deny:        splitList(fields["deny"]),
		Steps:       steps,
		Prompt:      body,
	}
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

func bundledNames(kind Kind) []string {
	entries, err := bundledProfiles.ReadDir(embedDir(kind))
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

func userFiles(kind Kind) []string {
	dir, err := userDir(kind)
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
