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
	Name    string   `json:"name"`
	Command []string `json:"command"`
	Status  string   `json:"status"` // idle | connected | failed
	Err     string   `json:"err,omitempty"`
}

// ListMCPServers returns the persisted servers with live status from the active
// manager overlaid by name (idle if a server isn't in the manager yet).
func (a *App) ListMCPServers() []MCPServerInfo {
	servers, err := config.LoadMCPServers()
	if err != nil {
		debuglog.Msg("mcp: load servers: %v", err)
	}
	out := make([]MCPServerInfo, 0, len(servers))
	for _, s := range servers {
		info := MCPServerInfo{Name: s.Name, Command: s.Command, Status: string(mcp.StatusIdle)}
		if c := a.findMCPClient(s.Name); c != nil {
			info.Status = string(c.Status())
			if e := c.Err(); e != nil {
				info.Err = e.Error()
			}
		}
		out = append(out, info)
	}
	return out
}

// AddMCPServer validates and persists a new local server, then rebuilds the
// engine so its tools register immediately. Command is argv (argv0 + args).
func (a *App) AddMCPServer(name string, command []string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("server name is required")
	}
	command = trimArgs(command)
	if len(command) == 0 {
		return fmt.Errorf("command is required")
	}

	servers, err := config.LoadMCPServers()
	if err != nil {
		return err
	}
	for _, s := range servers {
		if strings.EqualFold(s.Name, name) {
			return fmt.Errorf("a server named %q already exists", name)
		}
	}
	servers = append(servers, config.MCPServerConfig{Name: name, Command: command})
	if err := config.SaveMCPServers(servers); err != nil {
		return err
	}
	a.rebuildMCP()
	return nil
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
