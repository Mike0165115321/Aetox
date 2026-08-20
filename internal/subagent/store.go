package subagent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/connect"
	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// This file is the write half of a profile's life: reading the raw markdown a
// settings page edits, saving it under <DataRoot>/subagents, and the one derived
// write (SetModel). Bundled profiles are never touched — saving one writes a user
// file that shadows it, and deleting that file is how "revert to default" works,
// with no separate override store to keep in sync.

// ReadRaw returns the markdown text behind a profile — the name's owner, via
// the same resolution every other reader uses. ok is false when the name owns
// nothing.
func ReadRaw(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || !validName(name) {
		return "", false
	}
	return rawFor(name)
}

// Save was the sub-agents' write door, and now refuses everything: the
// helpers are part of the system (owner's call, 2026-08-06) — the bundled
// three are the whole set, so there is nothing a save here could legitimately
// do. Kept as a door rather than deleted so the refusal lives in the layer
// that owns the rule, and reads the same whichever caller knocks.
func Save(name, body string) error {
	return errors.New("ซับเอเจนฝังมากับระบบ เพิ่มหรือแก้ไขไม่ได้ — ถ้าต้องการคนทำงานแบบของคุณเอง สร้างเป็นเอเจนที่หน้าทีมเอเจน")
}

// SaveAgent writes an agent's file into the agents' home — the one write door
// profiles have left. The caller never writes the kind into the file; the
// home says it.
//
// A name the sub-agents own is refused at the door. A name has one owner
// (memory, jobs and chat history all key on it), and a clash the save could
// have named is not something to leave for the resolver to flag later.
func SaveAgent(name, body string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("ต้องตั้งชื่อก่อน")
	}
	if len([]rune(name)) > 40 {
		return errors.New("ชื่อยาวเกินไป (ไม่เกิน 40 ตัวอักษร)")
	}
	if !validName(name) {
		return errors.New(`ชื่อห้ามมีช่องว่างหรืออักขระเหล่านี้: \ / : * ? " < > |`)
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("เนื้อหาว่างเปล่า")
	}
	for _, p := range List() {
		if p.Name != name {
			continue
		}
		if p.Desk == "" {
			return errors.New("ชื่อ " + name + " เป็นของซับเอเจนอยู่แล้ว — ความจำและประวัติงานผูกกับชื่อ ต้องตั้งชื่ออื่น")
		}
		break
	}
	path, err := config.AgentDefinitionPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

// Delete removes the user's definition for name. A bundled profile of the same
// name reappears in its place — that is the revert, not a special case. A name
// with no user file is not an error: the bundled one was never theirs to delete.
//
// Two outcomes, because an agent's folder holds more than its definition and
// the two buttons mean different things:
//
//   - **Reverting a shipped agent** the user edited removes AGENT.md and
//     nothing else. Its memory belongs to the *name* — it is what this worker
//     learned doing the job, across every rewrite of the brief — so undoing an
//     edit to the brief must not throw it away. Skipping this distinction
//     would make "revert" quietly destroy months of accumulated work.
//   - **Deleting an agent the user hired** takes the whole folder. There is no
//     bundled definition to fall back to, so the name is gone; leaving its
//     memory behind would seed a future agent that reuses the name with a
//     stranger's notes, and leave a folder on disk that nothing lists.
func Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || !validName(name) {
		return errors.New("ชื่อไม่ถูกต้อง")
	}
	// Asked before anything is removed: after the definition goes, the resolver
	// can no longer tell a revert from a delete.
	revert := false
	for _, p := range List() {
		if p.Name == name {
			revert = p.Overrides
			break
		}
	}
	home, err := config.AgentHome(name)
	if err != nil {
		return err
	}
	if revert {
		if err := os.Remove(filepath.Join(home, config.AgentDefinitionFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if err := os.RemoveAll(home); err != nil {
		return err
	}
	// The sub-agents' home is closed and its files are never read, but one the
	// user put there before it closed is still theirs to remove from this door.
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, name+".md")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// SetModel points one agent at a specific model (the per-profile model
// dropdown) by rewriting a single frontmatter line and saving the result as a
// user file in the agents' home. An empty model removes the line, which is
// what "inherit whatever model is selected" means.
//
// A helper cannot be pointed anywhere: pinning a model is an edit, and the
// helpers are part of the system. They follow the chat's model, which is what
// an absent `model:` line has always meant.
func SetModel(name, modelName string) error {
	raw, ok := ReadRaw(name)
	if !ok {
		return errors.New("ไม่พบโปรไฟล์ชื่อ " + name)
	}
	p, ok := Load(name)
	if !ok || p.Desk == "" {
		return errors.New("ซับเอเจนฝังมากับระบบ แก้ไขไม่ได้ — โมเดลของมันตามโมเดลที่แชทใช้อยู่เสมอ")
	}
	return SaveAgent(name, setFrontmatterField(raw, "model", strings.TrimSpace(modelName)))
}

// setFrontmatterField replaces, inserts or (on an empty value) drops one
// "key: value" line inside the leading --- block, leaving every other line and
// the body exactly as the user wrote them. A document with no frontmatter gains
// one; this is text editing, not a parse-and-reserialize, because reserializing
// would silently drop any key this package does not know about.
func setFrontmatterField(raw, key, value string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	trimmed := strings.TrimLeft(normalized, "\n")
	if !strings.HasPrefix(trimmed, "---\n") {
		if value == "" {
			return raw
		}
		return "---\n" + key + ": " + value + "\n---\n\n" + strings.TrimLeft(normalized, "\n")
	}
	rest := trimmed[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return raw // unterminated frontmatter: refuse to guess where it ends
	}

	lines := strings.Split(rest[:end], "\n")
	out := make([]string, 0, len(lines)+1)
	replaced := false
	for _, line := range lines {
		if k, _, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), key) {
			replaced = true
			if value != "" {
				out = append(out, key+": "+value)
			}
			continue
		}
		out = append(out, line)
	}
	if !replaced && value != "" {
		out = append(out, key+": "+value)
	}
	return "---\n" + strings.Join(out, "\n") + rest[end:]
}

// FilterRegistry returns the registry to hand a sub-agent running under p: only
// the tools p allows *and* the desk allows, always minus the forced denials.
// Copying rather than filtering at dispatch time is what makes depth 1
// structural — `task` is simply not in the child's registry, so no counter can
// be got wrong.
//
// The ceiling is the desk the job runs at (§83/§84): the caller's desk for an
// ordinary delegate, the chair's own desk for a cross-desk dispatch. It is an
// intersection and never a union — a profile's `tools:` can only ever narrow
// what the desk already carries, because a delegate that could reach what its
// parent cannot would make the desk a façade. A nil ceiling is the pre-modes
// full desk and narrows nothing.
//
// A **chair** measures against a slightly taller ceiling than the desk's own
// assistant: mode.CarriesForChair adds the desk's `chairs:` list, which is what
// the office keeps in the room for its agents without putting it on the desk
// (owner's call, 2026-08-06 — the assistant must not carry the writers). This
// is not the union the paragraph above refuses: the desk still decides, in its
// own manifest, exactly which tools those are. What it stops being is a single
// list forced to answer two different questions.
//
// A delegate with no desk of its own gets the plain ceiling, unchanged. It is
// the caller working in a second context, and letting it reach past the caller
// is precisely the façade.
//
// This is the token filter, not the safety gate: see Profile.AllowsTool.
//
// ponytail: tools registered into the parent *after* this call — MCP servers
// finish connecting in the background — land in the parent only, so a sub-agent
// spawned earlier never sees them. Fine while a sub-agent is a short-lived
// delegate; revisit if one is ever kept alive across a server connect.
func FilterRegistry(parent *skill.Registry, p Profile, ceiling *mode.Mode) *skill.Registry {
	if parent == nil {
		return nil
	}
	// Which question the ceiling is asked, decided once by what p *is* rather
	// than per tool: a chair is another desk's job handed over the counter, a
	// delegate is this one's own work in a second context.
	carries := ceiling.Carries
	// The servers the user pointed at this agent, if any. Captured out here
	// because two decisions below need it: whether the ceiling lets the tool
	// through, and whether the profile's allowlist gets to filter it.
	var agentServers []string
	if p.Desk != "" {
		// The servers the user pointed at this agent in Settings — read once
		// per dispatch, not per tool. They are already connected and in the
		// parent's registry (the manager connects every enabled server); what
		// this decides is which of them the agent may be handed.
		//
		// This widens past the desk, and that is the point: an agent brings
		// tools of its own, which is why `for:` can name one. The authority is
		// not the desk manifest but the user's own toggle — the same single
		// gate every other right comes through, and visible in the same place.
		// The desk still decides what the *assistant* sitting there holds.
		agentServers = config.MCPServersForAgent(p.Name)
		// The connections pointed at this agent, for the same reason and from
		// the same toggle. Until 2026-08-10 only the servers made this trip,
		// and a connection placed on an agent was a grant nobody read: the
		// engine chip said n8n, the needs gate said satisfied — both read
		// `agent:` placement — while this cut asked the desk's ceiling, which
		// had never heard of it. Four readers agreed the agent was equipped,
		// and the one that hands out tools disagreed.
		agentConnections := config.ConnectionsForAgent(p.Name, connect.IDs(), connect.AgentDefaults())
		chairCarries := ceiling.CarriesForChair
		carries = func(name string, source skill.Source) bool {
			if chairCarries(name, source) {
				return true
			}
			if source == skill.SourceMCP && mcp.ToolBelongsTo(name, agentServers) {
				return true
			}
			// Only for tools a connection owns. connect.Allows answers yes to
			// any name no provider claims — right under a desk, which still
			// holds its own manifest below that answer, and an open door here,
			// where a yes is the grant itself.
			if _, owned := connect.ProviderOfTool(name); !owned {
				return false
			}
			return connect.Allows(name, agentConnections)
		}
	}
	filtered := skill.NewRegistry()
	var doors []string // the progressive-loading pair this worker keeps; see below
	for name, s := range parent.Snapshot() {
		source, ok := parent.SourceOf(name)
		if !ok {
			// Unreachable: name came out of this same snapshot. Skipping beats
			// inventing a source, which is how a tool ends up filed as
			// something the user installed.
			continue
		}
		// A server the user pointed at this agent skips the profile's
		// allowlist, but not its denials — see Profile.Permits for why an
		// allowlist cannot speak for tool names that arrive from a server.
		// A packed tool is one name in the block and several rights inside it
		// (skill.Packed), so a profile can name either spelling: the tool, or
		// the actions of it that this worker is meant to have. `tools: shell`
		// is a whole shell; `tools: shell_output` is permission to look in on a
		// background command and nothing else — and it is what an AGENT.md
		// written before the packing said, which has to keep meaning what it
		// said.
		//
		// Only the allowlist half is widened. `deny:` and forcedDenials still
		// answer first, through AllowsTool, for the tool's own name.
		packedActions := skill.PackedActions(name)
		if source == skill.SourceMCP && mcp.ToolBelongsTo(name, agentServers) {
			if !p.Permits(name) {
				continue
			}
		} else if !p.AllowsTool(name) {
			if len(packedActions) == 0 || !p.Permits(name) || !namesAnyAction(p, packedActions) {
				continue
			}
		}
		// `memory` is scoped to whoever holds it, and the parent's instance is
		// scoped to the main agent. Inheriting it would let a delegate write into
		// the main agent's memory; `task` registers a replacement bound to this
		// profile instead. Dropped here rather than in forcedDenials because a
		// profile is still allowed to refuse memory in its own frontmatter, and
		// forcedDenials would take that choice away by answering first.
		if name == "memory" {
			continue
		}
		if !carries(name, source) {
			continue
		}
		// Same move as `memory`, one shelf over: the parent's skills_list and
		// skill_view answer for ~/.aetox/skills and nothing else, so a worker
		// that inherited them could not open its own folder. Dropped here and
		// re-registered by attachOwnSkills pointed at both shelves — after every
		// filter above, so this only ever remembers a door the worker had
		// already won, never grants one.
		if p.Desk != "" && slices.Contains(openDoors, name) {
			doors = append(doors, name)
			continue
		}
		// Which actions of a packed tool this worker gets — the question the
		// tool itself cannot answer, because internal/skill has never heard of
		// a profile and must not learn.
		//
		// Two readings of the same `tools:` line, in the order the author meant
		// them: an action is in if the profile named it (or named the whole
		// tool, or named nothing at all), and out if the profile denied it. A
		// worker left with no actions is dropped rather than handed a tool that
		// refuses every call — the same failure as being handed a tool it does
		// not have, only harder to see from the outside.
		if packed, ok := s.(skill.Packed); ok && len(packedActions) > 0 {
			var named []string
			for _, action := range packedActions {
				if !p.Permits(action) {
					continue
				}
				if len(p.Tools) == 0 || slices.Contains(p.Tools, name) || slices.Contains(p.Tools, action) {
					named = append(named, action)
				}
			}
			if len(named) == 0 {
				continue
			}
			s = packed.Narrow(named)
		}
		if err := filtered.Register(s, source); err != nil {
			continue // a duplicate name can't happen in a fresh registry; ignore rather than panic
		}
	}
	// What this worker knows, out of its own folder (skills.go). Here rather
	// than at the call sites because this function is already the one place
	// that answers "what does this worker hold" — a delegation and a direct
	// chat both come through it, and a second place to add them is a day when
	// one door has the knowledge and the other does not.
	attachOwnSkills(filtered, p, doors)
	return filtered
}

// namesAnyAction reports whether a profile's allowlist asks for a packed tool
// by one of its action names rather than by the tool's own.
//
// Only reached when the allowlist did not name the tool itself, which is the
// case worth getting right: `tools: shell_output` was a complete sentence
// before the packing and has to stay one after it. Denials are re-asked here
// because Permits is the half of AllowsTool that outranks any grant, and an
// action the profile denied is not a reason to hand over the tool that carries
// it.
func namesAnyAction(p Profile, actions []string) bool {
	for _, action := range actions {
		if slices.Contains(p.Tools, action) && p.Permits(action) {
			return true
		}
	}
	return false
}

// AttendedRegistry is FilterRegistry for the runs that have a human on the
// other end — a direct chat with a chair (§85), as opposed to a job handed to
// it by `task`.
//
// It exists because exactly one entry in forcedDenials is not about reach.
// `ask_user` is denied to sub-agents because nobody is watching a delegate's
// loop, so a question it asks is a question nobody can answer and the deadline
// pays for it. In a chair chat that premise is simply false: the person who
// opened the conversation is sitting in it. Left denied, the agent asked its
// question as prose and then guessed, because words were the only way it had.
//
// A wrapper rather than a flag threaded through FilterRegistry, and not a
// second copy of the rules: every caller still goes through the one function
// that answers "what does this worker hold", and this one adds the single thing
// that depends on who is listening rather than on what the work is. Both places
// that read a chair's tools — the engine's dispatcher and the app's tools panel
// — must call this one, or the panel and the model disagree about a tool the
// model can actually call.
func AttendedRegistry(parent *skill.Registry, p Profile, ceiling *mode.Mode) *skill.Registry {
	filtered := FilterRegistry(parent, p, ceiling)
	if filtered == nil || !p.WantsToBeAsked() {
		return filtered
	}
	// Source stays what the host registered it as, so the tools panel files it
	// where the user already knows to look for it.
	if asker, ok := parent.Get("ask_user"); ok {
		source, known := parent.SourceOf("ask_user")
		if !known {
			source = skill.SourceWorkbench
		}
		// Same reasoning as the loop above: forcedDenials guarantees the name is
		// not already in `filtered`, so a collision cannot happen — and if one
		// somehow did, the chair simply asks in prose as it did before.
		_ = filtered.Register(asker, source)
	}
	return filtered
}

// missingAgentServers names the servers pointed at this agent whose tools are
// not in the parent registry yet.
//
// It exists because the registry is copied at dispatch and MCP servers arrive
// in it asynchronously (see FilterRegistry's ponytail). Answering "which ones
// are missing" rather than "is anything missing" lets the refusal name them,
// and a refusal that names the thing is one the user can act on.
//
// A disabled server is not missing — it is off, which the user chose, and
// config.MCPServersForAgent has already left it out.
func missingAgentServers(parent *skill.Registry, p Profile) []string {
	if parent == nil || p.Desk == "" {
		return nil
	}
	servers := config.MCPServersForAgent(p.Name)
	if len(servers) == 0 {
		return nil
	}
	present := map[string]bool{}
	for name := range parent.Snapshot() {
		if source, ok := parent.SourceOf(name); ok && source == skill.SourceMCP {
			for _, server := range servers {
				if mcp.ToolBelongsTo(name, []string{server}) {
					present[server] = true
				}
			}
		}
	}
	var missing []string
	for _, server := range servers {
		if !present[server] {
			missing = append(missing, server)
		}
	}
	return missing
}
