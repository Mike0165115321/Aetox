package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/oauth"
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
	// Locale ("th", "en") is consumed by the noop runtime alone — Aetox's own
	// built-in provider, which is an onboarding surface rather than a model and
	// therefore has to speak the user's language (ARCHITECTURE.md §40). Every
	// other runtime below ignores it.
	Locale string
}

func NewProvider(opts ProviderOptions) (Provider, error) {
	provider := NormalizeProvider(opts.Provider)
	if provider == "" {
		provider = "aetox"
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
		// The default-format URL is wrong for the alt wire format — only a
		// user-customized URL survives the switch.
		if baseURL == "" || baseURL == info.BaseURL {
			baseURL = info.AltBaseURL
		}
	} else if info.AltBaseURL != "" && baseURL == info.AltBaseURL {
		// Symmetric: a stale alt-format URL under the default wire format.
		baseURL = info.BaseURL
	}

	// A sign-in (internal/oauth) outranks a pasted key for the same provider:
	// the user did the more deliberate thing, and the two would otherwise
	// disagree silently. Every provider nobody has signed into gets a nil
	// tokenSource, which is exactly the path that existed before.
	tokenSource := oauth.TokenSource(provider)
	if endpoint := oauth.Endpoint(provider); endpoint != "" && (baseURL == "" || baseURL == info.BaseURL) {
		// Some sign-ins name the host the account is served from (Qwen). A
		// base URL the user typed themselves still wins over it.
		baseURL = endpoint
	}
	opts.BaseURL = baseURL

	switch runtime {
	case string(pvdr.RuntimeNoop):
		noop := NewNoopProvider(opts.Model)
		noop.Locale = opts.Locale
		return noop, nil
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
			TokenSource:   tokenSource,
		})
	case string(pvdr.RuntimeCodeAssist):
		return NewCodeAssistProvider(CodeAssistConfig{
			Provider:    provider,
			Model:       opts.Model,
			BaseURL:     opts.BaseURL,
			Project:     oauth.CodeAssistProject(),
			Timeout:     timeout,
			TokenSource: tokenSource,
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
