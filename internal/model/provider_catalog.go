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

	"github.com/Mike0165115321/Aetox/internal/oauth"
	"github.com/Mike0165115321/Aetox/internal/provider"
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
	// AltRuntime/AltBaseURL describe a second wire format this provider's API
	// also speaks (see provider.Spec.AltRuntime); empty when there is only one.
	AltRuntime   string
	AltBaseURL   string
	ModelChoices []string
	EnvKeys      []string
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
		AltRuntime:     string(spec.AltRuntime),
		AltBaseURL:     spec.AltBaseURL,
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

// DefaultModel delegates to provider.DefaultModel. It is a pure catalog
// lookup — use it for capability tables keyed by model id. To pick a model to
// actually run on, use ResolveDefaultModel.
func DefaultModel(name string) string {
	return provider.DefaultModel(name)
}

// ResolveDefaultModel picks the model a provider should start on when nothing
// is configured yet. Providers serving a published catalog carry a static
// fallback and use it. Local runtimes (Ollama, LM Studio) serve whatever the
// user installed, so the catalog carries no fallback for them and the name
// comes from the server itself. Returns "" when the server has nothing to
// offer — an honest empty beats a hardcoded guess that fails as "model not
// found" against a server that is working fine.
func ResolveDefaultModel(p, baseURL, apiKey string) string {
	canonical := provider.Normalize(p)
	if strings.TrimSpace(baseURL) == "" {
		baseURL = provider.DefaultBaseURL(canonical)
	}
	// Ask the runtime what it actually has loaded before falling back to
	// picking off a list. The list is sorted for the picker, so its first entry
	// is only "alphabetically first" — reading a default out of it silently
	// answers "which model is this server serving?" with a guess, and on LM
	// Studio that means addressing a model that is not in memory.
	if active := activeLocalModel(canonical, baseURL, apiKey); active != "" {
		return active
	}
	if models, err := ModelChoicesWithEndpointAndAPIKey(canonical, baseURL, apiKey); err == nil && len(models) > 0 {
		return models[0]
	}
	// The catalog's name is the last resort, not the first answer. It used to be
	// returned before anything was asked, which meant a model name written here
	// months ago outranked the list the provider was offering right now — and a
	// stale one is a 404 that reads like a bug in Aetox.
	return provider.DefaultModel(canonical)
}

// DefaultBaseURL delegates to provider.DefaultBaseURL.
func DefaultBaseURL(name string) string {
	return provider.DefaultBaseURL(name)
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

// ResolveStatus builds a human-readable status line for a provider/model
// combination.
func ResolveStatus(p, model string, _ error) string {
	canonical := provider.Normalize(p)
	if canonical == "" {
		canonical = "aetox"
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
	case provider.RuntimeResponses:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if apiKey == "" {
			if token, tokenErr := oauth.Token(ctx, canonical); tokenErr == nil {
				apiKey = token
			}
		}
		if baseURL == "" {
			baseURL = oauth.Endpoint(canonical)
		}
		return DiscoverResponsesModels(ctx, canonical, baseURL, oauth.Headers(canonical), apiKey)
	case provider.RuntimeAnthropic:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		models, err := DiscoverAnthropicModels(ctx, canonical, baseURL, apiKey)
		if err == nil && len(models) > 0 {
			return models, nil
		}
		// DeepSeek serves the Anthropic wire format but not its model list, and
		// does expose an OpenAI-compatible one on a second host.
		if spec, ok := provider.Lookup(canonical); ok && spec.AltRuntime == provider.RuntimeOpenAICompatible && spec.AltBaseURL != "" {
			return DiscoverOpenAICompatibleModels(canonical, spec.AltBaseURL, apiKey)
		}
		return nil, err
	default:
		return nil, fmt.Errorf("provider %q does not support remote model discovery", canonical)
	}
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

// activeLocalModel asks a local runtime which model it is serving right now,
// so the app addresses that one instead of guessing off an alphabetical list.
// Both runtimes will answer; neither says so through the OpenAI-compatible
// /v1/models list, which is why this exists:
//
//	LM Studio  GET /api/v0/models  -> entries carry state "loaded"/"not-loaded"
//	Ollama     GET /api/ps         -> the models resident in memory right now
//
// Returns "" whenever the runtime cannot say — nothing loaded yet, endpoint
// absent, older build, not a local provider. That is not a failure: the caller
// falls back to the discovery list, and an empty answer beats a confident wrong
// one. Errors are swallowed for the same reason; this is a best-effort hint on
// a path that already has a fallback.
func activeLocalModel(canonical, baseURL, apiKey string) string {
	var path string
	switch canonical {
	case "lmstudio":
		path = "/api/v0/models"
	case "ollama":
		path = "/api/ps"
	default:
		return ""
	}
	root := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(baseURL), "/"), "/v1")
	if root == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, root+path, nil)
	if err != nil {
		return ""
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var payload struct {
		// LM Studio
		Data []struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			State string `json:"state"`
		} `json:"data"`
		// Ollama
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, m := range payload.Data {
		// Embedding models also report "loaded" and cannot answer a chat
		// request, so an explicit non-llm type is skipped. An empty type is
		// kept: older builds omit it entirely.
		if m.State == "loaded" && (m.Type == "" || m.Type == "llm" || m.Type == "vlm") {
			if id := strings.TrimSpace(m.ID); id != "" {
				return id
			}
		}
	}
	for _, m := range payload.Models {
		if name := strings.TrimSpace(firstNonEmpty(m.Name, m.Model)); name != "" {
			return name
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func DiscoverOpenAICompatibleModels(p, baseURL, apiKey string) ([]string, error) {
	if p == "" {
		p = "openai"
	}

	endpoint := strings.TrimSpace(baseURL)
	if endpoint == "" {
		// A signed-in account may be served from its own host, which is only
		// known after login and so cannot live in the catalog.
		endpoint = oauth.Endpoint(p)
	}
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
	if apiKey == "" {
		// Signed-in providers reach discovery with no key at all — without this
		// the model picker is empty for exactly the providers whose model list
		// the user cannot guess.
		if token, tokenErr := oauth.Token(ctx, p); tokenErr == nil {
			apiKey = token
		}
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
	// Name is what the API actually sends — "models/gemini-2.5-flash".
	// baseModelId is documented but absent from every entry the live endpoint
	// returns, so reading only that skipped every model and left the picker
	// empty with "no valid IDs" for anyone using Gemini.
	Name                       string   `json:"name"`
	BaseModelID                string   `json:"baseModelId"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

// id is the bare model name a request needs, from whichever field the endpoint
// chose to populate.
func (m geminiModel) id() string {
	if v := strings.TrimSpace(m.BaseModelID); v != "" {
		return v
	}
	return strings.TrimPrefix(strings.TrimSpace(m.Name), "models/")
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
		id := item.id()
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
