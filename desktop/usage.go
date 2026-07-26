package main

import (
	"database/sql"
	"time"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// recordTokenUsage persists one model response's token usage. Wired into the
// agent via SetUsageReporter (applyConfig), so every API round — including
// each tool-loop iteration — lands here. Failures only log: usage stats must
// never break a chat turn.
func (a *App) recordTokenUsage(u model.Usage) {
	db, err := a.database()
	if err != nil {
		debuglog.Msg("usage: db unavailable: %v", err)
		return
	}
	// NULL, not 0, when the provider does no cache accounting — see
	// addedColumns in db.go. A local model has no cache to hit and must not be
	// averaged in as a 0% hit rate.
	var cached any
	if u.CacheReported {
		cached = u.CachedPromptTokens
	}
	_, err = db.Exec(
		`INSERT INTO token_usage(session_id, model, prompt_tokens, completion_tokens, cached_prompt_tokens, time)
		 VALUES(?,?,?,?,?,?)`,
		a.sessionID, a.cfg.ModelName, u.PromptTokens, u.CompletionTokens, cached, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		debuglog.Msg("usage: insert failed: %v", err)
	}
}

// UsageRow is one model's aggregated token usage within a period.
//
// CachedTokens/UncachedTokens split PromptTokens by where the input came from.
// CacheRows counts how many of this model's calls reported cache accounting at
// all: zero means the provider does not report it (a local runtime), and the
// UI must show "—" rather than a 0% hit rate the provider never claimed.
type UsageRow struct {
	Model            string `json:"model"`
	PromptTokens     int64  `json:"promptTokens"`
	CompletionTokens int64  `json:"completionTokens"`
	CachedTokens     int64  `json:"cachedTokens"`
	UncachedTokens   int64  `json:"uncachedTokens"`
	CacheRows        int64  `json:"cacheRows"`
	Calls            int64  `json:"calls"`
}

// DayPoint is one model's tokens on one calendar day, for the per-day chart
// and the activity heatmap. Aggregated in SQL: the frontend never sees a raw
// usage row, so memory stays flat no matter how long the history gets.
//
// CachedTokens/CacheRows carry the same claim as on UsageRow: CacheRows == 0
// means this model reported no cache accounting that day, so its input cannot
// be split into hit and miss and the chart must not draw it as all-miss.
type DayPoint struct {
	Day              string `json:"day"` // YYYY-MM-DD, local time
	Model            string `json:"model"`
	PromptTokens     int64  `json:"promptTokens"`
	CompletionTokens int64  `json:"completionTokens"`
	CachedTokens     int64  `json:"cachedTokens"`
	CacheRows        int64  `json:"cacheRows"`
}

// UsageTotals are the headline cards.
type UsageTotals struct {
	PromptTokens     int64  `json:"promptTokens"`
	CompletionTokens int64  `json:"completionTokens"`
	CachedTokens     int64  `json:"cachedTokens"`
	UncachedTokens   int64  `json:"uncachedTokens"`
	CacheRows        int64  `json:"cacheRows"`
	Calls            int64  `json:"calls"`
	Sessions         int64  `json:"sessions"`
	Messages         int64  `json:"messages"`
	ActiveDays       int64  `json:"activeDays"`
	CurrentStreak    int64  `json:"currentStreak"`
	TopModel         string `json:"topModel"`
	TopModelShare    int64  `json:"topModelShare"` // percent of total tokens
}

// UsageStats is everything the stats page renders.
type UsageStats struct {
	Today   []UsageRow  `json:"today"`
	Week    []UsageRow  `json:"week"`
	All     []UsageRow  `json:"all"`
	Totals  UsageTotals `json:"totals"`
	Daily   []DayPoint  `json:"daily"`   // last 30 days, per model
	Heatmap []DayPoint  `json:"heatmap"` // last 26 weeks, totalled per day
}

// localDay is the SQLite expression turning a stored RFC3339 timestamp into a
// local calendar day. Stored times carry an offset, so 'localtime' is what
// makes "today" mean the user's today rather than UTC's.
const localDay = `date(time, 'localtime')`

func (a *App) UsageStats() (UsageStats, error) {
	var out UsageStats
	db, err := a.database()
	if err != nil {
		return out, err
	}
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	if out.Today, err = usageByModel(db, midnight.Format(time.RFC3339)); err != nil {
		return out, err
	}
	if out.Week, err = usageByModel(db, now.AddDate(0, 0, -7).Format(time.RFC3339)); err != nil {
		return out, err
	}
	if out.All, err = usageByModel(db, ""); err != nil { // RFC3339 timestamps all sort after ""
		return out, err
	}
	if out.Daily, err = usageByDay(db, now.AddDate(0, 0, -30), true); err != nil {
		return out, err
	}
	if out.Heatmap, err = usageByDay(db, now.AddDate(0, 0, -26*7), false); err != nil {
		return out, err
	}
	if out.Totals, err = usageTotals(db, out.All); err != nil {
		return out, err
	}
	return out, nil
}

func usageByModel(db *sql.DB, since string) ([]UsageRow, error) {
	rows, err := db.Query(
		`SELECT model,
		        SUM(prompt_tokens),
		        SUM(completion_tokens),
		        COALESCE(SUM(cached_prompt_tokens), 0),
		        COUNT(cached_prompt_tokens),
		        COUNT(*)
		 FROM token_usage WHERE time >= ? GROUP BY model
		 ORDER BY SUM(prompt_tokens)+SUM(completion_tokens) DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []UsageRow
	for rows.Next() {
		var r UsageRow
		if err := rows.Scan(&r.Model, &r.PromptTokens, &r.CompletionTokens, &r.CachedTokens, &r.CacheRows, &r.Calls); err != nil {
			return nil, err
		}
		if r.UncachedTokens = r.PromptTokens - r.CachedTokens; r.UncachedTokens < 0 {
			r.UncachedTokens = 0
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// usageByDay returns one point per day, split per model when perModel is set
// and totalled otherwise (the heatmap only needs an intensity).
func usageByDay(db *sql.DB, since time.Time, perModel bool) ([]DayPoint, error) {
	groupBy, selectModel := localDay, `''`
	if perModel {
		groupBy, selectModel = localDay+`, model`, `model`
	}
	rows, err := db.Query(
		`SELECT `+localDay+` AS d, `+selectModel+`, SUM(prompt_tokens), SUM(completion_tokens),
		        COALESCE(SUM(cached_prompt_tokens), 0), COUNT(cached_prompt_tokens)
		 FROM token_usage WHERE time >= ?
		 GROUP BY `+groupBy+` ORDER BY d`, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []DayPoint
	for rows.Next() {
		var p DayPoint
		if err := rows.Scan(&p.Day, &p.Model, &p.PromptTokens, &p.CompletionTokens, &p.CachedTokens, &p.CacheRows); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// usageTotals derives the headline cards. The per-model totals are already in
// hand from the all-time query, so only the counts that live in other tables
// cost a round trip.
func usageTotals(db *sql.DB, all []UsageRow) (UsageTotals, error) {
	var t UsageTotals
	for _, r := range all {
		t.PromptTokens += r.PromptTokens
		t.CompletionTokens += r.CompletionTokens
		t.CachedTokens += r.CachedTokens
		t.UncachedTokens += r.UncachedTokens
		t.CacheRows += r.CacheRows
		t.Calls += r.Calls
		if total := r.PromptTokens + r.CompletionTokens; total > 0 && t.TopModel == "" {
			t.TopModel = r.Model // the all-time query is already ordered heaviest first
		}
	}
	if grand := t.PromptTokens + t.CompletionTokens; grand > 0 && t.TopModel != "" {
		for _, r := range all {
			if r.Model == t.TopModel {
				t.TopModelShare = (r.PromptTokens + r.CompletionTokens) * 100 / grand
				break
			}
		}
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&t.Sessions); err != nil {
		return t, err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&t.Messages); err != nil {
		return t, err
	}

	days, err := activeDays(db)
	if err != nil {
		return t, err
	}
	t.ActiveDays = int64(len(days))
	t.CurrentStreak = streakFrom(days, time.Now())
	return t, nil
}

// activeDays lists every local calendar day with at least one recorded call,
// newest first. One row per active day — bounded by how long the app has been
// used, not by how much it was used.
func activeDays(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT DISTINCT ` + localDay + ` AS d FROM token_usage ORDER BY d DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var days []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

// streakFrom counts consecutive active days ending today or yesterday. Ending
// yesterday still counts: a streak should not read as broken before the day it
// could still be continued in is over.
func streakFrom(daysDesc []string, now time.Time) int64 {
	if len(daysDesc) == 0 {
		return 0
	}
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	if daysDesc[0] != today && daysDesc[0] != yesterday {
		return 0
	}
	cursor, err := time.ParseInLocation("2006-01-02", daysDesc[0], now.Location())
	if err != nil {
		return 0
	}
	var streak int64
	for _, d := range daysDesc {
		if d != cursor.Format("2006-01-02") {
			break
		}
		streak++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return streak
}
