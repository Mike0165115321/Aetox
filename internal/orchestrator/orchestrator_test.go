package orchestrator

import (
	"testing"

	"github.com/Mike0165115321/Aetox/internal/cognitive"
)

func TestSpawnGetStop(t *testing.T) {
	o := New()

	id1 := o.Spawn(cognitive.AgentConfig{Model: "model-a"})
	id2 := o.Spawn(cognitive.AgentConfig{Model: "model-b"})
	if id1 == id2 {
		t.Fatalf("expected distinct ids, got %q twice", id1)
	}

	if _, ok := o.Get(id1); !ok {
		t.Fatalf("Get(%q): expected agent, got none", id1)
	}
	if got := len(o.List()); got != 2 {
		t.Fatalf("List(): expected 2 entries, got %d", got)
	}

	if err := o.Stop(id1); err != nil {
		t.Fatalf("Stop(%q): unexpected error: %v", id1, err)
	}
	if _, ok := o.Get(id1); ok {
		t.Fatalf("Get(%q): expected no agent after Stop", id1)
	}
	if got := len(o.List()); got != 1 {
		t.Fatalf("List() after Stop: expected 1 entry, got %d", got)
	}

	if err := o.Stop("does-not-exist"); err == nil {
		t.Fatal("Stop(unknown id): expected error, got nil")
	}
}
