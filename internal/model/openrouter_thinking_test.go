package model

import "testing"

// openRouterCatalogRows is a slice of the real models.dev table, captured
// 2026-08-23, for the OpenRouter models these tests name.
//
// Captured rather than invented: a row here must not be able to assert
// something about a model that is not true of it. The values are exactly what
// models.dev publishes — deepseek-r1 states no depths at all, gemini-2.5-pro
// states only a token budget, claude-sonnet-4 a toggle plus a budget, and
// claude-opus-5 the effort rungs.
func openRouterCatalogRows() map[string]ModelFacts {
	return map[string]ModelFacts{
		// Reasons; the catalog states no depths. The provider's own ladder is
		// the fallback, and this is the row that proves it is reached.
		"openrouter/deepseek/deepseek-r1": {Context: 163840, ToolCall: true, Output: []string{"text"}, Reasoning: true},
		// Reasons, but only through a token budget, which no runtime here can
		// put on the wire. Same fallback for a different reason.
		"openrouter/google/gemini-2.5-pro": {Context: 1048576, ToolCall: true, Output: []string{"text"}, Reasoning: true},
		"openrouter/anthropic/claude-sonnet-4": {
			Context: 1000000, ToolCall: true, Output: []string{"text"}, Reasoning: true, ReasoningToggle: true,
		},
		// The headline of this change: the prefix list stopped at
		// anthropic/claude-sonnet-4, so every Opus and every 5-series Claude on
		// OpenRouter was offered no thinking picker at all.
		"openrouter/anthropic/claude-opus-5": {
			Context: 1000000, ToolCall: true, Output: []string{"text"}, Reasoning: true, ReasoningToggle: true,
			ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"},
		},
		// Does not reason. The prefix list said everything under openai/ did,
		// which is how gpt-3.5-turbo came to have a thinking menu.
		"openrouter/openai/gpt-4o":          {Context: 128000, ToolCall: true, Output: []string{"text"}},
		"openrouter/openai/gpt-3.5-turbo":   {Context: 16385, ToolCall: true, Output: []string{"text"}},
		"openrouter/deepseek/deepseek-chat": {Context: 163840, ToolCall: true, Output: []string{"text"}},
	}
}

// withOpenRouterCatalog installs those rows for one test and puts back whatever
// was there. Every test that touches OpenRouter thinking needs it now: the
// answer comes from the catalog, so a test with none installed is testing the
// no-catalog path whether it meant to or not.
func withOpenRouterCatalog(t *testing.T) {
	t.Helper()
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{
		Source: "models.dev (captured 2026-08-23)",
		Models: openRouterCatalogRows(),
	})
}

// The bug this change was made for. On 2026-08-23 the prefix list in
// isKnownOpenRouterReasoningModel was measured against the catalog and was
// wrong for 171 of the 360 models OpenRouter serves: 129 that reason and were
// offered no picker, and 42 that do not and were offered levels going nowhere.
//
// Both halves are checked here, because fixing only the first would have left
// the dead menus in place.
func TestOpenRouterThinkingComesFromTheCatalogNotAPrefixList(t *testing.T) {
	withOpenRouterCatalog(t)

	// Flagship Claude on OpenRouter had no picker at all, because the list
	// stopped at claude-sonnet-4 and claude-opus-5 does not start with it.
	opus := ResolveThinkingCapabilities("openrouter", "anthropic/claude-opus-5")
	if !opus.Supported {
		t.Fatalf("claude-opus-5 reports no thinking dial: %+v", opus)
	}
	if opus.Runtime != ThinkingRuntimeReasoningObject {
		t.Errorf("runtime = %q; OpenRouter carries the setting in a nested reasoning object", opus.Runtime)
	}
	// Its own rungs, from the catalog, with the off position its toggle earns.
	want := []string{"off", "low", "medium", "high", "xhigh", "max"}
	if len(opus.Levels) != len(want) {
		t.Fatalf("levels = %v; want %v", opus.Levels, want)
	}
	for i := range want {
		if opus.Levels[i] != want[i] {
			t.Fatalf("levels = %v; want %v", opus.Levels, want)
		}
	}
	// "off" is the switch, never a value in the effort field.
	if _, sent := opus.Wire["off"]; sent {
		t.Error("off has a wire value; on this provider it is the absence of the setting")
	}

	// The other half: models that do not reason must be offered nothing.
	for _, id := range []string{"openai/gpt-4o", "openai/gpt-3.5-turbo", "deepseek/deepseek-chat"} {
		if caps := ResolveThinkingCapabilities("openrouter", id); caps.Supported {
			t.Errorf("%s offers %v; it has no thinking dial to drive", id, caps.Levels)
		}
	}
}

// A model that reasons and states no depths, and one that states only a token
// budget nothing here can send. Both are real and common, and both must fall to
// OpenRouter's own documented ladder rather than to an empty picker on a model
// that demonstrably thinks.
func TestOpenRouterFallsBackToItsOwnLadderWhenDepthsAreUnstated(t *testing.T) {
	withOpenRouterCatalog(t)

	for _, id := range []string{"deepseek/deepseek-r1", "google/gemini-2.5-pro", "anthropic/claude-sonnet-4"} {
		caps := ResolveThinkingCapabilities("openrouter", id)
		if !caps.Supported {
			t.Errorf("%s reasons and got no picker", id)
			continue
		}
		if len(caps.Levels) < 2 {
			t.Errorf("%s levels = %v; a one-entry picker is not a choice", id, caps.Levels)
		}
		if _, ok := caps.Wire[caps.Default]; !ok {
			t.Errorf("%s default %q has no wire value", id, caps.Default)
		}
	}
}

// An id the catalog has never described is not the same as one it says does not
// reason, and neither is a licence to guess. Both answer "no dial", which is
// what keeps a request from carrying a field the endpoint may reject.
func TestOpenRouterUnknownModelGetsNoDial(t *testing.T) {
	withOpenRouterCatalog(t)

	if caps := ResolveThinkingCapabilities("openrouter", "somebody/model-shipped-tomorrow"); caps.Supported {
		t.Errorf("an unknown id reports a dial (%v) — nothing has checked that it has one", caps.Levels)
	}
}

// With no catalog at all, whether a given model reasons is unknown, and unknown
// has to look like unknown. A narrow window that closes itself: OpenRouter needs
// a key and a network to be used, and the catalog arrives on that connection.
func TestOpenRouterWithNoCatalogOffersNothing(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(nil)

	if caps := ResolveThinkingCapabilities("openrouter", "anthropic/claude-opus-5"); caps.Supported {
		t.Errorf("guessed a dial with no catalog to read: %+v", caps)
	}
}

// The aliases used to be written by hand next to a hand-written level list.
// They are derived now, because the levels come from the catalog and differ per
// model, and a hand-written map can only be written for a hand-written list.
//
// The rule is nearest-along-the-ladder, keeping the shallower of two equal
// neighbours so an unrecognised request never silently costs more than asked.
func TestDerivedAliasesFoldOntoTheNearestOfferedLevel(t *testing.T) {
	aliases := deriveThinkingAliases([]string{"off", "low", "medium", "high", "xhigh", "max"})
	for level, want := range map[string]string{
		"none":     "off",
		"disabled": "off",
		"minimal":  "low",
		"ultra":    "max",
	} {
		if got := aliases[level]; got != want {
			t.Errorf("%q folds to %q; want %q", level, got, want)
		}
	}

	// A model with no off position must not be handed one. "off" folds onto
	// the shallowest rung it really has.
	narrow := deriveThinkingAliases([]string{"high", "max"})
	if got := narrow["off"]; got != "high" {
		t.Errorf("off folds to %q on a model that cannot stop thinking; want the shallowest rung", got)
	}
	if got := narrow["low"]; got != "high" {
		t.Errorf("low folds to %q; want high", got)
	}
}
