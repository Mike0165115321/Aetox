package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// Manager owns the lifetime of the configured MCP client connections. Build it
// once and keep it: connections are cached per client, so re-registering on an
// engine re-bootstrap (e.g. a model switch) reuses the live session instead of
// respawning every server.
type Manager struct {
	clients []*Client

	// done names the servers whose tools are already in the registry. A
	// registry refuses a duplicate name, so without this a second dispatch to
	// the same agent would produce one error per tool the server carries — and
	// two dispatches landing together would race for the same names.
	mu   sync.Mutex
	done map[string]bool
}

// NewManager creates clients for each server config, skipping disabled
// entries and ones with neither a command nor a URL. Nothing connects yet —
// connection is lazy (see Client).
func NewManager(servers []Server) *Manager {
	clients := make([]*Client, 0, len(servers))
	for _, s := range servers {
		if s.Name == "" || s.Disabled || (len(s.Command) == 0 && s.URL == "") {
			continue
		}
		clients = append(clients, New(s))
	}
	return &Manager{clients: clients, done: map[string]bool{}}
}

// Clients exposes the underlying clients (for status display / UI bindings).
func (m *Manager) Clients() []*Client {
	if m == nil {
		return nil
	}
	return m.clients
}

// PermissionRules returns the default "ask" rule for every configured server —
// derived from server names alone, with no connection. This lets the safety
// gate be installed synchronously at bootstrap (MCP tools never auto-run) even
// though tool registration itself is deferred to a background connect. Keep in
// sync with the rule Register attaches per server.
func (m *Manager) PermissionRules() []safety.PermissionRule {
	if m == nil {
		return nil
	}
	rules := make([]safety.PermissionRule, 0, len(m.clients))
	for _, c := range m.clients {
		rules = append(rules, safety.PermissionRule{
			Tool:    sanitize(c.Name()) + "_*",
			Pattern: "*",
			Action:  safety.PermissionAsk,
			// The app's opening position, not the user's decision — see
			// safety.PermissionRule.Default. Both places that build this rule
			// must set it; a copy without the flag is a copy that quietly
			// outranks the approval mode again.
			Default: true,
		})
	}
	return rules
}

// Register connects each server (lazily, cached) and registers every tool it
// exposes into registry as SourceExternal. It returns one default "ask"
// permission rule per server so MCP tools never auto-run before the user has
// decided anything.
//
// "Even under full-access" is what that line used to say, and it was the bug:
// a mode the user explicitly picked, on a card reading "รับทุกอย่างโดยไม่ถาม",
// was being overruled by a rule the app wrote for them. The rules are marked
// Default now and yield to the mode; a rule the *user* writes still wins. A server that fails to
// connect, or hasn't by the time ctx expires, is skipped and its error
// collected; the agent loop is unaffected.
//
// Connections run concurrently, not one after another: this is called
// synchronously during app startup (before the dispatcher snapshots the
// registry — see desktop/app.go, cmd/aetox/main.go), and a server like
// `npx -y pkg@latest` resolving/downloading on a cold cache can be slow.
// Sequentially, N slow servers meant N times the wait.
//
// ctx is enforced here, not just handed to the SDK: the go-sdk's
// Client.Connect/initialize handshake does not reliably abort on context
// cancellation while a subprocess has written nothing yet (confirmed against
// go-sdk v1.6.1 — CommandTransport.Connect doesn't even look at ctx, and the
// initialize round-trip's wait isn't guaranteed to select on ctx.Done()
// either). So a slow client's own goroutine can keep running past the
// deadline; Register itself still returns on time by racing a buffered
// channel against ctx.Done() rather than waiting on every goroutine. The
// buffer means an abandoned goroutine's eventual send never blocks it from
// exiting once its underlying call does return.
func (m *Manager) Register(ctx context.Context, registry *skill.Registry) ([]safety.PermissionRule, []error) {
	if m == nil || registry == nil {
		return nil, nil
	}
	type result struct {
		name  string
		tools []skill.Tool
		rule  safety.PermissionRule
		err   error
	}
	// Deferred servers are not connected here — see Server.Deferred. They come
	// up through RegisterFor when the agent that needs one is started.
	eager := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		if !c.Deferred() {
			eager = append(eager, c)
		}
	}
	resultsCh := make(chan result, len(eager))
	for _, c := range eager {
		go func(c *Client) {
			tools, err := c.SkillTools(ctx)
			if err != nil {
				resultsCh <- result{err: fmt.Errorf("mcp server %q: %w", c.Name(), err)}
				return
			}
			resultsCh <- result{
				name:  c.Name(),
				tools: tools,
				rule: safety.PermissionRule{
					Tool:    sanitize(c.Name()) + "_*",
					Pattern: "*",
					Action:  safety.PermissionAsk,
					// The app's opening position, not the user's decision — so
					// it yields to whichever approval mode they picked. See
					// safety.PermissionRule.Default.
					Default: true,
				},
			}
		}(c)
	}

	var rules []safety.PermissionRule
	var errs []error
	for remaining := len(eager); remaining > 0; {
		select {
		case res := <-resultsCh:
			remaining--
			if res.err != nil {
				errs = append(errs, res.err)
				continue
			}
			errs = append(errs, m.attach(registry, res.name, res.tools)...)
			rules = append(rules, res.rule)
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("mcp registration: %w (%d server(s) still connecting)", ctx.Err(), remaining))
			return rules, errs
		}
	}
	return rules, errs
}

// attach puts one server's tools in the registry, once. Re-registering a name
// the registry already holds is an error per tool, so a server that came up on
// an earlier dispatch is skipped rather than re-added.
func (m *Manager) attach(registry *skill.Registry, name string, tools []skill.Tool) []error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		m.done = map[string]bool{}
	}
	if m.done[name] {
		return nil
	}
	var errs []error
	for _, t := range tools {
		if err := registry.Register(t, skill.SourceMCP); err != nil {
			errs = append(errs, err)
		}
	}
	m.done[name] = true
	return errs
}

// RegisterFor brings up the named servers and puts their tools in the registry,
// for the case Register deliberately left out: a server no desk carries, which
// waits for the agent that does (Server.Deferred).
//
// Called on the path that starts such an agent, so the cost of connecting is
// paid by the job that needs it rather than by every launch. Names that are
// unknown, already connected, or not deferred are no-ops — the caller passes
// the agent's whole `for:` list rather than working out which of it is still
// outstanding, because a caller that had to work that out is a second copy of
// the rule this method exists to hold.
//
// Synchronous on purpose. The dispatch behind it is about to hand the agent a
// tool list, and an agent that starts before its server has finished connecting
// is the failure missingAgentServers exists to report — better to wait here,
// where the wait is attributable to the job that asked for it.
func (m *Manager) RegisterFor(ctx context.Context, registry *skill.Registry, names []string) []error {
	if m == nil || registry == nil || len(names) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(names))
	for _, n := range names {
		wanted[strings.ToLower(strings.TrimSpace(n))] = true
	}
	var errs []error
	for _, c := range m.clients {
		if !c.Deferred() || !wanted[strings.ToLower(c.Name())] {
			continue
		}
		m.mu.Lock()
		already := m.done[c.Name()]
		m.mu.Unlock()
		if already {
			continue
		}
		tools, err := c.SkillTools(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("mcp server %q: %w", c.Name(), err))
			continue
		}
		errs = append(errs, m.attach(registry, c.Name(), tools)...)
	}
	return errs
}

// Close shuts down every connected server subprocess.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	var firstErr error
	for _, c := range m.clients {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
