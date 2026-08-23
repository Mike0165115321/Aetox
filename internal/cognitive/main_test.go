package cognitive

import (
	"os"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// Seed the model catalog for this package.
//
// Thinking capabilities are answered per model from the fetched catalog now,
// and with none installed every answer is "unknown" — correct, and it would
// mean the tests here that drive a thinking level were exercising the
// no-catalog path rather than the one users are on.
//
// The rows are captured from models.dev (2026-08-23) rather than invented, so a
// test cannot come to depend on a capability the real model does not have. Add
// a row when a test needs a model this does not cover; do not soften a row to
// make a test pass.
func TestMain(m *testing.M) {
	model.SetModelCatalog(&model.ModelCatalog{
		Source: "models.dev (captured 2026-08-23)",
		Models: map[string]model.ModelFacts{
			// A toggle plus effort rungs: the shape DeepSeek's thinking block
			// and effort field are driven from.
			"deepseek/deepseek-v4-flash": {
				Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true,
				ReasoningLevels: []string{"low", "high", "max"},
				Input:           []string{"text"}, Output: []string{"text"},
			},
			"deepseek/deepseek-v4-pro": {
				Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true,
				ReasoningLevels: []string{"high", "max"},
				Input:           []string{"text"}, Output: []string{"text"},
			},
			"anthropic/claude-opus-5": {
				Context: 1000000, ToolCall: true, Reasoning: true,
				ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"},
				Input:           []string{"text", "image", "pdf"}, Output: []string{"text"},
			},
		},
	})
	os.Exit(m.Run())
}
