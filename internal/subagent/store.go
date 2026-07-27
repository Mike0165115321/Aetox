package subagent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

// This file is the write half of a profile's life: reading the raw markdown a
// settings page edits, saving it under <DataRoot>/subagents, and the one derived
// write (SetModel). Bundled profiles are never touched — saving one writes a user
// file that shadows it, and deleting that file is how "revert to default" works,
// with no separate override store to keep in sync.

// ReadRaw returns the markdown text behind a profile: the user's file if there is
// one, otherwise the bundled original. ok is false when neither exists.
func ReadRaw(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || !validName(name) {
		return "", false
	}
	if dir, err := Dir(); err == nil {
		if raw, err := os.ReadFile(filepath.Join(dir, name+".md")); err == nil {
			return string(raw), true
		}
	}
	if raw, err := bundledProfiles.ReadFile("profiles/" + name + ".md"); err == nil {
		return string(raw), true
	}
	return "", false
}

// Save writes body to <DataRoot>/subagents/<name>.md, creating the directory on
// first use. A name that could climb out of it is rejected rather than sanitized:
// it arrives from the UI, so this is the boundary.
func Save(name, body string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("ต้องตั้งชื่อซับเอเจนก่อน")
	}
	if len([]rune(name)) > 40 {
		return errors.New("ชื่อยาวเกินไป (ไม่เกิน 40 ตัวอักษร)")
	}
	if !validName(name) {
		return errors.New(`ชื่อห้ามมีช่องว่างหรืออักขระเหล่านี้: \ / : * ? " < > |`)
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("เนื้อหาซับเอเจนว่างเปล่า")
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644)
}

// Delete removes the user's file for name. A bundled profile of the same name
// reappears in its place — that is the revert, not a special case. A name with no
// user file is not an error: the bundled one was never theirs to delete.
func Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || !validName(name) {
		return errors.New("ชื่อซับเอเจนไม่ถูกต้อง")
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, name+".md")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// SetModel points one profile at a specific model (the settings page's
// per-profile model dropdown) by rewriting a single frontmatter line and saving
// the result as a user file. An empty model removes the line, which is what
// "inherit whatever model is selected" means.
func SetModel(name, modelName string) error {
	raw, ok := ReadRaw(name)
	if !ok {
		return errors.New("ไม่พบซับเอเจนชื่อ " + name)
	}
	return Save(name, setFrontmatterField(raw, "model", strings.TrimSpace(modelName)))
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
// the tools p allows, always minus the forced denials. Copying rather than
// filtering at dispatch time is what makes depth 1 structural — `task` is simply
// not in the child's registry, so no counter can be got wrong.
//
// This is the token filter, not the safety gate: see Profile.AllowsTool.
//
// ponytail: tools registered into the parent *after* this call — MCP servers
// finish connecting in the background — land in the parent only, so a sub-agent
// spawned earlier never sees them. Fine while a sub-agent is a short-lived
// delegate; revisit if one is ever kept alive across a server connect.
func FilterRegistry(parent *skill.Registry, p Profile) *skill.Registry {
	if parent == nil {
		return nil
	}
	filtered := skill.NewRegistry()
	for name, s := range parent.Snapshot() {
		if !p.AllowsTool(name) {
			continue
		}
		source, ok := parent.SourceOf(name)
		if !ok {
			// Unreachable: name came out of this same snapshot. Skipping beats
			// inventing a source, which is how a tool ends up filed as
			// something the user installed.
			continue
		}
		if err := filtered.Register(s, source); err != nil {
			continue // a duplicate name can't happen in a fresh registry; ignore rather than panic
		}
	}
	return filtered
}
