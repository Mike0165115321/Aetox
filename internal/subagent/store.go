package subagent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

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

// Save writes a sub-agent's file into the sub-agents' home; SaveAgent writes
// an agent's into the agents'. Two doors and no kind parameter, because a
// caller that had to pass one would be re-deciding the rule the homes already
// carry — and the caller never writes the kind into the file either, the home
// says it.
//
// A name the *other* home owns is refused at the door. A name has one owner
// across both homes (memory, jobs and chat history all key on it), and a
// conflict the save could have named is not something to leave for the
// resolver to flag later.
func Save(name, body string) error {
	return save(name, body, false)
}

// SaveAgent is the team page's door — see Save.
func SaveAgent(name, body string) error {
	return save(name, body, true)
}

func save(name, body string, agentHome bool) error {
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
		// A sick file (Invalid) has its Desk cleared, so it reads as owned by
		// the sub-agents' home — which is where it sits, and where saving over
		// it is how the user fixes it.
		if (p.Desk != "") != agentHome {
			owner := "ผู้ช่วยตัวแทน"
			if p.Desk != "" {
				owner = "ตัวแทน"
			}
			return errors.New("ชื่อ " + name + " เป็นของ" + owner + "อยู่แล้ว — ความจำและประวัติงานผูกกับชื่อ ต้องตั้งชื่ออื่น")
		}
		break
	}
	homeDir := Dir
	if agentHome {
		homeDir = AgentsDir
	}
	dir, err := homeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644)
}

// Delete removes the user's file for name, from whichever home owns it. A
// bundled profile of the same name reappears in its place — that is the
// revert, not a special case. A name with no user file is not an error: the
// bundled one was never theirs to delete.
func Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || !validName(name) {
		return errors.New("ชื่อไม่ถูกต้อง")
	}
	for _, homeDir := range []func() (string, error){AgentsDir, Dir} {
		dir, err := homeDir()
		if err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(dir, name+".md")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// SetModel points one profile at a specific model (the per-profile model
// dropdown) by rewriting a single frontmatter line and saving the result as a
// user file, through the door of whichever home owns the name. An empty model
// removes the line, which is what "inherit whatever model is selected" means.
func SetModel(name, modelName string) error {
	raw, ok := ReadRaw(name)
	if !ok {
		return errors.New("ไม่พบโปรไฟล์ชื่อ " + name)
	}
	body := setFrontmatterField(raw, "model", strings.TrimSpace(modelName))
	if p, ok := Load(name); ok && p.Desk != "" {
		return SaveAgent(name, body)
	}
	return Save(name, body)
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
	filtered := skill.NewRegistry()
	for name, s := range parent.Snapshot() {
		if !p.AllowsTool(name) {
			continue
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
		source, ok := parent.SourceOf(name)
		if !ok {
			// Unreachable: name came out of this same snapshot. Skipping beats
			// inventing a source, which is how a tool ends up filed as
			// something the user installed.
			continue
		}
		if !ceiling.Carries(name, source) {
			continue
		}
		if err := filtered.Register(s, source); err != nil {
			continue // a duplicate name can't happen in a fresh registry; ignore rather than panic
		}
	}
	return filtered
}
