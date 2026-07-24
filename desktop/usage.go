package main

import (
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
	_, err = db.Exec(
		`INSERT INTO token_usage(session_id, model, prompt_tokens, completion_tokens, time) VALUES(?,?,?,?,?)`,
		a.sessionID, a.cfg.ModelName, u.PromptTokens, u.CompletionTokens, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		debuglog.Msg("usage: insert failed: %v", err)
	}
}

// UsageRow is one model's aggregated token usage within a period.
type UsageRow struct {
	Model            string `json:"model"`
	PromptTokens     int64  `json:"promptTokens"`
	CompletionTokens int64  `json:"completionTokens"`
	Calls            int64  `json:"calls"`
}

// UsageStats aggregates token usage for the Settings page: since local
// midnight, the last 7 days, and all time — per model, heaviest first.
type UsageStats struct {
	Today []UsageRow `json:"today"`
	Week  []UsageRow `json:"week"`
	All   []UsageRow `json:"all"`
}

func (a *App) UsageStats() (UsageStats, error) {
	var out UsageStats
	db, err := a.database()
	if err != nil {
		return out, err
	}
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	query := func(since string) ([]UsageRow, error) {
		rows, err := db.Query(
			`SELECT model, SUM(prompt_tokens), SUM(completion_tokens), COUNT(*)
			 FROM token_usage WHERE time >= ? GROUP BY model
			 ORDER BY SUM(prompt_tokens)+SUM(completion_tokens) DESC`, since)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var result []UsageRow
		for rows.Next() {
			var r UsageRow
			if err := rows.Scan(&r.Model, &r.PromptTokens, &r.CompletionTokens, &r.Calls); err != nil {
				return nil, err
			}
			result = append(result, r)
		}
		return result, rows.Err()
	}
	if out.Today, err = query(midnight.Format(time.RFC3339)); err != nil {
		return out, err
	}
	if out.Week, err = query(now.AddDate(0, 0, -7).Format(time.RFC3339)); err != nil {
		return out, err
	}
	if out.All, err = query(""); err != nil { // RFC3339 timestamps all sort after ""
		return out, err
	}
	return out, nil
}
