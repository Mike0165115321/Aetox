package model

import "strings"

// What one model can do, answered in one place.
//
// Before this file the question was answered four times, in four files, by four
// functions that had each invented their own fallback philosophy — and every
// one of them turned out to be wrong in a different direction:
//
//   - thinking, from a prefix list: wrong for 171 of OpenRouter's 360 models
//   - vision, from substrings of the model id: 13 of opencode-go's 28 called
//     blind when they can see
//   - tool calling, from `return true` on the provider: 117 models handed tool
//     definitions they cannot use
//   - documents, from a provider allowlist of one
//
// The pattern under all four is the same, and it is the reason this file
// exists: a question about a MODEL was being answered at a level that cannot
// see the model.
//
// # The line
//
// Two kinds of fact, and confusing them is what produced every bug above:
//
//	about the MODEL                  → the fetched catalog, never written here
//	  reasons? at what depths?          it changes whenever a vendor ships, and
//	  sees images? reads pdf?           no list in this repo can keep up
//	  calls tools? context? price?
//
//	about the PROVIDER'S API         → written here and in internal/provider
//	  which field carries the effort    it does not change when a vendor ships a
//	  which envelope wraps a document   model, so a hand-written answer stays
//	  base URL, auth, error shape       true
//
// Adding a provider should therefore be one row in internal/provider/catalog.go
// and nothing else — no capability table, no new ResolveX. A guard test
// (capabilities_source_test.go) keeps that true by refusing to let a model id
// appear in any of these files.
//
// # Shape
//
// Modalities are LISTS, not a boolean each. That is opencode's shape and the
// reason to copy it: the catalog already describes audio (591 models), video
// (1,007) and pdf (1,608), so a boolean per modality is a promise to write a
// fifth, sixth and seventh resolver, each with its own fallback philosophy and
// its own way of being wrong.
type ModelCapabilities struct {
	Provider string
	Model    string

	// Tools is whether this model can be sent tool definitions at all.
	Tools bool

	// Input and Output are modality names: text, image, pdf, audio, video.
	// Ask with Accepts and Produces rather than indexing.
	Input  []string
	Output []string

	// Thinking is the depth dial, which is richer than a flag (levels, wire
	// spelling, aliases) and so keeps its own type.
	Thinking ThinkingCapabilities

	// Source names where the modality answer came from, for the same reason
	// ThinkingCapabilities carries one: "the model has no dial" and "nothing
	// here knows" look identical on a card and are not the same state.
	Source string
}

// Accepts reports whether this model can be sent a part of that modality.
func (c ModelCapabilities) Accepts(modality string) bool {
	return containsModality(c.Input, modality)
}

// Produces reports whether this model can answer with that modality.
func (c ModelCapabilities) Produces(modality string) bool {
	return containsModality(c.Output, modality)
}

func containsModality(list []string, modality string) bool {
	want := strings.ToLower(strings.TrimSpace(modality))
	for _, m := range list {
		if m == want {
			return true
		}
	}
	return false
}

// knownModalities filters the catalog's list to the vocabulary Aetox can act
// on, so a name nothing here can route never reaches a decision. The five are
// every value models.dev states across all 7,248 models it describes.
func knownModalities(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		switch v {
		case "text", "image", "pdf", "audio", "video":
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Accepts and Produces on the catalog record itself, so callers that already
// hold one do not have to build a whole ModelCapabilities to ask.
func (f ModelFacts) Accepts(modality string) bool  { return containsModality(f.Input, modality) }
func (f ModelFacts) Produces(modality string) bool { return containsModality(f.Output, modality) }

// textOnly is the answer for a model nothing describes and no name recognises.
// A separate variable rather than a literal at each return so that "we know
// nothing about this model" reads the same everywhere it is decided.
var textOnly = []string{"text"}

// ResolveCapabilities answers everything about one provider/model pair.
//
// The precedence is stated once here, having previously been rediscovered
// separately in each resolver:
//
//  1. Role markers win outright. An embedder that takes images is still not a
//     chat model, and no catalog row can make it one.
//  2. The catalog, wherever it knows the pair. It is the provider's own
//     published list, refreshed, rather than a guess from the shape of a name.
//  3. The provider's fallback, for what nothing published describes — which is
//     every model a user pulled onto their own GPU.
func ResolveCapabilities(provider, modelName string) ModelCapabilities {
	caps := resolveModalities(provider, modelName)
	caps.Thinking = ResolveThinkingCapabilities(provider, modelName)
	return caps
}

// resolveModalities is ResolveCapabilities without the thinking dial, which is
// the expensive half (it clones three maps). Split so that the per-turn callers
// asking only "can this model see" do not pay for a menu nobody is drawing.
func resolveModalities(provider, modelName string) ModelCapabilities {
	name := strings.ToLower(strings.TrimSpace(modelName))
	caps := ModelCapabilities{
		Provider: NormalizeProvider(provider),
		Model:    name,
		Input:    textOnly,
		Output:   textOnly,
		Tools:    modelToolCalling(provider, name, true),
		Source:   "unknown-model",
	}
	if name == "" {
		return caps
	}
	for _, marker := range textOnlyRoleMarkers {
		if strings.Contains(name, marker) {
			caps.Source = "role-marker"
			return caps
		}
	}

	installedCatalogMu.RLock()
	c := installedCatalog
	installedCatalogMu.RUnlock()
	if facts, known := c.For(provider, name); known {
		caps.Source = "models.dev"
		if len(facts.Input) > 0 {
			caps.Input = facts.Input
		}
		if len(facts.Output) > 0 {
			caps.Output = facts.Output
		}
		return caps
	}

	// Nothing published describes this model. The name is all that is left, and
	// for a local runtime it is all there ever will be: models.dev lists what
	// providers serve, not what somebody pulled onto their own machine.
	for _, marker := range visionModelMarkers {
		if strings.Contains(name, marker) {
			caps.Source = "name-marker"
			caps.Input = []string{"text", "image"}
			return caps
		}
	}
	return caps
}
