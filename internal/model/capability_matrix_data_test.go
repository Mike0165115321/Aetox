package model

// Real rows from models.dev, captured 2026-08-23.
//
// Two sources, merged: models chosen by what they can DO (one that reads pdf,
// one that only sees, one text-only, one that cannot call a tool) for every
// provider that catalog describes, plus every model the tests in this package
// name. The second half matters as much as the first — a resolver migrated to
// the catalog answers "unknown" for a model the fixture omits, which reads as
// a broken resolver rather than as a thin fixture.
//
// Keyed the way distill() keys them, by the MODELS.DEV provider name: Aetox
// says gemini and kimi where that table says google and moonshotai. The first
// draft keyed by the Aetox name and every gemini and qwen row silently missed;
// the matrix caught it on the first run. The qwen half of that lesson stopped
// applying on 2026-08-24, when the row was renamed to `alibaba` and the two
// catalogs started agreeing on it — see DECISIONS §175.
var capabilityMatrixRows = map[string]ModelFacts{
	// anthropic (anthropic): rich
	"anthropic/claude-opus-5": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// deepseek (deepseek): plain
	"deepseek/deepseek-chat": {Context: 1000000, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// deepseek (deepseek): seeing
	"deepseek/deepseek-v4-flash-vision-exp": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// gemini (google): rich
	"google/gemini-2.5-pro": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "audio", "video", "pdf"}, Output: []string{"text"}},
	// gemini (google): notool
	"google/gemini-3-pro-image": {Context: 131072, ToolCall: false, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "high"}, Input: []string{"text", "image"}, Output: []string{"text", "image"}},
	// gemini (google): seeing
	"google/gemma-4-31b-it": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text"}},
	// groq (groq): notool
	"groq/allam-2-7b": {Context: 4096, ToolCall: false, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// groq (groq): plain
	"groq/openai/gpt-oss-20b": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text"}, Output: []string{"text"}},
	// groq (groq): seeing
	"groq/qwen/qwen3.6-27b": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"none"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// kimi (moonshotai): plain
	"moonshotai/kimi-k2-thinking": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// kimi (moonshotai): seeing
	"moonshotai/kimi-k3": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: true, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// minimax (minimax): plain
	"minimax/minimax-m2": {Context: 196608, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// minimax (minimax): seeing
	"minimax/minimax-m3": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// mistral (mistral): notool
	"mistral/mistral-embed": {Context: 8000, ToolCall: false, Reasoning: false, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// mistral (mistral): plain
	"mistral/mistral-nemo": {Context: 128000, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// mistral (mistral): seeing
	"mistral/pixtral-12b": {Context: 128000, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text"}},
	// modelscope (modelscope): plain
	"modelscope/zhipuai/glm-4.5": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// nvidia (nvidia): notool
	"nvidia/baai/bge-m3": {Context: 8192, ToolCall: false, Reasoning: false, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// nvidia (nvidia): seeing
	"nvidia/moonshotai/kimi-k3": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: true, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// nvidia (nvidia): plain
	"nvidia/z-ai/glm-5.2": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// ollama-cloud (ollama-cloud): plain
	"ollama-cloud/glm-5.2": {Context: 976000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// ollama-cloud (ollama-cloud): seeing
	"ollama-cloud/kimi-k3": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: true, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openai (openai): plain
	"openai/gpt-4": {Context: 8192, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// openai (openai): seeing
	"openai/gpt-5": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"minimal", "low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openai (openai): notool
	"openai/gpt-image-1": {Context: 0, ToolCall: false, Reasoning: false, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"image"}},
	// openai (openai): rich
	"openai/o1": {Context: 200000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode-go (opencode-go): rich
	"opencode-go/gpt-5.6-luna": {Context: 1050000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode-go (opencode-go): plain
	"opencode-go/hy3": {Context: 256000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"none", "low", "high"}, Input: []string{"text"}, Output: []string{"text"}},
	// opencode-go (opencode-go): seeing
	"opencode-go/kimi-k3": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"max"}, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// opencode (opencode): plain
	"opencode/glm-5": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): seeing
	"opencode/gpt-5": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"minimal", "low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode (opencode): rich
	"opencode/gpt-5.5": {Context: 1050000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): rich
	"openrouter/openai/o1": {Context: 200000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): notool
	"openrouter/openai/o1-pro": {Context: 200000, ToolCall: false, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: nil, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): seeing
	"openrouter/z-ai/glm-4.5v": {Context: 65536, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openrouter (openrouter): plain
	"openrouter/z-ai/glm-5": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// alibaba (alibaba): plain
	"alibaba/glm-5.2": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// alibaba (alibaba): seeing
	"alibaba/qvq-max": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text"}},
	// alibaba (alibaba): notool
	"alibaba/qwen-vl-ocr": {Context: 34096, ToolCall: false, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text"}},
	// alibaba (alibaba): rich
	"alibaba/qwen3.8-max": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "xhigh"}, Input: []string{"text", "image", "video", "pdf"}, Output: []string{"text"}},
	// xai (xai): rich
	"xai/grok-4.6": {Context: 500000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high", "xhigh"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// xai (xai): notool
	"xai/grok-imagine-video": {Context: 1024, ToolCall: false, Reasoning: false, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: nil, Input: []string{"text", "image", "video", "pdf"}, Output: []string{"video"}},
	// zai (zai): seeing
	"zai/glm-4.5v": {Context: 64000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// zai (zai): plain
	"zai/glm-5": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// zai (zai): rich
	"zai/glm-5v-turbo": {Context: 200000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video", "pdf"}, Output: []string{"text"}},
	// anthropic (anthropic): named-by-tests
	"anthropic/claude-fable-5": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// anthropic (anthropic): named-by-tests
	"anthropic/claude-haiku-4-5": {Context: 200000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// anthropic (anthropic): named-by-tests
	"anthropic/claude-opus-4-8": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// anthropic (anthropic): named-by-tests
	"anthropic/claude-sonnet-4-5": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// anthropic (anthropic): named-by-tests
	"anthropic/claude-sonnet-5": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-4.1-mini": {Context: 1047576, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-4o": {Context: 128000, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-4o-mini": {Context: 128000, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-5-mini": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"minimal", "low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-5-nano": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"minimal", "low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-5-pro": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-5.1": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-5.2": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-5.5": {Context: 1050000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-5.6-luna": {Context: 1050000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/gpt-5.6-sol": {Context: 1050000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openai (openai): named-by-tests
	"openai/o4-mini": {Context: 200000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// gemini (google): named-by-tests
	"google/gemini-2.5-flash": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "audio", "video", "pdf"}, Output: []string{"text"}},
	// gemini (google): named-by-tests
	"google/gemini-2.5-flash-lite": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "audio", "video", "pdf"}, Output: []string{"text"}},
	// gemini (google): named-by-tests
	"google/gemini-3.7-flash": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image", "video", "audio", "pdf"}, Output: []string{"text"}},
	// groq (groq): named-by-tests
	"groq/llama-3.1-8b-instant": {Context: 131072, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// groq (groq): named-by-tests
	"groq/llama-3.3-70b-versatile": {Context: 131072, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// groq (groq): named-by-tests
	"groq/openai/gpt-oss-120b": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text"}, Output: []string{"text"}},
	// groq (groq): named-by-tests
	"groq/whisper-large-v3": {Context: 0, ToolCall: false, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"audio"}, Output: []string{"text"}},
	// kimi (moonshotai): named-by-tests
	"moonshotai/kimi-k2.5": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: true, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// kimi (moonshotai): named-by-tests
	"moonshotai/kimi-k2.6": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// minimax (minimax): named-by-tests
	"minimax/minimax-m2.5": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// minimax (minimax): named-by-tests
	"minimax/minimax-m2.7": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// nvidia (nvidia): named-by-tests
	"nvidia/google/gemma-3-12b-it": {Context: 131072, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text"}},
	// nvidia (nvidia): named-by-tests
	"nvidia/google/gemma-4-31b-it": {Context: 256000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// nvidia (nvidia): named-by-tests
	"nvidia/nvidia/nv-embed-v1": {Context: 32768, ToolCall: false, Reasoning: false, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// nvidia (nvidia): named-by-tests
	"nvidia/openai/gpt-oss-120b": {Context: 128000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text"}, Output: []string{"text"}},
	// nvidia (nvidia): named-by-tests
	"nvidia/openai/gpt-oss-20b": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text"}, Output: []string{"text"}},
	// ollama-cloud (ollama-cloud): named-by-tests
	"ollama-cloud/minimax-m2.5": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// ollama-cloud (ollama-cloud): named-by-tests
	"ollama-cloud/minimax-m2.7": {Context: 196608, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// ollama-cloud (ollama-cloud): named-by-tests
	"ollama-cloud/minimax-m3": {Context: 512000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high", "max"}, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// ollama-cloud (ollama-cloud): named-by-tests
	"ollama-cloud/deepseek-v4-flash": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// ollama-cloud (ollama-cloud): named-by-tests
	"ollama-cloud/deepseek-v4-pro": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// ollama-cloud (ollama-cloud): named-by-tests
	"ollama-cloud/kimi-k2.5": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text"}},
	// ollama-cloud (ollama-cloud): named-by-tests
	"ollama-cloud/kimi-k2.6": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/minimax-m2.5": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/minimax-m2.7": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/minimax-m3": {Context: 512000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/big-pickle": {Context: 200000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/claude-fable-5": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/claude-haiku-4-5": {Context: 200000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/claude-opus-4-8": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/claude-opus-5": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/claude-sonnet-4-5": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/claude-sonnet-5": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/deepseek-v4-flash": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/deepseek-v4-pro": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/gemini-3-pro": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "high"}, Input: []string{"text", "image", "video", "audio", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/gemini-3.7-flash": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image", "video", "audio", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/glm-4.6": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/glm-5.2": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/gpt-5-nano": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"minimal", "low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/gpt-5.1": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/gpt-5.1-codex": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/gpt-5.2": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/gpt-5.2-codex": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/gpt-5.6-luna": {Context: 1050000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/gpt-5.6-sol": {Context: 1050000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/grok-4.5": {Context: 500000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/grok-4.6": {Context: 500000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high", "xhigh"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/grok-code": {Context: 256000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/kimi-k2": {Context: 262144, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/kimi-k2-thinking": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/kimi-k2.5": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/kimi-k2.6": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// opencode (opencode): named-by-tests
	"opencode/kimi-k3": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"max"}, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/minimax-m2.5": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/minimax-m2.7": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/minimax-m3": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/deepseek-v4-flash": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/deepseek-v4-flash-vision-exp": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/deepseek-v4-pro": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/glm-5": {Context: 202752, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/glm-5.2": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/grok-4.5": {Context: 500000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/kimi-k2.5": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/kimi-k2.6": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/qwen3.7-plus": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// opencode-go (opencode-go): named-by-tests
	"opencode-go/qwen3.8-max": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/anthropic/claude-opus-5": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/anthropic/claude-sonnet-4": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"image", "text", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/deepseek/deepseek-chat": {Context: 163840, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/deepseek/deepseek-r1": {Context: 64000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/deepseek/deepseek-v4-flash": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"high", "xhigh"}, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/deepseek/deepseek-v4-flash-vision-exp": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/google/gemini-2.5-pro": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "audio", "video", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/google/gemini-3-pro-image": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text", "image"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/google/gemini-3.7-flash": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image", "video", "audio", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/google/gemma-3-12b-it": {Context: 131072, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/google/gemma-4-31b-it": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"image", "text", "video"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/minimax/minimax-m2": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/minimax/minimax-m3": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/moonshotai/kimi-k2-thinking": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/moonshotai/kimi-k3": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/openai/gpt-3.5-turbo": {Context: 16385, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/openai/gpt-4": {Context: 8191, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/openai/gpt-4o": {Context: 128000, ToolCall: true, Reasoning: false, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/openai/gpt-5": {Context: 400000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"minimal", "low", "medium", "high"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/openai/gpt-5.6-luna": {Context: 1050000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: true, ReasoningLevels: []string{"none", "low", "medium", "high", "xhigh", "max"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/openai/gpt-oss-120b": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/openai/gpt-oss-20b": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/qwen/qwen3-32b": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/qwen/qwen3-8b": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/qwen/qwen3.6-27b": {Context: 262144, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// openrouter (openrouter): named-by-tests
	"openrouter/z-ai/glm-5.2": {Context: 1048576, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"high", "xhigh"}, Input: []string{"text"}, Output: []string{"text"}},
	// alibaba (alibaba): named-by-tests
	"alibaba/qwen3.7-plus": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text", "image", "video"}, Output: []string{"text"}},
	// xai (xai): named-by-tests
	"xai/grok-4.5": {Context: 500000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"low", "medium", "high"}, Input: []string{"text", "image", "pdf"}, Output: []string{"text"}},
	// zai (zai): named-by-tests
	"zai/glm-4.5": {Context: 131072, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// zai (zai): named-by-tests
	"zai/glm-4.6": {Context: 204800, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// zai (zai): named-by-tests
	"zai/glm-5.2": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: []string{"high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// deepseek (deepseek): named-by-tests
	"deepseek/deepseek-reasoner": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: false, NoTemperature: false, ReasoningLevels: nil, Input: []string{"text"}, Output: []string{"text"}},
	// deepseek (deepseek): named-by-tests
	"deepseek/deepseek-v4-flash": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"low", "high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
	// deepseek (deepseek): named-by-tests
	"deepseek/deepseek-v4-pro": {Context: 1000000, ToolCall: true, Reasoning: true, ReasoningToggle: true, NoTemperature: false, ReasoningLevels: []string{"high", "max"}, Input: []string{"text"}, Output: []string{"text"}},
}

// capabilityMatrixPairs is the walk order: an Aetox provider name and one of
// its models, which is what every function under test is actually called with.
var capabilityMatrixPairs = []struct{ Provider, Model string }{
	{"anthropic", "claude-opus-5"},
	{"deepseek", "deepseek-chat"},
	{"deepseek", "deepseek-v4-flash-vision-exp"},
	{"gemini", "gemini-2.5-pro"},
	{"gemini", "gemini-3-pro-image"},
	{"gemini", "gemma-4-31b-it"},
	{"groq", "allam-2-7b"},
	{"groq", "openai/gpt-oss-20b"},
	{"groq", "qwen/qwen3.6-27b"},
	{"kimi", "kimi-k2-thinking"},
	{"kimi", "kimi-k3"},
	{"minimax", "MiniMax-M2"},
	{"minimax", "MiniMax-M3"},
	{"mistral", "mistral-embed"},
	{"mistral", "mistral-nemo"},
	{"mistral", "pixtral-12b"},
	{"modelscope", "ZhipuAI/GLM-4.5"},
	{"nvidia", "baai/bge-m3"},
	{"nvidia", "moonshotai/kimi-k3"},
	{"nvidia", "z-ai/glm-5.2"},
	{"ollama-cloud", "glm-5.2"},
	{"ollama-cloud", "kimi-k3"},
	{"openai", "gpt-4"},
	{"openai", "gpt-5"},
	{"openai", "gpt-image-1"},
	{"openai", "o1"},
	{"opencode-go", "gpt-5.6-luna"},
	{"opencode-go", "hy3"},
	{"opencode-go", "kimi-k3"},
	{"opencode", "glm-5"},
	{"opencode", "gpt-5"},
	{"opencode", "gpt-5.5"},
	{"openrouter", "openai/o1"},
	{"openrouter", "openai/o1-pro"},
	{"openrouter", "z-ai/glm-4.5v"},
	{"openrouter", "z-ai/glm-5"},
	{"alibaba", "glm-5.2"},
	{"alibaba", "qvq-max"},
	{"alibaba", "qwen-vl-ocr"},
	{"alibaba", "qwen3.8-max"},
	{"xai", "grok-4.6"},
	{"xai", "grok-imagine-video"},
	{"zai", "glm-4.5v"},
	{"zai", "glm-5"},
	{"zai", "glm-5v-turbo"},
	{"anthropic", "claude-fable-5"},
	{"anthropic", "claude-haiku-4-5"},
	{"anthropic", "claude-opus-4-8"},
	{"anthropic", "claude-sonnet-4-5"},
	{"anthropic", "claude-sonnet-5"},
	{"openai", "gpt-4.1-mini"},
	{"openai", "gpt-4o"},
	{"openai", "gpt-4o-mini"},
	{"openai", "gpt-5-mini"},
	{"openai", "gpt-5-nano"},
	{"openai", "gpt-5-pro"},
	{"openai", "gpt-5.1"},
	{"openai", "gpt-5.2"},
	{"openai", "gpt-5.5"},
	{"openai", "gpt-5.6-luna"},
	{"openai", "gpt-5.6-sol"},
	{"openai", "o4-mini"},
	{"gemini", "gemini-2.5-flash"},
	{"gemini", "gemini-2.5-flash-lite"},
	{"gemini", "gemini-3.7-flash"},
	{"groq", "llama-3.1-8b-instant"},
	{"groq", "llama-3.3-70b-versatile"},
	{"groq", "openai/gpt-oss-120b"},
	{"groq", "whisper-large-v3"},
	{"kimi", "kimi-k2.5"},
	{"kimi", "kimi-k2.6"},
	{"minimax", "MiniMax-M2.5"},
	{"minimax", "MiniMax-M2.7"},
	{"nvidia", "google/gemma-3-12b-it"},
	{"nvidia", "google/gemma-4-31b-it"},
	{"nvidia", "nvidia/nv-embed-v1"},
	{"nvidia", "openai/gpt-oss-120b"},
	{"nvidia", "openai/gpt-oss-20b"},
	{"ollama-cloud", "minimax-m2.5"},
	{"ollama-cloud", "minimax-m2.7"},
	{"ollama-cloud", "minimax-m3"},
	{"ollama-cloud", "deepseek-v4-flash"},
	{"ollama-cloud", "deepseek-v4-pro"},
	{"ollama-cloud", "kimi-k2.5"},
	{"ollama-cloud", "kimi-k2.6"},
	{"opencode", "minimax-m2.5"},
	{"opencode", "minimax-m2.7"},
	{"opencode", "minimax-m3"},
	{"opencode", "big-pickle"},
	{"opencode", "claude-fable-5"},
	{"opencode", "claude-haiku-4-5"},
	{"opencode", "claude-opus-4-8"},
	{"opencode", "claude-opus-5"},
	{"opencode", "claude-sonnet-4-5"},
	{"opencode", "claude-sonnet-5"},
	{"opencode", "deepseek-v4-flash"},
	{"opencode", "deepseek-v4-pro"},
	{"opencode", "gemini-3-pro"},
	{"opencode", "gemini-3.7-flash"},
	{"opencode", "glm-4.6"},
	{"opencode", "glm-5.2"},
	{"opencode", "gpt-5-nano"},
	{"opencode", "gpt-5.1"},
	{"opencode", "gpt-5.1-codex"},
	{"opencode", "gpt-5.2"},
	{"opencode", "gpt-5.2-codex"},
	{"opencode", "gpt-5.6-luna"},
	{"opencode", "gpt-5.6-sol"},
	{"opencode", "grok-4.5"},
	{"opencode", "grok-4.6"},
	{"opencode", "grok-code"},
	{"opencode", "kimi-k2"},
	{"opencode", "kimi-k2-thinking"},
	{"opencode", "kimi-k2.5"},
	{"opencode", "kimi-k2.6"},
	{"opencode", "kimi-k3"},
	{"opencode-go", "minimax-m2.5"},
	{"opencode-go", "minimax-m2.7"},
	{"opencode-go", "minimax-m3"},
	{"opencode-go", "deepseek-v4-flash"},
	{"opencode-go", "deepseek-v4-flash-vision-exp"},
	{"opencode-go", "deepseek-v4-pro"},
	{"opencode-go", "glm-5"},
	{"opencode-go", "glm-5.2"},
	{"opencode-go", "grok-4.5"},
	{"opencode-go", "kimi-k2.5"},
	{"opencode-go", "kimi-k2.6"},
	{"opencode-go", "qwen3.7-plus"},
	{"opencode-go", "qwen3.8-max"},
	{"openrouter", "anthropic/claude-opus-5"},
	{"openrouter", "anthropic/claude-sonnet-4"},
	{"openrouter", "deepseek/deepseek-chat"},
	{"openrouter", "deepseek/deepseek-r1"},
	{"openrouter", "deepseek/deepseek-v4-flash"},
	{"openrouter", "deepseek/deepseek-v4-flash-vision-exp"},
	{"openrouter", "google/gemini-2.5-pro"},
	{"openrouter", "google/gemini-3-pro-image"},
	{"openrouter", "google/gemini-3.7-flash"},
	{"openrouter", "google/gemma-3-12b-it"},
	{"openrouter", "google/gemma-4-31b-it"},
	{"openrouter", "minimax/minimax-m2"},
	{"openrouter", "minimax/minimax-m3"},
	{"openrouter", "moonshotai/kimi-k2-thinking"},
	{"openrouter", "moonshotai/kimi-k3"},
	{"openrouter", "openai/gpt-3.5-turbo"},
	{"openrouter", "openai/gpt-4"},
	{"openrouter", "openai/gpt-4o"},
	{"openrouter", "openai/gpt-5"},
	{"openrouter", "openai/gpt-5.6-luna"},
	{"openrouter", "openai/gpt-oss-120b"},
	{"openrouter", "openai/gpt-oss-20b"},
	{"openrouter", "qwen/qwen3-32b"},
	{"openrouter", "qwen/qwen3-8b"},
	{"openrouter", "qwen/qwen3.6-27b"},
	{"openrouter", "z-ai/glm-5.2"},
	{"alibaba", "qwen3.7-plus"},
	{"xai", "grok-4.5"},
	{"zai", "glm-4.5"},
	{"zai", "glm-4.6"},
	{"zai", "glm-5.2"},
	{"deepseek", "deepseek-reasoner"},
	{"deepseek", "deepseek-v4-flash"},
	{"deepseek", "deepseek-v4-pro"},
}
