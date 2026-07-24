package main

import (
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
)

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
