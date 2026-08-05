// Package mode turns a markdown file into a session mode: which of the
// capability groups from internal/skill/category.go are on the desk
// (Categories, plus Tools/Deny for the exceptions a group cannot express),
// which MCP servers are attached (MCP), and what direction the base prompt
// gains (the body). ARCHITECTURE.md §83.
//
// A mode is not an identity. §44.0 deleted selectable agents because a second
// answer to "who is the AI" fragments the one assistant; a mode answers "what
// is on the desk", never "who is sitting at it", and the identity layer
// (internal/prompt, §11) remains the only answer to who. A manifest body that
// starts describing who the assistant *is* rather than what the work *is* has
// become the thing §44.0 deleted, and does not belong here.
//
// A mode is also not a permission tier. The safety gate and the permission
// rules are identical in every mode; what a mode trims is which tool
// definitions a session carries at all — a token decision, exactly like
// subagent.Profile.AllowsTool, and enforced at the same place (the dispatcher
// the session is built with), never instead of the gate.
//
// Two sources, same shape as sub-agent profiles (§35) and prompt presets:
//
//   - **Bundled** — modes/*.md, compiled in, so a fresh install has its three
//     desks with nothing to create or download.
//   - **User** — <DataRoot>/modes/*.md. A user file with the same name as a
//     bundled one wins, so editing a shipped mode is copying it out; a fourth
//     mode is a file, not a feature request.
package mode

import (
	"embed"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

//go:embed modes/*.md
var bundledModes embed.FS

// Office is the desk the chairs sit at (COMPANY.md §4): its manifest is the
// ceiling every sub-agent profile that declares `desk: specialized` runs
// under, so a chair that writes `tools: shell` into itself still does not get
// shell. Named here because it is the one desk name the rest of the codebase
// has to be able to say — everything else about a desk is data.
const Office = "specialized"

// Default is the desk a window opens at when nothing has been remembered yet —
// a first run, or a preference file that predates the field.
//
// A seed, not a policy. Every later start reads the desk the user was last at
// (config.ModelPreference.LastDesk), and this name must never win over that:
// pinning the boot desk to a constant would mean walking into a room, closing
// the app, and finding yourself back in the entrance hall.
const Default = "assistant"

// Mode is one desk. JSON tags are for the settings page and the new-session
// picker, which render exactly these fields.
//
// The zero-value semantics differ by field, on purpose (§83):
//
//   - Categories and Tools both empty means **everything** — the sub-agent
//     precedent (an empty `tools:` hands a delegate the whole registry), and
//     what makes a manifest that only says `deny: shell` a valid way to
//     express "the full desk minus one drawer".
//   - MCP empty means **no servers**. Built-ins are the product and are
//     safety-gated one by one; MCP servers are user-added per purpose, arrive
//     ten tools at a time, and are exactly what a mode exists to keep off the
//     other desks. Defaulting them onto every desk would rebuild the pile
//     this package was written to take apart.
type Mode struct {
	Name        string   `json:"name"`        // the file's basename; also what sessions.mode stores
	Description string   `json:"description"` // shown on the picker card
	Categories  []string `json:"categories,omitempty"`
	Tools       []string `json:"tools,omitempty"` // individual allows beyond the categories
	Deny        []string `json:"deny,omitempty"`  // individual removals; wins over everything
	MCP         []string `json:"mcp,omitempty"`   // attached MCP servers, by configured name
	// Dispatch names the desks this one may hand a whole job to (COMPANY.md
	// §3). Default-closed like MCP, and for a stronger reason: dispatch is the
	// one door between desks, and a door that opens by omission is one nobody
	// decided to open. ผู้ช่วย declares the office; โค้ด declares nothing and so
	// talks to no one.
	Dispatch []string `json:"dispatch,omitempty"`
	Prompt   string   `json:"prompt"`         // direction folded into the system prompt
	Path     string   `json:"path,omitempty"` // on-disk path; "" for a bundled mode
	Builtin  bool     `json:"builtin"`
	// Overrides marks a user file shadowing a bundled mode of the same name.
	// Deleting one is a revert — the bundled mode comes back — not a removal,
	// and the settings page must not lie about that.
	Overrides bool `json:"overrides,omitempty"`
}

// Dir returns <DataRoot>/modes (not created here).
func Dir() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "modes"), nil
}

// List reports every available mode — bundled first, then the user's, each
// group alphabetical. A user mode shadows a bundled one of the same name and
// takes its place rather than appearing twice.
func List() []Mode {
	byName := map[string]Mode{}
	var order []string

	for _, name := range bundledNames() {
		raw, err := bundledModes.ReadFile("modes/" + name + ".md")
		if err != nil {
			continue
		}
		m := parse(name, string(raw))
		m.Builtin = true
		byName[name] = m
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
		m := parse(name, string(raw))
		m.Path = path
		m.Overrides = shadowed
		byName[name] = m
	}

	out := make([]Mode, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

// Load returns the mode named name, preferring the user's file so a bundled
// mode can be overridden by writing one with the same name.
//
// Load("") returns (nil, true): the empty name is every session from before
// modes existed, and it *means* the full desk — a real answer, not a miss. A
// nil *Mode allows every tool and every server (see AllowsTool/AllowsServer),
// so the caller needs no second branch for legacy sessions.
//
// ok is false only for a non-empty name nothing answers to — a user mode that
// was deleted after its sessions were created. What to do then is the host's
// decision to surface, not this package's to guess.
func Load(name string) (*Mode, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, true
	}
	// The name arrives from a database column or a picker and is about to be
	// joined onto a path — a trust boundary, not a formatting rule.
	if !validName(name) {
		return nil, false
	}
	if dir, err := Dir(); err == nil {
		path := filepath.Join(dir, name+".md")
		if raw, err := os.ReadFile(path); err == nil {
			m := parse(name, string(raw))
			m.Path = path
			_, err := bundledModes.ReadFile("modes/" + name + ".md")
			m.Overrides = err == nil
			return &m, true
		}
	}
	if raw, err := bundledModes.ReadFile("modes/" + name + ".md"); err == nil {
		m := parse(name, string(raw))
		m.Builtin = true
		return &m, true
	}
	return nil, false
}

// AllowsTool reports whether this desk carries the named tool. Like
// subagent.Profile.AllowsTool, this is a token decision, not the safety gate:
// a tool trimmed here is simply never sent to the model, while a tool the
// mode carries still passes every approval rule it always did.
//
// **For built-in and workbench tools only.** An MCP tool must be judged by
// AllowsServer on its server's name instead — CategoryOf answers the fallback
// category for names it does not know, and letting that fallback decide MCP
// membership would attach every installed server to every mode that carries
// the fallback's group (§83). The caller knows which registry source a name
// came from; this package deliberately does not.
//
// A nil Mode is the pre-modes full desk and allows everything.
func (m *Mode) AllowsTool(name string) bool {
	if m == nil {
		return true
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if slices.Contains(m.Deny, name) {
		return false
	}
	if slices.Contains(m.Tools, name) {
		return true
	}
	if len(m.Categories) == 0 && len(m.Tools) == 0 {
		return true
	}
	return slices.Contains(m.Categories, skill.CategoryOf(name))
}

// AllowsServer reports whether this desk attaches the named MCP server. An
// absent `mcp:` field attaches none — the one default-closed field, see the
// Mode doc for why — and a nil Mode (the pre-modes full desk) attaches all.
func (m *Mode) AllowsServer(server string) bool {
	if m == nil {
		return true
	}
	server = strings.ToLower(strings.TrimSpace(server))
	if server == "" {
		return false
	}
	return slices.Contains(m.MCP, server)
}

// Carries is the whole question — "is this registered tool on this desk?" —
// answered in one place, because answering it needs something a name alone
// cannot give: which registry source the tool came from.
//
// Three sources, three rules, and the middle one is the trap §83 named:
//
//   - **MCP** is judged by its server (CarriesMCP), never by AllowsTool.
//     CategoryOf answers `agent` for every name it does not know, so letting
//     it decide would attach every installed server to every desk that
//     carries the agent group — which all of them do.
//   - **A skill** is knowledge, not capability: a SKILL.md costs nothing until
//     the model asks for it, so every desk keeps every skill and the user's
//     `/skill-name` command works wherever they type it.
//   - **Everything else** — built-ins and the desktop's workbench tools — is
//     the categories/tools/deny question AllowsTool already answers.
//
// A nil Mode is the pre-modes full desk and carries everything.
func (m *Mode) Carries(name string, source skill.Source) bool {
	if m == nil {
		return true
	}
	switch source {
	case skill.SourceMCP:
		return m.CarriesMCP(name)
	case skill.SourceSkill:
		return true
	}
	return m.AllowsTool(name)
}

// CarriesMCP reports whether an MCP tool named name came from a server this
// desk attached. The server is read off the name's own prefix — mcp names
// every bridged tool `server_tool` — rather than passed in beside it, so there
// is no second place that could disagree about which server a tool belongs to.
func (m *Mode) CarriesMCP(name string) bool {
	if m == nil {
		return true
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for _, server := range m.MCP {
		if strings.HasPrefix(name, mcp.ToolPrefix(server)) {
			return true
		}
	}
	return false
}

// AllowsDispatch reports whether this desk may hand a whole job to the desk
// named desk (COMPANY.md §3 — the star with one center). Default-closed: a
// manifest that names no desks talks to none. A nil Mode is the pre-modes full
// desk, which could always reach every sub-agent and still can.
func (m *Mode) AllowsDispatch(desk string) bool {
	if m == nil {
		return true
	}
	desk = strings.ToLower(strings.TrimSpace(desk))
	if desk == "" {
		return false
	}
	return slices.Contains(m.Dispatch, desk)
}

// DeskName reports the desk's name, nil-safe, so a caller holding the legacy
// full desk can say which desk it is without a branch. "" is the full desk,
// which is exactly what sessions.mode stores for it.
func (m *Mode) DeskName() string {
	if m == nil {
		return ""
	}
	return m.Name
}

// Direction is the manifest body, nil-safe: what the base prompt gains from
// this desk. Never who the assistant is — see the package doc and §44.0.
func (m *Mode) Direction() string {
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m.Prompt)
}

// parse reads one mode file. The filename is the name, always: a `name:` key
// in the frontmatter that disagreed with the file it lives in is the exact
// product-vs-code split §35 was written about.
func parse(name, raw string) Mode {
	fields, body, err := skill.ParseFrontmatter(raw)
	if err != nil {
		// Unterminated frontmatter — treat the whole file as prompt rather
		// than dropping a mode the user can see on disk and can't explain
		// missing. All lists empty means the full desk with no servers, the
		// least surprising reading of a half-written file.
		return Mode{Name: name, Prompt: strings.TrimSpace(raw)}
	}
	return Mode{
		Name:        name,
		Description: fields["description"],
		Categories:  splitList(fields["categories"]),
		Tools:       splitList(fields["tools"]),
		Deny:        splitList(fields["deny"]),
		MCP:         splitList(fields["mcp"]),
		Dispatch:    splitList(fields["dispatch"]),
		Prompt:      body,
	}
}

// splitList reads a comma-separated frontmatter value into lowercased names,
// so "web, media, Files" and "web,media,files" mean the same thing.
func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.ToLower(strings.TrimSpace(part)); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// validName rejects anything that would escape the modes directory or fail to
// be a filename.
func validName(name string) bool {
	return name != "." && name != ".." &&
		!strings.ContainsAny(name, `\/:*?"<>|`) &&
		!strings.ContainsAny(name, " \t\n")
}

func bundledNames() []string {
	entries, err := bundledModes.ReadDir("modes")
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
