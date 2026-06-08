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

	info, ok := LookupProviderInfo(provider)
	if !ok {
		return nil, fmt.Errorf("unsupported model provider: %q", provider)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	requireAPIKey := info.RequiresAPIKey
	switch info.Runtime {
	case string(providerRuntimeNoop):
		return NewNoopProvider(opts.Model), nil
	case string(providerRuntimeOllama):
		return NewOllamaProvider(OllamaConfig{
			Model:   opts.Model,
			BaseURL: opts.BaseURL,
			Timeout: timeout,
		})
	case string(providerRuntimeOpenAICompatible):
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      provider,
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: &requireAPIKey,
		})
	default:
		return nil, fmt.Errorf("unsupported model provider: %q", provider)
	}
}
