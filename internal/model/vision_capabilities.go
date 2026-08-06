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

// textOnlyMarkers win over the list above. Some families ship a vision member
// and a text-only member whose names differ by one word, and matching the
// family name alone would call the text-only one sighted.
var textOnlyMarkers = []string{
	"deepseek", // no vision model in the family as of 2026-07
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
	for _, marker := range textOnlyMarkers {
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
