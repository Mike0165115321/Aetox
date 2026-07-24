package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewProviderDefaultsToNoop(t *testing.T) {
	p, err := NewProvider(ProviderOptions{})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if p == nil {
		t.Fatal("provider is nil")
	}
	if p.Name() != "aetox" {
		t.Fatalf("expected provider aetox, got %s", p.Name())
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

func TestNewProviderAnthropic(t *testing.T) {
	p, err := NewProvider(ProviderOptions{
		Provider: "anthropic",
		Model:    "claude-haiku-4-5",
		APIKey:   "api-key",
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Fatalf("expected provider anthropic, got %s", p.Name())
	}
}

// DeepSeek is routed through the Anthropic wire format but must still report
// its own name so name-keyed logic (toolLoopMaxTokens, status) stays correct.
func TestNewProviderDeepSeekUsesAnthropicRuntimeKeepsName(t *testing.T) {
	p, err := NewProvider(ProviderOptions{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   "api-key",
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if _, ok := p.(*AnthropicProvider); !ok {
		t.Fatalf("deepseek must use the Anthropic runtime, got %T", p)
	}
	if p.Name() != "deepseek" {
		t.Fatalf("deepseek provider must report name deepseek, got %s", p.Name())
	}
}

// The user picks DeepSeek's wire format explicitly in Settings — this must
// actually switch runtimes and hit the OpenAI-compatible endpoint, not just
// change internal bookkeeping.
func TestNewProviderDeepSeekWireFormatOpenAICompatible(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"deepseek-v4-flash","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	p, err := NewProvider(ProviderOptions{
		Provider:   "deepseek",
		Model:      "deepseek-v4-flash",
		APIKey:     "api-key",
		BaseURL:    server.URL,
		WireFormat: "openai-compatible",
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if _, ok := p.(*OpenAICompatibleProvider); !ok {
		t.Fatalf("WireFormat=openai-compatible must select the OpenAI-compatible runtime, got %T", p)
	}
	if p.Name() != "deepseek" {
		t.Fatalf("deepseek provider must report name deepseek, got %s", p.Name())
	}
	if _, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if !hit {
		t.Fatal("request never reached the OpenAI-compatible endpoint")
	}
}

// An unrecognized WireFormat must not silently break the provider — it falls
// back to the catalog's default runtime (Anthropic, for DeepSeek).
func TestNewProviderDeepSeekUnknownWireFormatFallsBackToDefault(t *testing.T) {
	p, err := NewProvider(ProviderOptions{
		Provider:   "deepseek",
		Model:      "deepseek-v4-flash",
		APIKey:     "api-key",
		WireFormat: "bogus-format",
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if _, ok := p.(*AnthropicProvider); !ok {
		t.Fatalf("unknown WireFormat must fall back to the catalog default (anthropic), got %T", p)
	}
}

func TestNewProviderAnthropicMissingAPIKey(t *testing.T) {
	_, err := NewProvider(ProviderOptions{
		Provider: "anthropic",
		Model:    "claude-haiku-4-5",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

// Regression (401 invalid x-api-key): an empty BaseURL — the normal case,
// since persisted preferences strip the catalog-default URL — must resolve to
// DeepSeek's own Anthropic-format endpoint, never api.anthropic.com.
func TestNewProviderDeepSeekEmptyBaseURLStaysOnDeepSeek(t *testing.T) {
	p, err := NewProvider(ProviderOptions{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   "sk-test",
		BaseURL:  "", // persisted-preference normal case
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ap, ok := p.(*AnthropicProvider)
	if !ok {
		t.Fatalf("expected AnthropicProvider, got %T", p)
	}
	if ap.baseURL != DefaultBaseURL("deepseek") {
		t.Fatalf("empty BaseURL must default to the provider's own endpoint, got %q", ap.baseURL)
	}
}

// Regression: switching to the alt wire format while cfg still carries the
// default-format URL must swap to the alt endpoint — OpenAI-format requests
// aimed at the /anthropic endpoint just 404.
func TestNewProviderDeepSeekAltFormatReplacesDefaultURL(t *testing.T) {
	p, err := NewProvider(ProviderOptions{
		Provider:   "deepseek",
		Model:      "deepseek-v4-flash",
		APIKey:     "sk-test",
		BaseURL:    DefaultBaseURL("deepseek"), // stale default-format URL
		WireFormat: "openai-compatible",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*AnthropicProvider); ok {
		t.Fatal("alt wire format must not build the Anthropic client")
	}
}

// Symmetric regression: a stale alt-format URL combined with the default wire
// format must snap back to the default endpoint — Anthropic-format requests
// aimed at the plain OpenAI endpoint 404.
func TestNewProviderDeepSeekDefaultFormatReplacesStaleAltURL(t *testing.T) {
	p, err := NewProvider(ProviderOptions{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   "sk-test",
		BaseURL:  "https://api.deepseek.com", // stale alt-format URL
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ap, ok := p.(*AnthropicProvider)
	if !ok {
		t.Fatalf("expected AnthropicProvider, got %T", p)
	}
	if ap.baseURL != DefaultBaseURL("deepseek") {
		t.Fatalf("stale alt URL must snap back to the default endpoint, got %q", ap.baseURL)
	}
}
