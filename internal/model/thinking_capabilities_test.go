package model

import "testing"

func TestResolveThinkingCapabilitiesDeepSeekNativeLevels(t *testing.T) {
	caps := ResolveThinkingCapabilities("deepseek", "deepseek-v4-flash")
	if !caps.Supported || !caps.Native {
		t.Fatalf("expected deepseek native thinking capabilities, got %+v", caps)
	}
	want := []string{"off", "high", "max"}
	if len(caps.Levels) != len(want) {
		t.Fatalf("unexpected levels: %#v", caps.Levels)
	}
	for i := range want {
		if caps.Levels[i] != want[i] {
			t.Fatalf("unexpected levels: %#v", caps.Levels)
		}
	}
	if caps.Default != "high" {
		t.Fatalf("expected default high, got %q", caps.Default)
	}
	if caps.Runtime != ThinkingRuntimeDeepSeek {
		t.Fatalf("expected deepseek runtime, got %q", caps.Runtime)
	}
}

func TestResolveThinkingCapabilitiesOpenAIReasoningFamilies(t *testing.T) {
	tests := []struct {
		model string
		want  []string
		def   string
	}{
		{model: "gpt-5.1", want: []string{"none", "low", "medium", "high"}, def: "none"},
		{model: "gpt-5.2", want: []string{"none", "minimal", "low", "medium", "high", "xhigh"}, def: "medium"},
		{model: "gpt-5-pro", want: []string{"high"}, def: "high"},
	}

	for _, tt := range tests {
		caps := ResolveThinkingCapabilities("openai", tt.model)
		if !caps.Supported || !caps.Native {
			t.Fatalf("expected native thinking capabilities for %s, got %+v", tt.model, caps)
		}
		if len(caps.Levels) != len(tt.want) {
			t.Fatalf("%s unexpected levels: %#v", tt.model, caps.Levels)
		}
		for i := range tt.want {
			if caps.Levels[i] != tt.want[i] {
				t.Fatalf("%s unexpected levels: %#v", tt.model, caps.Levels)
			}
		}
		if caps.Default != tt.def {
			t.Fatalf("%s expected default %q got %q", tt.model, tt.def, caps.Default)
		}
	}
}

func TestResolveThinkingCapabilitiesGeminiFamilies(t *testing.T) {
	flashLite := ResolveThinkingCapabilities("gemini", "gemini-2.5-flash-lite")
	if !flashLite.Supported || flashLite.Default != "medium" {
		t.Fatalf("expected gemini flash-lite thinking support, got %+v", flashLite)
	}
	wantFlashLite := []string{"none", "minimal", "low", "medium", "high"}
	for i := range wantFlashLite {
		if flashLite.Levels[i] != wantFlashLite[i] {
			t.Fatalf("unexpected gemini flash-lite levels: %#v", flashLite.Levels)
		}
	}

	pro := ResolveThinkingCapabilities("gemini", "gemini-2.5-pro")
	wantPro := []string{"minimal", "low", "medium", "high"}
	for i := range wantPro {
		if pro.Levels[i] != wantPro[i] {
			t.Fatalf("unexpected gemini pro levels: %#v", pro.Levels)
		}
	}
	if SupportsThinkingLevel("gemini", "gemini-2.5-pro", "none") {
		t.Fatal("gemini-2.5-pro should not support none")
	}

	legacyLite := ResolveThinkingCapabilities("gemini", "gemini-2.0-flash-lite")
	if legacyLite.Supported {
		t.Fatalf("expected gemini-2.0-flash-lite to not support thinking, got %+v", legacyLite)
	}
}

func TestResolveThinkingCapabilitiesGroqFamilies(t *testing.T) {
	gptOSS := ResolveThinkingCapabilities("groq", "openai/gpt-oss-20b")
	if !gptOSS.Supported || gptOSS.Runtime != ThinkingRuntimeGroq {
		t.Fatalf("expected groq thinking capabilities, got %+v", gptOSS)
	}
	if gptOSS.Default != "medium" {
		t.Fatalf("expected medium default, got %q", gptOSS.Default)
	}

	qwen := ResolveThinkingCapabilities("groq", "qwen/qwen3-32b")
	if !qwen.Supported || qwen.Default != "default" {
		t.Fatalf("expected qwen default thinking capabilities, got %+v", qwen)
	}
	want := []string{"default", "none"}
	for i := range want {
		if qwen.Levels[i] != want[i] {
			t.Fatalf("unexpected qwen levels: %#v", qwen.Levels)
		}
	}
}

func TestResolveThinkingCapabilitiesOpenRouterKnownReasoningFamilies(t *testing.T) {
	caps := ResolveThinkingCapabilities("openrouter", "deepseek/deepseek-r1")
	if !caps.Supported || !caps.Native {
		t.Fatalf("expected openrouter reasoning capabilities, got %+v", caps)
	}
	if caps.Runtime != ThinkingRuntimeReasoningObject {
		t.Fatalf("expected reasoning-object runtime, got %q", caps.Runtime)
	}
}

func TestNormalizeThinkingLevelDeepSeekMigratesLegacyValues(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "", want: "high"},
		{raw: "none", want: "off"},
		{raw: "low", want: "high"},
		{raw: "medium", want: "high"},
		{raw: "HIGH", want: "high"},
		{raw: "xhigh", want: "max"},
		{raw: "max", want: "max"},
		{raw: "off", want: "off"},
	}

	for _, tt := range tests {
		got := NormalizeThinkingLevel("deepseek", "deepseek-v4-flash", tt.raw)
		if got != tt.want {
			t.Fatalf("NormalizeThinkingLevel(%q): want %q got %q", tt.raw, tt.want, got)
		}
	}
}

func TestNormalizeThinkingLevelOpenAIMigratesOffThinkToNone(t *testing.T) {
	got := NormalizeThinkingLevel("openai", "gpt-5.1", "off")
	if got != "none" {
		t.Fatalf("expected none got %q", got)
	}
}

func TestNormalizeThinkingLevelGeminiMapsOffThinkToNoneWhenAllowed(t *testing.T) {
	got := NormalizeThinkingLevel("gemini", "gemini-2.5-flash-lite", "off")
	if got != "none" {
		t.Fatalf("expected none got %q", got)
	}
}

func TestFallbackThinkingCapabilitiesRemainAvailable(t *testing.T) {
	levels := SupportedThinkingLevels("unknown", "mystery-model")
	want := []string{"low", "medium", "high", "off"}
	if len(levels) != len(want) {
		t.Fatalf("unexpected levels: %#v", levels)
	}
	for i := range want {
		if levels[i] != want[i] {
			t.Fatalf("unexpected levels: %#v", levels)
		}
	}
}

func TestResolveThinkingCapabilities_KnownPrefixesResolveToSupported(t *testing.T) {
	tests := []struct {
		provider string
		models   []string
	}{
		{"deepseek", []string{"deepseek-v4", "deepseek-chat", "deepseek-reasoner"}},
		{"openai", []string{"gpt-5-pro", "gpt-5.1", "gpt-5.2", "o1", "o3", "o4", "gpt-4o"}},
		{"gemini", []string{"gemini-2.5-flash-lite", "gemini-2.5-pro", "gemini-2.5-flash", "gemini-3-pro"}},
		{"groq", []string{"openai/gpt-oss-20b", "qwen/qwen3-32b"}},
		{"openrouter", []string{"openai/gpt-4o", "deepseek/deepseek-r1", "google/gemini-2.5-pro", "anthropic/claude-sonnet-4"}},
	}
	for _, tt := range tests {
		for _, model := range tt.models {
			caps := ResolveThinkingCapabilities(tt.provider, model)
			if !caps.Supported {
				t.Errorf("%s/%s: expected Supported=true, got Source=%q", tt.provider, model, caps.Source)
			}
		}
	}
}

func TestThinkingLevel_OffMapsToProviderNative(t *testing.T) {
	got := NormalizeThinkingLevel("gemini", "gemini-4-future", "off")
	if got != "off" {
		t.Fatalf("expected off -> off for conservative fallback, got %q", got)
	}

	got = NormalizeThinkingLevel("openai", "gpt-5.2", "off")
	if got != "none" {
		t.Fatalf("expected off -> none for gpt-5.2, got %q", got)
	}

	got = NormalizeThinkingLevel("deepseek", "deepseek-v4-flash", "off")
	if got != "off" {
		t.Fatalf("expected off -> off for deepseek native, got %q", got)
	}
}
