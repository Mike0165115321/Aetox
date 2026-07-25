package model

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Providers with no discovery path and no OpenAI-compatible alt endpoint must
// say so clearly instead of guessing an endpoint.
func TestModelDiscoveryUnsupportedProviderFailsLoudly(t *testing.T) {
	_, err := ModelChoicesWithEndpointAndAPIKey("anthropic", "", "sk-test")
	if err == nil || !strings.Contains(err.Error(), "does not support remote model discovery") {
		t.Fatalf("expected a clear unsupported-discovery error, got %v", err)
	}
}

// DeepSeek's primary runtime (Anthropic format) has no /models endpoint — the
// discovery path must route through the OpenAI-compatible alt endpoint rather
// than reporting discovery as unsupported. (The HTTP call itself may fail in
// offline tests; NOT getting the unsupported-discovery error is the point.)
func TestModelDiscoveryDeepSeekRoutesThroughAltEndpoint(t *testing.T) {
	_, err := ModelChoicesWithEndpointAndAPIKey("deepseek", DefaultBaseURL("deepseek"), "")
	if err != nil && strings.Contains(err.Error(), "does not support remote model discovery") {
		t.Fatalf("deepseek must discover via its alt endpoint, got unsupported: %v", err)
	}
}

// A local runtime's model name must come from the server, never from a
// hardcoded catalog entry — guessing produced "model 'gemma3:4b' not found"
// against an Ollama that was running fine with entirely different models.
func TestResolveDefaultModelAsksLocalServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"models":[{"name":"ornith:9b"},{"name":"qwen3:8b"}]}`)
	}))
	defer srv.Close()

	if got := ResolveDefaultModel("ollama", srv.URL, ""); got != "ornith:9b" {
		t.Fatalf("ResolveDefaultModel(ollama): want the server's first model ornith:9b, got %q", got)
	}
}

// Nothing installed, or the server is down: return empty rather than a name
// that is guaranteed to fail downstream.
func TestResolveDefaultModelEmptyWhenLocalServerUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(http.NotFound))
	srv.Close()

	if got := ResolveDefaultModel("ollama", srv.URL, ""); got != "" {
		t.Fatalf("ResolveDefaultModel(ollama) with no server: want %q, got %q", "", got)
	}
}

// Cloud providers publish stable model names, so they keep their catalog
// fallback and must not pay for a discovery round trip to get it.
func TestResolveDefaultModelUsesCatalogForCloudProviders(t *testing.T) {
	if got := ResolveDefaultModel("anthropic", "http://127.0.0.1:1", ""); got != DefaultModel("anthropic") {
		t.Fatalf("ResolveDefaultModel(anthropic): want catalog default %q, got %q", DefaultModel("anthropic"), got)
	}
}
