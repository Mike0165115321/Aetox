package model

import (
	"fmt"
	"strings"
	"time"
)

type ProviderOptions struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	Timeout  time.Duration
}

func NewProvider(opts ProviderOptions) (Provider, error) {
	provider := strings.ToLower(strings.TrimSpace(opts.Provider))
	if provider == "" {
		provider = "noop"
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	requireAPIKey := func(v bool) *bool {
		return &v
	}

	switch provider {
	case "noop", "none", "stub":
		return NewNoopProvider(opts.Model), nil
	case "openrouter", "open-router", "openrouterai", "or":
		return NewOpenRouterProvider(OpenRouterConfig{
			Model:   opts.Model,
			APIKey:  opts.APIKey,
			BaseURL: opts.BaseURL,
			Timeout: timeout,
		})
	case "openai", "gpt", "chatgpt", "openai-compatible", "compatible":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "openai",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "deepseek", "deepseek-api", "deepseek-ai":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "deepseek",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "groq", "groqcloud":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "groq",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "mistral", "mistralai":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "mistral",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "together", "togetherai", "together-ai":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "together",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "perplexity", "perplexityai", "pplx":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "perplexity",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "cohere", "command-r":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "cohere",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "lmstudio", "localai", "local-ai":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider: provider,
			Model:    opts.Model,
			APIKey:   opts.APIKey,
			BaseURL:  opts.BaseURL,
			Timeout:  timeout,
			// LocalAI/LMStudio usually don't require cloud keys
			RequireAPIKey: requireAPIKey(false),
		})
	case "ollama", "ollamaai":
		return NewOllamaProvider(OllamaConfig{
			Model:   opts.Model,
			BaseURL: opts.BaseURL,
			Timeout: timeout,
		})
	default:
		return nil, fmt.Errorf("unsupported model provider: %q", provider)
	}
}
