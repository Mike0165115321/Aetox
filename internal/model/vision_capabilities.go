package model

import "strings"

// Which models can look at an image.
//
// This is a gate, not a preference: a text-only model handed an image part gets
// a 400 from some providers and silently drops it on others, and "silently
// drops it" is the worse of the two — the model then answers about a picture it
// never received and nothing in the transcript says so. So the rule is the same
// one §17 settled for tools: never prejudge what a model can do, but do refuse
// to send it something the API will reject on its behalf.
//
// Modelled on ResolveThinkingCapabilities (thinking_capabilities.go): resolve
// from provider + model name, and when the pair is unknown, answer with the
// safe side rather than a guess. Safe here means "no" — the fallback is
// image_ocr, which works everywhere and loses only the picture.

// visionModelMarkers are substrings that identify a vision-capable model across
// providers. Matching on the name rather than keeping a table of exact ids
// because ids churn weekly (dated snapshots, ":free" and ":nitro" suffixes on
// OpenRouter, ":q4_K_M" quantization tags on Ollama) and a table of exact
// matches is a table that is wrong a week after it is written.
var visionModelMarkers = []string{
	"gpt-4o", "gpt-4.1", "gpt-5", "o3", "o4-mini", // OpenAI
	"claude-3", "claude-4", "claude-5", "sonnet", "opus", "haiku", // Anthropic
	"gemini",                                                                                   // Google — every Gemini generation is multimodal
	"llava", "bakllava", "moondream", "minicpm-v", "llama3.2-vision", "qwen2-vl", "qwen2.5-vl", // local
	"pixtral", "vision", "-vl", "multimodal",
	// Kimi K3 has native visual understanding. Named exactly, not as "kimi":
	// K2 and earlier are text-only, and the family name would call them sighted.
	// Note the endpoint takes base64 only — no public image URLs — which is
	// already how convertMessagesToOpenAI sends them.
	"kimi-k3",
}

// textOnlyRoleMarkers win over the list above, and every one of them names a
// ROLE rather than a family. An embedder, a reranker, a transcriber and a
// speech synthesizer are not chat models at all, so no marker in their name can
// make one sighted: nomic-embed-vision really does take images, and handing a
// turn to it is still a mistake.
//
// A family name must never be added here, and that is the rule this list was
// missing. "deepseek" sat in it from the start, on the note "no vision model in
// the family as of 2026-07" — a fact with a date in it, filed in a place that
// has no way to notice the date passing. DeepSeek shipped one, the owner picked
// deepseek-v4-flash-vision-exp in the composer, and the family name beat the
// word "vision" in the model's own id: Aetox called a vision model blind and
// sent the screenshot to image_ocr instead of to the eyes that were right there
// (22 ส.ค.). It was dead weight besides — an unknown model already resolves to
// blind, so a family listed as text-only changed nothing except the one case
// where the member's own name disagreed with it.
//
// So the split is the point. A role marker is a fact about what the model IS
// and never expires; a family marker is a fact about what a company had SHIPPED
// on the day someone typed it, and the fallback below already covers that case
// without going stale.
var textOnlyRoleMarkers = []string{
	"embed", "rerank", "whisper", "tts",
}

// ResolveVision reports whether the named model accepts image input.
//
// provider is Aetox's own provider key, not the wire vendor: it matters only
// for the cases where the same model name means different things, so most of
// the decision is made on the model id.
func ResolveVision(provider, modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "" {
		return false
	}
	for _, marker := range textOnlyRoleMarkers {
		if strings.Contains(name, marker) {
			return false
		}
	}
	for _, marker := range visionModelMarkers {
		if strings.Contains(name, marker) {
			return true
		}
	}
	// An unknown model is treated as blind. It costs a working OCR path; the
	// other way costs an image silently discarded and an answer invented from
	// the caption.
	return false
}
