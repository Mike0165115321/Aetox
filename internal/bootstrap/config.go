package bootstrap

import (
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/mcp"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// ContextChars is the retained-history budget in characters, scaled to the
// model's real context window.
//
// The 4:1 chars-per-token ratio is the same rough conversion the rest of the
// codebase uses. It was written out twice inside the old wiring — once for the
// main agent and once for every sub-agent — and the two had to stay equal for a
// sub-agent's history to behave like its parent's. Naming it is how they stay
// equal. Zero means the caller's own default (cognitive.NewContext uses 128k).
func ContextChars(cfg config.Config) int {
	tokens := cfg.ModelContextTokens
	if tokens <= 0 {
		tokens = model.ContextWindowTokens(cfg.ModelProvider, cfg.ModelName)
	}
	return tokens * 4
}

// MCPServers translates the persisted config DTOs into mcp.Server values.
//
// The persisted form carries a timeout in milliseconds because JSON has no
// duration; mcp.Server wants a real one. Every host loads the same file, so the
// translation belongs here rather than next to whichever host happened to need
// it first.
func MCPServers(cfgs []config.MCPServerConfig) []mcp.Server {
	out := make([]mcp.Server, 0, len(cfgs))
	for _, c := range cfgs {
		out = append(out, mcp.Server{
			Name:        c.Name,
			Command:     c.Command,
			Cwd:         c.Cwd,
			Environment: resolveSecretRefs(c.Environment),
			URL:         c.URL,
			Headers:     resolveSecretRefs(c.Headers),
			Timeout:     time.Duration(c.TimeoutMs) * time.Millisecond,
			Disabled:    c.Disabled,
			Deferred:    agentsOnly(c.For),
			Tools:       c.Tools,
		})
	}
	return out
}

// agentsOnly reports that no desk carries this server — only named agents do —
// which is what lets the startup connect skip it (mcp.Server.Deferred).
//
// A nil list is "never placed", which every desk carries, so it is eager. An
// empty list is "shown to nobody", which nothing needs at startup or ever.
func agentsOnly(for_ []string) bool {
	if for_ == nil {
		return false
	}
	for _, entry := range for_ {
		if !strings.HasPrefix(strings.TrimSpace(entry), config.MCPAgentPrefix) {
			return false
		}
	}
	return true
}

// secretRef matches ${env:NAME} — a reference to a secret rather than the
// secret itself.
var secretRef = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)

// resolveSecretRefs expands ${env:NAME} inside an MCP server's environment and
// headers, at the moment the server is connected.
//
// An MCP server usually needs a key — the exa preset wants `x-api-key`, a
// GitHub server wants a token — and typing it into the settings form writes it
// verbatim into mcp-servers.json. That file is backed up, synced, pasted into
// issues and (until this) read by the agent's own file tools. Every mitigation
// for that is a way of *hiding* a secret that is still in the file.
//
// A reference is the alternative: the config holds ${env:EXA_API_KEY}, the
// value comes from the environment or <DataRoot>/.env at connect time, and the
// file never contains the secret to protect. That is what git's
// credential.helper, docker's credsStore and gh all do, and it is the cheap end
// of what Hermes builds out as secret_sources (1Password, Bitwarden, command) —
// the same idea, one source instead of four.
//
// An unset variable expands to empty rather than being left as the literal
// `${env:NAME}`: the server then fails to authenticate and says so, which is a
// diagnosable error. Sending the literal text as a bearer token produces a
// rejection that blames the wrong thing.
//
// Values that hold no reference pass through untouched, so a key pasted in
// directly keeps working — this is an option, not a migration.
func resolveSecretRefs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = secretRef.ReplaceAllStringFunc(v, func(match string) string {
			name := secretRef.FindStringSubmatch(match)[1]
			value := os.Getenv(name)
			// Registered so it cannot reach the debug log, the shell audit or
			// tool_runs — the same guarantee an API key from the credentials
			// file gets (debuglog.Redact).
			debuglog.Redact(value)
			return value
		})
	}
	return out
}
