package bootstrap

import (
	"os"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// Seed the model catalog for this package.
//
// Thinking capabilities are answered per model from the fetched catalog now,
// and with none installed every answer is "unknown" — correct, and it would
// leave the wire tests here proving that a level nobody offers sends nothing,
// rather than that the level a user picks reaches the request.
//
// Captured from models.dev (2026-08-23) rather than invented, so a test cannot
// come to rely on a capability the real model does not have. Add a row when a
// test needs a model this does not cover; never soften a row to make one pass.
func TestMain(m *testing.M) {
	model.SetModelCatalog(&model.ModelCatalog{
		Source: "models.dev (captured 2026-08-23)",
		Models: map[string]model.ModelFacts{
			// A toggle plus effort rungs: what DeepSeek's thinking block and
			// effort field are driven from.
			"deepseek/deepseek-v4-flash": {
				Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true,
				ReasoningLevels: []string{"low", "high", "max"},
				Input:           []string{"text"}, Output: []string{"text"},
			},
			// Moonshot is what models.dev files Kimi under.
			"moonshotai/kimi-k3": {
				Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true,
				ReasoningLevels: []string{"low", "high", "max"},
				Input:           []string{"text"}, Output: []string{"text"},
			},
			// MiniMax states a toggle and no rungs, which is the whole reason
			// its dial is a switch rather than a ladder.
			"minimax/minimax-m3": {
				Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true,
				Input: []string{"text", "image"}, Output: []string{"text"},
			},
		},
	})
	os.Exit(m.Run())
}
