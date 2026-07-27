package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type BootstrapOptions struct {
	Provider   string
	Model      string
	APIKey     string
	BaseURL    string
	Timeout    time.Duration
	WireFormat string
	// Locale reaches exactly one provider — Aetox's own built-in one, which
	// talks to a user who has configured nothing yet. Every real provider
	// ignores it (ARCHITECTURE.md §40).
	Locale string
}

type BootstrapResult struct {
	Provider Provider
	Warning  string
	Error    error
}

func BootstrapProvider(opts BootstrapOptions) BootstrapResult {
	provider, initErr := NewProvider(ProviderOptions{
		Provider:   opts.Provider,
		Model:      opts.Model,
		APIKey:     opts.APIKey,
		BaseURL:    opts.BaseURL,
		Timeout:    opts.Timeout,
		WireFormat: opts.WireFormat,
		Locale:     opts.Locale,
	})
	if initErr == nil {
		return BootstrapResult{
			Provider: provider,
			Warning:  "",
			Error:    nil,
		}
	}
	initErr = clarifyBootstrapError(opts, initErr)

	fallback, fallbackErr := NewProvider(ProviderOptions{
		Provider: "aetox",
		Model:    "aetox",
		Locale:   opts.Locale,
	})
	if fallbackErr != nil {
		return BootstrapResult{
			Provider: nil,
			Warning:  "cannot initialize aetox fallback provider",
			Error:    initErr,
		}
	}

	return BootstrapResult{
		Provider: fallback,
		Warning:  "model provider unavailable; using aetox fallback",
		Error:    initErr,
	}
}

// clarifyBootstrapError replaces "model name is required" with the reason a
// local runtime actually produced it. Ollama and LM Studio carry no catalog
// fallback model on purpose, so ResolveDefaultModel asks the server for a name
// and honestly returns "" when nothing answers — an empty model name on such a
// provider means the server is down or has nothing loaded, not that the user
// forgot to type one. Naming the endpoint is the whole point: "model name is
// required" sent people looking in the wrong place.
func clarifyBootstrapError(opts BootstrapOptions, err error) error {
	canonical := NormalizeProvider(opts.Provider)
	if !errors.Is(err, ErrMissingModel) || strings.TrimSpace(opts.Model) != "" || DefaultModel(canonical) != "" {
		return err
	}
	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL(canonical)
	}
	return fmt.Errorf("%s offered no model at %s — start its server and load a model", canonical, baseURL)
}
