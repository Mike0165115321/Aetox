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

// NewManager creates clients for each server config, skipping entries with no
// name or command. Nothing connects yet — connection is lazy (see Client).
func NewManager(servers []Server) *Manager {
	clients := make([]*Client, 0, len(servers))
	for _, s := range servers {
		if s.Name == "" || len(s.Command) == 0 {
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

// Register connects each server (lazily, cached) and registers every tool it
// exposes into registry as SourceExternal. It returns one default "ask"
// permission rule per server so MCP tools never auto-run — even under
// full-access — unless the user explicitly allows them. A server that fails to
// connect is skipped and its error collected; the agent loop is unaffected.
func (m *Manager) Register(ctx context.Context, registry *skill.Registry) ([]safety.PermissionRule, []error) {
	if m == nil || registry == nil {
		return nil, nil
	}
	var rules []safety.PermissionRule
	var errs []error
	for _, c := range m.clients {
		tools, err := c.SkillTools(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("mcp server %q: %w", c.Name(), err))
			continue
		}
		for _, t := range tools {
			if regErr := registry.Register(t, skill.SourceMCP); regErr != nil {
				errs = append(errs, regErr)
			}
		}
		rules = append(rules, safety.PermissionRule{
			Tool:    sanitize(c.Name()) + "_*",
			Pattern: "*",
			Action:  safety.PermissionAsk,
		})
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
