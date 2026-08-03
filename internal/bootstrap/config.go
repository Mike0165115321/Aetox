package bootstrap

import (
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
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
			Environment: c.Environment,
			URL:         c.URL,
			Headers:     c.Headers,
			Timeout:     time.Duration(c.TimeoutMs) * time.Millisecond,
			Disabled:    c.Disabled,
		})
	}
	return out
}
