package skill

import (
	"context"
	"testing"
)

func newDispatcherWith(t *testing.T, skills ...Skill) *Dispatcher {
	t.Helper()
	r := NewRegistry()
	for _, s := range skills {
		if err := r.Register(s, SourceBuiltin); err != nil {
			t.Fatalf("setup Register(%s): %v", s.Name(), err)
		}
	}
	return NewDispatcher(r)
}

func TestDispatcherExecuteRoutesToSkill(t *testing.T) {
	d := newDispatcherWith(t, &echoSkill{})
	out, handled, err := d.Execute(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true for a registered skill command")
	}
	if out.Content != "hello" {
		t.Errorf("Content = %q, want %q", out.Content, "hello")
	}
}

func TestDispatcherExecuteUnknownCommandNotHandled(t *testing.T) {
	d := newDispatcherWith(t, &echoSkill{})
	_, handled, err := d.Execute(context.Background(), "notregistered foo")
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false for an unregistered command")
	}
}

func TestDispatcherToolDefinitionsOnlyIncludesTools(t *testing.T) {
	// echoSkill implements Skill but not Tool (no ToolDefinition/ExecuteTool);
	// timeSkill implements both.
	d := newDispatcherWith(t, &echoSkill{}, &timeSkill{})
	defs := d.ToolDefinitions()
	if len(defs) != 1 {
		t.Fatalf("ToolDefinitions() returned %d entries, want 1 (only the Tool-implementing skill)", len(defs))
	}
	if defs[0].Function.Name != "time" {
		t.Errorf("ToolDefinitions()[0].Function.Name = %q, want %q", defs[0].Function.Name, "time")
	}
}

func TestDispatcherExecuteToolOnNonToolSkill(t *testing.T) {
	d := newDispatcherWith(t, &echoSkill{})
	_, handled, err := d.ExecuteTool(context.Background(), "echo", map[string]any{})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false: echoSkill does not implement Tool")
	}
}

func TestDispatcherExecuteToolSucceeds(t *testing.T) {
	d := newDispatcherWith(t, &timeSkill{})
	_, handled, err := d.ExecuteTool(context.Background(), "time", map[string]any{})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true for a registered Tool")
	}
}

func TestDispatcherNilRegistryIsSafe(t *testing.T) {
	d := NewDispatcher(nil)
	if _, handled, err := d.Execute(context.Background(), "anything"); handled || err != nil {
		t.Errorf("Execute on nil registry = handled=%v err=%v, want false, nil", handled, err)
	}
	if got := d.Names(); got != nil {
		t.Errorf("Names() on nil registry = %v, want nil", got)
	}
	if got := d.ToolDefinitions(); got != nil {
		t.Errorf("ToolDefinitions() on nil registry = %v, want nil", got)
	}
}
