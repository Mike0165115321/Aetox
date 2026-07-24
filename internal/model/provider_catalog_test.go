package model

import (
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
