package model

import (
	"strings"
)

type ThinkingRuntime string

const (
	ThinkingRuntimeUnknown         ThinkingRuntime = "unknown"
	ThinkingRuntimeReasoningObject ThinkingRuntime = "reasoning-object"
	ThinkingRuntimeReasoningEffort ThinkingRuntime = "reasoning-effort"
	ThinkingRuntimeDeepSeek        ThinkingRuntime = "deepseek-thinking"
	ThinkingRuntimeGroq            ThinkingRuntime = "groq-reasoning"
)

type ThinkingCapabilities struct {
	Supported bool
	Native    bool
	Levels    []string
	Default   string
	Runtime   ThinkingRuntime
	Source    string
}

var fallbackThinkingCapabilities = ThinkingCapabilities{
	Supported: true,
	Native:    false,
	Levels:    []string{"low", "medium", "high", "off"},
	Default:   "low",
	Runtime:   ThinkingRuntimeUnknown,
	Source:    "fallback",
}

var conservativeFallback = ThinkingCapabilities{
	Supported: true,
	Native:    false,
	Levels:    []string{"low", "medium", "high", "off"},
	Default:   "low",
	Runtime:   ThinkingRuntimeUnknown,
	Source:    "conservative-fallback",
}

var noThinkingCapabilities = ThinkingCapabilities{
	Supported: false,
	Native:    false,
	Levels:    nil,
	Default:   "",
	Runtime:   ThinkingRuntimeUnknown,
	Source:    "no-thinking-knob",
}

var unknownProviderCapabilities = ThinkingCapabilities{
	Supported: false,
	Native:    false,
	Levels:    nil,
	Default:   "",
	Runtime:   ThinkingRuntimeUnknown,
	Source:    "unknown-provider",
}

func ResolveThinkingCapabilities(provider, modelName string) ThinkingCapabilities {
	canonicalProvider := NormalizeProvider(provider)
	modelID := strings.ToLower(strings.TrimSpace(modelName))
	if modelID == "" {
		modelID = strings.ToLower(strings.TrimSpace(DefaultModel(canonicalProvider)))
	}

	switch canonicalProvider {
	case "deepseek":
		return cloneThinkingCapabilities(resolveDeepSeekThinkingCapabilities(modelID))
	case "gemini":
		return cloneThinkingCapabilities(resolveGeminiThinkingCapabilities(modelID))
	case "openai":
		return cloneThinkingCapabilities(resolveOpenAIThinkingCapabilities(modelID))
	case "openrouter":
		return cloneThinkingCapabilities(resolveOpenRouterThinkingCapabilities(modelID))
	case "groq":
		return cloneThinkingCapabilities(resolveGroqThinkingCapabilities(modelID))
	case "ollama", "lmstudio":
		return cloneThinkingCapabilities(noThinkingCapabilities)
	default:
		return cloneThinkingCapabilities(fallbackThinkingCapabilities)
	}
}

func SupportedThinkingLevels(provider, modelName string) []string {
	caps := ResolveThinkingCapabilities(provider, modelName)
	return append([]string{}, caps.Levels...)
}

func SupportsThinkingLevel(provider, modelName, level string) bool {
	normalized := strings.ToLower(strings.TrimSpace(level))
	if normalized == "" {
		return false
	}
	for _, supported := range SupportedThinkingLevels(provider, modelName) {
		if normalized == supported {
			return true
		}
	}
	return false
}

func NormalizeThinkingLevel(provider, modelName, requested string) string {
	caps := ResolveThinkingCapabilities(provider, modelName)
	if !caps.Supported {
		return ""
	}

	defaultLevel := strings.TrimSpace(caps.Default)
	if defaultLevel == "" {
		defaultLevel = strings.ToLower(strings.TrimSpace(requested))
	}

	normalized := strings.ToLower(strings.TrimSpace(requested))
	if normalized == "" {
		return defaultLevel
	}
	if SupportsThinkingLevel(provider, modelName, normalized) {
		return normalized
	}

	switch NormalizeProvider(provider) {
	case "deepseek":
		switch normalized {
		case "none", "off", "disabled":
			return "off"
		case "low", "medium":
			return "high"
		case "xhigh":
			return "max"
		}
	case "gemini":
		switch normalized {
		case "off", "disabled":
			if SupportsThinkingLevel(provider, modelName, "none") {
				return "none"
			}
		case "max":
			if SupportsThinkingLevel(provider, modelName, "high") {
				return "high"
			}
		}
	case "openai", "openrouter":
		switch normalized {
		case "off", "disabled":
			if SupportsThinkingLevel(provider, modelName, "none") {
				return "none"
			}
		case "max":
			if SupportsThinkingLevel(provider, modelName, "xhigh") {
				return "xhigh"
			}
		}
	case "groq":
		switch normalized {
		case "off", "disabled":
			if SupportsThinkingLevel(provider, modelName, "none") {
				return "none"
			}
		}
	}

	return defaultLevel
}

func resolveDeepSeekThinkingCapabilities(modelID string) ThinkingCapabilities {
	if modelID == "" || strings.HasPrefix(modelID, "deepseek-") || modelID == "deepseek-chat" || modelID == "deepseek-reasoner" {
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"off", "high", "max"},
			Default:   "high",
			Runtime:   ThinkingRuntimeDeepSeek,
			Source:    "deepseek-docs",
		}
	}
	return fallbackThinkingCapabilities
}

func resolveOpenAIThinkingCapabilities(modelID string) ThinkingCapabilities {
	if modelID == "" {
		return fallbackThinkingCapabilities
	}
	switch {
	case strings.HasPrefix(modelID, "gpt-5-pro"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"high"},
			Default:   "high",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "openai-chat-docs",
		}
	case strings.HasPrefix(modelID, "gpt-5.1"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "low", "medium", "high"},
			Default:   "none",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "openai-chat-docs",
		}
	case strings.HasPrefix(modelID, "gpt-5.2"), strings.HasPrefix(modelID, "gpt-5"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "minimal", "low", "medium", "high", "xhigh"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "openai-chat-docs",
		}
	case strings.HasPrefix(modelID, "o1"), strings.HasPrefix(modelID, "o3"), strings.HasPrefix(modelID, "o4"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "openai-chat-docs",
		}
	default:
		return cloneThinkingCapabilities(conservativeFallback)
	}
}

func resolveGeminiThinkingCapabilities(modelID string) ThinkingCapabilities {
	switch {
	case modelID == "":
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "gemini-openai-compat-docs",
		}
	case strings.HasPrefix(modelID, "gemini-2.0-flash-lite"):
		return ThinkingCapabilities{
			Supported: false,
			Native:    false,
			Levels:    nil,
			Default:   "",
			Runtime:   ThinkingRuntimeUnknown,
			Source:    "gemini-model-docs",
		}
	case strings.HasPrefix(modelID, "gemini-2.5-pro"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "gemini-openai-compat-docs",
		}
	case strings.HasPrefix(modelID, "gemini-2.5"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "gemini-openai-compat-docs",
		}
	case strings.HasPrefix(modelID, "gemini-3"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "gemini-openai-compat-docs",
		}
	default:
		return cloneThinkingCapabilities(conservativeFallback)
	}
}

func resolveOpenRouterThinkingCapabilities(modelID string) ThinkingCapabilities {
	if isKnownOpenRouterReasoningModel(modelID) {
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "minimal", "low", "medium", "high", "xhigh"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningObject,
			Source:    "openrouter-reasoning-docs",
		}
	}
	return cloneThinkingCapabilities(conservativeFallback)
}

func resolveGroqThinkingCapabilities(modelID string) ThinkingCapabilities {
	switch {
	case strings.HasPrefix(modelID, "openai/gpt-oss-"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeGroq,
			Source:    "groq-reasoning-docs",
		}
	case strings.HasPrefix(modelID, "qwen/qwen3-"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"default", "none"},
			Default:   "default",
			Runtime:   ThinkingRuntimeGroq,
			Source:    "groq-reasoning-docs",
		}
	default:
		return cloneThinkingCapabilities(conservativeFallback)
	}
}

func isKnownOpenRouterReasoningModel(modelID string) bool {
	switch {
	case strings.HasPrefix(modelID, "openai/"):
		return true
	case strings.HasPrefix(modelID, "deepseek/"):
		return true
	case strings.HasPrefix(modelID, "google/gemini-"):
		return true
	case strings.HasPrefix(modelID, "qwen/"):
		return true
	case strings.HasPrefix(modelID, "google/gemini-2.5"):
		return true
	case strings.HasPrefix(modelID, "anthropic/claude-3.7"):
		return true
	case strings.HasPrefix(modelID, "anthropic/claude-sonnet-4"):
		return true
	default:
		return false
	}
}

func cloneThinkingCapabilities(caps ThinkingCapabilities) ThinkingCapabilities {
	cloned := caps
	cloned.Levels = append([]string{}, caps.Levels...)
	return cloned
}

