package model

import (
	"context"
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

func TestOpenAICompatibleSendsToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	calls := 0
	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "openrouter",
		Model:    "deepseek/deepseek-r1",
		BaseURL:  server.URL,
		TokenSource: func(context.Context) (string, error) {
			calls++
			return "minted-token", nil
		},
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
	// Per request, not once at construction — signed-in tokens expire and a
	// session outlives them.
	if calls != 2 {
		t.Fatalf("token source consulted %d times for 2 requests; want 2", calls)
	}
}

func TestOpenAICompatibleReportsTokenFailure(t *testing.T) {
	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "openrouter",
		Model:    "deepseek/deepseek-r1",
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
	signIn(t, "openrouter", oauth.Credential{Type: "oauth", Access: "tok"})

	// No APIKey at all: a signed-in provider has no key to give, and demanding
	// one would make the whole feature unreachable.
	p, err := NewProvider(ProviderOptions{Provider: "open-router", Model: "deepseek/deepseek-r1"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "openrouter" {
		t.Fatalf("Name = %q; want openrouter", p.Name())
	}
}

func TestFactoryPrefersSignInEndpoint(t *testing.T) {
	signIn(t, "openrouter", oauth.Credential{
		Type:     "oauth",
		Access:   "tok",
		Endpoint: "https://account.example.com/v1",
	})

	p, err := NewProvider(ProviderOptions{Provider: "openrouter", Model: "deepseek/deepseek-r1"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	compatible, ok := p.(*OpenAICompatibleProvider)
	if !ok {
		t.Fatalf("provider type = %T; want the OpenAI-compatible runtime", p)
	}
	if compatible.baseURL != "https://account.example.com/v1" {
		t.Fatalf("baseURL = %q; want the host the sign-in named", compatible.baseURL)
	}
}

func TestFactoryKeepsUserBaseURLOverSignInEndpoint(t *testing.T) {
	signIn(t, "openrouter", oauth.Credential{
		Type:     "oauth",
		Access:   "tok",
		Endpoint: "https://account.example.com/v1",
	})

	p, err := NewProvider(ProviderOptions{
		Provider: "openrouter",
		Model:    "deepseek/deepseek-r1",
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
	if _, err := NewProvider(ProviderOptions{Provider: "anthropic", Model: "claude-sonnet-5", APIKey: "sk-ant"}); err != nil {
		t.Fatalf("NewProvider with a key: %v", err)
	}
}
