package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// ToolCounts is the "what can the agent do right now" readout the composer
// palette shows in one line. Derived from the live registry rather than a
// written-down number, so it cannot rot the way a docs count does.
type ToolCounts struct {
	Builtin   int `json:"builtin"`   // compiled in, always on
	Workbench int `json:"workbench"` // desktop-only tools (browser, ask_user, todo_write)
	MCP       int `json:"mcp"`       // bridged from configured MCP servers
	Skill     int `json:"skill"`     // SKILL.md documents the user added — not tools
}

func (a *App) ToolCounts() ToolCounts {
	var counts ToolCounts
	if a.registry == nil {
		return counts
	}
	for _, name := range a.registry.Names() {
		switch src, _ := a.registry.SourceOf(name); src {
		case skill.SourceBuiltin:
			counts.Builtin++
		case skill.SourceWorkbench:
			counts.Workbench++
		case skill.SourceMCP:
			counts.MCP++
		case skill.SourceSkill:
			counts.Skill++
		}
	}
	return counts
}

// SkillInfo is one entry in the registry, tool or skill, for the UI lists.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"` // builtin | workbench | mcp | skill
	// Category is what the tool is *for* (internal/skill/category.go). Source
	// says where a tool came from, which answers a question nobody asks; this
	// says what it lets the assistant do, which is the only thing a person
	// deciding can act on. Both are sent because the page still marks what the
	// user installed themselves.
	Category string `json:"category"`
}

// ListTools returns everything the AI can actually run — the compiled-in tools,
// the desktop's own, and anything bridged from an MCP server — sorted by name,
// each carrying the source so the UI can group them.
//
// Skills are deliberately not in here. A skill is a document that tells the AI
// how to do something; a tool is a thing it runs. They used to share one list
// and one word, which is how ask_user and the browser tools ended up shown to
// the user as add-ons they had installed.
func (a *App) ListTools() []SkillInfo {
	return a.registryEntries(func(src skill.Source) bool { return src != skill.SourceSkill })
}

// ListSkills returns only the SKILL.md documents the user added.
func (a *App) ListSkills() []SkillInfo {
	return a.registryEntries(func(src skill.Source) bool { return src == skill.SourceSkill })
}

func (a *App) registryEntries(keep func(skill.Source) bool) []SkillInfo {
	if a.registry == nil {
		return []SkillInfo{} // never nil: §34, a nil slice crashes the frontend
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
		if !keep(src) {
			continue
		}
		out = append(out, SkillInfo{
			Name: n, Description: s.Description(), Source: string(src),
			Category: skill.CategoryOf(n),
		})
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
	// Cwd and TimeoutMs round-trip through the settings form. They were in the
	// stored config from the start but not in this shape, so the form could not
	// show them and editing a server silently dropped whatever was set.
	Cwd       string `json:"cwd,omitempty"`
	TimeoutMs int    `json:"timeoutMs,omitempty"`
	Disabled  bool   `json:"disabled"`
	// For is who carries this server's tools — desk names, and "agent:<name>"
	// for one of the team. Empty means connected and attached nowhere, which is
	// a real state and not the same as Disabled (never connected at all).
	For    []string `json:"for"`
	Status string   `json:"status"` // idle | connected | failed | disabled
	Tools  int      `json:"tools"`  // tools seen on the last successful connect
	Err    string   `json:"err,omitempty"`
}

// PlacementTarget is one place something bolted onto the outside of the app can
// be switched on, for the settings page to render as a row of toggles. Built
// from the desks and the team that actually exist rather than a list typed into
// the page, so hiring an agent puts it on this list without anyone remembering
// to add it.
//
// Not MCPTarget any more: the connections page places accounts against the very
// same list, with the very same ids. Two names for one answer is how the two
// pages would eventually come to disagree about what a desk is called.
type PlacementTarget struct {
	// ID is what goes in a `for` list: a desk name, or "agent:<name>".
	ID   string `json:"id"`
	Name string `json:"name"` // what to show
	Kind string `json:"kind"` // "desk" | "agent"
}

// PlacementTargets lists everywhere a server or a connection can be pointed.
// The page shows one toggle per entry; SetMCPServerTargets and
// SetConnectionTargets are where a flip lands.
func (a *App) PlacementTargets() []PlacementTarget {
	var out []PlacementTarget
	for _, m := range mode.List() {
		name := m.Description
		if name == "" {
			name = m.Name
		}
		out = append(out, PlacementTarget{ID: m.Name, Name: name, Kind: "desk"})
	}
	for _, p := range subagent.List() {
		if p.Desk == "" || p.Invalid != "" {
			continue // the helpers are part of the system; a sick file is not a target
		}
		out = append(out, PlacementTarget{ID: config.MCPAgentPrefix + p.Name, Name: p.Name, Kind: "agent"})
	}
	return out
}

// SetMCPServerTargets replaces one server's `for` list and rebuilds the engine,
// so a toggle takes effect on the next turn rather than the next launch.
//
// Separate from SaveMCPServer for the same reason ToggleMCPServer is: switching
// where a server shows up is one click on a row, and routing it through the
// full-form save would make the page send back every field it happens to be
// holding — which is how an edit elsewhere in the form rides along unnoticed.
func (a *App) SetMCPServerTargets(name string, targets []string) error {
	servers, err := config.LoadMCPServers()
	if err != nil {
		return err
	}
	clean := make([]string, 0, len(targets))
	for _, t := range targets {
		if t = strings.TrimSpace(t); t != "" && !slices.Contains(clean, t) {
			clean = append(clean, t)
		}
	}
	for i := range servers {
		if !strings.EqualFold(servers[i].Name, name) {
			continue
		}
		// Non-nil even when empty: an explicit "attached nowhere" has to
		// survive, and the migration reads absence to mean "never set".
		if clean == nil {
			clean = []string{}
		}
		servers[i].For = clean
		if err := config.SaveMCPServers(servers); err != nil {
			return err
		}
		a.rebuildMCP()
		return nil
	}
	return fmt.Errorf("server %q not found", name)
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
			Cwd:         s.Cwd,
			TimeoutMs:   s.TimeoutMs,
			Disabled:    s.Disabled,
			For:         s.For,
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

// MCPConfigPath is the file the servers are persisted to, for the page to show.
//
// Read from config rather than written into the UI's strings, for the same
// reason SkillsDir is: a path the page states on its own authority is a path
// that can drift from the one actually used, which is exactly what had
// happened on the Skills page.
func (a *App) MCPConfigPath() string {
	path, err := config.MCPServersPath()
	if err != nil {
		return ""
	}
	return path
}

// OpenMCPFolder reveals the folder holding mcp-servers.json, so a server that
// will not connect can be inspected or backed up by hand — the same affordance
// the prompts, sub-agents and skills pages have.
func (a *App) OpenMCPFolder() error {
	path, err := config.MCPServersPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return openInFileManager(dir)
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
		// Both switches survive a form save: which desks and agents see this
		// server is set by SetMCPServerTargets, and being disabled by
		// ToggleMCPServer. Neither is on the edit form, so taking them from
		// the payload would reset them to whatever the form defaulted to.
		server.Disabled = servers[target].Disabled
		server.For = servers[target].For
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
