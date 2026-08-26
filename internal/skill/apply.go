package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Apply writes one approved edit to a skill on disk — the write side of the
// self-optimize loop's skill-refine flow (docs/architecture/self-optimize-loop-2026-08-26.md).
//
// It is the skill-kind twin of learned.Apply: never called speculatively, only
// from an approved pending_changes row, so "nothing rewrites a skill without a
// human clicking yes" holds the same way it does for memory. `name` is the skill,
// `op`/`before`/`body` the edit — an Edit over SKILL.md, not the entry model
// memory uses, because a SKILL.md is prose and not a list of bullets.
//
// Copy-on-first-edit is the one thing this does that memory does not have to.
// withBundled (bundled_skills.go) drops a bundled skill the moment a disk folder
// of its name exists, so the first edit of a bundled skill must copy its WHOLE
// folder out to ~/.aetox/skills first — SKILL.md and references/ together. A copy
// of only the SKILL.md would leave the disk override pointing at references that
// no longer travel with it, which is the exact "crippled skill" the folder
// embed was added to prevent.
func Apply(name, op, before, body string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("no skill named to edit")
	}
	base := DefaultSkillsDir()
	if base == "" {
		return fmt.Errorf("cannot resolve the skills directory")
	}
	dir := filepath.Join(base, name)
	skillPath := filepath.Join(dir, skillFileName)

	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		if err := copyBundledSkillOut(name, dir); err != nil {
			return err
		}
	}

	raw, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("skill %q has no %s to edit: %w", name, skillFileName, err)
	}
	text := string(raw)
	before = strings.TrimSpace(before)
	body = strings.TrimSpace(body)

	switch op {
	case "replace":
		if before == "" || body == "" {
			return fmt.Errorf("a replacement needs the text to change and its replacement")
		}
		if !strings.Contains(text, before) {
			// The SKILL.md was edited by hand, or an earlier approval already
			// changed this passage, between the proposal and this click. Same race
			// learned.Apply meets on a stale memory line — refuse, don't guess.
			return fmt.Errorf("the text this edit would change is no longer in the skill")
		}
		text = strings.Replace(text, before, body, 1)
	case "add":
		if body == "" {
			return fmt.Errorf("nothing to add")
		}
		text = strings.TrimRight(text, "\n") + "\n\n" + body + "\n"
	case "remove":
		if before == "" {
			return fmt.Errorf("nothing named to remove")
		}
		if !strings.Contains(text, before) {
			return nil // already gone — the state the approval asked for is on disk
		}
		text = strings.Replace(text, before, "", 1)
	default:
		return fmt.Errorf("unknown skill operation %q", op)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(skillPath, []byte(text), 0o644)
}

// Body returns a skill's current SKILL.md text — the disk override if the user
// has one, else the bundled copy. The generator reads it to draft an anchored
// edit against what the skill actually says right now.
func Body(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("no skill named")
	}
	if base := DefaultSkillsDir(); base != "" {
		if data, err := os.ReadFile(filepath.Join(base, name, skillFileName)); err == nil {
			return string(data), nil
		}
	}
	data, err := fs.ReadFile(bundledSkillFS, bundledSkillRoot+"/"+name+"/"+skillFileName)
	if err != nil {
		return "", fmt.Errorf("no skill named %q", name)
	}
	return string(data), nil
}

// copyBundledSkillOut copies a bundled skill's whole folder out to dir, so a
// disk override carries its references/ too, not only SKILL.md. Refuses a name
// that is not a bundled skill — there is nothing to edit and nothing to copy.
func copyBundledSkillOut(name, dir string) error {
	root := bundledSkillRoot + "/" + name
	if _, err := fs.Stat(bundledSkillFS, root+"/"+skillFileName); err != nil {
		return fmt.Errorf("no skill named %q to edit", name)
	}
	return fs.WalkDir(bundledSkillFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, err := fs.ReadFile(bundledSkillFS, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
}
