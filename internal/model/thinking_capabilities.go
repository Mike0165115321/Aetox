package model

import (
	"strings"
)

type ThinkingRuntime string

const (
	ThinkingRuntimeUnknown         ThinkingRuntime = "unknown"
	ThinkingRuntimeReasoningObject ThinkingRuntime = "reasoning-object"
	ThinkingRuntimeReasoningEffort ThinkingRuntime = "reasoning-effort"
	ThinkingRuntimeDeepSeek        ThinkingRuntime = "deepseek-thinking"
	ThinkingRuntimeMiniMax         ThinkingRuntime = "minimax-thinking"
	ThinkingRuntimeGroq            ThinkingRuntime = "groq-reasoning"
)

// ThinkingCapabilities is the single answer to "what thinking depths does this
// provider/model have, and what does each one look like on the wire".
//
// Both halves live here on purpose. They used to be four tables in three files
// — the picker's list, the fallback that folded unknown levels onto known ones,
// and one effort mapping per wire format — and no two of them agreed. DeepSeek
// was the proof: the picker offered off/high/max, one mapper knew medium and
// xhigh that the picker could never produce, the other knew only high and max,
// and none of them had `low`, which the API has had all along. A level is only
// real if it can be shown AND sent, so the two facts belong in one place.
type ThinkingCapabilities struct {
	Supported bool
	Native    bool
	Levels    []string
	Default   string
	Runtime   ThinkingRuntime
	Source    string

	// Wire is what each level in Levels is actually called in the request. Not
	// always the level's own name, and an entry may be "" — meaning the level
	// is expressed by leaving the effort field out entirely rather than by any
	// value (how "off" is sent everywhere: the thinking block carries it).
	//
	// A level in Levels with no Wire entry is a bug this package can catch:
	// see TestEveryOfferedLevelHasAWireValue.
	Wire map[string]string

	// Aliases fold a level this provider does not offer onto one it does, so a
	// setting carried over from another provider lands somewhere sensible
	// instead of silently reverting to the default.
	Aliases map[string]string
}

// WireEffort returns the effort string to put on the wire for level, and
// whether the field should be sent at all. Every wire format asks this one
// function rather than keeping its own switch.
func WireEffort(provider, modelName, level string) (string, bool) {
	caps := ResolveThinkingCapabilities(provider, modelName)
	if !caps.Supported {
		return "", false
	}
	normalized := strings.ToLower(strings.TrimSpace(level))
	if normalized == "" {
		return "", false
	}
	if alias, ok := caps.Aliases[normalized]; ok {
		normalized = alias
	}
	effort, ok := caps.Wire[normalized]
	if !ok || effort == "" {
		return "", false
	}
	return effort, true
}

// A model this table does not recognise gets no dial.
//
// These two used to offer low/medium/high/off on a guess, and the guess was
// wrong in the case that matters most: a provider's own default. OpenAI's is
// gpt-4o-mini and Groq's is llama-3.3-70b-versatile — neither has a reasoning
// knob — yet both drew a full picker, and because their providers *can* carry
// reasoning_effort the chosen level really was put on the wire, at a model that
// does not take it. A fabricated menu was the visible half; a request the API
// may reject was the other.
//
// The cost of this is a real new model losing its dial until it is added here.
// That is the right way round: a missing control is a thing the user can see
// and report, an inert one is not.
var fallbackThinkingCapabilities = ThinkingCapabilities{
	Supported: false,
	Native:    false,
	Levels:    nil,
	Default:   "",
	Runtime:   ThinkingRuntimeUnknown,
	Source:    "unknown-model",
}

var conservativeFallback = ThinkingCapabilities{
	Supported: false,
	Native:    false,
	Levels:    nil,
	Default:   "",
	Runtime:   ThinkingRuntimeUnknown,
	Source:    "unknown-model-of-known-provider",
}

var noThinkingCapabilities = ThinkingCapabilities{
	Supported: false,
	Native:    false,
	Levels:    nil,
	Default:   "",
	Runtime:   ThinkingRuntimeUnknown,
	Source:    "no-thinking-knob",
}

var unknownProviderCapabilities = ThinkingCapabilities{
	Supported: false,
	Native:    false,
	Levels:    nil,
	Default:   "",
	Runtime:   ThinkingRuntimeUnknown,
	Source:    "unknown-provider",
}

func ResolveThinkingCapabilities(provider, modelName string) ThinkingCapabilities {
	canonicalProvider := NormalizeProvider(provider)

	// Can this provider carry a thinking setting at all? The catalog decides,
	// and the request path already asks it the same question — a provider whose
	// runtime cannot put an effort anywhere gets no menu here.
	//
	// Without this the table answered on its own and offered four levels to
	// seven providers that send nothing: aetox, cohere, mistral, perplexity,
	// qwen, together and zai each drew a full off/low/medium/high picker in
	// which every entry, "off" included, was inert. A control that does nothing
	// is worse than an absent one — it teaches the user that the controls lie.
	known, canReason := providerReasoningCapability(canonicalProvider)
	if !known {
		return cloneThinkingCapabilities(unknownProviderCapabilities)
	}
	if !canReason {
		return cloneThinkingCapabilities(noThinkingCapabilities)
	}

	modelID := strings.ToLower(strings.TrimSpace(modelName))
	if modelID == "" {
		modelID = strings.ToLower(strings.TrimSpace(DefaultModel(canonicalProvider)))
	}

	switch canonicalProvider {
	case "deepseek":
		return cloneThinkingCapabilities(resolveDeepSeekThinkingCapabilities(modelID))
	case "gemini":
		return cloneThinkingCapabilities(resolveGeminiThinkingCapabilities(modelID))
	case "openai":
		return cloneThinkingCapabilities(resolveOpenAIThinkingCapabilities(modelID))
	case "openrouter":
		return cloneThinkingCapabilities(resolveOpenRouterThinkingCapabilities(modelID))
	case "groq":
		return cloneThinkingCapabilities(resolveGroqThinkingCapabilities(modelID))
	case "kimi":
		return cloneThinkingCapabilities(resolveKimiThinkingCapabilities(modelID))
	case "minimax":
		return cloneThinkingCapabilities(resolveMiniMaxThinkingCapabilities(modelID))
	case "anthropic":
		return cloneThinkingCapabilities(resolveAnthropicThinkingCapabilities(modelID))
	case "codex":
		return cloneThinkingCapabilities(responsesThinkingCapabilities)
	default:
		// Unreachable while every reasoning-capable provider in the catalog has
		// a case above; ollama and lmstudio no longer need one of their own,
		// because the capability gate at the top already turned them away.
		return cloneThinkingCapabilities(fallbackThinkingCapabilities)
	}
}

func SupportedThinkingLevels(provider, modelName string) []string {
	caps := ResolveThinkingCapabilities(provider, modelName)
	return append([]string{}, caps.Levels...)
}

func SupportsThinkingLevel(provider, modelName, level string) bool {
	normalized := strings.ToLower(strings.TrimSpace(level))
	if normalized == "" {
		return false
	}
	for _, supported := range SupportedThinkingLevels(provider, modelName) {
		if normalized == supported {
			return true
		}
	}
	return false
}

func NormalizeThinkingLevel(provider, modelName, requested string) string {
	caps := ResolveThinkingCapabilities(provider, modelName)
	if !caps.Supported {
		return ""
	}

	defaultLevel := strings.TrimSpace(caps.Default)
	if defaultLevel == "" {
		defaultLevel = strings.ToLower(strings.TrimSpace(requested))
	}

	normalized := strings.ToLower(strings.TrimSpace(requested))
	if normalized == "" {
		return defaultLevel
	}
	if SupportsThinkingLevel(provider, modelName, normalized) {
		return normalized
	}

	// One table, not a switch per provider. The switch that used to live here
	// was the second of the four places that answered "which levels does this
	// provider have", and it had already drifted from the first: it folded
	// DeepSeek's `low` onto `high` — a real level, thrown away — because the
	// picker did not offer `low` at the time.
	if alias, ok := caps.Aliases[normalized]; ok && SupportsThinkingLevel(provider, modelName, alias) {
		return alias
	}

	return defaultLevel
}

// DeepSeek's thinking dial: every value the API accepts, offered.
//
// The list is the API's own, read from the service rather than from the docs —
// sending a deliberately invalid effort returns a 400 naming the whole enum:
//
//	output_config.effort: unknown variant `banana`,
//	expected one of `low`, `medium`, `high`, `xhigh`, `ultra`, `max`
//
// Accepted is not the same as distinct, and only the distinct ones are offered.
//
// Six values pass validation, but three of them are not depths. `medium` and
// `xhigh` are documented as compatibility spellings the service folds onto
// `high`, and measurement agrees (medium 2,679 thinking chars against high's
// 3,002 — 5–6 runs per level, two problems, three request shapes, 2026-08-06).
// `ultra` appears in the enum and in no documentation at all. Listing all six
// produced a seven-row menu in which three rows changed nothing when clicked,
// which is worse than a short menu: a control that does nothing teaches the
// user that the controls do nothing.
//
// So they live in Aliases instead. A config carrying one still lands somewhere
// sensible; the picker only shows depths that differ.
//
// One measured fact the names do not tell you: `low` spends 2–3× MORE tokens
// than `high`, consistently, on every shape tested. The ladder is not ordered
// the way it reads. The names are still the API's, spelled its way — our own
// words for them would just be a second vocabulary to keep in step with theirs.
func resolveDeepSeekThinkingCapabilities(modelID string) ThinkingCapabilities {
	if modelID == "" || strings.HasPrefix(modelID, "deepseek-") || modelID == "deepseek-chat" || modelID == "deepseek-reasoner" {
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"off", "low", "high", "max"},
			Default:   "high",
			Runtime:   ThinkingRuntimeDeepSeek,
			Source:    "deepseek-effort-enum+measured-2026-08-06",
			Wire:      identityWire("low", "high", "max"),
			Aliases: map[string]string{
				"none":     "off",
				"disabled": "off",
				"minimal":  "low",
				"default":  "high",
				"medium":   "high", // the service folds these two itself
				"xhigh":    "high",
				"ultra":    "max", // undocumented; nearest depth we can explain
			},
		}
	}
	return fallbackThinkingCapabilities
}

func resolveOpenAIThinkingCapabilities(modelID string) ThinkingCapabilities {
	if modelID == "" {
		return fallbackThinkingCapabilities
	}
	switch {
	case strings.HasPrefix(modelID, "gpt-5-pro"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"high"},
			Default:   "high",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "openai-chat-docs",
			Wire:      identityWire("high"),
		}
	case strings.HasPrefix(modelID, "gpt-5.1"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "low", "medium", "high"},
			Default:   "none",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "openai-chat-docs",
			Wire:      identityWire("none", "low", "medium", "high"),
			Aliases:   map[string]string{"off": "none", "disabled": "none", "minimal": "low", "xhigh": "high", "max": "high"},
		}
	case strings.HasPrefix(modelID, "gpt-5.2"), strings.HasPrefix(modelID, "gpt-5"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "minimal", "low", "medium", "high", "xhigh"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "openai-chat-docs",
			Wire:      identityWire("none", "minimal", "low", "medium", "high", "xhigh"),
			Aliases:   map[string]string{"off": "none", "disabled": "none", "max": "xhigh"},
		}
	case strings.HasPrefix(modelID, "o1"), strings.HasPrefix(modelID, "o3"), strings.HasPrefix(modelID, "o4"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "openai-chat-docs",
			Wire:      identityWire("minimal", "low", "medium", "high"),
			Aliases:   map[string]string{"none": "minimal", "off": "minimal", "xhigh": "high", "max": "high"},
		}
	default:
		return cloneThinkingCapabilities(conservativeFallback)
	}
}

// Kimi K3's dial, and the first one in this table with no off position.
//
// K3 always thinks — "K3 always has thinking mode enabled" — and the K2-era
// `thinking` parameter is gone, replaced by a top-level reasoning_effort taking
// low/high/max, default max. So there is no "off" in Levels: offering one would
// be a switch that does nothing, which is the exact failure this table was
// rebuilt to stop. `off` comes in as an alias onto `low` instead, because a
// user arriving from a provider that can be silenced should land on the least
// thinking this one has rather than snap back to the default, which is max.
//
// Default is the API's own (max) rather than something cheaper of our choosing:
// picking a different default would make Aetox's idea of "unset" disagree with
// the service's, and the user cannot see which of the two they are getting.
//
// Unverified against the live API: no Kimi key on this machine, so unlike the
// DeepSeek row above, nothing here has been measured — it is the documentation
// taken at its word, and DeepSeek is a standing reminder that documentation and
// behaviour can differ.
func resolveKimiThinkingCapabilities(modelID string) ThinkingCapabilities {
	if modelID == "" || strings.HasPrefix(modelID, "kimi-k3") {
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"low", "high", "max"},
			Default:   "max",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "kimi-k3-docs",
			Wire:      identityWire("low", "high", "max"),
			Aliases: map[string]string{
				"off": "low", "none": "low", "disabled": "low", "minimal": "low",
				"medium": "high", "default": "max", "xhigh": "max", "ultra": "max",
			},
		}
	}
	// K2 and anything older drove thinking with a `thinking` parameter that K3
	// dropped. Rather than guess which spelling an unknown Kimi model wants,
	// this sends no effort at all (Native false) and lets the model use its own.
	return cloneThinkingCapabilities(conservativeFallback)
}

// MiniMax's dial is a switch, not a ladder: the API has a `thinking` block
// taking "adaptive" or "disabled" and no effort field anywhere. So there are
// two positions, and the table says two rather than inventing depths to fill a
// menu with.
//
// M3 is the only model where both positions are real. On M2.x "thinking cannot
// be disabled", so it gets the one position it actually has — and a one-entry
// picker is not a choice, so the UI does not draw it (Chat.svelte and
// Palette.svelte both require two).
//
// Unverified against the live API: no MiniMax key on this machine. This is the
// documentation taken at its word.
func resolveMiniMaxThinkingCapabilities(modelID string) ThinkingCapabilities {
	switch {
	case modelID == "" || strings.HasPrefix(modelID, "minimax-m3"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"off", "on"},
			Default:   "on",
			Runtime:   ThinkingRuntimeMiniMax,
			Source:    "minimax-docs",
			Wire:      map[string]string{"on": "adaptive"},
			Aliases: map[string]string{
				"none": "off", "disabled": "off",
				"minimal": "on", "low": "on", "medium": "on", "default": "on",
				"high": "on", "xhigh": "on", "ultra": "on", "max": "on",
			},
		}
	case strings.HasPrefix(modelID, "minimax-m2"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"on"},
			Default:   "on",
			Runtime:   ThinkingRuntimeMiniMax,
			Source:    "minimax-docs-m2-always-on",
			Wire:      map[string]string{"on": "adaptive"},
			Aliases: map[string]string{
				"off": "on", "none": "on", "disabled": "on",
				"minimal": "on", "low": "on", "medium": "on", "default": "on",
				"high": "on", "xhigh": "on", "ultra": "on", "max": "on",
			},
		}
	default:
		return cloneThinkingCapabilities(conservativeFallback)
	}
}

// ThinkingBlockType reports what the request's `thinking` block should say, for
// the providers that carry the switch there instead of in an effort field.
//
// One function keyed by runtime, rather than a provider-name check at the call
// site: cognitive used to test for "deepseek" by name when building a request,
// which is the same shape as every other drift found here — the next provider
// with a thinking block would have needed a second branch somewhere else.
func ThinkingBlockType(provider, modelName, level string) (string, bool) {
	caps := ResolveThinkingCapabilities(provider, modelName)
	if !caps.Supported {
		return "", false
	}
	off := NormalizeThinkingLevel(provider, modelName, level) == "off"
	switch caps.Runtime {
	case ThinkingRuntimeDeepSeek:
		if off {
			return "disabled", true
		}
		return "enabled", true
	case ThinkingRuntimeMiniMax:
		if off {
			return "disabled", true
		}
		return "adaptive", true
	default:
		return "", false
	}
}

func resolveGeminiThinkingCapabilities(modelID string) ThinkingCapabilities {
	switch {
	case modelID == "":
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "gemini-openai-compat-docs",
			Wire:      identityWire("none", "minimal", "low", "medium", "high"),
			Aliases:   map[string]string{"off": "none", "disabled": "none", "xhigh": "high", "max": "high"},
		}
	case strings.HasPrefix(modelID, "gemini-2.0-flash-lite"):
		return ThinkingCapabilities{
			Supported: false,
			Native:    false,
			Levels:    nil,
			Default:   "",
			Runtime:   ThinkingRuntimeUnknown,
			Source:    "gemini-model-docs",
		}
	case strings.HasPrefix(modelID, "gemini-2.5-pro"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "gemini-openai-compat-docs",
			Wire:      identityWire("minimal", "low", "medium", "high"),
			Aliases:   map[string]string{"none": "minimal", "off": "minimal", "xhigh": "high", "max": "high"},
		}
	case strings.HasPrefix(modelID, "gemini-2.5"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "gemini-openai-compat-docs",
			Wire:      identityWire("none", "minimal", "low", "medium", "high"),
			Aliases:   map[string]string{"off": "none", "disabled": "none", "xhigh": "high", "max": "high"},
		}
	case strings.HasPrefix(modelID, "gemini-3"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"minimal", "low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningEffort,
			Source:    "gemini-openai-compat-docs",
			Wire:      identityWire("minimal", "low", "medium", "high"),
			Aliases:   map[string]string{"none": "minimal", "off": "minimal", "xhigh": "high", "max": "high"},
		}
	default:
		return cloneThinkingCapabilities(conservativeFallback)
	}
}

func resolveOpenRouterThinkingCapabilities(modelID string) ThinkingCapabilities {
	if isKnownOpenRouterReasoningModel(modelID) {
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"none", "minimal", "low", "medium", "high", "xhigh"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeReasoningObject,
			Source:    "openrouter-reasoning-docs",
			Wire:      identityWire("none", "minimal", "low", "medium", "high", "xhigh"),
			Aliases:   map[string]string{"off": "none", "disabled": "none", "max": "xhigh"},
		}
	}
	return cloneThinkingCapabilities(conservativeFallback)
}

func resolveGroqThinkingCapabilities(modelID string) ThinkingCapabilities {
	switch {
	case strings.HasPrefix(modelID, "openai/gpt-oss-"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"low", "medium", "high"},
			Default:   "medium",
			Runtime:   ThinkingRuntimeGroq,
			Source:    "groq-reasoning-docs",
			Wire:      identityWire("low", "medium", "high"),
			Aliases:   map[string]string{"none": "low", "off": "low", "minimal": "low", "xhigh": "high", "max": "high"},
		}
	case strings.HasPrefix(modelID, "qwen/qwen3-"):
		return ThinkingCapabilities{
			Supported: true,
			Native:    true,
			Levels:    []string{"default", "none"},
			Default:   "default",
			Runtime:   ThinkingRuntimeGroq,
			Source:    "groq-reasoning-docs",
			Wire:      identityWire("default", "none"),
			Aliases:   map[string]string{"off": "none", "disabled": "none"},
		}
	default:
		return cloneThinkingCapabilities(conservativeFallback)
	}
}

func isKnownOpenRouterReasoningModel(modelID string) bool {
	switch {
	case strings.HasPrefix(modelID, "openai/"):
		return true
	case strings.HasPrefix(modelID, "deepseek/"):
		return true
	case strings.HasPrefix(modelID, "google/gemini-"):
		return true
	case strings.HasPrefix(modelID, "qwen/"):
		return true
	case strings.HasPrefix(modelID, "google/gemini-2.5"):
		return true
	case strings.HasPrefix(modelID, "anthropic/claude-3.7"):
		return true
	case strings.HasPrefix(modelID, "anthropic/claude-sonnet-4"):
		return true
	default:
		return false
	}
}

func cloneThinkingCapabilities(caps ThinkingCapabilities) ThinkingCapabilities {
	cloned := caps
	cloned.Levels = append([]string{}, caps.Levels...)
	cloned.Wire = cloneStringMap(caps.Wire)
	cloned.Aliases = cloneStringMap(caps.Aliases)
	return cloned
}

// The maps are package-level literals shared by every caller, so they are
// copied out with the slice — a caller that wrote to one would be editing the
// table for the whole process.
func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// identityWire is the common case: the level's name is what goes on the wire.
// "off" is never in here — it is carried by the thinking block, not by effort.
func identityWire(levels ...string) map[string]string {
	out := make(map[string]string, len(levels))
	for _, level := range levels {
		out[level] = level
	}
	return out
}

// ---------------------------------------------------------------------------
// Providers whose thinking knob Aetox drives directly
// ---------------------------------------------------------------------------

// anthropicThinkingCapabilities is a real dial, not a switch.
//
// The knob is output_config.effort, which takes low → max; thinking itself is
// only adaptive-or-off. The older thinking.budget_tokens form — the obvious
// place to map a "level" onto — is rejected with a 400 on every current Claude,
// so a runtime that reached for it could not talk to any of them.
//
// "high" rather than "xhigh" as the default: xhigh is the better setting for
// coding work and is what Claude Code uses, but it is also markedly more
// expensive, and a default that quietly spends more of someone's plan is not
// ours to choose. It is one click away.
var anthropicThinkingCapabilities = ThinkingCapabilities{
	Supported: true,
	Native:    true,
	Levels:    []string{"off", "low", "medium", "high", "xhigh", "max"},
	Default:   "high",
	Runtime:   ThinkingRuntimeReasoningEffort,
	Source:    "anthropic-effort",
	Wire:      identityWire("low", "medium", "high", "xhigh", "max"),
	Aliases: map[string]string{
		"none":     "off",
		"disabled": "off",
		"minimal":  "low",
		"default":  "medium",
		"ultra":    "max",
	},
}

// resolveAnthropicThinkingCapabilities keeps the switch off models that never
// had extended thinking. Matching on the family rather than the full id: the
// model list is live now, and a name written here would be the same staleness
// the catalog lists just stopped being.
func resolveAnthropicThinkingCapabilities(modelID string) ThinkingCapabilities {
	switch {
	case strings.Contains(modelID, "claude-3-5"), strings.Contains(modelID, "claude-3.5"),
		strings.Contains(modelID, "claude-3-opus"), strings.Contains(modelID, "claude-3-haiku"),
		strings.Contains(modelID, "claude-2"), strings.Contains(modelID, "claude-instant"):
		return noThinkingCapabilities
	default:
		// Claude 3.7 and everything after it thinks. Defaulting the unknown
		// name to "can think" is the right way round: the runtime sends
		// adaptive, and a provider that ignores it costs nothing, while hiding
		// the control on a model that supports it is a feature the user cannot
		// reach.
		return anthropicThinkingCapabilities
	}
}

// responsesThinkingCapabilities is the ChatGPT/Codex dial. The Responses API
// takes a real effort setting, which internal/model/responses.go passes through
// alongside summary:auto so the thinking is visible while it happens.
var responsesThinkingCapabilities = ThinkingCapabilities{
	Supported: true,
	Native:    true,
	Levels:    []string{"off", "low", "medium", "high", "xhigh"},
	Default:   "medium",
	Runtime:   ThinkingRuntimeReasoningEffort,
	Source:    "responses-reasoning-effort",
	Wire:      identityWire("low", "medium", "high", "xhigh"),
	Aliases: map[string]string{
		"none":     "off",
		"disabled": "off",
		"minimal":  "low",
		"default":  "medium",
		"max":      "xhigh",
		"ultra":    "xhigh",
	},
}
