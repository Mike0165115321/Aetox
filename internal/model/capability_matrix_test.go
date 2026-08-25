package model

import (
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/provider"
)

// The matrix nobody can hold in their head.
//
// Twenty-two provider rows, four wire dialects and four capabilities is 350-odd
// combinations, and the owner is right that following them by hand is not a
// job. Every capability bug this month was one cell of it: a model that could
// see called blind, a model that could not call tools handed some, a picker
// offering a depth the endpoint rejects.
//
// So these are properties rather than expected values. A test that pins
// "opencode-go/qwen3.7-plus sees images" goes stale the day that model does;
// a test that pins "whatever the catalog says about a model is what every door
// into this package answers" stays true forever and catches the next provider
// added wrong.

func withCapabilityMatrix(t *testing.T) {
	t.Helper()
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{
		Source: "models.dev (captured 2026-08-23)",
		Models: capabilityMatrixRows,
	})
}

// forEachMatrixRow walks every captured provider/model pair.
func forEachMatrixRow(t *testing.T, fn func(t *testing.T, provider, model string, facts ModelFacts)) {
	t.Helper()
	seen := 0
	for _, row := range capabilityMatrixPairs {
		// Looked up through the catalog itself rather than by indexing the
		// fixture, so the provider-name mapping (gemini -> google) is part of
		// what is under test instead of something the test works around.
		facts, ok := installedCatalog.For(row.Provider, row.Model)
		if !ok {
			t.Fatalf("%s/%s is in the fixture and the catalog cannot find it — check modelsDevProvider", row.Provider, row.Model)
		}
		seen++
		t.Run(row.Provider+"/"+row.Model, func(t *testing.T) { fn(t, row.Provider, row.Model, facts) })
	}
	if seen < 40 {
		t.Fatalf("walked only %d rows; the fixture is meant to cover the whole matrix", seen)
	}
}

// Whatever the catalog says about a model is what every door into this package
// answers. Four doors, one answer — which is the thing that was not true before
// ModelCapabilities existed, and the reason the same bug appeared four times.
func TestEveryDoorAgreesWithTheCatalog(t *testing.T) {
	withCapabilityMatrix(t)

	forEachMatrixRow(t, func(t *testing.T, p, m string, facts ModelFacts) {
		caps := ResolveCapabilities(p, m)

		if got, want := caps.Accepts("image"), facts.Accepts("image"); got != want {
			t.Errorf("Accepts(image) = %v; the catalog says %v", got, want)
		}
		if got, want := ResolveVision(p, m), caps.Accepts("image"); got != want {
			t.Errorf("ResolveVision = %v but ModelCapabilities.Accepts(image) = %v — two doors, two answers", got, want)
		}
		if got, want := caps.Accepts("pdf"), facts.Accepts("pdf"); got != want {
			t.Errorf("Accepts(pdf) = %v; the catalog says %v", got, want)
		}
		if got, want := caps.Produces("text"), facts.Produces("text"); got != want {
			t.Errorf("Produces(text) = %v; the catalog says %v", got, want)
		}
	})
}

// A model always takes and produces something. An empty list would mean the
// resolver fell through a branch, and every caller treats "no modalities" as
// "cannot do anything" — a silent way to disable a working model.
func TestNoRowResolvesToNothing(t *testing.T) {
	withCapabilityMatrix(t)

	forEachMatrixRow(t, func(t *testing.T, p, m string, _ ModelFacts) {
		caps := ResolveCapabilities(p, m)
		if len(caps.Input) == 0 {
			t.Error("resolved to no input modalities at all")
		}
		if len(caps.Output) == 0 {
			t.Error("resolved to no output modalities at all")
		}
		if !caps.Accepts("text") {
			t.Errorf("does not accept text; input = %v", caps.Input)
		}
		if caps.Provider != NormalizeProvider(p) {
			t.Errorf("Provider = %q, want the canonical %q", caps.Provider, NormalizeProvider(p))
		}
	})
}

// Tool calling may narrow and may never widen. Wrongly withholding tools turns
// a coding agent into a chat window, so the catalog's `false` is obeyed and its
// silence is not.
func TestToolCallingNeverWidensBeyondTheCatalog(t *testing.T) {
	withCapabilityMatrix(t)

	forEachMatrixRow(t, func(t *testing.T, p, m string, facts ModelFacts) {
		if got := ResolveCapabilities(p, m).Tools; got != facts.ToolCall {
			t.Errorf("Tools = %v; the catalog says %v", got, facts.ToolCall)
		}
	})
}

// Documents need BOTH halves: an endpoint whose dialect can carry the part, and
// a model that reads one. Claiming it without the first is a 400; without the
// second it is a document the model never sees.
func TestDocumentsRequireBothAnEndpointAndAModel(t *testing.T) {
	withCapabilityMatrix(t)

	forEachMatrixRow(t, func(t *testing.T, p, m string, facts ModelFacts) {
		got := ResolveDocuments(p, m)
		runtime := provider.RuntimeFor(NormalizeProvider(p))

		if got && !documentRuntimes[runtime] {
			t.Errorf("claims documents on runtime %q, which has no envelope to carry one", runtime)
		}
		if got && !facts.Accepts("pdf") {
			t.Errorf("claims documents; the catalog lists input %v", facts.Input)
		}
		if !got && documentRuntimes[runtime] && facts.Accepts("pdf") {
			t.Errorf("refuses documents on a %q endpoint for a model that reads pdf", runtime)
		}
		if got && !SupportsDocumentType("application/pdf") {
			t.Error("claims documents but the media-type gate takes no pdf")
		}
	})
}

// The picker and the request must not be able to disagree, on any row. This is
// the invariant thinking_wire_consistency_test.go pins for a hand-listed dozen;
// here it runs over the whole matrix, so a provider added tomorrow is covered
// without anyone remembering to add it.
func TestEveryOfferedDepthReachesTheWireOnEveryRow(t *testing.T) {
	withCapabilityMatrix(t)

	forEachMatrixRow(t, func(t *testing.T, p, m string, _ ModelFacts) {
		caps := ResolveThinkingCapabilities(p, m)
		if !caps.Supported {
			return
		}
		if len(caps.Levels) == 0 {
			t.Fatal("reports a dial with no levels in it")
		}
		if !SupportsThinkingLevel(p, m, caps.Default) {
			t.Errorf("defaults to %q, which is not in its own list %v", caps.Default, caps.Levels)
		}
		for _, level := range caps.Levels {
			if normalized := NormalizeThinkingLevel(p, m, level); normalized != level {
				t.Errorf("offers %q but normalizes it to %q", level, normalized)
			}
			if level == "off" {
				// Carried by the thinking block, never by an effort value.
				if _, sent := caps.Wire["off"]; sent {
					t.Error("off has a wire value; it is the switch, not a rung")
				}
				continue
			}
			if effort, ok := WireEffort(p, m, level); !ok || effort == "" {
				t.Errorf("offers %q and sends nothing for it", level)
			}
		}
		for from, to := range caps.Aliases {
			if !SupportsThinkingLevel(p, m, to) {
				t.Errorf("aliases %q to %q, which it does not offer", from, to)
			}
			if SupportsThinkingLevel(p, m, from) {
				t.Errorf("aliases %q even though it is an offered level", from)
			}
		}
	})
}

// A model the catalog says does not reason must be offered no depth at all, on
// every row. This is the half that would be missed by only checking that real
// dials work: 42 of OpenRouter's models had a menu that did nothing.
func TestNoDialIsOfferedWhereTheCatalogSaysThereIsNone(t *testing.T) {
	withCapabilityMatrix(t)

	forEachMatrixRow(t, func(t *testing.T, p, m string, facts ModelFacts) {
		if facts.Reasoning {
			return
		}
		if caps := ResolveThinkingCapabilities(p, m); caps.Supported {
			// Only the rows already migrated to the catalog can be held to
			// this; the nine on the debt register still answer from prefix
			// tables and are allowed to be wrong until they are moved.
			if migratedToCatalog(p) {
				t.Errorf("offers %v on a model the catalog says does not reason", caps.Levels)
			}
		}
	})
}

// migratedToCatalog names the rows whose thinking answer already comes from the
// catalog. It shrinks as the debt register in capabilities_source_test.go does,
// and the two are checked against each other below so they cannot drift.
func migratedToCatalog(p string) bool {
	// Eight of the ten moved. openai and gemini did not, and their reasons are
	// written on the resolvers themselves: OpenAI defaults differ per model and
	// the catalog states no defaults; Gemini states a token budget this package
	// has no runtime to spend.
	switch NormalizeProvider(p) {
	case "openai", "gemini":
		return false
	}
	return true
}

// The two lists that describe the same migration must agree. Without this,
// finishing a provider means remembering to edit two files, and the one nobody
// edits is the one that silently stops testing anything.
func TestTheMigrationListsAgree(t *testing.T) {
	migrated := 0
	for _, p := range provider.SupportedProviders() {
		if migratedToCatalog(p) {
			migrated++
		}
	}
	// Nine resolvers on the register plus the migrated rows account for the ten
	// that existed. `opencode` and `opencode-go` share one resolver, so the row
	// count and the resolver count are not the same number — compare intent
	// rather than arithmetic: every migrated row must NOT be on the register.
	raw := strings.Join(notYetMigrated, " ")
	for _, p := range provider.SupportedProviders() {
		if !migratedToCatalog(p) {
			continue
		}
		if strings.Contains(strings.ToLower(raw), strings.ToLower(strings.ReplaceAll(p, "-", ""))) {
			t.Errorf("%s is described as migrated and is still on the debt register", p)
		}
	}
	if migrated == 0 {
		t.Fatal("no row is described as migrated; openrouter was moved on 2026-08-23")
	}
}

// With no catalog installed nothing may panic and nothing may lose its tools.
// This is the state of a fresh install before the first fetch, and of every
// machine with no network.
func TestTheWholeMatrixSurvivesWithNoCatalog(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(nil)

	for _, p := range provider.SupportedProviders() {
		for _, m := range []string{"", "some-model", "vendor/some-model"} {
			caps := ResolveCapabilities(p, m)
			if len(caps.Input) == 0 || len(caps.Output) == 0 {
				t.Errorf("%s/%q resolved to no modalities with no catalog", p, m)
			}
			if !caps.Tools && provider.RuntimeFor(NormalizeProvider(p)) == provider.RuntimeOpenAICompatible {
				t.Errorf("%s/%q lost its tools with no catalog to justify it", p, m)
			}
			_ = ResolveVision(p, m)
			_ = ResolveDocuments(p, m)
			_ = ResolveThinkingCapabilities(p, m)
		}
	}
}
