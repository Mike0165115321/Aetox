package model

import (
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
