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

// providerReasoningCapability answers, from the catalog, whether this provider
// is known at all and whether its API can carry a thinking/effort setting.
//
// It lives here rather than being read inline because the two callers —
// ResolveThinkingCapabilities, which decides what the picker shows, and
// supportsNativeReasoning, which decides whether anything is put on the wire —
// must not be able to answer it differently. They used to: one read a hardcoded
// list, the other had a table of its own, and seven providers ended up with a
// full thinking menu that sent nothing.
//
// Unexported: outside this package the question to ask is
// ResolveThinkingCapabilities, which gives the levels as well.
func providerReasoningCapability(name string) (known, reasoning bool) {
	spec, ok := provider.Lookup(name)
	if !ok {
		return false, false
	}
	return true, spec.Capabilities.Reasoning
}

// RequiresAPIKey delegates to provider.RequiresAPIKey.
func RequiresAPIKey(name string) bool {
	return provider.RequiresAPIKey(name)
}

// AcceptsAPIKey delegates to provider.AcceptsAPIKey.
func AcceptsAPIKey(name string) bool {
	return provider.AcceptsAPIKey(name)
}

// APIKeyURL delegates to provider.APIKeyURL: where the user goes to create the
// key this provider asks for, or "" when there is nowhere to send them.
func APIKeyURL(name string) string {
	return provider.APIKeyURL(name)
}

// StatesQuota reports whether this provider says anything about a remaining
// rate-limit window. False means no window exists to wait for — a pay-as-you-go
// account like DeepSeek — so the UI must not promise a number that is never
// coming.
func StatesQuota(name string) bool {
	return provider.QuotaSourceFor(name) != provider.QuotaNone
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
	// Then the fetched catalog, by rule rather than by name — see
	// ModelCatalog.DefaultFor. This sits above the static name because the
	// static name is the thing that keeps dying: a sweep on 2026-08-15 found
	// four of them pointing at models their vendors had already withdrawn, the
	// same failure the Gemini entry had already been fixed for once.
	installedCatalogMu.RLock()
	installed := installedCatalog
	installedCatalogMu.RUnlock()
	if picked := installed.DefaultFor(canonical); picked != "" {
		return picked
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

// ModelChoices returns what a provider's picker can offer when the live list
// could not be fetched. A hint, never an authority: prefer
// ModelChoicesWithEndpointAndAPIKey, which asks the provider itself.
//
// The catalog's own FallbackModel is the last entry in the chain, and it is the
// half that used to be missing. Almost every row deliberately carries no
// RecommendedModels — GET /v1/models answers that, and a list written into the
// catalog goes stale within months — so when discovery fails there was nothing
// left to show and the picker went empty. Empty is the wrong answer for a
// provider Aetox can name a model for: a user who has just pasted a key sees a
// blank list and a box asking them to type a model id, which reads as "this
// provider is broken" when the truth may be as ordinary as a new xAI team with
// no credits yet, whose /v1/models answers 403 until it is funded.
//
// One name, not a list. FallbackModel is a single value the catalog already
// maintains per row, so this cannot rot the way a curated shelf does.
func ModelChoices(name string) []string {
	if recommended := provider.RecommendedModels(name); len(recommended) > 0 {
		return recommended
	}
	if fallback := strings.TrimSpace(provider.DefaultModel(name)); fallback != "" {
		return []string{fallback}
	}
	return nil
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
//
// The result is ordered, not filtered — see usableFirst. Ordering matters
// beyond the picker because ResolveDefaultModel takes the first entry as a
// cold start, and alphabetical-first is not an opinion about anything.
func ModelChoicesWithEndpointAndAPIKey(p, baseURL, apiKey string) ([]string, error) {
	models, err := discoverModelChoices(p, baseURL, apiKey)
	if err != nil {
		return nil, err
	}
	return usableFirst(p, models), nil
}

// usableFirst moves the ids the fetched catalog can vouch for as tool-calling
// chat models to the front, keeping each group in the order discovery returned
// (alphabetical). Nothing is removed.
//
// It exists because a provider's /v1/models is a shelf, not a statement about
// what a model can do or what an account may invoke. NVIDIA's returns 103 ids
// as bare {id, object, created, owned_by} records with one identical created
// stamp — no field separates gpt-oss-120b from nv-embed-v1, and none says
// whether the key can call either. Sorted alphabetically that put
// 01-ai/yi-large first, which is how a first turn on a fresh key answered
// 404 "Function not found for account": listed, not deployed.
//
// Ordering rather than filtering, because the catalog only describes 43 of
// those 103. Hiding the other 60 would hide most of what the provider serves
// on the strength of a table already measured to be wrong about this endpoint
// 43 times. Floating the known-good ones costs nothing and is honest about
// what it knows: everything is still there, in a better order.
//
// A no-op when the catalog has nothing to say — which is every unit test, and
// every local runtime whose model names no catalog has ever heard of.
func usableFirst(providerName string, models []string) []string {
	if len(models) < 2 {
		return models
	}
	installedCatalogMu.RLock()
	c := installedCatalog
	installedCatalogMu.RUnlock()

	vouched := make([]string, 0, len(models))
	rest := make([]string, 0, len(models))
	for _, id := range models {
		facts, ok := c.For(providerName, id)
		if ok && facts.ToolCall && facts.TextOut {
			vouched = append(vouched, id)
			continue
		}
		rest = append(rest, id)
	}
	if len(vouched) == 0 {
		return models
	}
	return append(vouched, rest...)
}

func discoverModelChoices(p, baseURL, apiKey string) ([]string, error) {
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
		// api.openai.com takes turns at /responses but still lists its models
		// the ordinary way, at GET /v1/models. The codex list below is the
		// ChatGPT backend's own endpoint — it answers what one *plan* may use,
		// takes a client_version, and replies in a shape of its own, so asking
		// it of the public API returns nothing and the picker comes up empty.
		//
		// Keyed on the row having an OpenAI-compatible alt rather than on the
		// name "openai", which is the same test the Anthropic branch below
		// already uses to find DeepSeek's model list.
		if spec, ok := provider.Lookup(canonical); ok && spec.AltRuntime == provider.RuntimeOpenAICompatible {
			listURL := baseURL
			if listURL == "" {
				listURL = spec.AltBaseURL
			}
			models, err := DiscoverOpenAICompatibleModels(canonical, listURL, apiKey)
			if err == nil && len(models) > 0 {
				return models, nil
			}
			return nil, err
		}
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
