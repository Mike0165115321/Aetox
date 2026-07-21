package mcp

import (
	"context"
	"testing"
	"time"
)

// A server whose command can't start must surface as StatusFailed with an
// error, never a panic — the whole point of Status-based error handling is that
// a broken server drops out silently instead of crashing the agent loop.
func TestBadCommandFailsGracefully(t *testing.T) {
	c := New(Server{
		Name:    "broken",
		Command: []string{"aetox-definitely-no-such-binary-xyz"},
		Timeout: 2 * time.Second,
	})

	tools, err := c.Tools(context.Background())
	if err == nil {
		t.Fatal("expected error for nonexistent command, got nil")
	}
	if tools != nil {
		t.Fatalf("expected no tools, got %d", len(tools))
	}
	if c.Status() != StatusFailed {
		t.Fatalf("status = %q, want %q", c.Status(), StatusFailed)
	}
	if c.Err() == nil {
		t.Fatal("Err() = nil, want the cached connect error")
	}
}

func TestEmptyCommandFails(t *testing.T) {
	c := New(Server{Name: "empty"})
	if _, err := c.CallTool(context.Background(), "x", nil); err == nil {
		t.Fatal("expected error for empty command")
	}
	if c.Status() != StatusFailed {
		t.Fatalf("status = %q, want %q", c.Status(), StatusFailed)
	}
}

// Close on a never-connected client is a no-op that leaves it reusable.
func TestCloseIdle(t *testing.T) {
	c := New(Server{Name: "idle", Command: []string{"echo"}})
	if err := c.Close(); err != nil {
		t.Fatalf("Close on idle client: %v", err)
	}
	if c.Status() != StatusIdle {
		t.Fatalf("status = %q, want %q", c.Status(), StatusIdle)
	}
}
