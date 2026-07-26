package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

// This file is the write half of a profile's life: reading the raw markdown a
// settings page edits, saving it under the right layer's directory, and the one
// derived write (SetModel). Bundled profiles are never touched — saving one
// writes a user file that shadows it, and deleting that file is how "revert to
// default" works, with no separate override store to keep in sync.
//
// Every function takes a Kind. Nothing here searches the other layer as a
// fallback: an agent and a sub-agent may share a name and remain two different
// profiles, so guessing which one a caller meant is not something this package
// is allowed to do.

// ReadRaw returns the markdown text behind a profile: the user's file if there
// is one, otherwise the bundled original. ok is false when neither exists in
// that layer.
func ReadRaw(name string, kind Kind) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || !validName(name) {
		return "", false
	}
	if dir, err := userDir(kind); err == nil {
		if raw, err := os.ReadFile(filepath.Join(dir, name+".md")); err == nil {
			return string(raw), true
		}
	}
	if raw, err := bundledProfiles.ReadFile(embedDir(kind) + "/" + name + ".md"); err == nil {
		return string(raw), true
	}
	return "", false
}

// Save writes body into the layer's user directory, creating it on first use.
// **kind is a real argument because the directory is the only thing that records
// it** — there is no frontmatter key to fall back on. A name that could climb out
// of the directory is rejected rather than sanitized: it arrives from the UI, so
// this is the boundary.
func Save(name, body string, kind Kind) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("ต้องตั้งชื่อเอเจนก่อน")
	}
	if len([]rune(name)) > 40 {
		return errors.New("ชื่อยาวเกินไป (ไม่เกิน 40 ตัวอักษร)")
	}
	if !validName(name) {
		return errors.New(`ชื่อห้ามมีช่องว่างหรืออักขระเหล่านี้: \ / : * ? " < > |`)
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("เนื้อหาเอเจนว่างเปล่า")
	}
	dir, err := userDir(normalizeKind(kind))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644)
}

// Delete removes the user's file for name in that layer. A bundled profile of
// the same name reappears in its place — that is the revert, not a special case.
// A name with no user file is not an error: the bundled one was never theirs to
// delete.
func Delete(name string, kind Kind) error {
	name = strings.TrimSpace(name)
	if name == "" || !validName(name) {
		return errors.New("ชื่อเอเจนไม่ถูกต้อง")
	}
	dir, err := userDir(kind)
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
// the result as a user file in the same layer. An empty model removes the line,
// which is what "inherit whatever model is selected" means.
func SetModel(name string, kind Kind, modelName string) error {
	raw, ok := ReadRaw(name, kind)
	if !ok {
		return errors.New("ไม่พบเอเจนชื่อ " + name)
	}
	return Save(name, setFrontmatterField(raw, "model", strings.TrimSpace(modelName)), kind)
}

// normalizeKind keeps an unrecognized kind out of the sub-agent layer: a typo
// must not be able to make a profile spawn-only.
func normalizeKind(kind Kind) Kind {
	if kind == KindSubagent {
		return KindSubagent
	}
	return KindAgent
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

// FilterRegistry returns the registry to hand an agent running under p: the same
// one when the profile filters nothing (the common case — nothing copied, nothing
// to drift), otherwise a new registry holding only what p allows.
//
// This is the token filter, not the safety gate: see Profile.AllowsTool. A
// sub-agent's forced denials (`task` above all) are applied here too, which is
// what makes depth 1 structural rather than a counter someone can get wrong.
//
// ponytail: tools registered into the parent *after* this call — MCP servers
// finish connecting in the background — land in the parent only, so a profile
// with an explicit tool list never sees them. Fine while no bundled profile
// lists tools and MCP together; revisit when one does.
func FilterRegistry(parent *skill.Registry, p Profile) *skill.Registry {
	if parent == nil {
		return nil
	}
	if len(p.Tools) == 0 && len(p.Deny) == 0 && !p.IsSubagent() {
		return parent
	}
	filtered := skill.NewRegistry()
	for name, s := range parent.Snapshot() {
		if !p.AllowsTool(name) {
			continue
		}
		source, ok := parent.SourceOf(name)
		if !ok {
			source = skill.SourceExternal
		}
		if err := filtered.Register(s, source); err != nil {
			continue // a duplicate name can't happen in a fresh registry; ignore rather than panic
		}
	}
	return filtered
}
