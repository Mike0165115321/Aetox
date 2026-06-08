package config

import (
	"os"
	"path/filepath"
	"testing"
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
	if cfg.ThinkLevel != "medium" {
		t.Fatalf("expected default think level medium, got %q", cfg.ThinkLevel)
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
	if cfg.ThinkLevel != "medium" {
		t.Fatalf("expected fallback think level medium, got %q", cfg.ThinkLevel)
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
