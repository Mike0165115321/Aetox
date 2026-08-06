package bootstrap

import (
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// An MCP server usually needs a key, and typing one into the settings form
// writes it verbatim into mcp-servers.json — a file that gets backed up,
// synced and pasted into issues. A reference means the file never holds the
// secret at all, which is what git's credential.helper and docker's credsStore
// do, and the cheap end of what Hermes builds as secret_sources.
func TestMCPSecretReferencesResolveFromTheEnvironment(t *testing.T) {
	t.Setenv("EXA_API_KEY", "sk-from-the-environment")

	got := MCPServers([]config.MCPServerConfig{{
		Name:        "exa",
		URL:         "https://mcp.exa.ai/mcp",
		Headers:     map[string]string{"x-api-key": "${env:EXA_API_KEY}"},
		Environment: map[string]string{"TOKEN": "prefix-${env:EXA_API_KEY}-suffix"},
	}})
	if len(got) != 1 {
		t.Fatalf("got %d servers", len(got))
	}
	if v := got[0].Headers["x-api-key"]; v != "sk-from-the-environment" {
		t.Errorf("header = %q, want the resolved value", v)
	}
	// Inside a larger string too: a token is often a prefix plus the secret.
	if v := got[0].Environment["TOKEN"]; v != "prefix-sk-from-the-environment-suffix" {
		t.Errorf("environment = %q, want the reference expanded in place", v)
	}
}

// A key pasted in directly keeps working. This is an option, not a migration —
// breaking every existing server to introduce a safer way of writing one would
// be a trade nobody asked for.
func TestMCPLiteralValuesArePassedThrough(t *testing.T) {
	got := MCPServers([]config.MCPServerConfig{{
		Name:    "exa",
		Headers: map[string]string{"x-api-key": "sk-pasted-in-directly"},
	}})
	if v := got[0].Headers["x-api-key"]; v != "sk-pasted-in-directly" {
		t.Errorf("header = %q, want it untouched", v)
	}
}

// An unset variable resolves to empty, not to the literal `${env:NAME}`. The
// server then fails to authenticate and says so, which is diagnosable; sending
// the literal text as a bearer token produces a rejection that blames the
// wrong thing.
func TestMCPUnsetSecretReferenceResolvesToEmpty(t *testing.T) {
	got := MCPServers([]config.MCPServerConfig{{
		Name:    "exa",
		Headers: map[string]string{"x-api-key": "${env:AETOX_DEFINITELY_UNSET_KEY}"},
	}})
	if v := got[0].Headers["x-api-key"]; v != "" {
		t.Errorf("header = %q, want empty rather than the literal reference", v)
	}
}
