package model

import (
	"time"
)

type BootstrapOptions struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	Timeout  time.Duration
}

type BootstrapResult struct {
	Provider Provider
	Warning  string
	Error    error
}

func BootstrapProvider(opts BootstrapOptions) BootstrapResult {
	provider, initErr := NewProvider(ProviderOptions{
		Provider: opts.Provider,
		Model:    opts.Model,
		APIKey:   opts.APIKey,
		BaseURL:  opts.BaseURL,
		Timeout:  opts.Timeout,
	})
	if initErr == nil {
		return BootstrapResult{
			Provider: provider,
			Warning:  "",
			Error:    nil,
		}
	}

	fallback, fallbackErr := NewProvider(ProviderOptions{
		Provider: "noop",
		Model:    "noop",
	})
	if fallbackErr != nil {
		return BootstrapResult{
			Provider: nil,
			Warning:  "cannot initialize noop fallback provider",
			Error:    initErr,
		}
	}

	return BootstrapResult{
		Provider: fallback,
		Warning:  "model provider unavailable; using noop fallback",
		Error:    initErr,
	}
}
