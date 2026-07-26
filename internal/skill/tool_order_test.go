package skill

import (
	"encoding/json"
	"testing"
)

// The tool block is serialized into the head of every request, ahead of the
// conversation, so it is the prompt prefix every provider caches on. Built from
// unsorted map iteration it was reshuffled on each call, missing that cache on
// every turn — measured at ~2s of extra prompt-eval per turn against a local
// Ollama. Byte-identical repeats are the whole fix, so that is what is asserted.
func TestToolDefinitionsAreByteStableAcrossCalls(t *testing.T) {
	d := newDispatcherWith(t, &timeSkill{}, &echoSkill{})

	first, err := json.Marshal(d.ToolDefinitions())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for i := 0; i < 20; i++ {
		next, err := json.Marshal(d.ToolDefinitions())
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(next) != string(first) {
			t.Fatalf("call %d produced a different tool payload — the prompt prefix changes every turn\n first: %s\n  then: %s", i+2, first, next)
		}
	}
}

// Two skills is enough to prove stability but not enough to prove *sorted*:
// with 20 registered tools an accidental map order would pass the byte-equality
// check only if Go happened to repeat itself, which it does not.
func TestRegistryNamesAreSorted(t *testing.T) {
	r := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	names := r.Names()
	if len(names) < 2 {
		t.Fatalf("expected the default registry to have several skills, got %v", names)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("Names() is not sorted at %d (%q > %q): %v", i, names[i-1], names[i], names)
		}
	}
}
