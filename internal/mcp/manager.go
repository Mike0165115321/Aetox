package mcp

import (
	"context"
	"fmt"

	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// Manager owns the lifetime of the configured MCP client connections. Build it
// once and keep it: connections are cached per client, so re-registering on an
// engine re-bootstrap (e.g. a model switch) reuses the live session instead of
// respawning every server.
type Manager struct {
	clients []*Client
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
		})
	}
	return rules
}

// Register connects each server (lazily, cached) and registers every tool it
// exposes into registry as SourceExternal. It returns one default "ask"
// permission rule per server so MCP tools never auto-run — even under
// full-access — unless the user explicitly allows them. A server that fails to
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
		tools []skill.Tool
		rule  safety.PermissionRule
		err   error
	}
	resultsCh := make(chan result, len(m.clients))
	for _, c := range m.clients {
		go func(c *Client) {
			tools, err := c.SkillTools(ctx)
			if err != nil {
				resultsCh <- result{err: fmt.Errorf("mcp server %q: %w", c.Name(), err)}
				return
			}
			resultsCh <- result{
				tools: tools,
				rule: safety.PermissionRule{
					Tool:    sanitize(c.Name()) + "_*",
					Pattern: "*",
					Action:  safety.PermissionAsk,
				},
			}
		}(c)
	}

	var rules []safety.PermissionRule
	var errs []error
	for remaining := len(m.clients); remaining > 0; {
		select {
		case res := <-resultsCh:
			remaining--
			if res.err != nil {
				errs = append(errs, res.err)
				continue
			}
			for _, t := range res.tools {
				if regErr := registry.Register(t, skill.SourceMCP); regErr != nil {
					errs = append(errs, regErr)
				}
			}
			rules = append(rules, res.rule)
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("mcp registration: %w (%d server(s) still connecting)", ctx.Err(), remaining))
			return rules, errs
		}
	}
	return rules, errs
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
