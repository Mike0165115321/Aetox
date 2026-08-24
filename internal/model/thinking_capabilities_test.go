package model

import (
	"strings"
	"testing"
)

func TestResolveThinkingCapabilitiesDeepSeekNativeLevels(t *testing.T) {
	caps := ResolveThinkingCapabilities("deepseek", "deepseek-v4-flash")
	if !caps.Supported || !caps.Native {
		t.Fatalf("expected deepseek native thinking capabilities, got %+v", caps)
	}
	// The depths that behave differently, plus off. The service also accepts
	// medium, xhigh and ultra, but folds the first two onto high itself and
	// documents the third nowhere — they are aliases, not menu entries.
	want := []string{"off", "low", "high", "max"}
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

	// qwen/qwen3-32b used to get a dial from a `qwen/qwen3-` prefix. Groq does
	// serve it and models.dev does not describe it at all, so the answer is now
	// "unknown", which is the honest one and a real if small loss: a model with
	// a dial is offered none until the catalog lists it.
	//
	// The alternative is the prefix that produced it, and that prefix is why
	// llama-3.3-70b was sent an include_reasoning field it answers 400 to. The
	// assertion is kept, inverted, so the day models.dev adds this id the test
	// says so instead of staying quietly green.
	qwen := ResolveThinkingCapabilities("groq", "qwen/qwen3-32b")
	if qwen.Supported {
		t.Fatalf("qwen3-32b is not in the catalog and reported a dial anyway: %+v", qwen)
	}
}

func TestResolveThinkingCapabilitiesOpenRouterKnownReasoningFamilies(t *testing.T) {
	// OpenRouter's answer comes from the catalog now, not from a prefix list,
	// so a test with none installed would be testing the no-catalog path.
	// (openrouter_thinking_test.go covers that path deliberately.)
	withOpenRouterCatalog(t)

	caps := ResolveThinkingCapabilities("openrouter", "deepseek/deepseek-r1")
	if !caps.Supported || !caps.Native {
		t.Fatalf("expected openrouter reasoning capabilities, got %+v", caps)
	}
	if caps.Runtime != ThinkingRuntimeReasoningObject {
		t.Fatalf("expected reasoning-object runtime, got %q", caps.Runtime)
	}
}

// A stored setting has to land somewhere valid, and a level DeepSeek really has
// must survive untouched.
//
// `low` is the part that changed. It used to be rewritten to "high" here
// because the picker did not offer it — a real depth of DeepSeek's, thrown
// away, so a config asking for one depth silently got another. medium, xhigh
// and ultra are still folded, but now for a reason that survives inspection:
// the service folds the first two itself, and documents the third nowhere.
func TestNormalizeThinkingLevelDeepSeekMigratesLegacyValues(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "", want: "high"},
		{raw: "none", want: "off"},
		{raw: "disabled", want: "off"},
		{raw: "minimal", want: "low"},
		{raw: "default", want: "high"},
		{raw: "nonsense", want: "high"}, // unknown, falls to the default
		{raw: "low", want: "low"},
		{raw: "medium", want: "high"},
		{raw: "HIGH", want: "high"},
		{raw: "xhigh", want: "high"},
		{raw: "ultra", want: "max"},
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

// An unknown provider gets no dial, not a guessed one.
//
// This used to offer low/medium/high/off to anything unrecognised. The menu was
// invented, and for a provider that can carry reasoning_effort the invented
// level really was sent — to a model nobody had checked takes it.
func TestUnknownProviderGetsNoThinkingDial(t *testing.T) {
	if levels := SupportedThinkingLevels("unknown", "mystery-model"); len(levels) != 0 {
		t.Fatalf("unknown provider offers %#v; it must offer nothing", levels)
	}
	if caps := ResolveThinkingCapabilities("unknown", "mystery-model"); caps.Supported {
		t.Fatalf("unknown provider reports a dial: %+v", caps)
	}
}

// The same rule one level down: a provider that does have a dial, asked about a
// model this table has never heard of, still says no. Groq and OpenAI are the
// cases that made this concrete — their own catalog defaults
// (llama-3.3-70b-versatile, gpt-4o-mini) are not reasoning models.
func TestUnknownModelOfAReasoningProviderGetsNoDial(t *testing.T) {
	for _, tc := range []struct{ provider, model string }{
		{"openai", "gpt-4o-mini"},
		{"groq", "llama-3.3-70b-versatile"},
		{"kimi", "kimi-k2"},
	} {
		if caps := ResolveThinkingCapabilities(tc.provider, tc.model); caps.Supported {
			t.Errorf("%s/%s reports a dial (%v) — nothing has checked that it has one",
				tc.provider, tc.model, caps.Levels)
		}
	}
}

// And the providers whose API carries no thinking setting at all offer nothing,
// however many levels the table might have been able to name.
func TestProvidersWithoutAReasoningKnobOfferNothing(t *testing.T) {
	for _, p := range []string{"aetox", "mistral", "alibaba", "zai", "ollama", "lmstudio"} {
		if levels := SupportedThinkingLevels(p, ""); len(levels) != 0 {
			t.Errorf("%s offers %#v but its runtime sends no thinking field at all", p, levels)
		}
	}
}

func TestResolveThinkingCapabilities_KnownPrefixesResolveToSupported(t *testing.T) {
	tests := []struct {
		provider string
		models   []string
	}{
		// deepseek-v4 and deepseek-chat left this row, and both departures are
		// the finding. models.dev serves no plain "deepseek-v4" — the real ids
		// are deepseek-v4-pro and deepseek-v4-flash — and it states
		// reasoning=false for deepseek-chat, which is the non-thinking sibling
		// of deepseek-reasoner. The old prefix said every `deepseek-` model
		// thinks, which is how a chat model came to have a thinking menu.
		{"deepseek", []string{"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-reasoner"}},
		{"openai", []string{"gpt-5-pro", "gpt-5.1", "gpt-5.2", "o1", "o3", "o4"}}, // gpt-4o has no effort knob
		{"gemini", []string{"gemini-2.5-flash-lite", "gemini-2.5-pro", "gemini-2.5-flash", "gemini-3-pro"}},
		// qwen/qwen3-32b is out for the reason above: Groq serves it, no catalog
		// describes it, and a prefix is what this change exists to remove.
		{"groq", []string{"openai/gpt-oss-20b", "openai/gpt-oss-120b"}},
		// openai/gpt-4o is gone from this row, and its absence is the finding
		// rather than a convenience. The openai row two lines up already
		// excludes gpt-4o with the note "gpt-4o has no effort knob" — the same
		// model, the same question, two answers in one table, because the
		// OpenRouter side was answered by a prefix that made everything under
		// openai/ a reasoning model. models.dev states reasoning=false for it.
		{"openrouter", []string{"deepseek/deepseek-r1", "google/gemini-2.5-pro", "anthropic/claude-sonnet-4"}},
	}
	// The whole matrix, not the openrouter slice: every provider in the table
	// below answers from the catalog now, so a fixture covering one of them
	// leaves the rest reporting "unknown model" and reads as a broken resolver.
	withCapabilityMatrix(t)
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
	// A gemini this table does not know reports no dial, so there is no level
	// to normalize to — empty, not a guessed "off".
	got := NormalizeThinkingLevel("gemini", "gemini-4-future", "off")
	if got != "" {
		t.Fatalf("expected an unknown model to have no level, got %q", got)
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

// Claude had no thinking control in the UI at all: ResolveThinkingCapabilities
// had no anthropic case, so it fell to the non-native fallback and
// App.SupportedThinkLevels returns nothing for those — the row simply did not
// render, on the provider whose runtime has been sending thinking:adaptive the
// whole time.
func TestThinkingCapabilitiesForProvidersAetoxDrivesDirectly(t *testing.T) {
	cases := []struct {
		provider, model string
		wantNative      bool
		wantLevels      []string
	}{
		{"anthropic", "claude-sonnet-5", true, []string{"off", "low", "medium", "high", "xhigh", "max"}},
		{"anthropic", "claude-opus-4-8", true, []string{"off", "low", "medium", "high", "xhigh", "max"}},
		// Pre-thinking Claude keeps the row hidden rather than offering a
		// switch that does nothing.
		{"anthropic", "claude-3-5-sonnet-20241022", false, nil},
	}
	for _, tc := range cases {
		caps := ResolveThinkingCapabilities(tc.provider, tc.model)
		if caps.Native != tc.wantNative {
			t.Errorf("%s/%s Native = %v; want %v", tc.provider, tc.model, caps.Native, tc.wantNative)
		}
		if strings.Join(caps.Levels, ",") != strings.Join(tc.wantLevels, ",") {
			t.Errorf("%s/%s levels = %v; want %v", tc.provider, tc.model, caps.Levels, tc.wantLevels)
		}
	}
}

// Every level offered must be one the rest of the system understands — "off" in
// particular, because think.Resolve special-cases exactly that spelling and any
// other way of saying it silently leaves thinking on.
func TestEveryOfferedThinkLevelIsAKnownOne(t *testing.T) {
	known := map[string]bool{
		"none": true, "minimal": true, "low": true, "medium": true,
		"high": true, "xhigh": true, "ultra": true, "max": true,
		"default": true, "off": true, "on": true, "adaptive": true,
	}
	for _, provider := range SupportedProviders() {
		for _, m := range append(ModelChoices(provider), "") {
			for _, level := range ResolveThinkingCapabilities(provider, m).Levels {
				if !known[level] {
					t.Errorf("%s offers think level %q, which nothing else in Aetox knows", provider, level)
				}
			}
		}
	}
}
