// Package subagent turns a markdown file into a sub-agent profile: who it is
// (the body), which tools it may be handed (Tools), which are denied outright
// (Deny), and how long its loop may run (Steps). ARCHITECTURE.md §44.
//
// A sub-agent is what the main agent hands work to. **There is no profile for
// the main agent** — it is one assistant with one identity, configured by the
// identity layer (internal/prompt, §11) and nothing else. Profiles here answer
// "who am I delegating to", never "who am I talking to" (§44.0).
//
// Two sources, same shape as prompt presets (§35):
//
//   - **Bundled** — profiles/*.md, compiled in, so a fresh install can delegate
//     with no folder to create and nothing to download.
//   - **User** — <DataRoot>/subagents/*.md. A user file with the same name as a
//     bundled one **wins**, so editing a shipped profile is copying it out and
//     changing it, never fighting the app.
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
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

//go:embed profiles/*.md
var bundledProfiles embed.FS

// defaultSteps caps a tool loop nobody is watching. The main agent runs
// unbounded on purpose (internal/cognitive/agent.go — the brakes are the
// approval layer and the Stop button, both of which need a human), and a
// sub-agent has neither, so it gets a number instead. One of the three things
// that make delegation cheaper rather than pricier (§44.7).
const defaultSteps = 24

// forcedDenials are refused to every sub-agent whatever its profile says:
// `task`/`task_result` because depth 1 is enforced by absence rather than a
// counter — and both halves have to go, or a delegate could collect work it was
// never allowed to start; `help` because its listing belongs to the parent's
// registry; `ask_user` / `todo_write` because no human is attached to a
// sub-agent's loop — a question nobody can see would just burn the tool deadline.
var forcedDenials = []string{"task", "task_result", "help", "ask_user", "todo_write"}

// Profile is one sub-agent definition. JSON tags are for the settings page,
// which renders exactly these fields as its row badges.
type Profile struct {
	Name        string   `json:"name"`        // the file's basename; also how `task` selects it
	Description string   `json:"description"` // shown in the settings row
	Model       string   `json:"model,omitempty"`
	Tools       []string `json:"tools,omitempty"` // empty = whatever the registry has
	Deny        []string `json:"deny,omitempty"`
	Steps       int      `json:"steps,omitempty"`
	Prompt      string   `json:"prompt"`
	Path        string   `json:"path,omitempty"` // on-disk path; "" for a bundled profile
	Builtin     bool     `json:"builtin"`
	// Overrides marks a user file that shadows a bundled profile of the same
	// name. The settings page needs it because deleting one is a **revert** — the
	// bundled profile comes back — not a removal, and a delete button that lies
	// about that is how a user loses a capability they meant to reset.
	Overrides bool `json:"overrides,omitempty"`
}

// Dir returns <DataRoot>/subagents (not created here).
func Dir() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "subagents"), nil
}

// List reports every available sub-agent — bundled first, then the user's, each
// group alphabetical. A user profile shadows a bundled one of the same name and
// takes its place rather than appearing twice.
func List() []Profile {
	byName := map[string]Profile{}
	var order []string

	for _, name := range bundledNames() {
		raw, err := bundledProfiles.ReadFile("profiles/" + name + ".md")
		if err != nil {
			continue
		}
		p := parse(name, string(raw))
		p.Builtin = true
		byName[name] = p
		order = append(order, name)
	}

	for _, path := range userFiles() {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		_, shadowed := byName[name]
		if !shadowed {
			order = append(order, name)
		}
		p := parse(name, string(raw))
		p.Path = path
		p.Overrides = shadowed
		byName[name] = p
	}

	out := make([]Profile, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

// Load returns the sub-agent named name, preferring the user's file so a bundled
// profile can be overridden by writing one with the same name. ok is false when
// nothing by that name exists.
func Load(name string) (Profile, bool) {
	name = strings.TrimSpace(name)
	// The name arrives from a tool call the model wrote and is about to be joined
	// onto a path — a trust boundary, not a formatting rule.
	if name == "" || !validName(name) {
		return Profile{}, false
	}
	if dir, err := Dir(); err == nil {
		path := filepath.Join(dir, name+".md")
		if raw, err := os.ReadFile(path); err == nil {
			p := parse(name, string(raw))
			p.Path = path
			_, err := bundledProfiles.ReadFile("profiles/" + name + ".md")
			p.Overrides = err == nil
			return p, true
		}
	}
	if raw, err := bundledProfiles.ReadFile("profiles/" + name + ".md"); err == nil {
		p := parse(name, string(raw))
		p.Builtin = true
		return p, true
	}
	return Profile{}, false
}

// MaxToolCalls is what cognitive.AgentConfig gets for this sub-agent.
func (p Profile) MaxToolCalls() int {
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
	steps, _ := strconv.Atoi(strings.TrimSpace(fields["steps"]))
	return Profile{
		Name:        name,
		Description: fields["description"],
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

func bundledNames() []string {
	entries, err := bundledProfiles.ReadDir("profiles")
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

func userFiles() []string {
	dir, err := Dir()
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
