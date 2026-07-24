package main

import (
	"context"
	"testing"

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
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("LOCALAPPDATA", base)
	a := &App{cfg: config.Config{ModelProvider: "noop", ModelName: "usage-fake-model", SandboxRoot: t.TempDir()}, dbDir: t.TempDir()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})

	a.applyConfig(a.cfg) // wires SetUsageReporter(a.recordTokenUsage)
	if a.agent == nil {
		t.Fatal("agent not built")
	}
	// Swap in a provider that reports usage (noop reports none), keeping the
	// reporter wiring applyConfig installed.
	a.agent.ReplaceModel(usageProvider{}, "usage-fake-model")

	if _, err := a.agent.Respond(context.Background(), "1+1?", turn.TurnOptions{ThinkLevel: think.LevelLow}); err != nil {
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

func TestRecordAndAggregateTokenUsage(t *testing.T) {
	a := &App{
		cfg:       config.Config{ModelName: "test-model"},
		sessionID: "s1",
		dbDir:     t.TempDir(),
	}
	// Close the SQLite handle before TempDir cleanup — Windows can't delete
	// an open file.
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})

	a.recordTokenUsage(model.Usage{PromptTokens: 100, CompletionTokens: 20})
	a.recordTokenUsage(model.Usage{PromptTokens: 50, CompletionTokens: 5})
	a.cfg.ModelName = "other-model"
	a.recordTokenUsage(model.Usage{PromptTokens: 7, CompletionTokens: 3})

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
