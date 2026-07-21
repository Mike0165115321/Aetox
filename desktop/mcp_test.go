package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

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

	var found *SkillInfo
	skills := a.ListSkills()
	for i := range skills {
		if skills[i].Source == "mcp" {
			found = &skills[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no mcp-sourced tool in ListSkills; got %+v", skills)
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
