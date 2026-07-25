package main

import (
	"encoding/json"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

// The workbench tools travel to the model in the same batch as the built-ins,
// and a provider validates the batch as a whole — one malformed schema here
// fails the request and takes every other tool down with it. internal/skill has
// the matching check for its own; this is the other half of what the app sends.
func TestWorkbenchToolDefinitionsAreWellFormed(t *testing.T) {
	app := &App{}
	tools := []skill.Skill{
		&browserOpenSkill{app: app},
		&browserReadSkill{app: app},
		&browserClickSkill{app: app},
		&browserTypeSkill{app: app},
		&askUserSkill{app: app},
		&todoWriteSkill{app: app},
	}

	seen := map[string]bool{}
	for _, s := range tools {
		tool, ok := s.(skill.Tool)
		if !ok {
			t.Errorf("%s is registered as a model-facing tool but has no ToolDefinition", s.Name())
			continue
		}
		def := tool.ToolDefinition()

		if def.Type != "function" {
			t.Errorf("%s: Type = %q, want \"function\"", s.Name(), def.Type)
		}
		// The dispatcher looks a skill up by the name the model called.
		if def.Function.Name != s.Name() {
			t.Errorf("%s: definition calls itself %q — the dispatcher would never find it", s.Name(), def.Function.Name)
		}
		if seen[def.Function.Name] {
			t.Errorf("duplicate tool name %q", def.Function.Name)
		}
		seen[def.Function.Name] = true
		if def.Function.Description == "" {
			t.Errorf("%s: no description — the model has nothing to choose it by", s.Name())
		}

		var schema map[string]any
		if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
			t.Errorf("%s: parameters are not valid JSON: %v", s.Name(), err)
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("%s: schema type = %v, want \"object\"", s.Name(), schema["type"])
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s: schema has no properties object", s.Name())
			continue
		}
		if required, present := schema["required"].([]any); present {
			for _, r := range required {
				key, _ := r.(string)
				if _, found := props[key]; !found {
					t.Errorf("%s: %q is required but is not a declared property", s.Name(), key)
				}
			}
		}
		for prop, raw := range props {
			spec, ok := raw.(map[string]any)
			if !ok {
				t.Errorf("%s: property %q is not an object", s.Name(), prop)
				continue
			}
			if spec["type"] == nil && spec["enum"] == nil && spec["anyOf"] == nil && spec["oneOf"] == nil {
				t.Errorf("%s: property %q declares no type", s.Name(), prop)
			}
		}
	}

	if len(seen) != len(tools) {
		t.Errorf("validated %d distinct tools, expected %d", len(seen), len(tools))
	}
}
