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
