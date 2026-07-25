package skill

import (
	"encoding/json"
	"testing"
)

// Every tool definition is sent to the model on every turn, and a provider
// validates the whole batch before running any of it — one malformed schema
// fails the request and takes all 25 tools down with it, not just its own. So
// the shape of every built-in is checked here rather than discovered live.
func TestBuiltinToolDefinitionsAreWellFormed(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})

	seen := map[string]string{}
	var tools int
	for name, s := range registry.Snapshot() {
		tool, ok := s.(Tool)
		if !ok {
			continue // Skill-only (CLI-facing); never reaches a model.
		}
		tools++
		def := tool.ToolDefinition()

		if def.Type != "function" {
			t.Errorf("%s: Type = %q, want \"function\"", name, def.Type)
		}
		if def.Function.Name == "" {
			t.Errorf("%s: tool definition has no name", name)
		}
		// The dispatcher looks the skill up by the name the model called, so a
		// definition naming itself something else is a tool that can never run.
		if def.Function.Name != name {
			t.Errorf("%s: definition calls itself %q — the dispatcher would never find it", name, def.Function.Name)
		}
		if prev, dup := seen[def.Function.Name]; dup {
			t.Errorf("%s and %s both claim the tool name %q", prev, name, def.Function.Name)
		}
		seen[def.Function.Name] = name

		if def.Function.Description == "" {
			t.Errorf("%s: no description — the model has nothing to choose it by", name)
		}

		var schema map[string]any
		if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
			t.Errorf("%s: parameters are not valid JSON: %v", name, err)
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("%s: schema type = %v, want \"object\"", name, schema["type"])
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s: schema has no properties object", name)
			continue
		}
		// Every required field must exist in properties, or a model that obeys
		// the schema still produces a call the tool rejects.
		if required, present := schema["required"].([]any); present {
			for _, r := range required {
				key, _ := r.(string)
				if _, found := props[key]; !found {
					t.Errorf("%s: %q is required but is not a declared property", name, key)
				}
			}
		}
		for prop, raw := range props {
			spec, ok := raw.(map[string]any)
			if !ok {
				t.Errorf("%s: property %q is not an object", name, prop)
				continue
			}
			if spec["type"] == nil && spec["enum"] == nil && spec["anyOf"] == nil && spec["oneOf"] == nil {
				t.Errorf("%s: property %q declares no type", name, prop)
			}
		}
	}

	if tools == 0 {
		t.Fatal("no model-facing tools found — the registry would offer nothing")
	}
	t.Logf("%d model-facing tool definitions validated", tools)
}
