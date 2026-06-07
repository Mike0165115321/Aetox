package model

import (
	"fmt"
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
	provider := NormalizeProvider(opts.Provider)
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
	case "noop":
		return NewNoopProvider(opts.Model), nil
	case "openrouter":
		return NewOpenRouterProvider(OpenRouterConfig{
			Model:   opts.Model,
			APIKey:  opts.APIKey,
			BaseURL: opts.BaseURL,
			Timeout: timeout,
		})
	case "openai":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "openai",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "deepseek":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "deepseek",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "groq":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "groq",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "mistral":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "mistral",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "together":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "together",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "perplexity":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "perplexity",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "cohere":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      "cohere",
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: requireAPIKey(true),
		})
	case "lmstudio":
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider: provider,
			Model:    opts.Model,
			APIKey:   opts.APIKey,
			BaseURL:  opts.BaseURL,
			Timeout:  timeout,
			// LocalAI/LMStudio usually don't require cloud keys
			RequireAPIKey: requireAPIKey(false),
		})
	case "ollama":
		return NewOllamaProvider(OllamaConfig{
			Model:   opts.Model,
			BaseURL: opts.BaseURL,
			Timeout: timeout,
		})
	default:
		return nil, fmt.Errorf("unsupported model provider: %q", provider)
	}
}
