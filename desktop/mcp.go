package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// SkillInfo is one tool the AI can currently call, for the Tools panel.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"` // builtin | external | mcp
}

// ListSkills returns the user-added tools the AI can use — MCP tools and
// discovered skills — sorted by name. Built-in tools are embedded in the engine
// and intentionally excluded: this decision lives here in the backend (which
// owns each tool's Source), not in the frontend, so the panel just renders what
// it receives.
func (a *App) ListSkills() []SkillInfo {
	if a.registry == nil {
		return nil
	}
	names := a.registry.Names()
	sort.Strings(names)
	out := make([]SkillInfo, 0, len(names))
	for _, n := range names {
		s, ok := a.registry.Get(n)
		if !ok || s == nil {
			continue
		}
		src, _ := a.registry.SourceOf(n)
		if src == skill.SourceBuiltin {
			continue
		}
		out = append(out, SkillInfo{Name: n, Description: s.Description(), Source: string(src)})
	}
	return out
}

// MCPServerInfo is one configured server plus its live connection status, for
// the Settings UI.
type MCPServerInfo struct {
	Name        string            `json:"name"`
	Command     []string          `json:"command,omitempty"`
	URL         string            `json:"url,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Disabled    bool              `json:"disabled"`
	Status      string            `json:"status"` // idle | connected | failed | disabled
	Tools       int               `json:"tools"`  // tools seen on the last successful connect
	Err         string            `json:"err,omitempty"`
}

// ListMCPServers returns the persisted servers with live status from the active
// manager overlaid by name (idle if a server isn't in the manager yet; disabled
// servers have no client at all and report status "disabled").
func (a *App) ListMCPServers() []MCPServerInfo {
	servers, err := config.LoadMCPServers()
	if err != nil {
		debuglog.Msg("mcp: load servers: %v", err)
	}
	out := make([]MCPServerInfo, 0, len(servers))
	for _, s := range servers {
		info := MCPServerInfo{
			Name:        s.Name,
			Command:     s.Command,
			URL:         s.URL,
			Environment: s.Environment,
			Headers:     s.Headers,
			Disabled:    s.Disabled,
			Status:      string(mcp.StatusIdle),
		}
		if s.Disabled {
			info.Status = "disabled"
		} else if c := a.findMCPClient(s.Name); c != nil {
			info.Status = string(c.Status())
			info.Tools = c.ToolCount()
			if e := c.Err(); e != nil {
				info.Err = e.Error()
			}
		}
		out = append(out, info)
	}
	return out
}

// AddMCPServer persists a new local stdio server (name + argv). Kept as the
// simple path the tests and quick-add flows use; SaveMCPServer below is the
// full-field variant.
func (a *App) AddMCPServer(name string, command []string) error {
	return a.SaveMCPServer("", config.MCPServerConfig{Name: name, Command: command})
}

// SaveMCPServer validates and persists one server, then rebuilds the engine so
// the change takes effect immediately. originalName == "" adds a new server;
// otherwise the entry with that name is updated in place (rename allowed, its
// enabled/disabled state preserved — toggling is ToggleMCPServer's job).
func (a *App) SaveMCPServer(originalName string, server config.MCPServerConfig) error {
	server.Name = strings.TrimSpace(server.Name)
	if server.Name == "" {
		return fmt.Errorf("server name is required")
	}
	server.Command = trimArgs(server.Command)
	server.URL = strings.TrimSpace(server.URL)
	if len(server.Command) == 0 && server.URL == "" {
		return fmt.Errorf("command or url is required")
	}
	server.Environment = trimMap(server.Environment)
	server.Headers = trimMap(server.Headers)

	servers, err := config.LoadMCPServers()
	if err != nil {
		return err
	}

	originalName = strings.TrimSpace(originalName)
	target := -1
	for i, s := range servers {
		if originalName != "" && strings.EqualFold(s.Name, originalName) {
			target = i
			continue
		}
		if strings.EqualFold(s.Name, server.Name) {
			return fmt.Errorf("a server named %q already exists", server.Name)
		}
	}
	if originalName == "" {
		servers = append(servers, server)
	} else if target == -1 {
		return fmt.Errorf("server %q not found", originalName)
	} else {
		server.Disabled = servers[target].Disabled
		servers[target] = server
	}

	if err := config.SaveMCPServers(servers); err != nil {
		return err
	}
	a.rebuildMCP()
	return nil
}

// ToggleMCPServer flips one server's disabled flag and rebuilds the engine, so
// switching a server off tears its subprocess down (and on reconnects it)
// without losing its configuration.
func (a *App) ToggleMCPServer(name string, disabled bool) error {
	servers, err := config.LoadMCPServers()
	if err != nil {
		return err
	}
	for i := range servers {
		if strings.EqualFold(servers[i].Name, name) {
			servers[i].Disabled = disabled
			if err := config.SaveMCPServers(servers); err != nil {
				return err
			}
			a.rebuildMCP()
			return nil
		}
	}
	return fmt.Errorf("server %q not found", name)
}

// RemoveMCPServer deletes a server by name and rebuilds the engine.
func (a *App) RemoveMCPServer(name string) error {
	servers, err := config.LoadMCPServers()
	if err != nil {
		return err
	}
	kept := servers[:0]
	for _, s := range servers {
		if !strings.EqualFold(s.Name, name) {
			kept = append(kept, s)
		}
	}
	if err := config.SaveMCPServers(kept); err != nil {
		return err
	}
	a.rebuildMCP()
	return nil
}

// TestMCPServer forces a fresh connection attempt (closing any cached failure)
// and reports the resulting status, so the user can retry a server they just
// fixed without restarting the app.
func (a *App) TestMCPServer(name string) MCPServerInfo {
	c := a.findMCPClient(name)
	if c == nil {
		return MCPServerInfo{Name: name, Status: string(mcp.StatusFailed), Err: "server not found"}
	}
	c.Close() // drop any sticky failure so ensure retries
	info := MCPServerInfo{Name: name, Command: c.Command(), Status: string(mcp.StatusConnected)}
	if _, err := c.Tools(context.Background()); err != nil {
		info.Status = string(mcp.StatusFailed)
		info.Err = err.Error()
	} else {
		info.Tools = c.ToolCount()
	}
	return info
}

func (a *App) findMCPClient(name string) *mcp.Client {
	if a.mcp == nil {
		return nil
	}
	for _, c := range a.mcp.Clients() {
		if strings.EqualFold(c.Name(), name) {
			return c
		}
	}
	return nil
}

// rebuildMCP closes the current manager and re-bootstraps from the saved config
// so added/removed servers take effect in the tool registry immediately.
func (a *App) rebuildMCP() {
	if a.mcp != nil {
		_ = a.mcp.Close()
		a.mcp = nil
	}
	a.applyConfig(a.cfg)
}

func trimArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// trimMap drops entries with blank keys and trims both sides; returns nil for
// an effectively-empty map so it stays omitted from the saved JSON.
func trimMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		if k = strings.TrimSpace(k); k != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
