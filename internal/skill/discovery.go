package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

// DiscoveredSkill describes one SKILL.md found on disk including where it
// lives — the Settings management surface needs Dir to delete or reveal it.
type DiscoveredSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Dir         string `json:"dir"`

	body string
}

// scanSkills is the one scan loop both public views share: each directory in
// paths is scanned for <dir>/*/SKILL.md. A missing scan directory is not an
// error (most default paths won't exist); a malformed SKILL.md is collected
// as an error but does not stop the scan.
func scanSkills(paths []string) ([]DiscoveredSkill, []error) {
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

// DiscoverSkills scans paths and wraps each SKILL.md into an invokable Skill.
func DiscoverSkills(paths []string) ([]Skill, []error) {
	discovered, errs := scanSkills(paths)
	skills := make([]Skill, 0, len(discovered))
	for _, d := range discovered {
		skills = append(skills, &markdownSkill{name: d.Name, description: d.Description, body: d.body})
	}
	return skills, errs
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
		if err := registry.Register(s, SourceExternal); err != nil {
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
// (internal/agent, ARCHITECTURE.md §44) are the same file shape with more keys,
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
