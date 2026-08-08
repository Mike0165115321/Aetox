package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// markdownSkill wraps an externally discovered SKILL.md file (opencode/Claude
// Code style: frontmatter name+description, free-form instruction body).
// Invoking it just hands the body back as tool output for the model to
// follow — there is no compiled behavior, unlike every other skill.Tool.
type markdownSkill struct {
	name        string
	description string
	body        string
}

func (s *markdownSkill) Name() string        { return s.name }
func (s *markdownSkill) Description() string { return s.description }

func (s *markdownSkill) Execute(_ context.Context, _ Input) (Output, error) {
	return newToolOutput(s.name, s.name, s.body, time.Now(), false, nil), nil
}

func (s *markdownSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	description := s.description
	if description == "" {
		description = "Discovered skill " + s.name
	}
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        s.name,
			Description: description,
			Parameters:  payload,
		},
	}
}

func (s *markdownSkill) ExecuteTool(ctx context.Context, _ map[string]any) (Output, error) {
	return s.Execute(ctx, nil)
}

// DefaultSkillsDir is Aetox's own skill directory: ~/.aetox/skills. Aetox owns
// its skill storage under its own home-level dotdir — the same convention as
// ~/.agents (opencode) and ~/.claude (Claude Code), but not shared with them.
// plugin_install writes here and discovery scans here; nothing reads or writes
// another tool's directory. Empty if the home directory can't be resolved.
func DefaultSkillsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".aetox", "skills")
}

// DefaultDiscoveryPaths returns the skill scan locations — Aetox's own dir only.
func DefaultDiscoveryPaths() []string {
	dir := DefaultSkillsDir()
	if dir == "" {
		return nil
	}
	return []string{dir}
}

// DiscoveredSkill describes one SKILL.md including where it lives — the
// Settings management surface needs Dir to delete or reveal it.
type DiscoveredSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Dir         string `json:"dir"`
	// Bundled marks a skill that ships inside the binary (bundled_skills.go).
	// It has no Dir, so it cannot be revealed, deleted, or read a second file
	// out of — the surfaces that offer those ask this rather than testing Dir
	// for emptiness, so "no folder" has one meaning in one place.
	Bundled bool `json:"bundled,omitempty"`

	body string
}

// scanSkills is the one scan loop every public view shares: the bundled skills,
// then each directory in paths scanned for <dir>/*/SKILL.md. A missing scan
// directory is not an error (most default paths won't exist); a malformed
// SKILL.md is collected as an error but does not stop the scan.
func scanSkills(paths []string) ([]DiscoveredSkill, []error) {
	found, errs := diskSkills(paths)
	return withBundled(found), errs
}

func diskSkills(paths []string) ([]DiscoveredSkill, []error) {
	var found []DiscoveredSkill
	var errs []error
	for _, dir := range paths {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("scan %s: %w", dir, err))
			}
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillDir := filepath.Join(dir, entry.Name())
			raw, readErr := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
			if readErr != nil {
				continue
			}
			name, description, body, parseErr := parseSkillMarkdown(string(raw))
			if parseErr != nil {
				errs = append(errs, fmt.Errorf("parse %s: %w", filepath.Join(skillDir, "SKILL.md"), parseErr))
				continue
			}
			if name == "" {
				name = entry.Name()
			}
			found = append(found, DiscoveredSkill{Name: name, Description: description, Dir: skillDir, body: body})
		}
	}
	return found, errs
}

// ListDiscovered reports every SKILL.md found under paths, with locations —
// for management UIs. Scan errors are dropped: a listing that shows what IS
// loadable is still useful when one stray file is malformed.
func ListDiscovered(paths []string) []DiscoveredSkill {
	found, _ := scanSkills(paths)
	return found
}

// ScanIssues is ListDiscovered's other half: the SKILL.md files the scan found
// but could not read. The settings page shows these, because a file dropped in
// the right folder that never appears in the list is otherwise indistinguishable
// from a folder the app is not looking at.
func ScanIssues(paths []string) ([]DiscoveredSkill, []error) {
	return scanSkills(paths)
}

// DiscoverFS is DiscoverScoped against an fs.FS rather than the disk — the
// same <dir>/*/SKILL.md layout, read out of an embedded filesystem.
//
// It exists so a skill that ships inside the binary and one the user wrote are
// the same kind of thing, read by the same parser, differing only in which
// filesystem they came out of. The alternative — a second format for shipped
// knowledge — is how "copy a shipped agent and change it" turns into a
// translation instead of a copy.
func DiscoverFS(fsys fs.FS, dir string) ([]Skill, []error) {
	if fsys == nil {
		return nil, nil
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, nil // an agent that ships no skills is the normal case
	}
	var found []DiscoveredSkill
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		file := path.Join(dir, entry.Name(), "SKILL.md")
		raw, readErr := fs.ReadFile(fsys, file)
		if readErr != nil {
			continue
		}
		name, description, body, parseErr := parseSkillMarkdown(string(raw))
		if parseErr != nil {
			errs = append(errs, fmt.Errorf("parse %s: %w", file, parseErr))
			continue
		}
		if name == "" {
			name = entry.Name()
		}
		found = append(found, DiscoveredSkill{Name: name, Description: description, Dir: path.Dir(file), body: body})
	}
	return wrapDiscovered(found), errs
}

// DiscoverScoped is DiscoverSkills without the shared shelf: it wraps only the
// SKILL.md files actually found under paths, and adds none of the bundled ones.
//
// The difference is the whole reason it exists. DiscoverSkills answers "what
// can this machine do", so it folds in what ships with Aetox. This answers
// "what does *this worker* know", and a scan that quietly added the shelf would
// hand every agent the same set — which is the one thing a per-agent skills
// folder is for preventing. The bundled skills are already in the parent
// registry anyway; adding them again here would only collide.
func DiscoverScoped(paths []string) ([]Skill, []error) {
	discovered, errs := diskSkills(paths)
	return wrapDiscovered(discovered), errs
}

// DiscoverSkills scans paths and wraps each SKILL.md into an invokable Skill.
func DiscoverSkills(paths []string) ([]Skill, []error) {
	discovered, errs := scanSkills(paths)
	return wrapDiscovered(discovered), errs
}

func wrapDiscovered(discovered []DiscoveredSkill) []Skill {
	skills := make([]Skill, 0, len(discovered))
	for _, d := range discovered {
		skills = append(skills, &markdownSkill{name: d.Name, description: d.Description, body: d.body})
	}
	return skills
}

// RegisterDiscovered scans paths for SKILL.md files and registers each into
// registry as SourceExternal. A name collision (with a built-in or another
// discovered skill) is reported, not fatal — mirrors the extraSkills
// collision handling in desktop/app.go's bootstrapFromConfig.
func RegisterDiscovered(registry *Registry, paths []string) []error {
	if registry == nil {
		return nil
	}
	discovered, errs := DiscoverSkills(paths)
	for _, s := range discovered {
		if err := registry.Register(s, SourceSkill); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// parseSkillMarkdown parses a SKILL.md file:
//
//	---
//	name: skill-name
//	description: what it does
//	---
//	body (markdown instructions for the model to follow)
//
// Only "name" and "description" keys are read — the rest of the frontmatter is
// none of a skill's business (see MCP-SUPPORT-PLAN.md, opencode's own SKILL.md
// format).
func parseSkillMarkdown(raw string) (name, description, body string, err error) {
	fields, body, err := ParseFrontmatter(raw)
	if err != nil {
		return "", "", "", err
	}
	return fields["name"], fields["description"], body, nil
}

// ParseFrontmatter splits a leading "---" block off a markdown document and
// returns its keys (lowercased, values unquoted) plus the trimmed body. A
// document with no frontmatter is not an error — it is all body.
//
// Deliberately not YAML: one "key: value" per line, no nesting, no lists;
// anything else on a line is ignored. Exported because agent profiles
// (internal/subagent, ARCHITECTURE.md §44) are the same file shape with more keys,
// and a second parser for the same format is a second set of edge cases.
func ParseFrontmatter(raw string) (map[string]string, string, error) {
	fields := map[string]string{}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	trimmed := strings.TrimLeft(raw, "\n")
	if !strings.HasPrefix(trimmed, "---\n") {
		return fields, strings.TrimSpace(raw), nil
	}
	rest := trimmed[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, "", errors.New("frontmatter is not terminated with a closing ---")
	}

	for _, line := range strings.Split(rest[:end], "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return fields, strings.TrimSpace(rest[end+len("\n---"):]), nil
}
