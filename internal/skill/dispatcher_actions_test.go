package skill

import (
	"context"
	"encoding/json"
	"testing"
)

// defOf finds one tool in what this dispatcher would send the model, and hands
// back its schema. Missing means the desk does not carry it, which is a real
// answer here rather than a failure — half these tests are about a tool being
// gone.
//
// The action enum is read with packed_test.go's enumOf: one reader for one
// question, so a change to the enum's shape breaks one place.
func defOf(t *testing.T, d *Dispatcher, name string) (map[string]any, bool) {
	t.Helper()
	for _, def := range d.ToolDefinitions() {
		if def.Function.Name != name {
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
			t.Fatalf("%s: parameters are not JSON: %v", name, err)
		}
		return schema, true
	}
	return nil, false
}

// raw returns one tool's parameters exactly as they would be sent, for the two
// tests that assert nothing changed.
func raw(t *testing.T, d *Dispatcher, name string) (string, bool) {
	t.Helper()
	for _, def := range d.ToolDefinitions() {
		if def.Function.Name == name {
			return string(def.Function.Parameters), true
		}
	}
	return "", false
}

// The whole point of the filter: a desk that allows some of a pack's actions is
// offered a tool with only those, not the pack whole and not nothing.
func TestDispatcherOffersOnlyTheAllowedActionsOfAPack(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	d := NewDispatcher(registry).WithActions(func(_, action string) bool {
		// The reading half of `shell`, which is the shape วางแผน wants.
		return action == "shell_output" || action == "shell_list"
	})

	schema, ok := defOf(t, d, "shell")
	if !ok {
		t.Fatal("shell is missing entirely, want it narrowed")
	}
	got := enumOf(t, schema)
	if len(got) != 2 {
		t.Fatalf("shell offers actions %v, want exactly output and list", got)
	}
	for _, unwanted := range []string{"run", "kill"} {
		for _, a := range got {
			if a == unwanted {
				t.Errorf("shell still offers %q", unwanted)
			}
		}
	}
}

// A pack with nothing left is not a tool that refuses every call — it is a tool
// this desk does not have, and it has to be gone from the block.
func TestDispatcherDropsAPackWithNoAllowedActions(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	d := NewDispatcher(registry).WithActions(func(tool, _ string) bool { return tool != "shell" })

	if _, ok := defOf(t, d, "shell"); ok {
		t.Error("shell is still in the tool block with no action allowed")
	}
}

// Narrowing the block without narrowing the door would be worse than not
// narrowing at all: the model can name an action it was never offered.
func TestDispatcherRefusesAnActionItNeverOffered(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	d := NewDispatcher(registry).WithActions(func(tool, _ string) bool { return tool != "shell" })

	_, handled, _ := d.ExecuteTool(context.Background(), "shell", map[string]any{
		"action": "run", "command": "echo denied",
	})
	if handled {
		t.Error("ExecuteTool ran a tool the block never carried")
	}
}

// A tool that is not packed must come through untouched, whatever the filter
// says — the filter has nothing to say about it.
func TestDispatcherLeavesUnpackedToolsAlone(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	plain := NewDispatcher(registry)
	filtered := NewDispatcher(registry).WithActions(func(string, string) bool { return false })

	before, ok := raw(t, plain, "read")
	if !ok {
		t.Fatal("read is missing from an unfiltered dispatcher")
	}
	after, ok := raw(t, filtered, "read")
	if !ok {
		t.Fatal("read was dropped by an action filter that has no business seeing it")
	}
	if before != after {
		t.Error("read's schema changed under an action filter")
	}
}

// A pack nobody narrowed must be the tool it was, byte for byte.
func TestDispatcherKeepsAWholePackIdentical(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	plain := NewDispatcher(registry)
	all := NewDispatcher(registry).WithActions(func(string, string) bool { return true })

	before, _ := raw(t, plain, "shell")
	after, ok := raw(t, all, "shell")
	if !ok {
		t.Fatal("shell was dropped by a filter that allows everything")
	}
	if before != after {
		t.Error("shell's schema changed under a filter that allows every action")
	}
}
