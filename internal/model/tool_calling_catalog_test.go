package model

import "testing"

func toolCallingProvider(t *testing.T, provider, model string) *OpenAICompatibleProvider {
	t.Helper()
	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: provider, Model: model, APIKey: "k", BaseURL: "https://example.invalid/v1",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return p
}

// `return true` was the whole method, and tool_support.go had already written
// down the cost: "OpenAICompatibleProvider claims tool support for every row it
// serves, so a model that turns out not to have it has nothing else to catch
// it." Measured against models.dev on 2026-08-23, the rows this client serves
// carry 117 such models — 69 of OpenRouter's 360, 39 of NVIDIA's 102, 9 of
// Groq's 15 — every one of them being handed tool definitions each turn.
//
// The rows below are captured from that table, not invented.
func TestToolCallingIsAnsweredPerModel(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{
		Source: "models.dev (captured 2026-08-23)",
		Models: map[string]ModelFacts{
			"groq/openai/gpt-oss-120b":      {Context: 131072, Output: []string{"text"}, ToolCall: true},
			"groq/whisper-large-v3":         {Context: 448, Output: []string{"text"}},
			"nvidia/nvidia/nemotron-3-nano": {Context: 131072, Output: []string{"text"}},
			"openrouter/openai/gpt-4":       {Context: 8191, Output: []string{"text"}},
			"openrouter/anthropic/claude-opus-5": {
				Context: 1000000, Output: []string{"text"}, ToolCall: true,
			},
		},
	})

	for _, tc := range []struct {
		provider, model string
		want            bool
	}{
		{"groq", "openai/gpt-oss-120b", true},
		{"groq", "whisper-large-v3", false},
		{"nvidia", "nvidia/nemotron-3-nano", false},
		{"openrouter", "openai/gpt-4", false},
		{"openrouter", "anthropic/claude-opus-5", true},
	} {
		got := toolCallingProvider(t, tc.provider, tc.model).SupportsToolCalling()
		if got != tc.want {
			t.Errorf("%s/%s SupportsToolCalling = %v; want %v", tc.provider, tc.model, got, tc.want)
		}
	}
}

// Only ever narrows, and this is the half that must not regress. Wrongly
// withholding tools turns a coding agent into a chat window, so a model the
// catalog has never described keeps the provider's answer — which is every
// local runtime, every id no table has heard of, and every turn taken before
// the first catalog fetch.
func TestToolCallingNeverNarrowsOnAModelItDoesNotKnow(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })

	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"openrouter/openai/gpt-4": {Context: 8191, Output: []string{"text"}},
	}})
	if !toolCallingProvider(t, "openrouter", "somebody/model-shipped-tomorrow").SupportsToolCalling() {
		t.Error("an id the catalog has never described lost its tools")
	}

	SetModelCatalog(nil)
	for _, tc := range [][2]string{
		{"openrouter", "openai/gpt-4"},
		{"lmstudio", "some-local-build"},
		{"opencode-go", "qwen3.7-plus"},
	} {
		if !toolCallingProvider(t, tc[0], tc[1]).SupportsToolCalling() {
			t.Errorf("%s/%s lost its tools with no catalog installed", tc[0], tc[1])
		}
	}
}

// An empty model name is the provider's default, not a statement about a model,
// so there is nothing to narrow against.
func TestToolCallingWithNoModelNameKeepsTheProviderAnswer(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"openrouter/openai/gpt-4": {Context: 8191, Output: []string{"text"}},
	}})

	if !modelToolCalling("openrouter", "", true) {
		t.Error("narrowed on an empty model name")
	}
}

// A provider whose runtime cannot carry tools at all stays false. The catalog
// describes models, not what a client is able to put on the wire, so it must
// never be able to turn that answer back on.
func TestToolCallingCannotOverrideARuntimeThatCarriesNone(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"openrouter/anthropic/claude-opus-5": {Context: 1000000, Output: []string{"text"}, ToolCall: true},
	}})

	if modelToolCalling("openrouter", "anthropic/claude-opus-5", false) {
		t.Error("the catalog turned tools back on for a runtime that cannot send them")
	}
}
