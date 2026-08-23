package model

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

// visionModelMarkers is the answer of last resort, for the models no catalog
// describes.
//
// It used to be the whole answer, and matching substrings of an id was already
// the better half of a bad choice — a table of exact ids would have been wrong
// faster. It was still wrong: measured against models.dev on 2026-08-23 it
// called 13 of opencode-go's 28 models blind when they can see, 18 of 93 on
// opencode, and 99 of 360 on openrouter. The owner met it as a screenshot going
// to image_ocr instead of to qwen3.7-plus, a model that takes text, image and
// video. No name in that list resembles "qwen3.7-plus", and none ever could:
// a company names its models before this file hears about them.
//
// It stays because it is the only answer for a local runtime. models.dev
// describes what providers serve, not what somebody pulled onto their own GPU,
// so llava and moondream on Ollama have nothing else to be recognised by. The
// order in ResolveVision is what matters now: the catalog first where it knows
// the model, this list only where it does not.
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
// Three answers are consulted, in descending order of how much they know:
//
//  1. The role markers, which outrank everything. An embedder that takes images
//     is still not a chat model, and no catalog entry changes that.
//  2. The fetched catalog, per model. This is the authority wherever it has
//     heard of the pair — it is the provider's own published modality list,
//     refreshed, rather than a guess from the shape of a name.
//  3. The name markers, for the models nothing published describes: whatever
//     the user pulled onto their own machine.
//
// Catalog above markers rather than beside them, because the two disagree in
// exactly one direction that matters. A model the catalog says cannot see and
// whose name happens to contain "vision" is the deepseek-v4-flash-vision-exp
// case in reverse, and believing the name there sends an image part to a
// backend that will 400 or, worse, drop it and answer about a picture it never
// received.
// One question asked of the one record, rather than a fourth resolver with a
// fourth idea of what to do when the answer is not known. The precedence it
// used to spell out for itself now lives in capabilities.go, stated once for
// every modality — which is the point: pdf, audio and video are already in the
// catalog, and none of them should need a function of its own.
//
// An unknown model is still treated as blind. That costs a working OCR path;
// the other way costs an image silently discarded and an answer invented from
// the caption.
func ResolveVision(provider, modelName string) bool {
	return resolveModalities(provider, modelName).Accepts("image")
}
