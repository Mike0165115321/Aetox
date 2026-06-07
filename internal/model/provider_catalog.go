package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type ProviderMetadata struct {
	Canonical      string
	Aliases        []string
	RequiresAPIKey bool
	DefaultModel   string
	BaseURL        string
	ModelChoices   []string
	EnvKeys        []string
}

type providerCatalogEntry struct {
	aliases        []string
	requiresAPIKey bool
	defaultModel   string
	baseURL        string
	modelChoices   []string
	envKeys        []string
}

var providerCatalog = map[string]providerCatalogEntry{
	"noop": {
		aliases:        []string{"noop", "none", "stub"},
		requiresAPIKey: false,
		defaultModel:   "noop",
		baseURL:        "",
		modelChoices:   nil,
		envKeys:        nil,
	},
	"openrouter": {
		aliases:        []string{"openrouter", "open-router", "openrouterai", "or"},
		requiresAPIKey: true,
		defaultModel:   "deepseek/deepseek-r1",
		baseURL:        "https://openrouter.ai/api/v1",
		modelChoices: []string{
			"deepseek/deepseek-r1",
			"deepseek/deepseek-chat",
			"deepseek/deepseek-coder",
			"google/gemini-2.0-flash-001",
			"openai/gpt-4o-mini",
			"openai/gpt-4o",
			"meta-llama/llama-4-maverick-17b-128e-instruct",
			"mistralai/mixtral-8x22b-instruct",
		},
		envKeys: []string{
			"OPENROUTER_API_KEY",
		},
	},
	"openai": {
		aliases:        []string{"openai", "chatgpt", "gpt", "openai-compatible", "compatible"},
		requiresAPIKey: true,
		defaultModel:   "gpt-4o-mini",
		baseURL:        "https://api.openai.com/v1",
		modelChoices: []string{
			"gpt-4o-mini",
			"gpt-4o",
			"gpt-4.1",
			"gpt-4.1-mini",
			"o4-mini",
		},
		envKeys: []string{
			"OPENAI_API_KEY",
			"OPENAI_TOKEN",
		},
	},
	"deepseek": {
		aliases:        []string{"deepseek", "deepseek-api", "deepseek-ai"},
		requiresAPIKey: true,
		defaultModel:   "deepseek-chat",
		baseURL:        "https://api.deepseek.com/v1",
		modelChoices: []string{
			"deepseek-chat",
			"deepseek-coder",
			"deepseek-reasoner",
		},
		envKeys: []string{
			"DEEPSEEK_API_KEY",
		},
	},
	"groq": {
		aliases:        []string{"groq", "groqcloud"},
		requiresAPIKey: true,
		defaultModel:   "llama-3.3-70b-versatile",
		baseURL:        "https://api.groq.com/openai/v1",
		modelChoices: []string{
			"llama-3.3-70b-versatile",
			"llama-3.1-70b-versatile",
			"llama-3.1-8b-instant",
			"mixtral-8x7b-32768",
		},
		envKeys: []string{
			"GROQ_API_KEY",
		},
	},
	"mistral": {
		aliases:        []string{"mistral", "mistralai"},
		requiresAPIKey: true,
		defaultModel:   "mistral-small",
		baseURL:        "https://api.mistral.ai/v1",
		modelChoices: []string{
			"mistral-small",
			"mistral-small-3.2",
			"ministral-8b",
			"pixtral-large",
		},
		envKeys: []string{
			"MISTRAL_API_KEY",
		},
	},
	"together": {
		aliases:        []string{"together", "togetherai", "together-ai"},
		requiresAPIKey: true,
		defaultModel:   "google/gemma-2-9b-it",
		baseURL:        "https://api.together.xyz/v1",
		modelChoices: []string{
			"google/gemma-2-27b-it",
			"meta-llama/Llama-3-70b-chat-hf",
			"meta-llama/Llama-3-8b-chat-hf",
		},
		envKeys: []string{
			"TOGETHER_API_KEY",
		},
	},
	"perplexity": {
		aliases:        []string{"perplexity", "perplexityai", "pplx"},
		requiresAPIKey: true,
		defaultModel:   "llama-3.1-sonar-small-128k-online",
		baseURL:        "https://api.perplexity.ai",
		modelChoices: []string{
			"llama-3.1-sonar-small-128k-online",
			"llama-3.1-sonar-large-128k-online",
			"llama-3.1-sonar-huge-128k-online",
		},
		envKeys: []string{
			"PERPLEXITY_API_KEY",
		},
	},
	"cohere": {
		aliases:        []string{"cohere", "command-r"},
		requiresAPIKey: true,
		defaultModel:   "command-r-plus",
		baseURL:        "https://api.cohere.com/v1",
		modelChoices: []string{
			"command-r-plus",
			"command-r",
			"command-r7b-12-2024",
		},
		envKeys: []string{
			"COHERE_API_KEY",
		},
	},
	"lmstudio": {
		aliases:        []string{"lmstudio", "localai", "local-ai"},
		requiresAPIKey: false,
		defaultModel:   "local-model",
		baseURL:        "http://localhost:1234/v1",
		modelChoices: []string{
			"local-model",
		},
		envKeys: []string{
			"LLM_API_KEY",
			"OPENAI_API_KEY",
		},
	},
	"ollama": {
		aliases:        []string{"ollama", "ollamaai"},
		requiresAPIKey: false,
		defaultModel:   "gemma3:4b",
		baseURL:        "http://localhost:11434",
		modelChoices: []string{
			"gemma3:4b",
			"qwen2.5:7b",
			"llama3.1:8b",
			"llama3.1:70b",
		},
		envKeys: nil,
	},
}

var canonicalProviderOrder = []string{
	"openrouter",
	"ollama",
}

func NormalizeProvider(provider string) string {
	key := strings.ToLower(strings.TrimSpace(provider))
	if key == "" {
		return "noop"
	}

	for canonical, info := range providerCatalog {
		for _, alias := range info.aliases {
			if key == alias {
				return canonical
			}
		}
	}

	return key
}

func ProviderInfo(provider string) (ProviderMetadata, bool) {
	canonical := NormalizeProvider(provider)
	info, ok := providerCatalog[canonical]
	if !ok {
		return ProviderMetadata{}, false
	}

	return ProviderMetadata{
		Canonical:      canonical,
		Aliases:        append([]string{}, info.aliases...),
		RequiresAPIKey: info.requiresAPIKey,
		DefaultModel:   info.defaultModel,
		BaseURL:        info.baseURL,
		ModelChoices:   append([]string{}, info.modelChoices...),
		EnvKeys:        append([]string{}, info.envKeys...),
	}, true
}

func LookupProviderInfo(provider string) (ProviderMetadata, bool) {
	return ProviderInfo(provider)
}

func SupportedProviders() []string {
	return append([]string{}, canonicalProviderOrder...)
}

func RequiresAPIKey(provider string) bool {
	info, ok := LookupProviderInfo(provider)
	return ok && info.RequiresAPIKey
}

func DefaultModel(provider string) string {
	info, ok := LookupProviderInfo(provider)
	if !ok {
		return ""
	}
	return info.DefaultModel
}

func DefaultBaseURL(provider string) string {
	info, ok := LookupProviderInfo(provider)
	if !ok {
		return ""
	}
	return info.BaseURL
}

func ModelChoices(provider string) []string {
	info, ok := LookupProviderInfo(provider)
	if !ok {
		return nil
	}
	return append([]string{}, info.ModelChoices...)
}

func ResolveModelAPIKey(provider string) string {
	info, ok := LookupProviderInfo(provider)
	if !ok || len(info.EnvKeys) == 0 {
		return ""
	}
	for _, key := range info.EnvKeys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func ModelChoicesWithEndpoint(provider string, baseURL string) ([]string, error) {
	canonical := NormalizeProvider(provider)
	switch canonical {
	case "ollama":
		models, err := DiscoverOllamaModels(baseURL)
		if err == nil && len(models) > 0 {
			return models, nil
		}
		return nil, err
	default:
		return ModelChoices(canonical), nil
	}
}

func ResolveStatus(provider, model string, _ error) string {
	canonical := NormalizeProvider(provider)
	if canonical == "" {
		canonical = "noop"
	}
	label := resolveStatusModelLabel(canonical, strings.TrimSpace(model))
	status := canonical + "/" + label
	return status
}

func resolveStatusModelLabel(provider, model string) string {
	if model == "" || strings.EqualFold(model, "default") {
		if value := DefaultModel(provider); value != "" {
			switch provider {
			case "openrouter":
				return "openrouter default"
			default:
				return value
			}
		}
		return "default"
	}
	return model
}

func FormatProviderMenuLabel(provider string, keyFound bool) string {
	label := provider
	if keyFound {
		return label + " (env key found)"
	}
	return label + " (needs key)"
}

func FormatSupportedProviderMenu(provider string, keyFound bool) string {
	return FormatProviderMenuLabel(provider, keyFound)
}

type ollamaTagResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func DiscoverOllamaModels(baseURL string) ([]string, error) {
	endpoint := strings.TrimSpace(baseURL)
	if endpoint == "" {
		endpoint = DefaultBaseURL("ollama")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/api/tags"

	ctxClient := &http.Client{Timeout: 2 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := ctxClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var payload ollamaTagResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(payload.Models))
	result := make([]string, 0, len(payload.Models))
	for _, model := range payload.Models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}

	sort.Strings(result)
	return result, nil
}
