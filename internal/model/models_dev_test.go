package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A trimmed copy of the real payload's shape, recorded from models.dev on
// 2026-08-15. Two providers whose Aetox name differs from the catalog's, one
// model the catalog lists without pricing, and DeepSeek's real numbers — whose
// fifty-to-one gap between input and cache_read is the whole reason the two are
// separate fields.
const modelsDevFixture = `{
  "deepseek": {
    "id": "deepseek",
    "models": {
      "deepseek-v4-flash": {
        "cost": {"input": 0.14, "output": 0.28, "cache_read": 0.0028},
        "limit": {"context": 1000000, "output": 384000}
      },
      "deepseek-v4-pro": {
        "cost": {"input": 0.435, "output": 0.87, "cache_read": 0.003625}
      }
    }
  },
  "google": {
    "id": "google",
    "models": {
      "gemini-2.5-flash": {"cost": {"input": 0.3, "output": 2.5, "cache_read": 0.03}}
    }
  },
  "moonshotai": {
    "id": "moonshotai",
    "models": {
      "kimi-k3": {"cost": {"input": 0.6, "output": 2.5}}
    }
  },
  "someprovider": {
    "id": "someprovider",
    "models": {
      "unpriced-model": {"limit": {"context": 8192, "output": 4096}}
    }
  }
}`

func fixtureCatalog(t *testing.T) *ModelCatalog {
	t.Helper()
	catalog, err := distill([]byte(modelsDevFixture), time.Now())
	if err != nil {
		t.Fatalf("distill: %v", err)
	}
	return catalog
}

// Aetox names the product a user picks; models.dev names the vendor. Four
// providers disagree, and a lookup that does not translate them silently prices
// Gemini and Kimi at nothing.
func TestPriceBookTranslatesProviderNames(t *testing.T) {
	catalog := fixtureCatalog(t)
	for _, tc := range []struct{ provider, model string }{
		{"gemini", "gemini-2.5-flash"}, // catalog files it under "google"
		{"kimi", "kimi-k3"},            // catalog files it under "moonshotai"
		{"deepseek", "deepseek-v4-flash"},
	} {
		if _, ok := catalog.For(tc.provider, tc.model); !ok {
			t.Errorf("%s/%s has no price — the provider name was not translated", tc.provider, tc.model)
		}
	}
	// Aliases resolve too: the book is keyed by canonical name.
	if _, ok := catalog.For("google-gemini", "gemini-2.5-flash"); !ok {
		t.Error("an alias of gemini did not reach the google entry")
	}
}

// The rule that matters most. A model nobody priced must not come back with a
// price — a zero renders as "this model is free", which is a worse answer than
// silence and one the user would act on.
//
// Knowing a model and pricing it are separate facts, and the catalog really
// does state one without the other: plenty of entries carry a context window
// and no cost. So the lookup answers "known", and Priced answers "quotable".
func TestTheCatalogInventsNoPrices(t *testing.T) {
	catalog := fixtureCatalog(t)
	if _, ok := catalog.For("deepseek", "deepseek-v9-imaginary"); ok {
		t.Error("a model the catalog never listed came back known")
	}

	facts, ok := catalog.For("someprovider", "unpriced-model")
	if !ok {
		t.Fatal("a model listed with a window but no cost was dropped entirely")
	}
	if facts.Price.Priced() {
		t.Error("a model the catalog lists WITHOUT a cost reported a price")
	}
	if facts.Context != 8192 {
		t.Errorf("its window was lost: %+v", facts)
	}

	// And an empty or absent catalog answers the same way rather than panicking.
	var absent *ModelCatalog
	if _, ok := absent.For("deepseek", "deepseek-v4-flash"); ok {
		t.Error("a nil catalog answered a lookup")
	}
	if _, ok := (&ModelCatalog{}).For("deepseek", "deepseek-v4-flash"); ok {
		t.Error("an empty catalog answered a lookup")
	}
}

// Cached input is charged at cache_read, not at the input rate. On DeepSeek the
// two differ by fifty times, so getting this wrong does not shade the figure —
// it changes its order of magnitude.
func TestCostChargesCachedInputAtTheCacheRate(t *testing.T) {
	catalog := fixtureCatalog(t)
	facts, ok := catalog.For("deepseek", "deepseek-v4-flash")
	if !ok {
		t.Fatal("fixture lost its deepseek price")
	}

	// One million tokens of each, so the arithmetic is readable.
	got := facts.Price.Cost(1_000_000, 1_000_000, 1_000_000)
	want := 0.14 + 0.0028 + 0.28
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("cost = %v; want %v", got, want)
	}

	// The bug this guards: pricing the cached half as fresh input.
	naive := 0.14 + 0.14 + 0.28
	if got >= naive {
		t.Fatalf("cost %v did not benefit from the cache rate at all (naive would be %v)", got, naive)
	}
}

// A provider that publishes no cache_read rate charges cache reads as ordinary
// input. Treating the missing field as zero would report those calls as free.
func TestCostWithoutACacheRateFallsBackToInput(t *testing.T) {
	catalog := fixtureCatalog(t)
	facts, ok := catalog.For("kimi", "kimi-k3") // fixture gives it no cache_read
	if !ok {
		t.Fatal("fixture lost its kimi price")
	}
	got := facts.Price.Cost(0, 1_000_000, 0)
	if diff := got - 0.6; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("cached input with no cache rate cost %v; want the input rate 0.6", got)
	}
}

// The owner's real fortnight, priced. Not a round number on purpose: this is
// the figure the usage page will show, and it is worth one test that the whole
// chain from recorded tokens to money lands where hand-arithmetic says it does.
func TestCostOfARealFortnight(t *testing.T) {
	catalog := fixtureCatalog(t)
	flash, _ := catalog.For("deepseek", "deepseek-v4-flash")
	pro, _ := catalog.For("deepseek", "deepseek-v4-pro")

	// From token_usage, 1-14 Aug 2026.
	flashCost := flash.Price.Cost(16_829_510-15_723_008, 15_723_008, 547_745)
	proCost := pro.Price.Cost(12_390_682-12_167_040, 12_167_040, 59_189)
	total := flashCost + proCost

	// 29.2M input tokens for well under a dollar, because 95% of it was cached.
	if total < 0.50 || total > 0.60 {
		t.Fatalf("a fortnight of DeepSeek priced at $%.4f; expected roughly $0.55", total)
	}
}

// token_usage records a model name and no provider, so the usage page has to
// price history by name alone. That has to work, and it has to refuse when the
// name is genuinely ambiguous rather than pick a side.
func TestForModelPricesByNameAndRefusesWhenProvidersDisagree(t *testing.T) {
	catalog := fixtureCatalog(t)
	facts, ok := catalog.ForModel("deepseek-v4-flash")
	if !ok {
		t.Fatal("a distinctive model id could not be priced by name")
	}
	if facts.Price.Input != 0.14 {
		t.Errorf("input rate = %v; want 0.14", facts.Price.Input)
	}
	if _, ok := catalog.ForModel("model-nobody-sells"); ok {
		t.Error("an unknown name came back priced")
	}

	// Two hosts reselling one model at different rates: unknowable, so unknown.
	catalog.Models["hosta/shared-model"] = ModelFacts{Price: ModelPrice{Input: 1, Output: 2}}
	catalog.Models["hostb/shared-model"] = ModelFacts{Price: ModelPrice{Input: 5, Output: 9}}
	if _, ok := catalog.ForModel("shared-model"); ok {
		t.Error("a model priced differently by two providers was given a price anyway")
	}

	// Agreeing on the price is not ambiguous, so it still answers.
	catalog.Models["hostc/agreed-model"] = ModelFacts{Price: ModelPrice{Input: 1, Output: 2}}
	catalog.Models["hostd/agreed-model"] = ModelFacts{Price: ModelPrice{Input: 1, Output: 2}}
	if _, ok := catalog.ForModel("agreed-model"); !ok {
		t.Error("two providers quoting the same price was treated as a conflict")
	}
}

func TestModelCatalogRoundTripsThroughDisk(t *testing.T) {
	dir := t.TempDir()
	catalog := fixtureCatalog(t)
	if err := SaveModelCatalog(dir, catalog); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadModelCatalog(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("the saved catalog did not come back")
	}
	// The window has to survive the round trip too, not just the rates: the
	// context meter reads it off this cache on every launch that starts offline.
	if facts, ok := loaded.For("deepseek", "deepseek-v4-flash"); !ok || facts.Context != 1_000_000 {
		t.Errorf("cached context window = %+v; want 1,000,000", facts)
	}
	if _, ok := loaded.For("deepseek", "deepseek-v4-flash"); !ok {
		t.Error("the cached table lost its prices")
	}
	// No temp file left behind by the atomic write.
	if _, err := LoadModelCatalog(filepath.Join(dir, "nope")); err != nil {
		t.Errorf("loading from a missing directory should be silent, got %v", err)
	}
}

// Aetox renders offline by design. A missing cache is a fresh install, not a
// failure, and a corrupt one is the next refresh's problem — neither may
// surface as an error that a caller has to handle.
func TestLoadModelCatalogIsSilentWhenThereIsNothingGood(t *testing.T) {
	dir := t.TempDir()
	catalog, err := LoadModelCatalog(dir)
	if err != nil || catalog != nil {
		t.Fatalf("missing cache gave (%v, %v); want (nil, nil)", catalog, err)
	}
	if err := writeFileForTest(filepath.Join(dir, modelCatalogFile), "{not json"); err != nil {
		t.Fatal(err)
	}
	catalog, err = LoadModelCatalog(dir)
	if err != nil || catalog != nil {
		t.Fatalf("corrupt cache gave (%v, %v); want (nil, nil)", catalog, err)
	}
}

// A refresh that cannot reach the network must leave the user with the prices
// they already had, not with none.
func TestRefreshFallsBackToTheCachedTable(t *testing.T) {
	// RefreshModelCatalog INSTALLS what it resolves — that is its job — so this
	// test replaces the package catalog as a side effect and has to put it
	// back. It was harmless while nothing read the installed catalog; once
	// TestMain began seeding one, every test ordered after this one silently
	// ran against the cached fixture this test wrote instead.
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })

	dir := t.TempDir()
	if err := SaveModelCatalog(dir, fixtureCatalog(t)); err != nil {
		t.Fatal(err)
	}
	// A context already cancelled stands in for "no network".
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	catalog, err := RefreshModelCatalog(ctx, dir)
	if err == nil {
		t.Error("a failed refresh reported success")
	}
	if catalog == nil {
		t.Fatal("a failed refresh threw away the cached prices")
	}
	if _, ok := catalog.For("deepseek", "deepseek-v4-flash"); !ok {
		t.Error("the fallback table has no prices in it")
	}
}

func TestFetchModelCatalogParsesTheLiveShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(modelsDevFixture))
	}))
	defer server.Close()

	catalog, err := FetchModelCatalog(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if catalog.Fetched.IsZero() {
		t.Error("the table came back with no fetch time, so nothing can label it as an estimate")
	}
	if _, ok := catalog.For("gemini", "gemini-2.5-flash"); !ok {
		t.Error("a fetched table is missing a price the fixture states")
	}
}

func writeFileForTest(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// The context meter reads ContextWindowTokens on every turn, and until now that
// was a curated per-provider table with a fallback — which is what most models
// actually got. The catalog states a window per model, so it goes first.
//
// Installed state is global, so every case here restores it.
func TestContextWindowPrefersTheCatalogAndFallsBackToTheTables(t *testing.T) {
	// Restore what was installed, not nil. TestMain seeds a catalog for the
	// package now, and a cleanup that wipes it leaves every test running
	// after this one in a world with none.
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })

	// Nothing installed: exactly the behaviour that shipped before this.
	SetModelCatalog(nil)
	if got := ContextWindowTokens("zai", "glm-4.5"); got != 128_000 {
		t.Fatalf("with no catalog, glm-4.5 = %d; want the curated 128,000", got)
	}

	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		// The catalog is the more exact of the two where both answer.
		"zai/glm-4.5": {Context: 131_072},
		// A model no curated table has ever heard of. This is the case that
		// mattered: Gemini's branch knows one prefix and an account lists 37
		// models, so everything else was measured against a flat guess.
		"google/gemini-3.7-flash": {Context: 2_000_000},
		// Stated as zero, which is the catalog declining to say — not a real
		// limit, and reading it as one would show a full meter on an empty chat.
		"deepseek/deepseek-v4-flash": {Context: 0},
	}})

	if got := ContextWindowTokens("zai", "glm-4.5"); got != 131_072 {
		t.Errorf("glm-4.5 = %d; want the catalog's exact 131,072", got)
	}
	// Provider names are translated on the way in: Aetox says gemini, the
	// catalog says google.
	if got := ContextWindowTokens("gemini", "gemini-3.7-flash"); got != 2_000_000 {
		t.Errorf("gemini-3.7-flash = %d; want the catalog's 2,000,000", got)
	}
	if got := ContextWindowTokens("deepseek", "deepseek-v4-flash"); got != 1_000_000 {
		t.Errorf("a catalog window of 0 overrode the curated table: got %d, want 1,000,000", got)
	}
	// A model in neither still falls through to the provider's curated answer.
	if got := ContextWindowTokens("anthropic", "claude-haiku-4-5"); got != 200_000 {
		t.Errorf("anthropic fell out of its curated table: got %d", got)
	}
}

// Every model name ever written into Go has rotted. DefaultFor exists so none
// has to be, and the rule has to survive the two ways a naive one fails:
// cheapest-only picks a safety classifier, newest-only picks the flagship.
func TestDefaultForPicksTheCheapCurrentWorkhorse(t *testing.T) {
	catalog := &ModelCatalog{Models: map[string]ModelFacts{
		// The flagship: newest, but nobody's sensible cold-start default.
		"acme/acme-max-2": {
			Price: ModelPrice{Input: 9, Output: 30}, Context: 200_000,
			ToolCall: true, Output: []string{"text"}, Released: "2026-08-01",
		},
		// The small current model beside it. This is the answer.
		"acme/acme-flash-2": {
			Price: ModelPrice{Input: 0.2, Output: 0.8}, Context: 200_000,
			ToolCall: true, Output: []string{"text"}, Released: "2026-07-20",
		},
		// Cheaper still, but two generations old — the leftovers a price-only
		// rule reaches for.
		"acme/acme-tiny-1": {
			Price: ModelPrice{Input: 0.01, Output: 0.02}, Context: 8_000,
			ToolCall: true, Output: []string{"text"}, Released: "2023-01-01",
		},
		// Cheapest of all and useless: a classifier that cannot call a tool.
		"acme/acme-guard": {
			Price: ModelPrice{Input: 0.001, Output: 0.002}, Context: 8_000,
			ToolCall: false, Output: []string{"text"}, Released: "2026-07-25",
		},
		// An embedding model: no output price at all.
		"acme/acme-embed": {
			Price: ModelPrice{Input: 0.01}, Context: 8_000,
			ToolCall: true, Output: []string{"text"}, Released: "2026-07-25",
		},
	}}
	if got := catalog.DefaultFor("acme"); got != "acme-flash-2" {
		t.Fatalf("DefaultFor = %q; want acme-flash-2", got)
	}
}

// Where a provider ships nothing that calls tools, insisting on it would return
// nothing and leave the caller on a static name already known to be dead. A
// real model that cannot call a tool is the better of two bad answers.
func TestDefaultForRelaxesToolCallingWhenNobodyOffersIt(t *testing.T) {
	catalog := &ModelCatalog{Models: map[string]ModelFacts{
		"search-co/search-basic": {
			Price: ModelPrice{Input: 1, Output: 1}, Context: 128_000,
			ToolCall: false, Output: []string{"text"}, Released: "2026-05-01",
		},
		"search-co/search-deep": {
			Price: ModelPrice{Input: 2, Output: 8}, Context: 128_000,
			ToolCall: false, Output: []string{"text"}, Released: "2026-05-01",
		},
	}}
	if got := catalog.DefaultFor("search-co"); got != "search-basic" {
		t.Fatalf("DefaultFor = %q; want the cheaper search-basic", got)
	}
}

// A silent catalog must change nothing: the caller stays on the live model list
// it already asked for, and only then on the static name.
func TestDefaultForIsSilentWhenItKnowsNothing(t *testing.T) {
	var absent *ModelCatalog
	if got := absent.DefaultFor("acme"); got != "" {
		t.Errorf("a nil catalog answered %q", got)
	}
	if got := (&ModelCatalog{}).DefaultFor("acme"); got != "" {
		t.Errorf("an empty catalog answered %q", got)
	}
	// A provider it has never heard of is the same case.
	if got := fixtureCatalog(t).DefaultFor("nobody"); got != "" {
		t.Errorf("an unknown provider answered %q", got)
	}
}
