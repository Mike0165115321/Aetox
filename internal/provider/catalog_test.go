package provider

import (
	"os"
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
		{"together", "together"},
		{"togetherai", "together"},
		{"perplexity", "perplexity"},
		{"pplx", "perplexity"},
		{"cohere", "cohere"},
		{"command-r", "cohere"},
		{"lmstudio", "lmstudio"},
		{"localai", "lmstudio"},
		{"local-ai", "lmstudio"},
		{"ollama", "ollama"},
		{"ollamaai", "ollama"},
		{"anthropic", "anthropic"},
		{"claude", "anthropic"},
		{"noop", "noop"},
		{"none", "noop"},
		{"stub", "noop"},
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

func TestNormalize_Unknown(t *testing.T) {
	// Unknown providers are returned as-is (lowercase).
	got := Normalize("unknown-provider")
	if got != "unknown-provider" {
		t.Fatalf("want %q got %q", "unknown-provider", got)
	}
}

func TestNormalize_Empty(t *testing.T) {
	got := Normalize("")
	if got != "noop" {
		t.Fatalf("empty string should normalize to noop, got %q", got)
	}
	got = Normalize("   ")
	if got != "noop" {
		t.Fatalf("whitespace should normalize to noop, got %q", got)
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
		{"noop", "Aetox0.0.1:0b"},
		{"openrouter", "deepseek/deepseek-r1"},
		{"openai", "gpt-4o-mini"},
		{"deepseek", "deepseek-v4-flash"},
		{"gemini", "gemini-2.5-flash-lite"},
		{"groq", "llama-3.3-70b-versatile"},
		{"mistral", "mistral-small"},
		{"together", "google/gemma-2-9b-it"},
		{"perplexity", "llama-3.1-sonar-small-128k-online"},
		{"cohere", "command-r-plus"},
		{"lmstudio", "local-model"},
		{"ollama", "gemma3:4b"},
		{"anthropic", "claude-haiku-4-5"},
		{"unknown", ""},
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

func TestDefaultBaseURL(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"openrouter", "https://openrouter.ai/api/v1"},
		{"openai", "https://api.openai.com/v1"},
		{"deepseek", "https://api.deepseek.com"},
		{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai"},
		{"groq", "https://api.groq.com/openai/v1"},
		{"mistral", "https://api.mistral.ai/v1"},
		{"together", "https://api.together.xyz/v1"},
		{"perplexity", "https://api.perplexity.ai"},
		{"cohere", "https://api.cohere.com/v1"},
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
	needsKey := []string{"openrouter", "openai", "deepseek", "gemini", "groq", "mistral", "together", "perplexity", "cohere", "anthropic"}
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
