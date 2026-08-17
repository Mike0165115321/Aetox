package model

import "strings"

// ContextWindowTokens reports a model's total context window in tokens,
// curated per provider the same way thinking_capabilities.go curates levels.
// 0 means unknown — callers decide their own fallback. User overrides
// (ModelContextTokens config/flag) always win at the call site.
func ContextWindowTokens(provider, modelName string) int {
	canonical := NormalizeProvider(provider)
	modelID := strings.ToLower(strings.TrimSpace(modelName))
	if modelID == "" {
		modelID = strings.ToLower(strings.TrimSpace(DefaultModel(canonical)))
	}

	// The fetched catalog first, because it states the window per model where
	// everything below states it per provider with a fallback — and the
	// fallback is what most models actually get. The Gemini branch knows one
	// prefix while an account's own discovery lists 37 models, so every 3.x
	// model was measured against a guess, and that guess is the percentage on
	// the composer the user reads all day. Where both answer they agree
	// (deepseek-v4: 1M either way) and the catalog is the more exact of the two
	// (glm-4.5 is 131,072, not the 128,000 the default rounds it to).
	//
	// Absent or silent, it changes nothing: the curated tables below are still
	// the answer, which is what makes this safe to consult on a path this hot.
	if tokens := catalogContextWindow(canonical, modelID); tokens > 0 {
		return tokens
	}

	switch canonical {
	case "deepseek":
		return deepseekContextWindow(modelID)
	case "openai":
		return openaiContextWindow(modelID)
	case "codex":
		// Codex serves OpenAI's models through a subscription, so the window is
		// OpenAI's. Asked here by recursion — through the catalog under
		// "openai" first, then the curated table — rather than by teaching
		// modelsDevProvider that codex is openai, which is the shorter change
		// and the wrong one: ModelCatalog.For is also what PRICES a model, and
		// filing Codex under openai would put OpenAI's per-token rates on a
		// flat monthly plan. That is the exact bill token_usage.provider was
		// added to prevent (db.go, migration 15). One provider, two facts, and
		// only one of them is allowed to travel.
		//
		// Until 2026-08-18 this was the `default` case, and the 0 it returned
		// was not read as "unknown" by the desktop: App.contextWindowTokens
		// fell back to the agent's char budget over four, so the meter drew a
		// 32,000-token window on a model that had already accepted 43,434 in
		// one request. The fallback is gone; this is the half that makes the
		// answer real rather than merely absent.
		return ContextWindowTokens("openai", modelID)
	case "anthropic":
		return 200_000
	case "gemini":
		return geminiContextWindow(modelID)
	case "zai":
		return zaiContextWindow(modelID)
	case "groq":
		return 128_000
	case "kimi":
		return kimiContextWindow(modelID)
	case "openrouter":
		// OpenRouter ids are "vendor/model" — resolve by the underlying vendor.
		if vendor, name, ok := strings.Cut(modelID, "/"); ok {
			return ContextWindowTokens(vendor, name)
		}
		return 0
	default:
		return 0 // ollama and unknown providers: no promise we can keep
	}
}

func deepseekContextWindow(modelID string) int {
	if strings.HasPrefix(modelID, "deepseek-v4") {
		return 1_000_000 // V4 series (incl. -flash): 1M context per DeepSeek docs
	}
	return 128_000 // deepseek-chat / deepseek-reasoner / V3.x
}

func kimiContextWindow(modelID string) int {
	if strings.HasPrefix(modelID, "kimi-k3") {
		return 1_000_000 // "a 1M-token context window" — K3 quickstart
	}
	return 128_000 // K2 and the moonshot-v1 line
}

func openaiContextWindow(modelID string) int {
	switch {
	case strings.HasPrefix(modelID, "gpt-5"):
		return 400_000
	case strings.HasPrefix(modelID, "gpt-4.1"):
		return 1_000_000
	case strings.HasPrefix(modelID, "o3"), strings.HasPrefix(modelID, "o4"):
		return 200_000
	default:
		return 128_000 // gpt-4o and friends
	}
}

func geminiContextWindow(modelID string) int {
	if strings.HasPrefix(modelID, "gemini-1.5-pro") {
		return 2_000_000
	}
	return 1_000_000 // 1.5-flash, 2.x series
}

func zaiContextWindow(modelID string) int {
	if strings.HasPrefix(modelID, "glm-4.6") {
		return 200_000
	}
	return 128_000
}
