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

// Kimi stays single-format on purpose, and this is the only place that says so
// — the catalog entry is deliberately left untouched, because the finding is
// about an endpoint we do NOT use and does not belong in the description of
// the one we do.
//
// Moonshot serves an Anthropic-format adapter at api.moonshot.ai/anthropic, so
// wiring it up as an AltRuntime looks free, and it is reachable: verified live
// 2026-08-20, /anthropic/v1/messages answers 401 invalid_authentication_error
// where /anthropic/messages answers 404, and the x-api-key header
// AnthropicProvider sends is read rather than ignored (no header at all answers
// a different error). Reachable is not the same as usable:
//
//   - Kimi's effort dial does not exist on that wire. Moonshot documents it as
//     the top-level OpenAI field reasoning_effort, which is what
//     resolveKimiThinkingCapabilities reads. The Anthropic wire has nowhere to
//     put it, so the picker would offer a ladder that sent nothing.
//   - An enabled thinking block is reported to 400 on multi-turn tool calls
//     whose prior tool_use carries no reasoning_content (MoonshotAI/Kimi-K2#129,
//     open and unanswered). That is not an edge case here, it is the agent loop.
//   - That adapter has no published contract at all — #129 is a request for one.
//   - It serves no model list: /anthropic/v1/models is a 404.
//
// DeepSeek's alt format earns its cost by escaping a measured defect (DSML tool
// calls leaking into plain text). Kimi's OpenAI path is the documented one and
// has no such defect, so an alt here would trade a working dial for an
// undocumented endpoint and buy nothing. A reachable second endpoint is not a
// reason on its own.
func TestKimiOffersNoAnthropicWireFormat(t *testing.T) {
	info, ok := LookupProviderInfo("kimi")
	if !ok {
		t.Fatal("kimi missing from the catalog")
	}
	if info.AltRuntime != "" || info.AltBaseURL != "" {
		t.Fatalf("kimi grew a second wire format (%q at %q); see the catalog entry for why it must not",
			info.AltRuntime, info.AltBaseURL)
	}
}

// The endpoint kimi actually runs on, pinned. A provider whose base URL moves
// silently is a provider whose bill moves silently.
func TestNewProviderKimiUsesDocumentedOpenAIEndpoint(t *testing.T) {
	p, err := NewProvider(ProviderOptions{
		Provider: "kimi",
		Model:    "kimi-k3",
		APIKey:   "api-key",
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	oc, ok := p.(*OpenAICompatibleProvider)
	if !ok {
		t.Fatalf("kimi must run on the OpenAI-compatible runtime, got %T", p)
	}
	if want := "https://api.moonshot.ai/v1"; oc.baseURL != want {
		t.Fatalf("kimi endpoint moved: got %q, want %q", oc.baseURL, want)
	}
}

// A provider with no alt format must ignore a saved wire format rather than
// break on it — an oauth.json or preference file written when someone was
// experimenting must not strand the user on a runtime that was removed.
func TestNewProviderKimiIgnoresAStaleWireFormat(t *testing.T) {
	p, err := NewProvider(ProviderOptions{
		Provider:   "kimi",
		Model:      "kimi-k3",
		APIKey:     "api-key",
		WireFormat: "anthropic",
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if _, ok := p.(*OpenAICompatibleProvider); !ok {
		t.Fatalf("a wire format kimi does not offer must fall back to its only runtime, got %T", p)
	}
}
