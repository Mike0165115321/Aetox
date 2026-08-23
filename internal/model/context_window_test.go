package model

import "testing"

// A subscription is a different door to the same models, not a different set of
// models, so it must not be a different answer about how big they are.
//
// Codex fell out of the switch entirely and returned 0. On its own that was
// correct behaviour for "unknown" and harmless; what made it cost something was
// the desktop, which read the 0 and substituted the agent's char budget over
// four. Every Codex user was shown a 32,000-token window on models the catalog
// puts at 1,050,000, and the same installs had token_usage rows at 43,434 —
// the app's own history disproving the number the app was drawing.
//
// The alias is deliberately NOT in modelsDevProvider, and this test is where
// that choice is defended: ModelCatalog.For is what prices a model too, so
// filing codex under openai there would inherit OpenAI's per-token rates onto a
// flat monthly plan. The window travels; the price must not.
func TestCodexResolvesOpenAIsWindowsWithoutInheritingItsPrices(t *testing.T) {
	// Restore what was installed, not nil. TestMain seeds a catalog for the
	// package now, and a cleanup that wipes it leaves every test running
	// after this one in a world with none.
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(nil)

	// Curated tables alone: whatever openai answers, codex answers.
	for _, name := range []string{"gpt-5.5", "gpt-5.6-luna", "gpt-5.1-codex", "gpt-4o"} {
		want := ContextWindowTokens("openai", name)
		if want <= 0 {
			t.Fatalf("openai/%s answers %d, so this test proves nothing", name, want)
		}
		if got := ContextWindowTokens("codex", name); got != want {
			t.Errorf("codex/%s = %d, openai/%s = %d; a subscription is the same model", name, got, name, want)
		}
	}

	// And through the catalog, which is where the exact figure lives. The row
	// is filed under openai; nothing in the table says "codex" at all, which is
	// the state of the real models.dev document.
	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"openai/gpt-5.6-luna": {Context: 1_050_000, Price: ModelPrice{Input: 0.2, Output: 1.2}},
	}})
	if got := ContextWindowTokens("codex", "gpt-5.6-luna"); got != 1_050_000 {
		t.Errorf("codex/gpt-5.6-luna = %d; want the catalog's 1,050,000 read through openai", got)
	}

	// The half that must NOT have happened: pricing still cannot find a Codex
	// row, so a subscription's calls stay unpriced (db.go migration 15).
	if _, ok := (&ModelCatalog{Models: map[string]ModelFacts{
		"openai/gpt-5.6-luna": {Context: 1_050_000, Price: ModelPrice{Input: 0.2, Output: 1.2}},
	}}).For("codex", "gpt-5.6-luna"); ok {
		t.Error("codex now resolves to a priced catalog row; that bills a flat monthly plan per token")
	}
}

func TestContextWindowTokensCuratedModels(t *testing.T) {
	// These are the CURATED tables, which only answer when the fetched catalog
	// does not. TestMain seeds a catalog for the package, and the catalog wins
	// by design — so this test has to say which of the two worlds it is in
	// rather than depending on which one it happened to get.
	//
	// It matters more than it reads: with the catalog installed, deepseek-chat
	// answers 1,000,000 here instead of the 128,000 written below, because the
	// table is older than the model. That is the catalog doing its job.
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(nil)

	cases := []struct {
		provider string
		model    string
		want     int
	}{
		{"deepseek", "deepseek-v4-flash", 1_000_000},
		{"deepseek", "deepseek-v4", 1_000_000},
		{"deepseek", "deepseek-chat", 128_000},
		{"deepseek", "deepseek-reasoner", 128_000},
		{"anthropic", "claude-sonnet-4-5", 200_000},
		{"openai", "gpt-4o", 128_000},
		{"openai", "gpt-4.1-mini", 1_000_000},
		{"openai", "gpt-5-mini", 400_000},
		{"gemini", "gemini-2.5-flash", 1_000_000},
		{"gemini", "gemini-1.5-pro", 2_000_000},
		{"zai", "glm-4.6", 200_000},
		// OpenRouter resolves through the underlying vendor.
		{"openrouter", "deepseek/deepseek-v4-flash", 1_000_000},
		{"openrouter", "anthropic/claude-sonnet-4-5", 200_000},
		// Codex resolves through OpenAI: same models, different door.
		{"codex", "gpt-5.5", 400_000},
		{"codex", "gpt-5.1-codex", 400_000},
		{"codex", "gpt-5.6-luna", 400_000},
		// No promise we can keep → 0, caller falls back.
		{"ollama", "qwen3:8b", 0},
		{"nonsense", "mystery", 0},
	}
	for _, tc := range cases {
		if got := ContextWindowTokens(tc.provider, tc.model); got != tc.want {
			t.Errorf("ContextWindowTokens(%q, %q) = %d, want %d", tc.provider, tc.model, got, tc.want)
		}
	}
}
