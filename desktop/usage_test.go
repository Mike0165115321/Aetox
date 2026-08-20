package main

import (
	"context"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// usageProvider is a minimal model.Provider whose responses carry usage.
type usageProvider struct{}

func (usageProvider) Name() string { return "usage-fake" }
func (usageProvider) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	return model.Response{Text: "2", Usage: &model.Usage{PromptTokens: 42, CompletionTokens: 1}}, nil
}

// End-to-end through the real wiring, no UI: applyConfig registers the usage
// reporter on the agent; a model response with usage must land in SQLite and
// come back aggregated from UsageStats. This is the chain the Settings page
// shows.
func TestUsagePipelineEndToEnd(t *testing.T) {
	isolateUserDirs(t)
	a := seed(&App{cfg: config.Config{ModelProvider: "noop", ModelName: "usage-fake-model", SandboxRoot: t.TempDir()}, dbDir: t.TempDir()}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})

	a.applyConfig(a.cur(), a.cfg) // wires SetUsageReporter(a.recordTokenUsage)
	if a.cur().agent == nil {
		t.Fatal("agent not built")
	}
	// Swap in a provider that reports usage (noop reports none), keeping the
	// reporter wiring applyConfig installed.
	a.cur().agent.ReplaceModel(usageProvider{}, "usage-fake-model")

	if _, err := a.cur().agent.Respond(context.Background(), "1+1?", turn.TurnOptions{ThinkLevel: think.LevelLow}); err != nil {
		t.Fatalf("Respond: %v", err)
	}

	stats, err := a.UsageStats()
	if err != nil {
		t.Fatalf("UsageStats: %v", err)
	}
	if len(stats.Today) != 1 || stats.Today[0].Model != "usage-fake-model" ||
		stats.Today[0].PromptTokens != 42 || stats.Today[0].CompletionTokens != 1 || stats.Today[0].Calls != 1 {
		t.Fatalf("pipeline result = %+v, want one usage-fake-model row 42/1", stats.Today)
	}
}

// The counts were always there; the price was the missing half. This is the
// whole chain: recorded tokens, a cached catalog, money on the row.
//
// The numbers are the owner's real DeepSeek fortnight, which is what made the
// case for building this — 29.2M input tokens that no screen in the app could
// turn into a figure, so "why did my balance drain" had to be answered by
// reading SQLite by hand.
func TestUsageStatsPutsMoneyOnRowsItCanPrice(t *testing.T) {
	isolateUserDirs(t)
	// Named outright rather than inferred from the isolated home: the price
	// cache is the one file these two tests must not share, and a sibling that
	// leaks its catalog in would make the no-catalog test below pass or fail on
	// execution order.
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := seed(&App{cfg: config.Config{ModelProvider: "deepseek", ModelName: "deepseek-v4-flash"}, dbDir: t.TempDir()}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	root, err := config.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := model.SaveModelCatalog(root, &model.ModelCatalog{
		Fetched: time.Now(),
		Models: map[string]model.ModelFacts{
			"deepseek/deepseek-v4-flash": {Price: model.ModelPrice{Input: 0.14, Output: 0.28, CacheRead: 0.0028}, Context: 1_000_000},
		},
	}); err != nil {
		t.Fatal(err)
	}

	a.recordTokenUsage(a.cur(), model.Usage{
		PromptTokens: 16_829_510, CachedPromptTokens: 15_723_008,
		CompletionTokens: 547_745, CacheReported: true,
	})
	// A model the catalog has never heard of, recorded in the same table.
	a.cur().cfg.ModelName = "some-local-model"
	a.recordTokenUsage(a.cur(), model.Usage{PromptTokens: 1000, CompletionTokens: 100})

	stats, err := a.UsageStats()
	if err != nil {
		t.Fatalf("UsageStats: %v", err)
	}

	var priced, unpriced *UsageRow
	for i := range stats.All {
		switch stats.All[i].Model {
		case "deepseek-v4-flash":
			priced = &stats.All[i]
		case "some-local-model":
			unpriced = &stats.All[i]
		}
	}
	if priced == nil || unpriced == nil {
		t.Fatalf("expected both rows, got %+v", stats.All)
	}

	if !priced.Priced {
		t.Fatal("a model the catalog prices came back unpriced")
	}
	if priced.Cost < 0.34 || priced.Cost > 0.37 {
		t.Errorf("cost = $%.4f; hand arithmetic says about $0.352", priced.Cost)
	}

	// The rule the whole feature stands on: unknown is not free. A zero here
	// renders as "this model costs nothing", which the user would act on.
	if unpriced.Priced {
		t.Error("a model with no catalog entry was marked as priced")
	}
	if unpriced.Cost != 0 {
		t.Errorf("an unpriced row carries a cost of %v", unpriced.Cost)
	}

	// The headline counts only what it could price, and says so, so a total
	// built from half the models is never mistaken for the bill.
	if stats.Totals.PricedCalls != 1 {
		t.Errorf("PricedCalls = %d; want 1 of the 2 calls", stats.Totals.PricedCalls)
	}
	if stats.Totals.PricesFetched == "" {
		t.Error("no fetch time, so nothing can label the figure as an estimate")
	}
}

// Prices are a bonus on top of a page that worked without them. No catalog must
// cost the user their token counts.
func TestUsageStatsStillWorksWithNoPriceCatalog(t *testing.T) {
	isolateUserDirs(t)
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // guaranteed empty: no catalog here
	a := seed(&App{cfg: config.Config{ModelProvider: "deepseek", ModelName: "deepseek-v4-flash"}, dbDir: t.TempDir()}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.recordTokenUsage(a.cur(), model.Usage{PromptTokens: 500, CompletionTokens: 50})

	stats, err := a.UsageStats()
	if err != nil {
		t.Fatalf("UsageStats: %v", err)
	}
	if len(stats.All) != 1 || stats.All[0].PromptTokens != 500 {
		t.Fatalf("the counts did not survive a missing catalog: %+v", stats.All)
	}
	if stats.All[0].Priced || stats.Totals.Cost != 0 || stats.Totals.PricesFetched != "" {
		t.Errorf("money appeared with no catalog to price it from: %+v", stats.Totals)
	}
}

func TestRecordAndAggregateTokenUsage(t *testing.T) {
	a := seed(&App{
		cfg:   config.Config{ModelName: "test-model"},
		dbDir: t.TempDir(),
	}, &conversation{id: "s1"})
	// Close the SQLite handle before TempDir cleanup — Windows can't delete
	// an open file.
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})

	a.recordTokenUsage(a.cur(), model.Usage{PromptTokens: 100, CompletionTokens: 20})
	a.recordTokenUsage(a.cur(), model.Usage{PromptTokens: 50, CompletionTokens: 5})
	a.cur().cfg.ModelName = "other-model"
	a.recordTokenUsage(a.cur(), model.Usage{PromptTokens: 7, CompletionTokens: 3})

	stats, err := a.UsageStats()
	if err != nil {
		t.Fatalf("UsageStats: %v", err)
	}
	for _, period := range []struct {
		name string
		rows []UsageRow
	}{{"today", stats.Today}, {"week", stats.Week}, {"all", stats.All}} {
		if len(period.rows) != 2 {
			t.Fatalf("%s: got %d models, want 2 (%+v)", period.name, len(period.rows), period.rows)
		}
		// Heaviest first: test-model (175 tokens) before other-model (10).
		if period.rows[0].Model != "test-model" || period.rows[0].PromptTokens != 150 ||
			period.rows[0].CompletionTokens != 25 || period.rows[0].Calls != 2 {
			t.Fatalf("%s: unexpected first row %+v", period.name, period.rows[0])
		}
	}
}

// A subscription is not a meter. Codex answers with models OpenAI also sells
// per token, so pricing those calls at the API rate would invent a bill nobody
// was sent — the user already paid a flat monthly fee for them.
//
// This is what the provider column was added for: the model name alone cannot
// tell the two apart, and `gpt-5.6-luna` really is both.
func TestSubscriptionUsageIsCountedButNotPriced(t *testing.T) {
	isolateUserDirs(t)
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := seed(&App{cfg: config.Config{ModelProvider: "codex", ModelName: "gpt-5.6-luna"}, dbDir: t.TempDir()}, newConversation())
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	root, err := config.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	// The catalog does price this model — under OpenAI, where it is sold per
	// token. That is exactly the trap.
	if err := model.SaveModelCatalog(root, &model.ModelCatalog{
		Fetched: time.Now(),
		Models: map[string]model.ModelFacts{
			"openai/gpt-5.6-luna": {Price: model.ModelPrice{Input: 1.25, Output: 10}, Context: 400_000},
		},
	}); err != nil {
		t.Fatal(err)
	}

	a.recordTokenUsage(a.cur(), model.Usage{PromptTokens: 15_778_607, CompletionTokens: 102_333})

	stats, err := a.UsageStats()
	if err != nil {
		t.Fatalf("UsageStats: %v", err)
	}
	if len(stats.All) != 1 {
		t.Fatalf("expected one row, got %+v", stats.All)
	}
	row := stats.All[0]
	// The tokens are still counted — the page's original job is untouched.
	if row.PromptTokens != 15_778_607 {
		t.Errorf("the subscription's tokens went missing: %+v", row)
	}
	if row.Provider != "codex" {
		t.Errorf("provider = %q; want the row to remember who served it", row.Provider)
	}
	// But no money, because none was spent per token.
	if row.Priced || row.Cost != 0 {
		t.Errorf("a subscription turn was billed per token: cost=%v priced=%v", row.Cost, row.Priced)
	}
	if stats.Totals.Cost != 0 || stats.Totals.PricedCalls != 0 {
		t.Errorf("the headline counted subscription usage as spend: %+v", stats.Totals)
	}
}
