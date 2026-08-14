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
