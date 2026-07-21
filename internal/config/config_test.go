package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/safety"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "env-key")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	cfg := Load(ConfigOptions{
		ModelProvider: "openrouter",
	})

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		t.Fatalf("abs cwd failed: %v", err)
	}
	absRoot, err := filepath.Abs(cfg.SandboxRoot)
	if err != nil {
		t.Fatalf("abs root failed: %v", err)
	}

	if absRoot != absCwd {
		t.Fatalf("expected root fallback to cwd, got %q", cfg.SandboxRoot)
	}
	if cfg.MaxRetries != 2 {
		t.Fatalf("expected max retries 2, got %d", cfg.MaxRetries)
	}
	if cfg.MaxPlanRetries != 0 {
		t.Fatalf("expected max plan retries 0, got %d", cfg.MaxPlanRetries)
	}
	if cfg.ApprovalTimeoutSec != 60 {
		t.Fatalf("expected approval timeout 60, got %d", cfg.ApprovalTimeoutSec)
	}
	if cfg.ModelTimeoutSec != 30 {
		t.Fatalf("expected model timeout 30, got %d", cfg.ModelTimeoutSec)
	}
	if cfg.ModelProvider != "openrouter" {
		t.Fatalf("expected model provider openrouter, got %q", cfg.ModelProvider)
	}
	if cfg.ModelAPIKey != "env-key" {
		t.Fatalf("expected API key from env, got %q", cfg.ModelAPIKey)
	}
	if cfg.ThinkLevel != "low" {
		t.Fatalf("expected default think level low, got %q", cfg.ThinkLevel)
	}
}

func TestLoadInvalidValues(t *testing.T) {
	root := t.TempDir()

	cfg := Load(ConfigOptions{
		RootPath:        root,
		MaxRetries:      0,
		MaxPlanRetries:  -1,
		ApprovalTimeout: 0,
		ModelTimeout:    0,
		ModelProvider:   "",
	})

	if cfg.SandboxRoot != root {
		t.Fatalf("expected configured root %q, got %q", root, cfg.SandboxRoot)
	}
	if cfg.MaxRetries != 2 {
		t.Fatalf("expected fallback max retries 2, got %d", cfg.MaxRetries)
	}
	if cfg.MaxPlanRetries != 0 {
		t.Fatalf("expected fallback max plan retries 0, got %d", cfg.MaxPlanRetries)
	}
	if cfg.ApprovalTimeoutSec != 60 {
		t.Fatalf("expected fallback approval timeout 60, got %d", cfg.ApprovalTimeoutSec)
	}
	if cfg.ModelTimeoutSec != 30 {
		t.Fatalf("expected fallback model timeout 30, got %d", cfg.ModelTimeoutSec)
	}
	if cfg.ModelProvider != "noop" {
		t.Fatalf("expected model provider fallback noop, got %q", cfg.ModelProvider)
	}
	if cfg.ThinkLevel != "low" {
		t.Fatalf("expected fallback think level low, got %q", cfg.ThinkLevel)
	}
}

func TestSaveAndLoadModelPreferenceThinkLevel(t *testing.T) {
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("LOCALAPPDATA", base)

	want := ModelPreference{
		ModelProvider: "openrouter",
		ModelName:     "deepseek/deepseek-r1",
		ThinkLevel:    "high",
	}
	if err := SaveModelPreference(want); err != nil {
		t.Fatalf("save preference failed: %v", err)
	}

	got, ok, err := LoadModelPreference()
	if err != nil {
		t.Fatalf("load preference failed: %v", err)
	}
	if !ok {
		t.Fatal("expected saved preference to exist")
	}
	if got.ThinkLevel != want.ThinkLevel {
		t.Fatalf("expected think level %q, got %q", want.ThinkLevel, got.ThinkLevel)
	}
}

func TestLoadPermissionsMissingFileReturnsEmpty(t *testing.T) {
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("LOCALAPPDATA", base)

	got, err := LoadPermissions()
	if err != nil {
		t.Fatalf("load permissions failed: %v", err)
	}
	if len(got.Rules) != 0 {
		t.Fatalf("expected no rules when file is missing, got %v", got.Rules)
	}
}

func TestLoadMCPServersMissingFileReturnsNil(t *testing.T) {
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("LOCALAPPDATA", base)

	got, err := LoadMCPServers()
	if err != nil {
		t.Fatalf("load mcp servers failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no servers when file is missing, got %v", got)
	}
}

func TestSaveAndLoadMCPServers(t *testing.T) {
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("LOCALAPPDATA", base)

	want := []MCPServerConfig{
		{Name: "fs", Command: []string{"npx", "-y", "server-filesystem", "/tmp"}, TimeoutMs: 5000},
		{Name: "git", Command: []string{"uvx", "mcp-git"}, Environment: map[string]string{"TOKEN": "x"}},
	}
	if err := SaveMCPServers(want); err != nil {
		t.Fatalf("save mcp servers failed: %v", err)
	}

	got, err := LoadMCPServers()
	if err != nil {
		t.Fatalf("load mcp servers failed: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d servers, got %d", len(want), len(got))
	}
	if got[0].Name != "fs" || len(got[0].Command) != 4 || got[0].TimeoutMs != 5000 {
		t.Fatalf("server 0 round-trip mismatch: %+v", got[0])
	}
	if got[1].Environment["TOKEN"] != "x" {
		t.Fatalf("server 1 environment not preserved: %+v", got[1])
	}
}

func TestSaveAndLoadPermissions(t *testing.T) {
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("LOCALAPPDATA", base)

	want := safety.PermissionConfig{Rules: []safety.PermissionRule{
		{Tool: "shell", Pattern: "rm *", Action: safety.PermissionDeny},
		{Tool: "git", Pattern: "status", Action: safety.PermissionAllow},
	}}
	if err := SavePermissions(want); err != nil {
		t.Fatalf("save permissions failed: %v", err)
	}

	got, err := LoadPermissions()
	if err != nil {
		t.Fatalf("load permissions failed: %v", err)
	}
	if len(got.Rules) != len(want.Rules) {
		t.Fatalf("expected %d rules, got %d", len(want.Rules), len(got.Rules))
	}
	for i, rule := range want.Rules {
		if got.Rules[i] != rule {
			t.Fatalf("rule %d mismatch: want %+v, got %+v", i, rule, got.Rules[i])
		}
	}
}
