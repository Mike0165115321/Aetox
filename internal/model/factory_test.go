package model

import "testing"

func TestNewProviderDefaultsToNoop(t *testing.T) {
	p, err := NewProvider(ProviderOptions{})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if p == nil {
		t.Fatal("provider is nil")
	}
	if p.Name() != "noop" {
		t.Fatalf("expected provider noop, got %s", p.Name())
	}
}

func TestNewProviderUnknownProvider(t *testing.T) {
	_, err := NewProvider(ProviderOptions{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNewProviderOpenRouterMissingAPIKey(t *testing.T) {
	_, err := NewProvider(ProviderOptions{
		Provider: "openrouter",
		Model:    "my-model",
		APIKey:   "",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewProviderOpenRouterMissingModel(t *testing.T) {
	_, err := NewProvider(ProviderOptions{
		Provider: "openrouter",
		APIKey:   "api-key",
		Model:    "",
	})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}
