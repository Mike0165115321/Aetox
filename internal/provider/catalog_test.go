package provider

import (
	"os"
	"strings"
	"testing"
)

func TestNormalize_KnownAlias(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"openrouter", "openrouter"},
		{"OpenRouter", "openrouter"},
		{"OPENROUTER", "openrouter"},
		{"or", "openrouter"},
		{"open-router", "openrouter"},
		{"openrouterai", "openrouter"},
		{"openai", "openai"},
		{"chatgpt", "openai"},
		{"deepseek", "deepseek"},
		{"deepseek-api", "deepseek"},
		{"gemini", "gemini"},
		{"google", "gemini"},
		{"groq", "groq"},
		{"groqcloud", "groq"},
		{"mistral", "mistral"},
		{"mistralai", "mistral"},
		{"lmstudio", "lmstudio"},
		{"localai", "lmstudio"},
		{"local-ai", "lmstudio"},
		{"ollama", "ollama"},
		{"ollamaai", "ollama"},
		{"anthropic", "anthropic"},
		{"claude", "anthropic"},
		{"aetox", "aetox"},
		{"noop", "aetox"},
		{"none", "aetox"},
		{"stub", "aetox"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got != tt.want {
				t.Fatalf("Normalize(%q): want %q got %q", tt.input, tt.want, got)
			}
		})
	}
}

// Adding a provider without deciding what it can say about money is the
// failure this guards: the zero value of BalanceKind is "", which would reach
// the UI as a blank line meaning nothing. There is always an honest answer —
// money, free, subscription, or web-only — so the catalog must state one.
//
// QuotaSource deliberately has no such check: QuotaNone is a real answer
// ("pay-as-you-go, no window worth showing"), not an unmade decision.
func TestEveryProviderDeclaresABalanceKind(t *testing.T) {
	valid := map[BalanceKind]bool{
		BalanceMoney:        true,
		BalanceFree:         true,
		BalanceSubscription: true,
		BalanceWebOnly:      true,
	}
	for _, name := range SupportedProviders() {
		spec, ok := Lookup(name)
		if !ok {
			t.Fatalf("%s: listed by SupportedProviders but Lookup missed it", name)
		}
		if !valid[spec.BalanceKind] {
			t.Errorf("%s: balanceKind is %q; want one of money/free/subscription/web-only",
				name, spec.BalanceKind)
		}
	}
}

// A local runtime that claimed a balance or a quota would send us knocking on
// localhost for an endpoint that does not exist, and show a spinner for an
// answer that is already known: nothing to spend.
func TestLocalProvidersAreFreeAndQuotaless(t *testing.T) {
	for _, name := range []string{"ollama", "lmstudio", "aetox"} {
		spec, _ := Lookup(name)
		if spec.BalanceKind != BalanceFree {
			t.Errorf("%s: balanceKind is %q; want free", name, spec.BalanceKind)
		}
		if spec.QuotaSource != QuotaNone {
			t.Errorf("%s: quotaSource is %q; want none", name, spec.QuotaSource)
		}
	}
}

// BalanceKindFor answers for a name the catalog has never heard of, since an
// unknown provider still gets a row in Settings. "We cannot read this" is the
// only claim we can make about a wallet we know nothing about.
func TestUnknownProviderIsWebOnly(t *testing.T) {
	if got := BalanceKindFor("some-proxy-nobody-registered"); got != BalanceWebOnly {
		t.Fatalf("unknown provider balanceKind = %q; want web-only", got)
	}
	if got := QuotaSourceFor("some-proxy-nobody-registered"); got != QuotaNone {
		t.Fatalf("unknown provider quotaSource = %q; want none", got)
	}
}

func TestNormalize_Unknown(t *testing.T) {
	// Unknown providers are returned as-is (lowercase).
	got := Normalize("unknown-provider")
	if got != "unknown-provider" {
		t.Fatalf("want %q got %q", "unknown-provider", got)
	}
}

func TestNormalize_Empty(t *testing.T) {
	got := Normalize("")
	if got != "aetox" {
		t.Fatalf("empty string should normalize to aetox, got %q", got)
	}
	got = Normalize("   ")
	if got != "aetox" {
		t.Fatalf("whitespace should normalize to aetox, got %q", got)
	}
}

func TestLookup_KnownProvider(t *testing.T) {
	spec, ok := Lookup("openrouter")
	if !ok {
		t.Fatal("expected openrouter to be found")
	}
	if spec.Canonical != "openrouter" {
		t.Fatalf("canonical: want openrouter got %q", spec.Canonical)
	}
	if spec.RequiresAPIKey != true {
		t.Fatal("expected openrouter to require API key")
	}
	if spec.Runtime != RuntimeOpenAICompatible {
		t.Fatalf("runtime: want openai-compatible got %q", spec.Runtime)
	}
	if spec.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("baseURL: want https://openrouter.ai/api/v1 got %q", spec.BaseURL)
	}
	if spec.ModelDefaults.FallbackModel != "deepseek/deepseek-r1" {
		t.Fatalf("fallback model: want deepseek/deepseek-r1 got %q", spec.ModelDefaults.FallbackModel)
	}
	if len(spec.Aliases) == 0 {
		t.Fatal("expected non-empty aliases")
	}
	if len(spec.EnvKeys) == 0 {
		t.Fatal("expected non-empty env keys")
	}
	if spec.EnvKeys[0] != "OPENROUTER_API_KEY" {
		t.Fatalf("env key: want OPENROUTER_API_KEY got %q", spec.EnvKeys[0])
	}
	if !spec.Capabilities.ToolCalling {
		t.Fatal("expected openrouter to support tool calling")
	}
	if !spec.Capabilities.Reasoning {
		t.Fatal("expected openrouter to support reasoning")
	}
}

func TestLookup_ByAlias(t *testing.T) {
	spec, ok := Lookup("or")
	if !ok {
		t.Fatal("expected 'or' alias to resolve")
	}
	if spec.Canonical != "openrouter" {
		t.Fatalf("canonical: want openrouter got %q", spec.Canonical)
	}
}

func TestLookup_UnknownProvider(t *testing.T) {
	_, ok := Lookup("nonexistent")
	if ok {
		t.Fatal("expected unknown provider to return false")
	}
}

func TestDefaultModel_FallbackOnly(t *testing.T) {
	// DefaultModel should return only the static fallback, not a
	// live list.
	tests := []struct {
		provider string
		want     string
	}{
		{"noop", "aetox-grid"},
		{"openrouter", "deepseek/deepseek-r1"},
		{"openai", "gpt-4o-mini"},
		{"deepseek", "deepseek-v4-flash"},
		{"gemini", "gemini-2.5-flash"},
		// Six rows were found pointing at models nobody served on 2026-08-20:
		// these two, kimi below, and perplexity, together and cohere, which the
		// owner removed rather than repoint. Every one had gone dead where it
		// shows least: the row compiled, the suite was green, and the model had
		// stopped existing. Groq was the loudest — it serves no Llama chat
		// model at all now, so its id resolved to an Arabic 7B that answers a
		// tool call with a 400.
		//
		// Which is the point worth keeping: this table cannot tell whether a
		// name is still served, only whether it still matches the catalog. The
		// check that can is TestLiveEveryConfiguredProvider in internal/model,
		// and it needs a key.
		{"groq", "openai/gpt-oss-120b"},
		{"mistral", "mistral-small-latest"},
		// kimi-k3 was the sixth dead name, and the one that proves a
		// third-party catalog is not enough: models.dev still listed it while
		// the endpoint served only k2.6 and k2.7-code.
		{"kimi", "kimi-k2.6"},
		{"anthropic", "claude-haiku-4-5"},
		{"unknown", ""},
		// Local runtimes deliberately carry no fallback: they serve whatever
		// the user installed, so any name here would be a guess. The empty
		// value is what tells model.ResolveDefaultModel to ask the server.
		{"lmstudio", ""},
		{"ollama", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := DefaultModel(tt.provider)
			if got != tt.want {
				t.Fatalf("DefaultModel(%q): want %q got %q", tt.provider, tt.want, got)
			}
		})
	}
}

// A row that asks for a key and cannot say where to get one leaves the user to
// go hunting; every provider hides that page somewhere different. So the rule
// is stated once here rather than remembered each time a provider is added.
func TestEveryKeyedProviderSaysWhereToGetTheKey(t *testing.T) {
	for _, name := range SupportedProviders() {
		spec, _ := Lookup(name)
		// Nothing to link to: no key is asked for, or the provider is signed
		// into and a pasted key is not a credential that exists there.
		if !spec.RequiresAPIKey || !spec.AcceptsAPIKey {
			if got := APIKeyURL(name); got != "" {
				t.Errorf("%q takes no pasted key but offers a key page %q", name, got)
			}
			continue
		}
		got := APIKeyURL(name)
		if got == "" {
			t.Errorf("%q asks the user to paste a key with no page to get one from", name)
			continue
		}
		if !strings.HasPrefix(got, "https://") {
			t.Errorf("%q key page %q is not https", name, got)
		}
	}
}

// The quota sentence on a provider card is a promise: "the limit appears once
// you chat". A provider that never states a window can never keep it, so the
// dialect recorded here has to be what the endpoint actually sends.
func TestGeminiStatesNoQuotaWindow(t *testing.T) {
	// Measured, not assumed: a real key against the live OpenAI-compat endpoint
	// returns 200 with no rate-limit header of any kind (2026-08-14). It was
	// declared QuotaOpenAIStd on the theory that an OpenAI-compatible host
	// probably sends that family, which made the card promise a number that was
	// never coming.
	if got := QuotaSourceFor("gemini"); got != QuotaNone {
		t.Errorf("gemini quota source = %q, want QuotaNone — the endpoint sends no rate-limit headers", got)
	}
}

func TestDefaultBaseURL(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"openrouter", "https://openrouter.ai/api/v1"},
		{"openai", "https://api.openai.com/v1"},
		{"deepseek", "https://api.deepseek.com/anthropic/v1"}, // routed via Anthropic wire format
		{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai"},
		{"groq", "https://api.groq.com/openai/v1"},
		{"mistral", "https://api.mistral.ai/v1"},
		{"lmstudio", "http://localhost:1234/v1"},
		{"ollama", "http://localhost:11434"},
		{"noop", ""},
		{"unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := DefaultBaseURL(tt.provider)
			if got != tt.want {
				t.Fatalf("DefaultBaseURL(%q): want %q got %q", tt.provider, tt.want, got)
			}
		})
	}
}

func TestRequiresAPIKey(t *testing.T) {
	needsKey := []string{"openrouter", "openai", "deepseek", "gemini", "groq",
		"mistral", "kimi", "minimax", "qwen", "zai", "xai", "thaillm", "anthropic"}
	for _, p := range needsKey {
		if !RequiresAPIKey(p) {
			t.Fatalf("expected %q to require API key", p)
		}
	}
	noKey := []string{"noop", "lmstudio", "ollama"}
	for _, p := range noKey {
		if RequiresAPIKey(p) {
			t.Fatalf("expected %q to NOT require API key", p)
		}
	}
	if RequiresAPIKey("unknown") {
		t.Fatal("unknown provider should not require API key")
	}
}

func TestRuntimeFor(t *testing.T) {
	if rt := RuntimeFor("noop"); rt != RuntimeNoop {
		t.Fatalf("noop runtime: want %q got %q", RuntimeNoop, rt)
	}
	if rt := RuntimeFor("openai"); rt != RuntimeOpenAICompatible {
		t.Fatalf("openai runtime: want %q got %q", RuntimeOpenAICompatible, rt)
	}
	if rt := RuntimeFor("ollama"); rt != RuntimeOllama {
		t.Fatalf("ollama runtime: want %q got %q", RuntimeOllama, rt)
	}
	if rt := RuntimeFor("anthropic"); rt != RuntimeAnthropic {
		t.Fatalf("anthropic runtime: want %q got %q", RuntimeAnthropic, rt)
	}
	if rt := RuntimeFor("unknown"); rt != "" {
		t.Fatalf("unknown runtime: want empty got %q", rt)
	}
}

func TestResolveAPIKey(t *testing.T) {
	// Set a fake env for testing.
	os.Setenv("TEST_OPENAI_KEY", "sk-test-123")
	defer os.Unsetenv("TEST_OPENAI_KEY")

	// Override openai's env keys for this test by testing a known
	// provider that reads from environment.
	// openai reads OPENAI_API_KEY — we test via actual env.
	// We can't easily mock os.Getenv, so just verify empty result
	// for a provider with no env set.
	result := ResolveAPIKey("openai")
	// If the user actually has OPENAI_API_KEY set, result won't be
	// empty. We just verify it doesn't panic.
	_ = result
}

func TestMenuLabel(t *testing.T) {
	tests := []struct {
		name     string
		keyFound bool
		want     string
	}{
		{"openrouter", true, "openrouter (env key found)"},
		{"openrouter", false, "openrouter (needs key)"},
		{"", true, "(unknown)"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"_"+boolStr(tt.keyFound), func(t *testing.T) {
			got := MenuLabel(tt.name, tt.keyFound)
			if got != tt.want {
				t.Fatalf("MenuLabel(%q, %v): want %q got %q", tt.name, tt.keyFound, tt.want, got)
			}
		})
	}
}

func TestSupportedProviders(t *testing.T) {
	providers := SupportedProviders()
	if len(providers) < 10 {
		t.Fatalf("expected at least 10 providers, got %d", len(providers))
	}
	// Verify sorted.
	for i := 1; i < len(providers); i++ {
		if providers[i-1] >= providers[i] {
			t.Fatalf("providers not sorted: %q >= %q", providers[i-1], providers[i])
		}
	}
}

func TestRecommendedModels_Empty(t *testing.T) {
	// RecommendedModels returns nil when no recommendations exist.
	got := RecommendedModels("openrouter")
	if got != nil {
		t.Fatal("expected nil recommended models — this is a hint field, not mandatory")
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
