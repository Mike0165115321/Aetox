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
	// the providers that send nothing — aetox, mistral, qwen and zai — each
	// drew a full off/low/medium/high picker in
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
	case "openai":
		return cloneThinkingCapabilities(resolveOpenAIThinkingCapabilities(modelID))
	case "gemini":
		return cloneThinkingCapabilities(resolveGeminiThinkingCapabilities(modelID))
	case "openrouter", "anthropic", "deepseek", "kimi", "minimax", "xai",
		"groq", "opencode", "opencode-go":
		return resolveFromCatalog(canonicalProvider, modelID, thinkingProfiles[canonicalProvider])
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

// thinkingLadder is Aetox's own vocabulary for depth, shallowest first.
//
// A fact about Aetox rather than about any vendor: it is what the picker offers
// and what a saved setting is spelled in. Every effort value models.dev states
// falls inside it (checked across all 7,248 models it describes: none, minimal,
// low, medium, high, xhigh, max and nothing else), which is what lets levels be
// taken from that catalog without a translation table per provider.
//
// "off" is deliberately absent. It is not a rung, it is the switch, and it
// sorts below every rung wherever a provider has one.
var thinkingLadder = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

func knownThinkingLevel(v string) bool {
	for _, rung := range thinkingLadder {
		if v == rung {
			return true
		}
	}
	return false
}

// orderByLadder puts the catalog's values in Aetox's own order and drops
// duplicates. The catalog lists them shallowest-first already; this makes that
// a guarantee rather than an observation, because the picker draws them in
// order and a shuffled row reads as a different control.
func orderByLadder(values []string) []string {
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		seen[v] = true
	}
	out := make([]string, 0, len(values))
	for _, rung := range thinkingLadder {
		if seen[rung] {
			out = append(out, rung)
		}
	}
	return out
}

// deriveThinkingAliases folds every level Aetox can be asked for onto the
// nearest one this model actually offers.
//
// Derived rather than written down because the levels it is derived from now
// come from the catalog and differ per model — a hand-written alias map can
// only be written for a hand-written level list. "Nearest" is measured along
// thinkingLadder, keeping the shallower of two equal neighbours so an
// unrecognised request never silently costs more than the user asked for.
func deriveThinkingAliases(levels []string) map[string]string {
	offered := make(map[string]bool, len(levels))
	for _, l := range levels {
		offered[l] = true
	}
	nearest := func(rung string) string {
		want := -1
		for i, r := range thinkingLadder {
			if r == rung {
				want = i
				break
			}
		}
		if want < 0 {
			return ""
		}
		best, bestDist := "", len(thinkingLadder)+1
		for i, r := range thinkingLadder {
			if !offered[r] {
				continue
			}
			d := i - want
			if d < 0 {
				d = -d
			}
			if d < bestDist {
				best, bestDist = r, d
			}
		}
		return best
	}

	aliases := map[string]string{}
	for _, rung := range thinkingLadder {
		if !offered[rung] {
			if to := nearest(rung); to != "" {
				aliases[rung] = to
			}
		}
	}
	// Aetox's own synonyms, which are not any vendor's words. "off" means the
	// switch where the provider has one and the shallowest rung where it does
	// not, because a model that cannot stop thinking must not be handed a
	// control that pretends it can.
	shallowest := "off"
	if !offered["off"] {
		if shallowest = nearest("none"); shallowest == "" {
			return aliases
		}
	}
	for _, word := range []string{"off", "none", "disabled"} {
		if !offered[word] {
			aliases[word] = shallowest
		}
	}
	if deepest := nearest("max"); deepest != "" && !offered["ultra"] {
		aliases["ultra"] = deepest
	}
	return aliases
}

// thinkingProfiles is the migrated set: eleven rows across two providers today,
// growing as each resolver comes off the debt register in
// capabilities_source_test.go.
//
// Every entry is a fact about an API. Nothing here names a model, and the guard
// test refuses to let one appear.
var thinkingProfiles = map[string]thinkingProfile{
	"openrouter": {
		// The nested `reasoning: {effort}` object rather than the flat field.
		// Their own dialect, and the reason that runtime constant exists.
		runtime:      ThinkingRuntimeReasoningObject,
		defaultOrder: []string{"medium", "high", "low"},
		unstated:     []string{"none", "minimal", "low", "medium", "high", "xhigh"},
		source:       "openrouter-reasoning-docs",
	},
	"anthropic": {
		runtime: ThinkingRuntimeReasoningEffort,
		// alwaysHasOff, and it is not a preference. On this API "do not think"
		// is the ABSENCE of the thinking parameter, so the off position exists
		// on every model whatever the catalog says about a toggle — and the
		// catalog says several of them have none.
		alwaysHasOff: true,
		defaultOrder: []string{"high", "medium", "max", "low"},
		unstated:     []string{"low", "medium", "high", "xhigh", "max"},
		source:       "anthropic-effort-docs",
	},
	"deepseek": {
		// The switch rides in a `thinking` block and the depth in an effort
		// field beside it, which is why this runtime exists. alwaysHasOff for
		// the same reason as anthropic: the block can always say disabled.
		runtime:      ThinkingRuntimeDeepSeek,
		alwaysHasOff: true,
		aliasOverrides: map[string]string{
			// The service folds these onto high itself; the ladder would fold
			// medium down to low instead and spend less than was asked for.
			"medium": "high",
			"xhigh":  "high",
		},
		defaultOrder: []string{"high", "medium", "max", "low"},
		unstated:     []string{"low", "high", "max"},
		source:       "deepseek-effort-enum+measured-2026-08-06",
	},
	"kimi": {
		runtime: ThinkingRuntimeReasoningEffort,
		// K3 states a toggle and this endpoint has only an effort field to
		// carry it, so the off position would be a control that changes
		// nothing. Its own table said the same before the migration.
		ignoresToggle: true,
		defaultOrder:  []string{"max", "high", "medium", "low"},
		unstated:      []string{"low", "high", "max"},
		source:        "kimi-k3-docs",
	},
	"minimax": {
		// A switch and nothing else, and `on` goes on the wire as "adaptive" —
		// the one place left where Aetox and a vendor spell the same idea
		// differently.
		runtime:      ThinkingRuntimeMiniMax,
		toggleOnly:   true,
		wire:         map[string]string{"on": "adaptive"},
		defaultOrder: []string{"on"},
		unstated:     []string{"off", "on"},
		source:       "minimax-docs",
	},
	"xai": {
		// No off position: the grok-4.x line reasons unconditionally and xAI
		// documents no value that stops it. The one id that opts out says so in
		// its own name, and the catalog states it — which is why that check
		// stopped needing to live in Go.
		runtime:      ThinkingRuntimeReasoningEffort,
		defaultOrder: []string{"high", "medium", "xhigh", "low"},
		unstated:     []string{"low", "medium", "high", "xhigh"},
		source:       "xai-grok-4-6-docs",
	},
	"groq": {
		runtime:      ThinkingRuntimeGroq,
		defaultOrder: []string{"medium", "high", "low"},
		unstated:     []string{"low", "medium", "high"},
		source:       "groq-reasoning-docs",
	},
	"gemini": {
		runtime:      ThinkingRuntimeReasoningEffort,
		defaultOrder: []string{"medium", "high", "low"},
		unstated:     []string{"none", "minimal", "low", "medium", "high"},
		source:       "gemini-thinking-docs",
	},
	"openai": {
		runtime:      ThinkingRuntimeReasoningEffort,
		defaultOrder: []string{"medium", "high", "low"},
		unstated:     []string{"none", "minimal", "low", "medium", "high", "xhigh"},
		source:       "openai-chat-docs",
	},
	"opencode": {
		// parseOpenAiVariant reads reasoning_effort at the top level; the
		// gateway forwards it untouched, so which depths work is the upstream
		// model's business and therefore the catalog's.
		runtime:      ThinkingRuntimeReasoningEffort,
		defaultOrder: []string{"medium", "high", "low"},
		unstated:     []string{"low", "medium", "high"},
		source:       "opencode-zen-variant-parser",
	},
	"opencode-go": {
		runtime:      ThinkingRuntimeReasoningEffort,
		defaultOrder: []string{"medium", "high", "low"},
		unstated:     []string{"low", "medium", "high"},
		source:       "opencode-zen-variant-parser",
	},
}

// resolveOpenAIThinkingCapabilities is the one resolver that stayed, and the
// reason is a fact the catalog does not carry: OpenAI DEFAULTS differ per
// model, and models.dev states the rungs but never which one the service picks
// when nothing is sent.
//
// gpt-5.1 defaults to `none` and gpt-5.2 to `medium`, while both offer `none`
// among their levels — so no per-provider preference can produce both. A
// default that disagrees with the service is invisible to the user: they get a
// depth they did not choose and nothing on screen says which.
//
// Everything else about this row IS the catalog's: it was the ten-resolver
// sweep that removed the other nine, and this one is on the debt register in
// capabilities_source_test.go so the exception is counted rather than
// forgotten. It comes off when models.dev states a default, or when someone
// measures what each model does with the field omitted.
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

// resolveGeminiThinkingCapabilities is the second resolver that stayed, and it
// stayed for the same reason as OpenAI's: a per-model fact the catalog does
// not carry.
//
// Gemini states its dial as a TOKEN BUDGET (`{type: budget_tokens, min, max}`)
// rather than as effort rungs, and nothing in Aetox can put a token budget on
// the wire — so models.dev describes these models as reasoning and says
// nothing this package can offer. Falling back to a per-provider ladder gets
// the shape right and the edges wrong: gemini-2.5-pro cannot be told not to
// think, and a generic list hands it a `none` that goes nowhere.
//
// It comes off the register when either the catalog states effort values for
// this vendor, or a budget-token runtime exists here to spend them.
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

// thinkingProfile is what stays ours after a resolver is migrated: the facts
// about a provider's API that no model catalog can state, because they are not
// facts about a model.
//
// A vendor shipping a new model changes none of these. That is the test for
// whether something belongs here rather than in the catalog, and it is why a
// migrated provider is ten lines of profile instead of a resolver.
type thinkingProfile struct {
	// runtime is the wire dialect: which field carries the setting. It drives
	// ThinkingBlockType, so it is behaviour rather than a label.
	runtime ThinkingRuntime
	// wire translates an Aetox level into this provider's spelling. Nil means
	// the two agree and the identity map is built from the levels in use.
	wire map[string]string
	// toggleOnly is a dial with no rungs at all, only a switch.
	toggleOnly bool
	// ignoresToggle is for an API with nowhere to put one. The catalog states a
	// toggle as a fact about the MODEL — the weights can stop reasoning — and
	// that is not the same as the endpoint having a field to ask with. On a
	// plain reasoning_effort surface there is none: omitting the field leaves
	// the model on its own default, which is to think. Offering an off
	// position there is offering a switch that does nothing, which is the one
	// thing this file has said all along it will not do.
	//
	// Kimi is the case, and its old table had it right: K3 states a toggle and
	// its endpoint takes only an effort, so off folds onto the floor instead.
	ignoresToggle bool
	// alwaysHasOff is for an API where "do not think" is the ABSENCE of the
	// setting rather than a value in it — so the off position exists whatever
	// the catalog says about a toggle. Anthropic is the case: omitting the
	// thinking parameter is always allowed.
	alwaysHasOff bool
	// defaultOrder is the first-available preference for the depth used when
	// the user has expressed none. Not one value: the rungs differ per model,
	// and naming one this model lacks leaves the picker unable to select its
	// own default.
	defaultOrder []string
	// aliasOverrides pin a fold the ladder cannot derive, because the service
	// itself treats two depths as one. DeepSeek is the case: it accepts
	// `medium` and answers identically to `high`, so folding medium onto the
	// nearest RUNG (low) would quietly make every unrecognised request cheaper
	// than the caller asked for. Measured 2026-08-06, and it is a fact about
	// the service rather than about any model, which is why it can live here.
	aliasOverrides map[string]string
	// unstated is what to offer when the catalog says the model reasons and
	// does not say with what depths — common enough to matter (MiniMax states
	// options for one model in seven). One list per provider, not per model, so
	// it cannot go stale when a vendor ships.
	unstated []string
	source   string
}

// resolveFromCatalog is the migrated path, shared by every provider that has
// been moved off a prefix table.
//
// Written once on purpose. Nine resolvers each answering "does this model
// reason" their own way is how they came to disagree, and copying this logic
// per provider would rebuild that by hand.
func resolveFromCatalog(canonicalProvider, modelID string, p thinkingProfile) ThinkingCapabilities {
	if modelID == "" {
		return cloneThinkingCapabilities(conservativeFallback)
	}
	installedCatalogMu.RLock()
	c := installedCatalog
	installedCatalogMu.RUnlock()

	// No catalog is not a licence to guess. Until one is fetched, whether a
	// given model reasons is unknown, and unknown must look like unknown.
	if c == nil || len(c.Models) == 0 {
		return cloneThinkingCapabilities(conservativeFallback)
	}
	facts, known := c.For(canonicalProvider, modelID)
	if !known {
		return cloneThinkingCapabilities(conservativeFallback)
	}
	if !facts.Reasoning {
		return cloneThinkingCapabilities(noThinkingCapabilities)
	}

	var levels []string
	source := "models.dev"
	if p.toggleOnly {
		levels = []string{"off", "on"}
	} else {
		if (facts.ReasoningToggle && !p.ignoresToggle) || p.alwaysHasOff {
			levels = append(levels, "off")
		}
		levels = append(levels, orderByLadder(facts.ReasoningLevels)...)
		if len(levels) == 0 || (len(levels) == 1 && levels[0] == "off") {
			levels = append(levels[:0:0], p.unstated...)
			if p.alwaysHasOff && !containsLevel(levels, "off") {
				levels = append([]string{"off"}, levels...)
			}
			source = p.source + "+catalog-silent-on-levels"
		}
	}
	if len(levels) == 0 {
		return cloneThinkingCapabilities(conservativeFallback)
	}

	wire := make(map[string]string, len(levels))
	for _, l := range levels {
		// "off" never goes in the effort field. Everywhere it exists it is the
		// absence of the setting or a thinking block that says disabled, and
		// sending it as a value is how a switch becomes a 400.
		if l != "off" {
			wire[l] = l
		}
	}
	for from, to := range p.wire {
		if _, offered := wire[from]; offered {
			wire[from] = to
		}
	}
	return ThinkingCapabilities{
		Supported: true,
		Native:    true,
		Levels:    levels,
		Default:   preferredLevel(levels, p.defaultOrder),
		Runtime:   p.runtime,
		Source:    source,
		Wire:      wire,
		Aliases:   applyAliasOverrides(deriveThinkingAliases(levels), p.aliasOverrides, levels),
	}
}

// applyAliasOverrides lets a provider correct a fold the ladder got wrong,
// without letting it invent one: an override onto a depth this model does not
// offer is dropped rather than pointing the picker at nothing.
func applyAliasOverrides(aliases, overrides map[string]string, levels []string) map[string]string {
	for from, to := range overrides {
		if containsLevel(levels, from) || !containsLevel(levels, to) {
			continue
		}
		aliases[from] = to
	}
	return aliases
}

func containsLevel(levels []string, want string) bool {
	for _, l := range levels {
		if l == want {
			return true
		}
	}
	return false
}

// preferredLevel is the depth asked for when the user has expressed none.
func preferredLevel(levels, order []string) string {
	for _, want := range order {
		if containsLevel(levels, want) {
			return want
		}
	}
	for _, l := range levels {
		if l != "off" {
			return l
		}
	}
	return levels[0]
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
