package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

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

	discovered, _ := scanSkills(s.scanPaths())
	for _, d := range discovered {
		if strings.EqualFold(d.Name, name) {
			return newToolOutput(s.Name(), s.Name()+" "+d.Name, d.body, start, false, nil), nil
		}
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
