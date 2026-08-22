package model

import "strings"

// ContextWindowTokens reports a model's total context window in tokens,
// curated per provider the same way thinking_capabilities.go curates levels.
// 0 means unknown — callers decide their own fallback. User overrides
// (ModelContextTokens config/flag) always win at the call site.
func ContextWindowTokens(provider, modelName string) int {
	canonical := NormalizeProvider(provider)
	modelID := strings.ToLower(strings.TrimSpace(modelName))
	if modelID == "" {
		modelID = strings.ToLower(strings.TrimSpace(DefaultModel(canonical)))
	}

	// The fetched catalog first, because it states the window per model where
	// everything below states it per provider with a fallback — and the
	// fallback is what most models actually get. The Gemini branch knows one
	// prefix while an account's own discovery lists 37 models, so every 3.x
	// model was measured against a guess, and that guess is the percentage on
	// the composer the user reads all day. Where both answer they agree
	// (deepseek-v4: 1M either way) and the catalog is the more exact of the two
	// (glm-4.5 is 131,072, not the 128,000 the default rounds it to).
	//
	// Absent or silent, it changes nothing: the curated tables below are still
	// the answer, which is what makes this safe to consult on a path this hot.
	if tokens := catalogContextWindow(canonical, modelID); tokens > 0 {
		return tokens
	}

	switch canonical {
	case "deepseek":
		return deepseekContextWindow(modelID)
	case "openai":
		return openaiContextWindow(modelID)
	case "codex":
		// Codex serves OpenAI's models through a subscription, so the window is
		// OpenAI's. Asked here by recursion — through the catalog under
		// "openai" first, then the curated table — rather than by teaching
		// modelsDevProvider that codex is openai, which is the shorter change
		// and the wrong one: ModelCatalog.For is also what PRICES a model, and
		// filing Codex under openai would put OpenAI's per-token rates on a
		// flat monthly plan. That is the exact bill token_usage.provider was
		// added to prevent (db.go, migration 15). One provider, two facts, and
		// only one of them is allowed to travel.
		//
		// Until 2026-08-18 this was the `default` case, and the 0 it returned
		// was not read as "unknown" by the desktop: App.contextWindowTokens
		// fell back to the agent's char budget over four, so the meter drew a
		// 32,000-token window on a model that had already accepted 43,434 in
		// one request. The fallback is gone; this is the half that makes the
		// answer real rather than merely absent.
		return ContextWindowTokens("openai", modelID)
	case "anthropic":
		return 200_000
	case "gemini":
		return geminiContextWindow(modelID)
	case "zai":
		return zaiContextWindow(modelID)
	case "groq":
		return 128_000
	case "xai":
		return xaiContextWindow(modelID)
	case "thaillm":
		return thaiLLMContextWindow(modelID)
	case "kimi":
		return kimiContextWindow(modelID)
	case "openrouter":
		// OpenRouter ids are "vendor/model" — resolve by the underlying vendor.
		if vendor, name, ok := strings.Cut(modelID, "/"); ok {
			return ContextWindowTokens(vendor, name)
		}
		return 0
	default:
		// ollama and unknown providers: no promise we can keep.
		//
		// modelscope, nvidia and ollama-cloud are here by decision, not by
		// omission. The fetched catalog above already answers for the models it
		// describes, and for the rest — which on these three is most of them,
		// since all three serve far more than any table lists — the only
		// figures available are checkpoint numbers off model cards —
		// which is the source that put 32,768 on a ThaiLLM deployment serving
		// 16,384. A curated line here would be that same guess with a nicer
		// home. Fill it in when the endpoint refuses something and says so.
		return 0
	}
}

// ThaiLLM is the one row where both sources are silent: models.dev has never
// heard of it (192 providers, no thaillm / typhoon / scb10x), and the service
// serves no window of its own.
//
// 16,384 is not read off a model card. It is what the service itself said when
// it refused a real turn on 2026-08-20:
//
//	400: 'max_tokens' is too large: 8192. This model's maximum context length
//	is 16384 tokens and your request has 9791 input tokens
//
// That matters more than it looks, because this table first shipped with
// 32,768 for the same model, taken from max_position_embeddings in the weights
// on Hugging Face. The weights were not wrong; they were the wrong source. What
// a checkpoint can address and what a deployment is configured to serve are two
// different numbers, and only the second one answers a request. Any figure
// added here later should come from the endpoint refusing something, not from a
// config file.
//
// One model proved it and the other three are assumed to share it: all four are
// 8B checkpoints of the same foundation model on one platform's deployment. If
// that assumption is wrong it is wrong downward, and this figure only ever
// makes Aetox ask for less.
//
// The two Qwen models get nothing. They are Alibaba's, on a deployment config
// nobody has published, and neither this service nor their cards say what
// window they are given — a guess there would be a number Aetox made up, which
// is exactly what the zero is for.
func thaiLLMContextWindow(modelID string) int {
	switch {
	case strings.HasPrefix(modelID, "qwen"):
		return 0
	case strings.Contains(modelID, "thaillm"):
		return 16_384
	default:
		return 0
	}
}

// Grok's windows are not one number: the newer and smarter the model, the
// SMALLER the window, which is the opposite of the usual direction and the
// reason this is a function rather than a constant. grok-4.6 and 4.5 hold
// 500k, while the older 4.3 and the 4.20 line hold a full 1M, and the
// build-tuned model holds 256k (docs.x.ai/developers/models, read 2026-08-20).
//
// The default is 4.6's 500k rather than the largest, because over-promising is
// the failure that matters here: the composer meter reads this number, and a
// meter that says 40% full when the request is about to be rejected is worse
// than one that says 80% on a model that could have taken more.
func xaiContextWindow(modelID string) int {
	switch {
	case strings.HasPrefix(modelID, "grok-build"):
		return 256_000
	case strings.HasPrefix(modelID, "grok-4.3"), strings.HasPrefix(modelID, "grok-4.20"):
		return 1_000_000
	default:
		return 500_000 // grok-4.6, grok-4.5, and anything newer on that line
	}
}

func deepseekContextWindow(modelID string) int {
	if strings.HasPrefix(modelID, "deepseek-v4") {
		return 1_000_000 // V4 series (incl. -flash): 1M context per DeepSeek docs
	}
	return 128_000 // deepseek-chat / deepseek-reasoner / V3.x
}

func kimiContextWindow(modelID string) int {
	if strings.HasPrefix(modelID, "kimi-k3") {
		return 1_000_000 // "a 1M-token context window" — K3 quickstart
	}
	return 128_000 // K2 and the moonshot-v1 line
}

func openaiContextWindow(modelID string) int {
	switch {
	case strings.HasPrefix(modelID, "gpt-5"):
		return 400_000
	case strings.HasPrefix(modelID, "gpt-4.1"):
		return 1_000_000
	case strings.HasPrefix(modelID, "o3"), strings.HasPrefix(modelID, "o4"):
		return 200_000
	default:
		return 128_000 // gpt-4o and friends
	}
}

func geminiContextWindow(modelID string) int {
	if strings.HasPrefix(modelID, "gemini-1.5-pro") {
		return 2_000_000
	}
	return 1_000_000 // 1.5-flash, 2.x series
}

func zaiContextWindow(modelID string) int {
	if strings.HasPrefix(modelID, "glm-4.6") {
		return 200_000
	}
	return 128_000
}
