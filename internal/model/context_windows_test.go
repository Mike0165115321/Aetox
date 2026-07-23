package model

import "testing"

func TestContextWindowTokensCuratedModels(t *testing.T) {
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
