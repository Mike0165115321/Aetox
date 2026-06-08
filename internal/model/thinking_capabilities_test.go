package model

import "testing"

func TestResolveThinkingCapabilitiesDeepSeekNativeLevels(t *testing.T) {
	caps := ResolveThinkingCapabilities("deepseek", "deepseek-v4-flash")
	if !caps.Supported || !caps.Native {
		t.Fatalf("expected deepseek native thinking capabilities, got %+v", caps)
	}
	want := []string{"off-think", "high", "max"}
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
		{raw: "none", want: "off-think"},
		{raw: "low", want: "high"},
		{raw: "medium", want: "high"},
		{raw: "HIGH", want: "high"},
		{raw: "xhigh", want: "max"},
		{raw: "max", want: "max"},
		{raw: "off-think", want: "off-think"},
	}

	for _, tt := range tests {
		got := NormalizeThinkingLevel("deepseek", "deepseek-v4-flash", tt.raw)
		if got != tt.want {
			t.Fatalf("NormalizeThinkingLevel(%q): want %q got %q", tt.raw, tt.want, got)
		}
	}
}

func TestNormalizeThinkingLevelOpenAIMigratesOffThinkToNone(t *testing.T) {
	got := NormalizeThinkingLevel("openai", "gpt-5.1", "off-think")
	if got != "none" {
		t.Fatalf("expected none got %q", got)
	}
}

func TestFallbackThinkingCapabilitiesRemainAvailable(t *testing.T) {
	levels := SupportedThinkingLevels("unknown", "mystery-model")
	want := []string{"low", "medium", "high", "off-think"}
	if len(levels) != len(want) {
		t.Fatalf("unexpected levels: %#v", levels)
	}
	for i := range want {
		if levels[i] != want[i] {
			t.Fatalf("unexpected levels: %#v", levels)
		}
	}
}
