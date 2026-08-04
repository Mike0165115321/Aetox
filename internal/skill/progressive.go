package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// Progressive skill loading: skills_list and skill_view are how the model
// reaches SKILL.md documents. Discovered skills are still registered (the
// user's /skill-name command and collision detection need them) but their
// per-skill tool definitions are no longer sent to the model — see
// Dispatcher.ToolDefinitions.
//
// Why: every discovered skill used to add its own entry to the tool block of
// every request, so a user with 50 skills paid ~50 schemas of context on
// every call whether or not any skill was relevant — the cost grew linearly
// with the library. These two definitions replace all of them at a flat
// price: the model pays for a skill's listing only when it asks, and for a
// body only when it opens one. Same shape as the capability boundary in
// ARCHITECTURE.md §44 — knowing a thing exists is cheap, knowing how it
// works is paid on use.
//
// Both tools scan the discovery paths at call time rather than holding the
// registry: a skill installed mid-session (plugin_install writes to
// ~/.aetox/skills) is listable immediately, before any re-bootstrap.

// skillsListSkill is L0: one line per installed skill, name + description.
type skillsListSkill struct {
	// paths overrides the scan locations; nil means DefaultDiscoveryPaths(),
	// resolved at call time so tests can point it at a fixture directory.
	paths []string
}

func (s *skillsListSkill) scanPaths() []string {
	if s.paths != nil {
		return s.paths
	}
	return DefaultDiscoveryPaths()
}

func (s *skillsListSkill) Name() string { return "skills_list" }

func (s *skillsListSkill) Description() string {
	return "List the skill documents installed on this machine — task instructions the user has added. " +
		"Returns one line per skill: name — description. Read one with skill_view before doing a task it covers."
}

func (s *skillsListSkill) Execute(ctx context.Context, _ Input) (Output, error) {
	return s.ExecuteTool(ctx, nil)
}

func (s *skillsListSkill) ToolDefinition() model.ToolDefinition {
	schema, _ := json.Marshal(map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	})
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        s.Name(),
			Description: s.Description(),
			Parameters:  schema,
		},
	}
}

func (s *skillsListSkill) ExecuteTool(_ context.Context, _ map[string]any) (Output, error) {
	start := time.Now()
	discovered := ListDiscovered(s.scanPaths())
	if len(discovered) == 0 {
		return newToolOutput(s.Name(), s.Name(), "No skills installed.", start, false, nil), nil
	}
	sort.Slice(discovered, func(i, j int) bool { return discovered[i].Name < discovered[j].Name })
	var b strings.Builder
	for _, d := range discovered {
		b.WriteString(d.Name)
		if d.Description != "" {
			b.WriteString(" — ")
			b.WriteString(d.Description)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nRead one with skill_view {\"name\": \"...\"} before doing a task it covers.")
	return newToolOutput(s.Name(), s.Name(), b.String(), start, false, nil), nil
}

// readSkillFile returns one supporting file from inside a skill's folder.
//
// The containment check is the point. `path` is a string the model wrote, and
// it is about to be joined onto a directory — "references/../../../.ssh/id_rsa"
// is the whole attack, and it is a plausible thing for a skill document to
// contain by accident as well as on purpose. Resolved and compared against the
// skill's own directory, so what the gate judges is where the path *lands*,
// never how it was spelled.
//
// Deliberately not routed through resolveSandboxPath: a skill lives in
// ~/.aetox/skills, outside the workspace, so the sandbox would refuse every
// read here. This is the same rule applied to a different root, and it is the
// only place that root is readable from.
func readSkillFile(dir, sub string) (string, error) {
	base, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("cannot read inside this skill")
	}
	full, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(sub)))
	if err != nil {
		return "", fmt.Errorf("cannot read %q", sub)
	}
	rel, err := filepath.Rel(base, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is outside the skill's own folder", sub)
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("%q is not in this skill — the skill body lists what it has", sub)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a folder; name a file inside it", sub)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("could not read %q: %w", sub, err)
	}
	if len(data) > maxSkillFileBytes {
		cut := maxSkillFileBytes
		for cut > 0 && !utf8.RuneStart(data[cut]) {
			cut--
		}
		return string(data[:cut]) + fmt.Sprintf("\n\n[cut — file is %d bytes, showed the first %d]", len(data), cut), nil
	}
	return string(data), nil
}

// skillViewSkill is L1: the full body of one skill, fetched by name.
type skillViewSkill struct {
	paths []string
}

func (s *skillViewSkill) scanPaths() []string {
	if s.paths != nil {
		return s.paths
	}
	return DefaultDiscoveryPaths()
}

func (s *skillViewSkill) Name() string { return "skill_view" }

func (s *skillViewSkill) Description() string {
	return "Read one installed skill document by name (as listed by skills_list) and follow its instructions."
}

// maxSkillFileBytes caps one L2 read. A skill may legitimately ship a large
// data file; sending all of it costs the same context whether the model needed
// the whole thing or the first page, and the cut is reported so a truncated
// read is never mistaken for a short file.
const maxSkillFileBytes = 32 << 10

// supportingFiles lists what else lives in a skill's folder, so the model can
// find a reference it was never told about.
//
// L1 without this is a dead end whenever a skill is too long for one file:
// SKILL.md can name its own references, but nothing makes it, and a skill
// written by the agent itself (which splits work into files precisely because
// it grew past one) would be unreachable past its first page. Directories are
// walked, so references/a.md shows as references/a.md rather than as a folder
// the model has to guess the contents of.
func supportingFiles(dir string) []string {
	var out []string
	root := filepath.Clean(dir)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "SKILL.md" {
			return nil
		}
		// Slash-separated regardless of platform: this string goes back to the
		// model as something to pass to `path`, and it is joined here, so one
		// spelling everywhere is one less thing for it to get wrong.
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func (s *skillViewSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := map[string]any{}
	if input != nil {
		if raw, ok := input["args"].(string); ok {
			args["name"] = strings.TrimSpace(raw)
		}
	}
	return s.ExecuteTool(ctx, args)
}

func (s *skillViewSkill) ToolDefinition() model.ToolDefinition {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name exactly as skills_list reported it",
			},
			// L2. Named files are opened only when the model decides it needs
			// them, which is the whole point of the three levels: a skill can be
			// as long as it needs to be without any of that length reaching the
			// context of a session that did not use it.
			"path": map[string]any{
				"type":        "string",
				"description": "Optional: a supporting file inside the skill (as listed at the end of the skill body), e.g. references/formats.md",
			},
		},
		"required":             []string{"name"},
		"additionalProperties": false,
	})
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        s.Name(),
			Description: s.Description(),
			Parameters:  schema,
		},
	}
}

func (s *skillViewSkill) ExecuteTool(_ context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	name, _ := args["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		err := fmt.Errorf("name is required — call skills_list to see what is installed")
		return newToolOutput(s.Name(), s.Name(), err.Error(), start, false, err), err
	}
	sub, _ := args["path"].(string)
	sub = strings.TrimSpace(sub)

	discovered, _ := scanSkills(s.scanPaths())
	for _, d := range discovered {
		if !strings.EqualFold(d.Name, name) {
			continue
		}
		if sub != "" {
			content, err := readSkillFile(d.Dir, sub)
			if err != nil {
				return newToolOutput(s.Name(), s.Name()+" "+d.Name, err.Error(), start, false, err), err
			}
			return newToolOutput(s.Name(), s.Name()+" "+d.Name+"/"+sub, content, start, false, nil), nil
		}
		body := d.body
		if files := supportingFiles(d.Dir); len(files) > 0 {
			body += "\n\nFiles in this skill — read one with skill_view {\"name\": \"" + d.Name +
				"\", \"path\": \"…\"}:\n- " + strings.Join(files, "\n- ") + "\n"
		}
		return newToolOutput(s.Name(), s.Name()+" "+d.Name, body, start, false, nil), nil
	}

	// Name the alternatives in the error: the model asked for something that
	// is not there, and the cheapest recovery is handing it the real list now
	// instead of making it burn a round on skills_list.
	names := make([]string, 0, len(discovered))
	for _, d := range discovered {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	var err error
	if len(names) == 0 {
		err = fmt.Errorf("skill %q not found — no skills are installed", name)
	} else {
		err = fmt.Errorf("skill %q not found — installed skills: %s", name, strings.Join(names, ", "))
	}
	return newToolOutput(s.Name(), s.Name(), err.Error(), start, false, err), err
}
