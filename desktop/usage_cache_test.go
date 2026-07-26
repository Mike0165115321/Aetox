package main

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// The schema is all CREATE TABLE IF NOT EXISTS, so a database that already has
// token_usage never sees a new column from it. That is the case every existing
// install is in, and without the ALTER step the INSERT fails on every turn —
// silently, because usage writes only log. This builds the old table by hand
// and proves the upgrade path, which no fresh-database test can.
func TestOpeningAnOlderDatabaseAddsMissingColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aetox.db")

	old, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := old.Exec(`CREATE TABLE token_usage (
		id                INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id        TEXT NOT NULL DEFAULT '',
		model             TEXT NOT NULL,
		prompt_tokens     INTEGER NOT NULL,
		completion_tokens INTEGER NOT NULL,
		time              TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	// A row from before the column existed must survive with NULL, not be lost.
	if _, err := old.Exec(`INSERT INTO token_usage(model, prompt_tokens, completion_tokens, time) VALUES(?,?,?,?)`,
		"legacy-model", 100, 10, time.Now().Format(time.RFC3339)); err != nil {
		t.Fatalf("legacy insert: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	a := &App{cfg: config.Config{ModelName: "new-model"}, dbDir: dir}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})

	// Opening runs the schema then the ALTER; recording must now succeed.
	a.recordTokenUsage(model.Usage{PromptTokens: 4011, CachedPromptTokens: 3968, CacheReported: true, CompletionTokens: 1})

	stats, err := a.UsageStats()
	if err != nil {
		t.Fatalf("UsageStats: %v", err)
	}
	byModel := map[string]UsageRow{}
	for _, r := range stats.All {
		byModel[r.Model] = r
	}
	legacy, ok := byModel["legacy-model"]
	if !ok {
		t.Fatalf("legacy row lost after migration: %+v", stats.All)
	}
	if legacy.PromptTokens != 100 || legacy.CacheRows != 0 {
		t.Errorf("legacy row = %+v, want 100 prompt tokens and no cache accounting", legacy)
	}
	fresh, ok := byModel["new-model"]
	if !ok {
		t.Fatalf("new row never inserted — the ALTER did not run: %+v", stats.All)
	}
	if fresh.PromptTokens != 4011 || fresh.CachedTokens != 3968 || fresh.UncachedTokens != 43 || fresh.CacheRows != 1 {
		t.Errorf("new row = %+v, want 4011 prompt / 3968 cached / 43 uncached / 1 reporting call", fresh)
	}
}

// Running the migration twice must be a no-op, since it runs on every open.
func TestAddedColumnsIsIdempotent(t *testing.T) {
	a := &App{cfg: config.Config{ModelName: "m"}, dbDir: t.TempDir()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := applyAddedColumns(db); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
}

// A local runtime reports no cache accounting at all. Storing that as 0 would
// be indistinguishable from "measured, nothing hit", and the page would show a
// local model a 0% hit rate it never claimed. NULL keeps the two apart.
func TestUnreportedCacheStaysDistinctFromZero(t *testing.T) {
	a := &App{cfg: config.Config{ModelName: "ollama-model"}, dbDir: t.TempDir()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})

	a.recordTokenUsage(model.Usage{PromptTokens: 3005, CompletionTokens: 12}) // no cache accounting
	a.cfg.ModelName = "api-model"
	a.recordTokenUsage(model.Usage{PromptTokens: 4011, CachedPromptTokens: 0, CacheReported: true, CompletionTokens: 1}) // measured: nothing hit

	stats, err := a.UsageStats()
	if err != nil {
		t.Fatalf("UsageStats: %v", err)
	}
	byModel := map[string]UsageRow{}
	for _, r := range stats.All {
		byModel[r.Model] = r
	}
	if got := byModel["ollama-model"].CacheRows; got != 0 {
		t.Errorf("local model reported cache accounting on %d calls, want 0 (UI shows an em dash)", got)
	}
	if got := byModel["api-model"].CacheRows; got != 1 {
		t.Errorf("API model cache-reporting calls = %d, want 1 (UI shows 0%%)", got)
	}
}

func TestStreakFrom(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 0, 0, 0, time.Local)
	day := func(offset int) string { return now.AddDate(0, 0, offset).Format("2006-01-02") }

	for _, tc := range []struct {
		name string
		days []string
		want int64
	}{
		{"no activity", nil, 0},
		{"today only", []string{day(0)}, 1},
		{"three in a row ending today", []string{day(0), day(-1), day(-2)}, 3},
		// Not broken until the day it could still be continued in is over.
		{"ending yesterday still counts", []string{day(-1), day(-2)}, 2},
		{"stale streak is over", []string{day(-3), day(-4)}, 0},
		{"gap stops the count", []string{day(0), day(-2), day(-3)}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := streakFrom(tc.days, now); got != tc.want {
				t.Errorf("streakFrom(%v) = %d, want %d", tc.days, got, tc.want)
			}
		})
	}
}

// The per-day series must come back already grouped, one row per day per model
// — that is what keeps memory flat as history grows.
func TestDailySeriesIsAggregatedPerDay(t *testing.T) {
	a := &App{cfg: config.Config{ModelName: "m1"}, dbDir: t.TempDir()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	for i := 0; i < 5; i++ {
		a.recordTokenUsage(model.Usage{PromptTokens: 10, CompletionTokens: 1})
	}
	a.cfg.ModelName = "m2"
	a.recordTokenUsage(model.Usage{PromptTokens: 7, CompletionTokens: 2})

	stats, err := a.UsageStats()
	if err != nil {
		t.Fatalf("UsageStats: %v", err)
	}
	if len(stats.Daily) != 2 {
		t.Fatalf("6 calls today collapsed to %d points, want 2 (one per model): %+v", len(stats.Daily), stats.Daily)
	}
	if len(stats.Heatmap) != 1 {
		t.Fatalf("heatmap has %d points, want 1 day totalled: %+v", len(stats.Heatmap), stats.Heatmap)
	}
	if stats.Heatmap[0].PromptTokens != 57 {
		t.Errorf("heatmap day total = %d, want 57", stats.Heatmap[0].PromptTokens)
	}
	if stats.Totals.ActiveDays != 1 || stats.Totals.CurrentStreak != 1 {
		t.Errorf("totals = %+v, want 1 active day and a streak of 1", stats.Totals)
	}
	// m1 = 5 calls x (10+1) = 55, m2 = 7+2 = 9, so 55 of 64 tokens.
	if stats.Totals.TopModel != "m1" || stats.Totals.TopModelShare != 85 {
		t.Errorf("top model = %q at %d%%, want m1 at 85%% (55 of 64 tokens)", stats.Totals.TopModel, stats.Totals.TopModelShare)
	}
}
