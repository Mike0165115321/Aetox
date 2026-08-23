package model

import "testing"

// withVisionCatalog installs a slice of the real models.dev table, captured
// 2026-08-23, for the models these tests name. Captured rather than invented so
// a row cannot assert something about a model that is not true of it.
func withVisionCatalog(t *testing.T) {
	t.Helper()
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{
		Source: "models.dev (captured 2026-08-23)",
		Models: map[string]ModelFacts{
			// modalities.input = [text, image, video]. The model in the
			// owner's screenshot.
			"opencode-go/qwen3.7-plus": {Context: 1000000, ToolCall: true, Output: []string{"text"}, Input: []string{"text", "image"}},
			"opencode-go/kimi-k2.6":    {Context: 262144, ToolCall: true, Output: []string{"text"}, Input: []string{"text", "image"}},
			"opencode-go/minimax-m3":   {Context: 1000000, ToolCall: true, Output: []string{"text"}, Input: []string{"text", "image"}},
			"opencode-go/grok-4.5":     {Context: 2000000, ToolCall: true, Output: []string{"text"}, Input: []string{"text", "image"}},
			// Text only, and its name says nothing either way.
			"opencode-go/deepseek-v4-pro": {Context: 1000000, ToolCall: true, Output: []string{"text"}},
			// The catalog contradicting a name that looks sighted. This is the
			// direction that costs a failed turn if the name is believed.
			"openrouter/some/model-vision-classifier": {Context: 8192, Output: []string{"text"}},
		},
	})
}

// The bug the owner hit on 2026-08-23: a screenshot handed to qwen3.7-plus went
// to image_ocr instead of to the model, because visionModelMarkers is a list of
// substrings and no substring of "qwen3.7-plus" was ever going to be in it.
//
// It is the same shape as the deepseek-v4-flash-vision-exp regression (§167.3):
// a name-shaped guess deciding what a model can do. The fix is the same one as
// for openrouter's thinking dial — ask the catalog, which publishes it.
func TestVisionComesFromTheCatalogNotTheModelName(t *testing.T) {
	withVisionCatalog(t)

	for _, id := range []string{"qwen3.7-plus", "kimi-k2.6", "minimax-m3", "grok-4.5"} {
		if !ResolveVision("opencode-go", id) {
			t.Errorf("%s is called blind; the catalog lists image among its inputs", id)
		}
	}
	if ResolveVision("opencode-go", "deepseek-v4-pro") {
		t.Error("deepseek-v4-pro reports vision; the catalog lists text only")
	}
}

// The catalog outranks the name, and this is the direction that matters. A name
// containing "vision" on a model that cannot take an image part is how an image
// reaches a backend that will reject it, or worse silently drop it and answer
// about a picture it never received.
func TestCatalogBeatsASightedLookingName(t *testing.T) {
	withVisionCatalog(t)

	if ResolveVision("openrouter", "some/model-vision-classifier") {
		t.Error("believed the name over the catalog: the marker list would call this sighted")
	}
}

// A role marker still wins over everything. nomic-embed-vision really does take
// images and handing a turn to it is still a mistake, so no catalog row can
// make an embedder into a chat model.
func TestRoleMarkersStillOutrankTheCatalog(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"ollama/nomic-embed-vision": {Context: 8192, Input: []string{"text", "image"}},
	}})

	if ResolveVision("ollama", "nomic-embed-vision") {
		t.Error("an embedder was handed a turn because the catalog said it takes images")
	}
}

// The name markers are not dead code, they are the answer for everything no
// catalog describes: models.dev lists what providers serve, not what somebody
// pulled onto their own GPU.
func TestLocalModelsStillResolveByName(t *testing.T) {
	withVisionCatalog(t)

	for _, id := range []string{"llava:13b", "moondream", "llama3.2-vision:11b"} {
		if !ResolveVision("ollama", id) {
			t.Errorf("%s lost its vision; nothing but the name can identify a pulled model", id)
		}
	}
	if ResolveVision("ollama", "llama3.3:70b") {
		t.Error("a text-only local model reports vision")
	}
}

// With no catalog installed the behaviour is exactly what shipped before this
// change. Unlike the thinking dial, guessing wrong here is not a dead control:
// the fallback is image_ocr, which works everywhere and costs only the layout.
// So there is no reason to withdraw an answer that has been serving.
func TestNoCatalogFallsBackToTheOldBehaviour(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(nil)

	if !ResolveVision("anthropic", "claude-opus-5") {
		t.Error("claude-opus-5 lost its vision with no catalog installed")
	}
	if ResolveVision("opencode-go", "qwen3.7-plus") {
		t.Error("guessed vision from a name the marker list does not contain")
	}
}
