package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/oauth"
)

// signIn writes a credential into an isolated store for the duration of one
// test, so the factory's automatic OAuth resolution can be exercised without
// touching the developer's real logins.
func signIn(t *testing.T, provider string, cred oauth.Credential) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if err := oauth.Set(provider, cred); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
}

func oauthCredential(access, account string) oauth.Credential {
	return oauth.Credential{Type: "oauth", Access: access, Account: account}
}

func TestAnthropicSubscriptionSendsBearerNotAPIKey(t *testing.T) {
	var gotAuth, gotAPIKey, gotBeta string
	var gotSystem []anthropicSystemBlock
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		gotBeta = r.Header.Get("anthropic-beta")

		var payload struct {
			System []anthropicSystemBlock `json:"system"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotSystem = payload.System

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn"}`))
	}))
	defer server.Close()

	p, err := NewAnthropicProvider(AnthropicConfig{
		Provider: "anthropic",
		Model:    "claude-sonnet-5",
		BaseURL:  server.URL,
		TokenSource: func(context.Context) (string, error) {
			return "oat-token", nil
		},
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider: %v", err)
	}

	_, err = p.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are Aetox."},
			{Role: RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if gotAuth != "Bearer oat-token" {
		t.Fatalf("Authorization = %q; want the bearer token", gotAuth)
	}
	// Sending both is a 401 from an endpoint that accepts either alone.
	if gotAPIKey != "" {
		t.Fatalf("x-api-key = %q; want it absent on the subscription path", gotAPIKey)
	}
	if gotBeta != oauth.AnthropicBeta {
		t.Fatalf("anthropic-beta = %q; want %q", gotBeta, oauth.AnthropicBeta)
	}
	// The endpoint compares the first block byte-for-byte, so the prefix must
	// be alone in its own block — concatenation is what drew the fake 429.
	if len(gotSystem) != 2 || gotSystem[0].Text != oauth.AnthropicOAuthSystemPrefix {
		t.Fatalf("system blocks = %+v; want the exact prefix alone in block 0", gotSystem)
	}
	if gotSystem[1].Text != "You are Aetox." {
		t.Fatalf("system blocks = %+v; want Aetox's own prompt as block 1", gotSystem)
	}
}

func TestAnthropicAPIKeyPathUnchanged(t *testing.T) {
	var gotAuth, gotAPIKey, gotSystem string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		var payload struct {
			System string `json:"system"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotSystem = payload.System
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn"}`))
	}))
	defer server.Close()

	p, err := NewAnthropicProvider(AnthropicConfig{
		Provider: "anthropic",
		Model:    "claude-sonnet-5",
		APIKey:   "sk-ant-key",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider: %v", err)
	}
	if _, err := p.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are Aetox."},
			{Role: RoleUser, Content: "hello"},
		},
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if gotAPIKey != "sk-ant-key" || gotAuth != "" {
		t.Fatalf("api-key path sent Authorization=%q x-api-key=%q", gotAuth, gotAPIKey)
	}
	// The Claude Code prefix is a condition of the OAuth credential only — it
	// must not leak into requests paid for by an API key.
	if strings.Contains(gotSystem, oauth.AnthropicOAuthSystemPrefix) {
		t.Fatalf("system = %q; want no OAuth prefix on the api-key path", gotSystem)
	}
}

func TestAnthropicSubscriptionDoesNotDoublePrefix(t *testing.T) {
	req := Request{Messages: []Message{
		{Role: RoleSystem, Content: oauth.AnthropicOAuthSystemPrefix + "\n\nYou are Aetox."},
		{Role: RoleUser, Content: "hi"},
	}}
	payload, err := buildAnthropicRequest("anthropic", "claude-sonnet-5", req, false, true)
	if err != nil {
		t.Fatalf("buildAnthropicRequest: %v", err)
	}
	blocks, ok := payload.System.([]anthropicSystemBlock)
	if !ok || len(blocks) != 2 {
		t.Fatalf("system = %+v; want two blocks", payload.System)
	}
	if blocks[0].Text != oauth.AnthropicOAuthSystemPrefix || strings.Contains(blocks[1].Text, oauth.AnthropicOAuthSystemPrefix) {
		t.Fatalf("system blocks = %+v; want the prefix exactly once, alone in block 0", blocks)
	}
}

func TestOpenAICompatibleSendsTokenAndProviderHeaders(t *testing.T) {
	var gotAuth, gotIntegration string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotIntegration = r.Header.Get("Copilot-Integration-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	calls := 0
	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "github-copilot",
		Model:    "gpt-4.1",
		BaseURL:  server.URL,
		TokenSource: func(context.Context) (string, error) {
			calls++
			return "minted-token", nil
		},
		Headers: oauth.CopilotHeaders(),
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider: %v", err)
	}

	for range 2 {
		if _, err := p.Complete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: "hello"}},
		}); err != nil {
			t.Fatalf("Complete: %v", err)
		}
	}

	if gotAuth != "Bearer minted-token" {
		t.Fatalf("Authorization = %q; want the minted token", gotAuth)
	}
	if gotIntegration == "" {
		t.Fatal("Copilot-Integration-Id was not sent; Copilot answers 400 without it")
	}
	// Per request, not once at construction — a Copilot token dies after ~25
	// minutes and a session outlives that.
	if calls != 2 {
		t.Fatalf("token source consulted %d times for 2 requests; want 2", calls)
	}
}

func TestOpenAICompatibleReportsTokenFailure(t *testing.T) {
	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "github-copilot",
		Model:    "gpt-4.1",
		BaseURL:  "https://example.invalid",
		TokenSource: func(context.Context) (string, error) {
			return "", context.DeadlineExceeded
		},
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider: %v", err)
	}
	_, err = p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "sign-in") {
		t.Fatalf("err = %v; want a sign-in failure the user can act on", err)
	}
}

func TestFactoryUsesSignInInsteadOfAPIKey(t *testing.T) {
	signIn(t, "github-copilot", oauth.Credential{Type: "oauth", Access: "tok"})

	// No APIKey at all: a signed-in provider has no key to give, and demanding
	// one would make the whole feature unreachable.
	p, err := NewProvider(ProviderOptions{Provider: "copilot", Model: "gpt-4.1"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "github-copilot" {
		t.Fatalf("Name = %q; want github-copilot", p.Name())
	}
}

func TestFactoryPrefersSignInEndpoint(t *testing.T) {
	signIn(t, "qwen", oauth.Credential{
		Type:     "oauth",
		Access:   "tok",
		Endpoint: "https://portal.qwen.ai/v1",
	})

	p, err := NewProvider(ProviderOptions{Provider: "qwen", Model: "qwen3-coder-plus"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	compatible, ok := p.(*OpenAICompatibleProvider)
	if !ok {
		t.Fatalf("provider type = %T; want the OpenAI-compatible runtime", p)
	}
	if compatible.baseURL != "https://portal.qwen.ai/v1" {
		t.Fatalf("baseURL = %q; want the host the sign-in named", compatible.baseURL)
	}
}

func TestFactoryKeepsUserBaseURLOverSignInEndpoint(t *testing.T) {
	signIn(t, "qwen", oauth.Credential{
		Type:     "oauth",
		Access:   "tok",
		Endpoint: "https://portal.qwen.ai/v1",
	})

	p, err := NewProvider(ProviderOptions{
		Provider: "qwen",
		Model:    "qwen3-coder-plus",
		BaseURL:  "http://localhost:8080/v1",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	compatible := p.(*OpenAICompatibleProvider)
	if compatible.baseURL != "http://localhost:8080/v1" {
		t.Fatalf("baseURL = %q; want the URL the user typed", compatible.baseURL)
	}
}

func TestFactoryUnaffectedWhenNobodySignedIn(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	if _, err := NewProvider(ProviderOptions{Provider: "anthropic", Model: "claude-sonnet-5"}); err == nil {
		t.Fatal("a provider with no key and no sign-in was accepted")
	}
	p, err := NewProvider(ProviderOptions{Provider: "anthropic", Model: "claude-sonnet-5", APIKey: "sk-ant"})
	if err != nil {
		t.Fatalf("NewProvider with a key: %v", err)
	}
	if anthropic, ok := p.(*AnthropicProvider); !ok || anthropic.usesSubscription() {
		t.Fatal("an api-key provider was built as a subscription one")
	}
}
