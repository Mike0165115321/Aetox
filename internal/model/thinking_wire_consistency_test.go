package model

import "testing"

// Every provider whose thinking dial Aetox drives. Kept explicit rather than
// derived: a new provider should have to be added here on purpose, which is the
// moment someone asks whether its levels were filled in.
var thinkingProviders = []struct{ provider, model string }{
	{"deepseek", "deepseek-v4-flash"},
	{"deepseek", "deepseek-v4-pro"},
	{"anthropic", "claude-opus-5"},
	{"anthropic", "claude-sonnet-5"},
	{"openai", "gpt-5.2"},
	{"openai", "gpt-5.1"},
	{"openai", "o3"},
	{"gemini", "gemini-3-pro"},
	{"gemini", "gemini-2.5-flash"},
	{"openrouter", "deepseek/deepseek-r1"},
	{"groq", "openai/gpt-oss-120b"},
	{"groq", "qwen/qwen3-32b"},
	{"kimi", "kimi-k3"},
	{"minimax", "MiniMax-M3"},
	{"minimax", "MiniMax-M2.5"},
	{"codex", "gpt-5.2-codex"},
}

// A level the picker offers must be a level the request can carry.
//
// This is the invariant the old four-table arrangement could not state, let
// alone check. DeepSeek's picker offered levels one mapper had never heard of
// and omitted levels the other mapper knew, and nothing anywhere compared the
// two lists — the drift was only visible by reading three files side by side.
//
// "off" is exempt: it is carried by the thinking block (type: disabled), not by
// an effort value, so it correctly has no wire entry.
func TestEveryOfferedLevelHasAWireValue(t *testing.T) {
	// The openrouter entry in thinkingProviders is answered from the catalog
	// now. Without one installed it resolves to "no dial" and every check
	// below skips it on !Supported — the row would still be listed and
	// silently never tested, which is worse than not listing it.
	withOpenRouterCatalog(t)
	for _, tc := range thinkingProviders {
		caps := ResolveThinkingCapabilities(tc.provider, tc.model)
		if !caps.Supported {
			continue
		}
		for _, level := range caps.Levels {
			if level == "off" {
				continue
			}
			if _, ok := caps.Wire[level]; !ok {
				t.Errorf("%s/%s offers level %q with no Wire value: the picker can show it, the request cannot say it",
					tc.provider, tc.model, level)
			}
		}
	}
}

// The default has to be a level the provider actually offers. A default outside
// its own list is how a picker ends up showing a value that is not in its menu.
func TestEveryDefaultIsAnOfferedLevel(t *testing.T) {
	withOpenRouterCatalog(t)
	for _, tc := range thinkingProviders {
		caps := ResolveThinkingCapabilities(tc.provider, tc.model)
		if !caps.Supported {
			continue
		}
		if !SupportsThinkingLevel(tc.provider, tc.model, caps.Default) {
			t.Errorf("%s/%s defaults to %q, which is not in its own level list %v",
				tc.provider, tc.model, caps.Default, caps.Levels)
		}
	}
}

// An alias must land on a real level, and must never shadow one. An alias
// sharing a name with an offered level would silently rewrite a valid choice.
func TestAliasesResolveToOfferedLevels(t *testing.T) {
	withOpenRouterCatalog(t)
	for _, tc := range thinkingProviders {
		caps := ResolveThinkingCapabilities(tc.provider, tc.model)
		for from, to := range caps.Aliases {
			if !SupportsThinkingLevel(tc.provider, tc.model, to) {
				t.Errorf("%s/%s aliases %q -> %q, which it does not offer", tc.provider, tc.model, from, to)
			}
			if SupportsThinkingLevel(tc.provider, tc.model, from) {
				t.Errorf("%s/%s aliases %q even though %q is an offered level: the alias would override a real choice",
					tc.provider, tc.model, from, from)
			}
		}
	}
}

// Round trip: what the picker shows, normalized the way a host normalizes it,
// still reaches the wire as itself. This is the path a user's click takes.
func TestOfferedLevelsSurviveNormalizationToTheWire(t *testing.T) {
	withOpenRouterCatalog(t)
	for _, tc := range thinkingProviders {
		caps := ResolveThinkingCapabilities(tc.provider, tc.model)
		if !caps.Supported {
			continue
		}
		for _, level := range caps.Levels {
			normalized := NormalizeThinkingLevel(tc.provider, tc.model, level)
			if normalized != level {
				t.Errorf("%s/%s: picking %q normalizes to %q — a level it offers is not a level it keeps",
					tc.provider, tc.model, level, normalized)
			}
			if level == "off" {
				continue
			}
			effort, ok := WireEffort(tc.provider, tc.model, normalized)
			if !ok || effort == "" {
				t.Errorf("%s/%s: level %q survives normalization but sends no effort", tc.provider, tc.model, level)
			}
		}
	}
}

// Every effort the API accepts must be reachable — but only the ones that
// behave differently are offered as choices.
//
// `low` is the regression this pins: it is a real depth of DeepSeek's that the
// old tables threw away twice over, once by never offering it and once by
// folding any incoming `low` onto `high`. The other three are the opposite
// mistake, made later and just as visible: medium, xhigh and ultra all passed
// validation, so they were all listed, and the picker grew three rows that
// changed nothing when clicked.
func TestDeepSeekEffortsAreReachableAndOnlyDistinctOnesAreOffered(t *testing.T) {
	const model = "deepseek-v4-flash"

	// Distinct depths, offered by name and sent unchanged.
	for _, effort := range []string{"low", "high", "max"} {
		if !SupportsThinkingLevel("deepseek", model, effort) {
			t.Errorf("deepseek does not offer %q, which is a distinct depth", effort)
			continue
		}
		got, ok := WireEffort("deepseek", model, effort)
		if !ok || got != effort {
			t.Errorf("deepseek level %q goes on the wire as %q (sent=%v), want it unchanged", effort, got, ok)
		}
	}

	// Accepted by the API but not depths: not in the menu, still reachable, and
	// each one lands on something the service actually distinguishes.
	for _, tc := range []struct{ effort, wire string }{
		{"medium", "high"}, {"xhigh", "high"}, {"ultra", "max"},
	} {
		if SupportsThinkingLevel("deepseek", model, tc.effort) {
			t.Errorf("deepseek offers %q as a choice, but it does not differ from %q", tc.effort, tc.wire)
		}
		got, ok := WireEffort("deepseek", model, tc.effort)
		if !ok || got != tc.wire {
			t.Errorf("deepseek %q resolves to %q (sent=%v), want %q", tc.effort, got, ok, tc.wire)
		}
	}

	if !SupportsThinkingLevel("deepseek", model, "off") {
		t.Error("deepseek lost its off switch")
	}
}
