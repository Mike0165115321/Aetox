package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"aetox-cli/internal/provider"
)

// ProviderMetadata is the public metadata shape exposed by the model
// package for backward compatibility. It delegates to provider.Spec.
type ProviderMetadata struct {
	Canonical      string
	Aliases        []string
	RequiresAPIKey bool
	Runtime        string
	DefaultModel   string
	BaseURL        string
	ModelChoices   []string
	EnvKeys        []string
}

// NormalizeProvider delegates to provider.Normalize.
func NormalizeProvider(name string) string {
	return provider.Normalize(name)
}

// ProviderInfo delegates to provider.Lookup and converts the
// result to ProviderMetadata for backward compatibility.
func ProviderInfo(name string) (ProviderMetadata, bool) {
	spec, ok := provider.Lookup(name)
	if !ok {
		return ProviderMetadata{}, false
	}
	return ProviderMetadata{
		Canonical:      spec.Canonical,
		Aliases:        spec.Aliases,
		RequiresAPIKey: spec.RequiresAPIKey,
		Runtime:        string(spec.Runtime),
		DefaultModel:   spec.ModelDefaults.FallbackModel,
		BaseURL:        spec.BaseURL,
		ModelChoices:   spec.ModelDefaults.RecommendedModels,
		EnvKeys:        spec.EnvKeys,
	}, true
}

// LookupProviderInfo delegates to ProviderInfo.
func LookupProviderInfo(name string) (ProviderMetadata, bool) {
	return ProviderInfo(name)
}

// SupportedProviders delegates to provider.SupportedProviders.
func SupportedProviders() []string {
	return provider.SupportedProviders()
}

// RequiresAPIKey delegates to provider.RequiresAPIKey.
func RequiresAPIKey(name string) bool {
	return provider.RequiresAPIKey(name)
}

// DefaultModel delegates to provider.DefaultModel.
func DefaultModel(name string) string {
	return provider.DefaultModel(name)
}

// DefaultBaseURL delegates to provider.DefaultBaseURL.
func DefaultBaseURL(name string) string {
	return provider.DefaultBaseURL(name)
}

// RuntimeForProvider returns the runtime class as a string by delegating
// to provider.RuntimeFor.
func RuntimeForProvider(name string) string {
	return string(provider.RuntimeFor(name))
}

// ModelChoices returns the static recommended-model list for a provider.
// This is a fallback hint only; live model lists should be fetched via
// ModelChoicesWithEndpointAndAPIKey when possible.
func ModelChoices(name string) []string {
	return provider.RecommendedModels(name)
}

// ResolveModelAPIKey delegates to provider.ResolveAPIKey.
func ResolveModelAPIKey(name string) string {
	return provider.ResolveAPIKey(name)
}

// FormatProviderMenuLabel delegates to provider.MenuLabel.
func FormatProviderMenuLabel(name string, keyFound bool) string {
	return provider.MenuLabel(name, keyFound)
}

// FormatSupportedProviderMenu delegates to FormatProviderMenuLabel.
func FormatSupportedProviderMenu(name string, keyFound bool) string {
	return FormatProviderMenuLabel(name, keyFound)
}

// ProviderEnvKeys delegates to provider.EnvKeys.
func ProviderEnvKeys() []string {
	return provider.EnvKeys()
}

// ResolveStatus builds a human-readable status line for a provider/model
// combination.
func ResolveStatus(p, model string, _ error) string {
	canonical := provider.Normalize(p)
	if canonical == "" {
		canonical = "noop"
	}
	label := resolveStatusModelLabel(canonical, strings.TrimSpace(model))
	return canonical + "/" + label
}

func resolveStatusModelLabel(prov, model string) string {
	if model == "" || strings.EqualFold(model, "default") {
		if value := provider.DefaultModel(prov); value != "" {
			switch prov {
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

// ---------------------------------------------------------------------------
// Live model discovery (HTTP) — stays in internal/model
// ---------------------------------------------------------------------------

func ModelChoicesWithEndpointAndAPIKEY(p, baseURL, apiKey string) ([]string, error) {
	return ModelChoicesWithEndpointAndAPIKey(p, baseURL, apiKey)
}

// ModelChoicesWithEndpointAndAPIKey fetches model names from the
// provider's API. This is the live discovery path; static fallbacks
// live in provider.RecommendedModels.
func ModelChoicesWithEndpointAndAPIKey(p, baseURL, apiKey string) ([]string, error) {
	canonical := provider.Normalize(p)
	switch provider.RuntimeFor(canonical) {
	case provider.RuntimeOllama:
		models, err := DiscoverOllamaModels(baseURL)
		if err == nil && len(models) > 0 {
			return models, nil
		}
		return nil, err
	case provider.RuntimeOpenAICompatible:
		if canonical == "gemini" {
			models, err := DiscoverGeminiModels(baseURL, apiKey)
			if err == nil && len(models) > 0 {
				return models, nil
			}
			return nil, err
		}
		models, err := DiscoverOpenAICompatibleModels(canonical, baseURL, apiKey)
		if err == nil && len(models) > 0 {
			return models, nil
		}
		return nil, err
	default:
		return nil, fmt.Errorf("provider %q does not support remote model discovery", canonical)
	}
}

// ModelChoicesWithEndpoint delegates to ModelChoicesWithEndpointAndAPIKey
// using the resolved API key.
func ModelChoicesWithEndpoint(p, baseURL string) ([]string, error) {
	return ModelChoicesWithEndpointAndAPIKey(p, baseURL, provider.ResolveAPIKey(p))
}

// ---------------------------------------------------------------------------
// HTTP discovery helpers
// ---------------------------------------------------------------------------

type ollamaTagResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func DiscoverOllamaModels(baseURL string) ([]string, error) {
	endpoint := strings.TrimSpace(baseURL)
	if endpoint == "" {
		endpoint = provider.DefaultBaseURL("ollama")
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

type openAIModel struct {
	ID string `json:"id"`
}

type openAIModelsResponse struct {
	Data []openAIModel `json:"data"`
}

func DiscoverOpenAICompatibleModels(p, baseURL, apiKey string) ([]string, error) {
	if p == "" {
		p = "openai"
	}

	endpoint := strings.TrimSpace(baseURL)
	if endpoint == "" {
		endpoint = provider.DefaultBaseURL(p)
	}
	if endpoint == "" {
		return nil, fmt.Errorf("provider %q missing base URL", p)
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/models"

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s models endpoint failed with status %d: %s", p, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload openAIModelsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%s models response parse failed: %w", p, err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("%s models endpoint returned no models", p)
	}

	seen := make(map[string]struct{}, len(payload.Data))
	result := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil, fmt.Errorf("%s models endpoint returned no valid IDs", p)
	}
	return result, nil
}

type geminiModel struct {
	BaseModelID                string   `json:"baseModelId"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

type geminiModelsResponse struct {
	Models []geminiModel `json:"models"`
}

func DiscoverGeminiModels(baseURL, apiKey string) ([]string, error) {
	endpoint := strings.TrimSpace(baseURL)
	if endpoint == "" {
		endpoint = provider.DefaultBaseURL("gemini")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(endpoint, "/openai") {
		endpoint = strings.TrimSuffix(endpoint, "/openai")
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/models"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	query := req.URL.Query()
	if strings.TrimSpace(apiKey) != "" {
		query.Set("key", strings.TrimSpace(apiKey))
	}
	req.URL.RawQuery = query.Encode()

	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini models endpoint failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload geminiModelsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("gemini models response parse failed: %w", err)
	}
	seen := make(map[string]struct{}, len(payload.Models))
	result := make([]string, 0, len(payload.Models))
	for _, item := range payload.Models {
		if !supportsGeminiGenerateContent(item.SupportedGenerationMethods) {
			continue
		}
		id := strings.TrimSpace(item.BaseModelID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil, fmt.Errorf("gemini models endpoint returned no valid IDs")
	}
	return result, nil
}

func supportsGeminiGenerateContent(methods []string) bool {
	for _, method := range methods {
		if strings.EqualFold(strings.TrimSpace(method), "generateContent") {
			return true
		}
	}
	return false
}
