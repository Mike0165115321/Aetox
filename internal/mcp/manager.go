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

	// mu serializes attach. A registry refuses a duplicate name, so two
	// registrations landing together would otherwise both find a tool absent
	// and both register it — the check and the write have to be one step.
	//
	// What it deliberately does NOT guard is a memory of what has been attached
	// already; see attach for why the registry is asked instead.
	mu sync.Mutex
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
	return &Manager{clients: clients}
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
			errs = append(errs, m.attach(registry, res.tools)...)
			rules = append(rules, res.rule)
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("mcp registration: %w (%d server(s) still connecting)", ctx.Err(), remaining))
			return rules, errs
		}
	}
	return rules, errs
}

// attach puts one server's tools in the registry, once per registry — and the
// last three words are the whole of this function.
//
// It used to be once per *manager*: a `done[server]` map, set by whoever
// attached first. But a Manager is built once and survives every re-bootstrap
// by design, while a registry is rebuilt by each one — so the first registry to
// ask got the tools and every later one was handed nothing, with no error to
// say so. The desktop bootstraps twice on startup (focusNone, then
// openAtRememberedDesk), which meant the tools landed in the throwaway registry
// on every single launch and no MCP tool was ever reachable in a session; the
// same flag then made a mid-session model switch lose them for good. What the
// user saw was every specialist agent refusing to start, told to "try again in
// a moment" by a check that could never come true (subagent.missingAgentServers).
//
// So the registry is the record now, and there is no second place left to
// disagree with it: a tool it already holds is skipped — that is the second
// dispatch to the same agent, which the flag was really there for — and one it
// does not hold is registered, whatever some earlier registry received.
func (m *Manager) attach(registry *skill.Registry, tools []skill.Tool) []error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var errs []error
	for _, t := range tools {
		if _, held := registry.Get(t.Name()); held {
			continue
		}
		if err := registry.Register(t, skill.SourceMCP); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// holds reports whether this registry already carries the server's tools.
//
// Asked of the registry rather than remembered, for the reason attach gives:
// a fresh registry has to be able to answer for itself instead of inheriting
// the last one's answer. Same question subagent.missingAgentServers asks before
// it starts an agent, so the two cannot end up disagreeing about whether a
// server's tools are there.
func holds(registry *skill.Registry, server string) bool {
	for name := range registry.Snapshot() {
		if source, ok := registry.SourceOf(name); ok && source == skill.SourceMCP && ToolBelongsTo(name, []string{server}) {
			return true
		}
	}
	return false
}

// RegisterFor makes sure the named servers' tools are in THIS registry, now,
// and connects whatever is not up yet to do it.
//
// Called on the two paths that are about to cut a registry down for one agent —
// opening a chat with it (bootstrap.Engine) and handing it a job (task's
// EnsureServers) — because both take a *copy*, and a copy is only as good as
// what was in the registry the instant it was taken. Synchronous on purpose:
// better to wait here, where the wait belongs to the agent that asked for it,
// than to hand it a tool list with a hole in it.
//
// It used to skip anything not `Deferred`, on the reasoning that an eager server
// was Register's business and would already be there. That reasoning had an
// ordering in it that nothing enforced. Opening a chair chat re-bootstraps the
// engine, and a re-bootstrap builds a NEW registry whose background Register has
// not started yet — measured at 5ms behind, deterministically, every time — so
// the cut was taken from a registry that had no MCP tools in it at all. The
// agent then reported, correctly and uselessly, that it had no such tool, while
// the settings page showed the server connected and switched on for it. Nothing
// was racing; the eager path simply had no one waiting for it.
//
// So `Deferred` is not consulted here. It is a *startup* policy — who pays to
// spawn this at launch — and it has no business answering "is this registry
// furnished yet". `holds` answers that, by asking the registry. A server already
// present is a no-op, and one that is up but absent from this registry costs a
// cached round-trip, not a spawn. Names that are unknown are no-ops too: the
// caller passes the agent's whole `for:` list rather than working out which part
// of it is outstanding, because a caller that had to work that out would be a
// second copy of the rule this method exists to hold.
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
		if !wanted[strings.ToLower(c.Name())] {
			continue
		}
		// Already in THIS registry — not merely somewhere once. A caller asking
		// after a re-bootstrap is asking on behalf of a registry that has never
		// had these tools, and it has to be able to get them.
		if holds(registry, c.Name()) {
			continue
		}
		tools, err := c.SkillTools(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("mcp server %q: %w", c.Name(), err))
			continue
		}
		errs = append(errs, m.attach(registry, tools)...)
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
