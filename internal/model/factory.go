package model

import (
	"fmt"
	"time"

	pvdr "github.com/Mike0165115321/Aetox/internal/provider"
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
	case string(pvdr.RuntimeNoop):
		return NewNoopProvider(opts.Model), nil
	case string(pvdr.RuntimeOllama):
		return NewOllamaProvider(OllamaConfig{
			Model:   opts.Model,
			BaseURL: opts.BaseURL,
			Timeout: timeout,
		})
	case string(pvdr.RuntimeOpenAICompatible):
		return NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider:      provider,
			Model:         opts.Model,
			APIKey:        opts.APIKey,
			BaseURL:       opts.BaseURL,
			Timeout:       timeout,
			RequireAPIKey: &requireAPIKey,
		})
	case string(pvdr.RuntimeAnthropic):
		return NewAnthropicProvider(AnthropicConfig{
			Model:   opts.Model,
			APIKey:  opts.APIKey,
			BaseURL: opts.BaseURL,
			Timeout: timeout,
		})
	default:
		return nil, fmt.Errorf("unsupported model provider: %q", provider)
	}
}
