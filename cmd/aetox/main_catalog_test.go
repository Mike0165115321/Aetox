package main

import (
	"os"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// Seed the model catalog for this package.
//
// main() installs the cached table before anything asks what a model can do
// (model.InstallCachedCatalog), and the tests here call defaultThinkLevel and
// formatModelModeLabel directly — so without this they would be measuring the
// no-catalog path and calling it the CLI's behaviour.
//
// That is not a hypothetical distinction. It is exactly the gap that made this
// package go red: capabilities moved to the catalog and neither entry point
// installed one, so the CLI reported no thinking level for a model that has
// one, and the desktop's depended on whether its usage panel had run.
//
// Captured from models.dev (2026-08-23), not invented. Add a row when a test
// needs a model this does not cover; never soften one to make a test pass.
func TestMain(m *testing.M) {
	model.SetModelCatalog(&model.ModelCatalog{
		Source: "models.dev (captured 2026-08-23)",
		Models: map[string]model.ModelFacts{
			"deepseek/deepseek-v4-flash": {
				Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true,
				ReasoningLevels: []string{"low", "high", "max"},
				Input:           []string{"text"}, Output: []string{"text"},
			},
		},
	})
	os.Exit(m.Run())
}
