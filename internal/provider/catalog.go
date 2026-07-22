// Package provider holds metadata, aliases, capabilities, and
// validation helpers for every supported model provider.
//
// This package MUST NOT make HTTP requests. It is a registry (ทะเบียนบ้าน),
// not a client (คนออกไปทำงานจริง).
//
// Hardcoded model names and capability flags in this package are
// provider-level fallbacks only. They do not represent guaranteed
// capabilities for every model behind a provider. Final model-selection
// and capability checks should eventually move to a model-level catalog
// or runtime discovery layer (internal/model is responsible for calling
// provider APIs to fetch live model lists).
package provider

import (
	"os"
	"sort"
	"strings"
)

// Runtime classifies how a provider is invoked.
type Runtime string

const (
	RuntimeNoop             Runtime = "noop"
	RuntimeOpenAICompatible Runtime = "openai-compatible"
	RuntimeOllama           Runtime = "ollama"
	RuntimeAnthropic        Runtime = "anthropic"
)

// ModelDefaults holds the static fallback model names for a provider.
// These are used ONLY when no model name has been configured and no
// live model list could be fetched. They are not authoritative.
type ModelDefaults struct {
	// FallbackModel is the model to use when nothing else is configured.
	// This is a best-effort default; it may become unavailable.
	FallbackModel string

	// RecommendedModels is an optional short list of well-known models
	// that work well with this provider. It is a discovery hint, not
	// a complete catalog.
	RecommendedModels []string
}

// Capabilities records provider-level or runtime-level feature support.
//
// These flags represent what the provider API surface can express, NOT
// what every specific model behind that provider guarantees. Model-level
// capability checks belong in a future model-level catalog or runtime
// discovery layer.
type Capabilities struct {
	// ToolCalling reports whether this provider supports the
	// tools / tool_calls API protocol.
	ToolCalling bool

	// Reasoning reports whether this provider supports a dedicated
	// reasoning / thinking effort knob (e.g. OpenRouter).
	Reasoning bool
}

// Spec is the canonical metadata for one supported provider.
type Spec struct {
	Canonical      string
	Aliases        []string
	RequiresAPIKey bool
	Runtime        Runtime
	BaseURL        string
	EnvKeys        []string
	ModelDefaults  ModelDefaults
	Capabilities   Capabilities
}

// ---------------------------------------------------------------------------
// Catalog
// ---------------------------------------------------------------------------

type entry struct {
	canonical      string
	aliases        []string
	requiresAPIKey bool
	runtime        Runtime
	baseURL        string
	envKeys        []string
	modelDefaults  ModelDefaults
	capabilities   Capabilities
}

// The provider catalog is the single source of truth for provider
// metadata. Model names inside ModelDefaults are static fallbacks
// only — they are not guaranteed to be live, available, or appropriate
// for every model behind the provider.
var catalog = map[string]*entry{
	"noop": {
		canonical:      "noop",
		aliases:        []string{"noop", "none", "stub"},
		requiresAPIKey: false,
		runtime:        RuntimeNoop,
		baseURL:        "",
		envKeys:        nil,
		modelDefaults: ModelDefaults{
			FallbackModel:     "Aetox0.0.1:0b",
			RecommendedModels: []string{"Aetox0.0.1:0b"},
		},
		capabilities: Capabilities{},
	},
	"openrouter": {
		canonical:      "openrouter",
		aliases:        []string{"openrouter", "open-router", "openrouterai", "or"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://openrouter.ai/api/v1",
		envKeys:        []string{"OPENROUTER_API_KEY"},
		modelDefaults: ModelDefaults{
			FallbackModel: "deepseek/deepseek-r1",
		},
		capabilities: Capabilities{ToolCalling: true, Reasoning: true},
	},
	"openai": {
		canonical:      "openai",
		aliases:        []string{"openai", "chatgpt", "gpt", "openai-compatible", "compatible"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.openai.com/v1",
		envKeys:        []string{"OPENAI_API_KEY", "OPENAI_TOKEN"},
		modelDefaults:  ModelDefaults{FallbackModel: "gpt-4o-mini"},
		capabilities:   Capabilities{ToolCalling: true, Reasoning: true},
	},
	"deepseek": {
		canonical:      "deepseek",
		aliases:        []string{"deepseek", "deepseek-api", "deepseek-ai"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.deepseek.com",
		envKeys:        []string{"DEEPSEEK_API_KEY"},
		modelDefaults:  ModelDefaults{FallbackModel: "deepseek-v4-flash"},
		capabilities:   Capabilities{ToolCalling: true, Reasoning: true},
	},
	"zai": {
		canonical:      "zai",
		aliases:        []string{"zai", "z.ai", "zhipu", "zhipuai"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.z.ai/api/paas/v4",
		envKeys:        []string{"ZAI_API_KEY"},
		modelDefaults:  ModelDefaults{FallbackModel: "glm-4.6"},
		capabilities:   Capabilities{ToolCalling: true},
	},
	"gemini": {
		canonical:      "gemini",
		aliases:        []string{"gemini", "google", "google-ai", "googleai", "google-gemini"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://generativelanguage.googleapis.com/v1beta/openai",
		envKeys:        []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"},
		modelDefaults: ModelDefaults{
			FallbackModel: "gemini-2.5-flash-lite",
			RecommendedModels: []string{
				"gemini-2.5-flash-lite",
				"gemini-2.5-flash",
				"gemini-3.5-flash",
				"gemini-2.0-flash-lite",
				"gemini-2.5-pro",
			},
		},
		capabilities: Capabilities{ToolCalling: true, Reasoning: true},
	},
	"groq": {
		canonical:      "groq",
		aliases:        []string{"groq", "groqcloud"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.groq.com/openai/v1",
		envKeys:        []string{"GROQ_API_KEY"},
		modelDefaults:  ModelDefaults{FallbackModel: "llama-3.3-70b-versatile"},
		capabilities:   Capabilities{ToolCalling: true, Reasoning: true},
	},
	"mistral": {
		canonical:      "mistral",
		aliases:        []string{"mistral", "mistralai"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.mistral.ai/v1",
		envKeys:        []string{"MISTRAL_API_KEY"},
		modelDefaults:  ModelDefaults{FallbackModel: "mistral-small"},
		capabilities:   Capabilities{ToolCalling: true},
	},
	"together": {
		canonical:      "together",
		aliases:        []string{"together", "togetherai", "together-ai"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.together.xyz/v1",
		envKeys:        []string{"TOGETHER_API_KEY"},
		modelDefaults:  ModelDefaults{FallbackModel: "google/gemma-2-9b-it"},
		capabilities:   Capabilities{ToolCalling: true},
	},
	"perplexity": {
		canonical:      "perplexity",
		aliases:        []string{"perplexity", "perplexityai", "pplx"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.perplexity.ai",
		envKeys:        []string{"PERPLEXITY_API_KEY"},
		modelDefaults:  ModelDefaults{FallbackModel: "llama-3.1-sonar-small-128k-online"},
		capabilities:   Capabilities{ToolCalling: true},
	},
	"cohere": {
		canonical:      "cohere",
		aliases:        []string{"cohere", "command-r"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.cohere.com/v1",
		envKeys:        []string{"COHERE_API_KEY"},
		modelDefaults:  ModelDefaults{FallbackModel: "command-r-plus"},
		capabilities:   Capabilities{ToolCalling: true},
	},
	"lmstudio": {
		canonical:      "lmstudio",
		aliases:        []string{"lmstudio", "localai", "local-ai"},
		requiresAPIKey: false,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "http://localhost:1234/v1",
		envKeys:        []string{"LLM_API_KEY", "OPENAI_API_KEY"},
		modelDefaults:  ModelDefaults{FallbackModel: "local-model"},
		capabilities:   Capabilities{ToolCalling: true},
	},
	"ollama": {
		canonical:      "ollama",
		aliases:        []string{"ollama", "ollamaai"},
		requiresAPIKey: false,
		runtime:        RuntimeOllama,
		baseURL:        "http://localhost:11434",
		envKeys:        nil,
		modelDefaults: ModelDefaults{
			FallbackModel: "gemma3:4b",
		},
		capabilities: Capabilities{ToolCalling: true},
	},
	"anthropic": {
		canonical:      "anthropic",
		aliases:        []string{"anthropic", "claude"},
		requiresAPIKey: true,
		runtime:        RuntimeAnthropic,
		baseURL:        "https://api.anthropic.com/v1",
		envKeys:        []string{"ANTHROPIC_API_KEY"},
		modelDefaults: ModelDefaults{
			FallbackModel: "claude-haiku-4-5",
			RecommendedModels: []string{
				"claude-opus-4-8",
				"claude-sonnet-5",
				"claude-haiku-4-5",
			},
		},
		capabilities: Capabilities{ToolCalling: true, Reasoning: true},
	},
}

var canonicalOrder = sortedCanonicalKeys()

func sortedCanonicalKeys() []string {
	out := make([]string, 0, len(catalog))
	for k := range catalog {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// Resolution and lookup
// ---------------------------------------------------------------------------

// Normalize resolves any alias or canonical name to its canonical
// provider identifier. Returns the input unchanged if it is not a known
// alias.
func Normalize(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return "noop"
	}
	for canonical, e := range catalog {
		for _, alias := range e.aliases {
			if key == alias {
				return canonical
			}
		}
	}
	return key
}

// Lookup returns the full Spec for a provider (by canonical or alias
// name). The second return value is false when the provider is unknown.
func Lookup(name string) (Spec, bool) {
	canonical := Normalize(name)
	e, ok := catalog[canonical]
	if !ok {
		return Spec{}, false
	}
	return Spec{
		Canonical:      canonical,
		Aliases:        append([]string{}, e.aliases...),
		RequiresAPIKey: e.requiresAPIKey,
		Runtime:        e.runtime,
		BaseURL:        e.baseURL,
		EnvKeys:        append([]string{}, e.envKeys...),
		ModelDefaults:  e.modelDefaults,
		Capabilities:   e.capabilities,
	}, true
}

// SupportedProviders returns the canonical IDs of every registered
// provider, in sorted order.
func SupportedProviders() []string {
	return append([]string{}, canonicalOrder...)
}

// RequiresAPIKey reports whether the provider (by canonical or alias
// name) requires an API key to operate.
func RequiresAPIKey(name string) bool {
	s, ok := Lookup(name)
	return ok && s.RequiresAPIKey
}

// DefaultModel returns the static fallback model name for a provider, or
// "" for unknown providers. This is not authoritative — the actual model
// should come from user configuration or live model discovery first.
func DefaultModel(name string) string {
	s, ok := Lookup(name)
	if !ok {
		return ""
	}
	return s.ModelDefaults.FallbackModel
}

// RecommendedModels returns the optional short list of well-known models
// for a provider, or nil for unknown providers. This list is a static
// discovery hint, not a complete or guaranteed catalog.
func RecommendedModels(name string) []string {
	s, ok := Lookup(name)
	if !ok || len(s.ModelDefaults.RecommendedModels) == 0 {
		return nil
	}
	return append([]string{}, s.ModelDefaults.RecommendedModels...)
}

// DefaultBaseURL returns the standard API endpoint for a provider, or ""
// for unknown providers.
func DefaultBaseURL(name string) string {
	s, ok := Lookup(name)
	if !ok {
		return ""
	}
	return s.BaseURL
}

// Runtime returns the Runtime class for a provider, or "" if unknown.
func RuntimeFor(name string) Runtime {
	s, ok := Lookup(name)
	if !ok {
		return ""
	}
	return s.Runtime
}

// ---------------------------------------------------------------------------
// Validation and helpers
// ---------------------------------------------------------------------------

// ResolveAPIKey reads the first non-empty environment variable from the
// provider's configured EnvKeys list.
func ResolveAPIKey(name string) string {
	s, ok := Lookup(name)
	if !ok || len(s.EnvKeys) == 0 {
		return ""
	}
	for _, key := range s.EnvKeys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// MenuLabel returns the display label used in provider-selection menus.
func MenuLabel(name string, keyFound bool) string {
	label := strings.TrimSpace(name)
	if label == "" {
		return "(unknown)"
	}
	if keyFound {
		return label + " (env key found)"
	}
	return label + " (needs key)"
}
