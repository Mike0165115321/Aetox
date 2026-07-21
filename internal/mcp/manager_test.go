package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// Register wires a real server's tools into a registry as SourceExternal and
// emits a default ask-rule for the server — the safety gate the plan requires.
func TestManagerRegister(t *testing.T) {
	bin := buildEchoServer(t)
	m := NewManager([]Server{{Name: "echo", Command: []string{bin}, Timeout: 10 * time.Second}})
	t.Cleanup(func() { m.Close() })

	reg := skill.NewRegistry()
	rules, errs := m.Register(context.Background(), reg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if src, ok := reg.SourceOf("echo_echo"); !ok || src != skill.SourceMCP {
		t.Fatalf("echo_echo source = %q ok=%v, want mcp", src, ok)
	}

	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Tool != "echo_*" || rules[0].Action != safety.PermissionAsk {
		t.Fatalf("rule = %+v, want echo_* / ask", rules[0])
	}
}

// A broken server contributes no tools and no rule, but doesn't break the batch
// or return a nil-panic — other servers (and the agent) carry on.
func TestManagerSkipsBrokenServer(t *testing.T) {
	m := NewManager([]Server{
		{Name: "broken", Command: []string{"aetox-no-such-binary-xyz"}, Timeout: 2 * time.Second},
	})
	t.Cleanup(func() { m.Close() })

	reg := skill.NewRegistry()
	rules, errs := m.Register(context.Background(), reg)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
	if len(rules) != 0 {
		t.Fatalf("got %d rules, want 0 (broken server gates nothing)", len(rules))
	}
	if len(reg.Names()) != 0 {
		t.Fatalf("registry has %d tools, want 0", len(reg.Names()))
	}
}

// NewManager drops entries with no name or command so they can't later panic.
func TestNewManagerSkipsInvalid(t *testing.T) {
	m := NewManager([]Server{
		{Name: "", Command: []string{"x"}},
		{Name: "y", Command: nil},
		{Name: "ok", Command: []string{"x"}},
	})
	if len(m.Clients()) != 1 {
		t.Fatalf("got %d clients, want 1", len(m.Clients()))
	}
}
