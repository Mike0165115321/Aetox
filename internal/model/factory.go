package model

import (
	"fmt"
	"strings"
	"time"

	pvdr "github.com/Mike0165115321/Aetox/internal/provider"
)

type ProviderOptions struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	Timeout  time.Duration
	// WireFormat picks between a provider's two wire formats when it has one
	// (ProviderMetadata.AltRuntime/AltBaseURL) — e.g. "openai-compatible" or
	// "anthropic" for DeepSeek. Empty uses the catalog's default runtime.
	// Unknown values, or a provider with no alt, are ignored (default wins).
	WireFormat string
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

	runtime := info.Runtime
	baseURL := opts.BaseURL
	if wf := strings.TrimSpace(opts.WireFormat); wf != "" && info.AltRuntime != "" && wf == info.AltRuntime {
		runtime = info.AltRuntime
		if baseURL == "" {
			baseURL = info.AltBaseURL
		}
	}
	opts.BaseURL = baseURL

	switch runtime {
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
			Provider: provider,
			Model:    opts.Model,
			APIKey:   opts.APIKey,
			BaseURL:  opts.BaseURL,
			Timeout:  timeout,
		})
	default:
		return nil, fmt.Errorf("unsupported model provider: %q", provider)
	}
}
