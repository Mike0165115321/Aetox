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

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/connect"
	"github.com/Mikedev115/Aetox/internal/mcp"
	"github.com/Mikedev115/Aetox/internal/skill"
)

//go:embed modes/*.md
var bundledModes embed.FS

// Office is the desk the chairs sit at (COMPANY.md §4): its manifest is the
// ceiling every sub-agent profile that declares `desk: specialized` runs
// under, so a chair that writes `tools: shell` into itself still does not get
// shell. Named here because it is the one desk name the rest of the codebase
// has to be able to say — everything else about a desk is data.
const Office = "specialized"

// The two memory architectures a desk can declare (Mode.Memory, §184).
//
//   - MemoryShared: an unqualified remembered line lands in MEMORY.md, the
//     file every session reads. The assistant's architecture, and the only
//     behaviour that existed before desks declared one — which is why it is
//     also what an empty or unrecognised value means: a manifest that says
//     nothing gets what every manifest got.
//   - MemoryProject: an unqualified line lands in the focused project's own
//     file, because this desk's work is settling things — and "we decided X
//     here" carried into another repository is not knowledge, it is a rumour
//     (§116). With no project focused it falls back to MemoryShared, since
//     ProjectScope of nothing is the shared file by construction.
const (
	MemoryShared  = "shared"
	MemoryProject = "project"
)

// MemoryRule reports this desk's memory architecture, nil-safe and never an
// unknown word: the legacy full desk (nil) and any manifest value that is not
// MemoryProject both mean MemoryShared, so a half-written user manifest
// degrades to the oldest behaviour rather than to a junk destination.
func (m *Mode) MemoryRule() string {
	if m == nil || m.Memory != MemoryProject {
		return MemoryShared
	}
	return MemoryProject
}

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
	// MCP is the servers this desk carries — **resolved, not declared**. It is
	// filled in by parse() from each server's own `for:` list
	// (config.MCPServersForDesk); a `mcp:` line in a manifest is not read.
	//
	// Ownership sits on the server because only the server can be written at
	// the right moment (owner's call, 2026-08-06). A bundled manifest is
	// compiled before any of the user's servers exist, so it could name none of
	// them — and with an empty list meaning "no servers", every configured
	// server connected, registered, and was then filtered off every desk. The
	// assistant reported having no MCP tools, correctly from where it stood.
	//
	// The field stays because the *question* is still the desk's: Carries is
	// the one place a registered tool is judged, and it needs the answer here.
	// What moved is who supplies it.
	MCP []string `json:"mcp,omitempty"`
	// Connections are the external accounts this desk may use — resolved, not
	// declared, exactly as MCP above is and for the same reason. A bundled
	// manifest is compiled before the user has connected anything, so it could
	// name nothing; ownership sits on the connection, where it can be written
	// at the moment the user chooses.
	//
	// A tool that exists because of a connection is invisible to a desk that
	// does not hold it. That is the same rule MCP tools follow, and it is the
	// only honest one: a tool definition the model can see but cannot use is a
	// door painted on a wall.
	Connections []string `json:"connections,omitempty"`
	// Chairs are in the room but not on the desk: tools this desk's *agents*
	// may hold, which the desk's own assistant never carries.
	//
	// A manifest answers two questions that used to share one list — what the
	// assistant sitting here holds, and what a chair working here may be handed
	// (FilterRegistry's ceiling). While they were the same list, taking a tool
	// off the desk took it away from the chair too, and the office lost the
	// ability to make the very thing it exists to make.
	//
	// Owner's call, 2026-08-06: the assistant must not carry the document,
	// workbook and deck writers — that is why the agents exist. It hands the
	// job over and collects the file. This field is how the office keeps the
	// tools while the desk puts them down.
	//
	// Narrower than Tools on purpose: it widens the ceiling for a **chair**
	// (a profile with a desk of its own) and for nothing else. An ordinary
	// delegate is the caller doing its own work in a second context, so
	// handing it something the caller cannot reach would make the desk a
	// façade — which is the exact rule FilterRegistry is built on.
	Chairs []string `json:"chairs,omitempty"`
	// Dispatch names the desks this one may hand a whole job to (COMPANY.md
	// §3). Default-closed like MCP, and for a stronger reason: dispatch is the
	// one door between desks, and a door that opens by omission is one nobody
	// decided to open. ผู้ช่วย declares the office; โค้ด declares nothing and so
	// talks to no one.
	Dispatch []string `json:"dispatch,omitempty"`
	// Memory is this desk's memory architecture (§184): where a remembered
	// line lands when the model does not say. The two desks answer it
	// differently on purpose — assistant work learns about the user and the
	// machine, so its lines belong in the file every session reads; coding
	// work settles decisions, and a decision kept anywhere but the project it
	// was made in arrives as advice in the next one (§116).
	//
	// A rule, not a destination: the desk never has a memory file of its own.
	// `modes/<desk>.md` remains readable (internal/learned.FileFor folds it
	// into the prompt if a person writes one) but nothing proposes into it —
	// measured before removal, 25 ส.ค.: 11 memory calls ever, zero chose it.
	Memory  string `json:"memory,omitempty"`
	Prompt  string `json:"prompt"`         // direction folded into the system prompt
	Path    string `json:"path,omitempty"` // on-disk path; "" for a bundled mode
	Builtin bool   `json:"builtin"`
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

// AllowsAction is AllowsTool one level down, for a packed tool: not "is
// `browser` on this desk" but "is `browser_read`".
//
// Every list a manifest is written in already speaks per-action names — that is
// what the both-spellings rule in internal/skill/category.go exists for, and
// why the specialized desk can write `tools: read, write, list, glob` and mean
// it. So this needs no new vocabulary: it asks the ordinary question about the
// action, and adds the two answers a manifest can only give about the pack.
//
//   - `deny:` naming the PACK refuses all of it. A user who wrote "not the
//     browser" meant the browser, not eleven twelfths of it.
//   - `tools:` naming the PACK grants all of it, which is the one-word way of
//     saying so and what every manifest written before packing existed meant.
func (m *Mode) AllowsAction(tool, action string) bool {
	if m == nil {
		return true
	}
	tool = strings.ToLower(strings.TrimSpace(tool))
	if slices.Contains(m.Deny, tool) {
		return false
	}
	if m.AllowsTool(action) {
		return true
	}
	return slices.Contains(m.Tools, tool)
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
	// A connection's tools are judged before the desk's own lists get a say:
	// `categories:` groups tools by what they do, and no grouping can express
	// "this one reaches an account the user placed elsewhere".
	if !connect.Allows(name, m.Connections) {
		return false
	}
	// A packed tool is carried when ANY of its actions is, because that is what
	// carrying it now means: the dispatcher hands the session a copy offering
	// only the allowed ones (skill.Dispatcher.cut). Asking AllowsTool about the
	// packed name alone would drop `browser` from a desk that allows nine of
	// its twelve rights — the coarse answer AllowsAction exists to replace, and
	// the one this line used to give.
	if actions := skill.PackedActions(name); len(actions) > 0 {
		for _, action := range actions {
			if m.AllowsAction(name, action) {
				return true
			}
		}
		return false
	}
	return m.AllowsTool(name)
}

// CarriesForChair is Carries as a **chair** sees it: what the desk holds, plus
// what it keeps in the room for its agents (Chairs).
//
// Two questions, two methods, one field apart — rather than a flag on Carries,
// because the caller that must not use this one is the main agent's dispatcher
// and a boolean argument there would read as a detail. A method named for who
// is asking cannot be passed by accident.
//
// Deny still wins. A tool the desk removed outright is removed for everyone:
// `chairs:` says "in the room", not "above the rules".
//
// MCP and skills are answered entirely by Carries. A skill is on every desk
// already, and an MCP tool belongs to a server the desk either attached or did
// not — judging one by a tool-name list is the trap §83 named, and the fix is
// not to open a second door onto it.
func (m *Mode) CarriesForChair(name string, source skill.Source) bool {
	if m == nil {
		return true
	}
	if m.Carries(name, source) {
		return true
	}
	switch source {
	case skill.SourceMCP, skill.SourceSkill:
		return false
	}
	// Nor can a connection come back through `chairs:`. The switch says which
	// desks may reach an account at all, and "in the room" is not "above the
	// rules" — the same sentence Deny is held to two lines down.
	if !connect.Allows(name, m.Connections) {
		return false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if slices.Contains(m.Deny, name) {
		return false
	}
	if slices.Contains(m.Chairs, name) {
		return true
	}
	// A `chairs:` list names actions as readily as tools - the office's says
	// `edit, edits, delete`, which are three acts of one packed `change`. Kept
	// in the room when any one of them is, the same rule Carries uses one
	// method up, or a desk that keeps an act for its agents would lose it the
	// day that act was packed.
	for _, action := range skill.PackedActions(name) {
		if slices.Contains(m.Chairs, action) {
			return true
		}
	}
	return false
}

// CarriesMCP reports whether an MCP tool named name came from a server this
// desk attached. The server is read off the name's own prefix — mcp names
// every bridged tool `server_tool` — rather than passed in beside it, so there
// is no second place that could disagree about which server a tool belongs to.
func (m *Mode) CarriesMCP(name string) bool {
	if m == nil {
		return true
	}
	return mcp.ToolBelongsTo(name, m.MCP)
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
		// Not splitList(fields["mcp"]) — see Mode.MCP. Read fresh on every
		// parse rather than cached, so switching a server on in settings takes
		// effect at the next session without a restart. The file is a few
		// hundred bytes and a desk is loaded a handful of times a turn.
		MCP: config.MCPServersForDesk(name),
		// Read fresh here too, and for the same reason: switching a connection
		// on in settings has to reach the next session without a restart.
		Connections: config.ConnectionsForDesk(name, connect.IDs()),
		Chairs:      splitList(fields["chairs"]),
		Dispatch:    splitList(fields["dispatch"]),
		Memory:      strings.ToLower(strings.TrimSpace(fields["memory"])),
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
