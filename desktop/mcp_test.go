package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// newMCPTestApp builds an App with a temp config dir so binding tests never touch
// the real ~/.config/aetox, using the noop provider so a re-bootstrap succeeds
// without an API key.
func newMCPTestApp(t *testing.T) *App {
	t.Helper()
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("LOCALAPPDATA", base)
	return &App{cfg: config.Config{ModelProvider: "noop", SandboxRoot: t.TempDir()}}
}

func TestAddListRemoveMCPServer(t *testing.T) {
	a := newMCPTestApp(t)

	if err := a.AddMCPServer("fs", []string{"npx", "server-filesystem"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	servers := a.ListMCPServers()
	if len(servers) != 1 || servers[0].Name != "fs" {
		t.Fatalf("list after add = %+v", servers)
	}
	if len(servers[0].Command) != 2 {
		t.Fatalf("command not persisted: %+v", servers[0])
	}

	// Duplicate name is rejected.
	if err := a.AddMCPServer("fs", []string{"echo"}); err == nil {
		t.Fatal("expected duplicate-name error")
	}

	if err := a.RemoveMCPServer("fs"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := a.ListMCPServers(); len(got) != 0 {
		t.Fatalf("expected empty after remove, got %+v", got)
	}
}

func TestSaveToggleMCPServer(t *testing.T) {
	a := newMCPTestApp(t)

	if err := a.AddMCPServer("fs", []string{"npx", "server-filesystem"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Toggle off: persisted, reported as disabled, and no client is built.
	if err := a.ToggleMCPServer("fs", true); err != nil {
		t.Fatalf("toggle off: %v", err)
	}
	servers := a.ListMCPServers()
	if !servers[0].Disabled || servers[0].Status != "disabled" {
		t.Fatalf("after toggle off: %+v", servers[0])
	}
	if a.findMCPClient("fs") != nil {
		t.Fatal("disabled server still has a live client")
	}

	// Update in place (rename + new env) keeps the disabled state.
	err := a.SaveMCPServer("fs", config.MCPServerConfig{
		Name:        "files",
		Command:     []string{"npx", "-y", "server-filesystem"},
		Environment: map[string]string{"TOKEN": "x", " ": "dropped"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	servers = a.ListMCPServers()
	if len(servers) != 1 || servers[0].Name != "files" || !servers[0].Disabled {
		t.Fatalf("after update: %+v", servers)
	}
	if len(servers[0].Environment) != 1 || servers[0].Environment["TOKEN"] != "x" {
		t.Fatalf("environment not cleaned/persisted: %+v", servers[0].Environment)
	}

	// A remote (URL-only) server is valid; renaming onto an existing name isn't.
	if err := a.SaveMCPServer("", config.MCPServerConfig{Name: "exa", URL: "https://mcp.exa.ai/mcp"}); err != nil {
		t.Fatalf("add remote: %v", err)
	}
	if err := a.SaveMCPServer("exa", config.MCPServerConfig{Name: "files", URL: "https://x"}); err == nil {
		t.Fatal("expected duplicate-name error on rename collision")
	}
	if err := a.ToggleMCPServer("missing", true); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestAddMCPServerValidation(t *testing.T) {
	a := newMCPTestApp(t)

	if err := a.AddMCPServer("", []string{"x"}); err == nil {
		t.Fatal("expected error for empty name")
	}
	if err := a.AddMCPServer("x", nil); err == nil {
		t.Fatal("expected error for empty command")
	}
	if err := a.AddMCPServer("x", []string{"  ", ""}); err == nil {
		t.Fatal("expected error for all-whitespace command")
	}
}

// ListSkills must exclude embedded built-ins — the Tools panel is only for
// user-added tools (MCP / discovered). With none configured, it's empty even
// though the engine has its 12 built-ins registered.
func TestListSkillsExcludesBuiltins(t *testing.T) {
	a := newMCPTestApp(t)
	a.applyConfig(a.cfg) // bootstrap so a.registry is populated with built-ins

	for _, s := range a.ListSkills() {
		if s.Source == "builtin" {
			t.Fatalf("built-in %q leaked into ListSkills; panel should only show added tools", s.Name)
		}
	}
}

// End-to-end backend detection: adding a real MCP server makes its tool show up
// in ListSkills as source "mcp" — proving the panel is driven by what the
// backend actually connected to, not a hardcoded list.
func TestListSkillsSurfacesMCPTool(t *testing.T) {
	a := newMCPTestApp(t)
	bin := buildEchoServer(t)

	if err := a.AddMCPServer("echo", []string{bin}); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Kill the server subprocess before TempDir cleanup, else Windows can't
	// delete the still-open echoserver.exe.
	t.Cleanup(func() {
		if a.mcp != nil {
			a.mcp.Close()
		}
	})

	// MCP tools now register on a background goroutine (see applyConfig) so
	// startup isn't blocked on a cold connect — poll until the echo server's
	// tool surfaces instead of reading once.
	var found *SkillInfo
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range a.ListSkills() {
			if s.Source == "mcp" {
				sc := s
				found = &sc
				break
			}
		}
		if found != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if found == nil {
		t.Fatalf("no mcp-sourced tool in ListSkills after 15s; got %+v", a.ListSkills())
	}
	if found.Name != "echo_echo" {
		t.Fatalf("mcp tool name = %q, want echo_echo", found.Name)
	}
}

// buildEchoServer compiles the internal/mcp testdata MCP server to a temp
// binary so the desktop layer can drive a real end-to-end connection.
func buildEchoServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "echoserver")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", bin,
		"github.com/Mike0165115321/Aetox/internal/mcp/testdata/echoserver").CombinedOutput()
	if err != nil {
		t.Fatalf("build echoserver: %v\n%s", err, out)
	}
	return bin
}
