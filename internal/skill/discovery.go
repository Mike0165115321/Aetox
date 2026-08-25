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

	"github.com/Mikedev115/Aetox/internal/model"
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
	// It has no Dir, so it cannot be revealed or deleted — the surfaces that
	// offer those ask this rather than testing Dir for emptiness, so "no
	// folder" has one meaning in one place.
	Bundled bool `json:"bundled,omitempty"`

	body string
	// files is the skill's own folder when that folder is not on this disk:
	// the embedded filesystem it was read out of, rooted at the skill itself.
	//
	// Dir and this are deliberately not the same field. Dir means "a directory
	// on the user's machine" and is what reveal, delete and every disk read act
	// on; an embed.FS path handed to those would resolve against the process's
	// working directory. Keeping them apart is what lets a shipped skill carry
	// references/ and templates/ — which every published skill does — while
	// staying a thing with no folder to open in Explorer.
	//
	// nil for a skill that ships as one document, which is the older shape and
	// still a valid one.
	files fs.FS
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
func EmbeddedSkills(fsys fs.FS, dir string) ([]DiscoveredSkill, []error) {
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
		// Bundled, and with no Dir: the folder this came out of is a path inside
		// an embed.FS, and Dir is opened on the disk. Handing it an FS path
		// would resolve it against the process's working directory — a shipped
		// skill's name turning into a read of ./profiles/agents/... on
		// somebody's machine. "No folder" is the honest answer and is already
		// the one Bundled means.
		//
		// What the folder *does* carry travels in files instead: a sub-FS
		// rooted at this skill, so skill_view can serve references/x.md without
		// any path of its own to get wrong. A shipped skill that splits itself
		// across files is the normal shape — every published one does — and
		// before this the second file was simply unreachable.
		sub, subErr := fs.Sub(fsys, path.Join(dir, entry.Name()))
		if subErr != nil {
			sub = nil // one document, then; the body still works
		}
		found = append(found, DiscoveredSkill{Name: name, Description: description, Bundled: true, body: body, files: sub})
	}
	return found, errs
}

// DiscoverFS wraps EmbeddedSkills for callers that only need to register them.
func DiscoverFS(fsys fs.FS, dir string) ([]Skill, []error) {
	found, errs := EmbeddedSkills(fsys, dir)
	return wrapDiscovered(found), errs
}

// ScopedSkills is DiscoverScoped's answer before the wrapping: the documents
// found under paths, each still carrying its folder and its body.
//
// The wrapping is lossy on purpose — a registered skill is a name, a
// description and a body, and nothing downstream of the registry needs more.
// A caller that has to hand these to skills_list and skill_view does: those two
// answer with a body they did not execute, and open a second file out of the
// folder beside it. Same scan, one layer earlier.
func ScopedSkills(paths []string) ([]DiscoveredSkill, []error) {
	return diskSkills(paths)
}

// AsSkill wraps one discovered document as an invokable skill, so a caller
// holding the detailed form can still register it.
func (d DiscoveredSkill) AsSkill() Skill {
	return &markdownSkill{name: d.Name, description: d.Description, body: d.body}
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
//
// One piece of real YAML is read, and it was not a choice: a **block scalar**.
// `description: >-` followed by indented lines is how every generator wraps a
// description long enough to need wrapping, and it is what a published skill
// arrives written in. Read one key at a time this parser saw `>-` as the value
// and the wrapped text as lines with no colon, so the description became the
// literal string ">-". The skill installed, listed, and told the model nothing
// about itself — `senior-architect-agent` sat like that on the owner's machine
// (2026-08-20) until its blank line in `skills_list` was noticed by eye.
//
// Silent, and wider than skills: `internal/subagent` reads agent profiles with
// this same function, so an AGENT.md written the same way loses the sentence
// that decides whether the assistant hands it any work.
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

	lines := strings.Split(rest[:end], "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if literal, isBlock := blockScalar(value); isBlock {
			var consumed int
			fields[name], consumed = readBlock(lines[i+1:], literal)
			i += consumed
			continue
		}
		fields[name] = strings.Trim(value, `"'`)
	}
	return fields, strings.TrimSpace(rest[end+len("\n---"):]), nil
}

// blockScalar reports whether a value is a YAML block indicator, and whether it
// is the literal kind. `|` keeps the line breaks, `>` folds them into spaces;
// the trailing `-`/`+` is about trailing newlines, which TrimSpace settles
// either way, so it is accepted and ignored.
func blockScalar(value string) (literal, ok bool) {
	switch value {
	case ">", ">-", ">+":
		return false, true
	case "|", "|-", "|+":
		return true, true
	}
	return false, false
}

// readBlock takes the indented lines belonging to a block scalar and returns
// the value plus how many lines it used.
//
// Indentation ends the block, which is the whole of the YAML rule this needs:
// the next key sits at column zero. A blank line inside stays a break in both
// kinds — a folded scalar turns single newlines into spaces and keeps the blank
// ones, which is what makes a two-paragraph description survive folding.
func readBlock(lines []string, literal bool) (string, int) {
	var out []string
	used := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			used++
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			break // back at column zero: this is the next key
		}
		out = append(out, strings.TrimSpace(line))
		used++
	}
	if literal {
		return strings.TrimSpace(strings.Join(out, "\n")), used
	}
	// Folded: single newlines become spaces, blank lines stay breaks.
	var b strings.Builder
	for i, line := range out {
		switch {
		case line == "":
			b.WriteString("\n")
		case i > 0 && out[i-1] != "" && b.Len() > 0:
			b.WriteString(" " + line)
		default:
			b.WriteString(line)
		}
	}
	return strings.TrimSpace(b.String()), used
}
