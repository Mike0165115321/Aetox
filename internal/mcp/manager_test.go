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

// Register must connect clients concurrently, not one after another — it runs
// synchronously during app startup (desktop/app.go, cmd/aetox/main.go), and a
// server like `npx -y pkg@latest` resolving on a cold cache can be slow.
// Self-calibrating rather than asserting an absolute duration: per-connection
// overhead (process spawn + MCP handshake) varies a lot by machine — what
// must hold is that N servers with the same artificial delay don't cost
// N times a single server's duration.
func TestManagerRegisterRunsConcurrently(t *testing.T) {
	bin := buildEchoServer(t)
	delay := map[string]string{"AETOX_TEST_DELAY_MS": "500"}
	newServer := func(name string) Server {
		return Server{Name: name, Command: []string{bin}, Environment: delay, Timeout: 5 * time.Second}
	}

	one := NewManager([]Server{newServer("solo")})
	t.Cleanup(func() { one.Close() })
	start := time.Now()
	if _, errs := one.Register(context.Background(), skill.NewRegistry()); len(errs) != 0 {
		t.Fatalf("unexpected errors (solo): %v", errs)
	}
	soloElapsed := time.Since(start)

	four := NewManager([]Server{newServer("s1"), newServer("s2"), newServer("s3"), newServer("s4")})
	t.Cleanup(func() { four.Close() })
	start = time.Now()
	if _, errs := four.Register(context.Background(), skill.NewRegistry()); len(errs) != 0 {
		t.Fatalf("unexpected errors (four): %v", errs)
	}
	fourElapsed := time.Since(start)

	// Sequential would scale ~4x; parallel stays close to the solo case
	// regardless of per-connection overhead. 2x leaves generous slack for
	// scheduling noise while still catching a regression to sequential.
	if fourElapsed > 2*soloElapsed {
		t.Fatalf("4 servers took %v vs 1 server's %v — looks sequential, not parallel", fourElapsed, soloElapsed)
	}
}

// The caller's ctx deadline must actually bound Register — this is the fix
// for the real report: a slow server (npx resolving on a cold cache) used to
// block the whole app's startup for up to its own 30s default Timeout, with
// no way for the caller to cap it. desktop/app.go and cmd/aetox/main.go now
// pass a short-lived ctx around this exact call.
func TestManagerRegisterBoundedByCallerContext(t *testing.T) {
	bin := buildEchoServer(t)
	m := NewManager([]Server{
		// Configured Timeout is generous (5s); the caller's ctx should win.
		{Name: "slow", Command: []string{bin}, Environment: map[string]string{"AETOX_TEST_DELAY_MS": "5000"}, Timeout: 5 * time.Second},
	})
	t.Cleanup(func() { m.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, errs := m.Register(ctx, skill.NewRegistry())
	elapsed := time.Since(start)

	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1 (the slow server should fail to connect in time)", len(errs))
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Register took %v — caller's 200ms ctx deadline didn't bound it", elapsed)
	}
}

// NewManager drops disabled entries and ones with neither command nor URL so
// they can't later panic; a URL-only (remote) entry is kept.
func TestNewManagerSkipsInvalid(t *testing.T) {
	m := NewManager([]Server{
		{Name: "", Command: []string{"x"}},
		{Name: "y", Command: nil},
		{Name: "off", Command: []string{"x"}, Disabled: true},
		{Name: "remote", URL: "http://localhost:1/mcp"},
		{Name: "ok", Command: []string{"x"}},
	})
	if len(m.Clients()) != 2 {
		t.Fatalf("got %d clients, want 2 (remote + ok)", len(m.Clients()))
	}
}
