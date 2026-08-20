package main

import (
	"database/sql"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/provider"
)

// recordTokenUsage persists one model response's token usage. Wired into the
// agent via SetUsageReporter (applyConfig), so every API round — including
// each tool-loop iteration — lands here. Failures only log: usage stats must
// never break a chat turn.
func (a *App) recordTokenUsage(conv *conversation, u model.Usage) {
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
		`INSERT INTO token_usage(session_id, model, provider, prompt_tokens, completion_tokens, cached_prompt_tokens, time)
		 VALUES(?,?,?,?,?,?,?)`,
		conv.id, conv.cfg.ModelName, model.NormalizeProvider(conv.cfg.ModelProvider),
		u.PromptTokens, u.CompletionTokens, cached, time.Now().Format(time.RFC3339),
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
// Cost/Priced turn the counts into money. Priced is carried separately rather
// than inferred from a zero Cost, because "this model is not in the price
// catalog" and "this model cost nothing" are different sentences and only one
// of them is ever true. The UI must show a dash for the first.
type UsageRow struct {
	Model string `json:"model"`
	// Provider is empty on every row written before it was recorded, which is
	// not the same as unknown-forever: those get priced by model name, the same
	// way they were before this column existed.
	Provider         string  `json:"provider"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	CachedTokens     int64   `json:"cachedTokens"`
	UncachedTokens   int64   `json:"uncachedTokens"`
	CacheRows        int64   `json:"cacheRows"`
	Calls            int64   `json:"calls"`
	Cost             float64 `json:"cost"`
	Priced           bool    `json:"priced"`
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

	// Cost is the sum over the rows that could be priced, and PricedTokens says
	// how much of the input the figure actually covers. Both are needed: a
	// total built from half the models is not the bill, and presenting it as
	// one would be the most confident wrong number on the page.
	Cost        float64 `json:"cost"`
	PricedCalls int64   `json:"pricedCalls"`
	// PricesFetched is when the catalog was last read, empty when there is no
	// catalog at all. The UI labels every figure with it: these are third-party
	// list prices, not an invoice, and the reader has to be able to see how old
	// they are.
	PricesFetched string `json:"pricesFetched"`
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
	a.priceUsage(&out)
	return out, nil
}

// priceUsage puts money on the rows it can and leaves the rest alone.
//
// Deliberately the last step and deliberately unable to fail: the stats page
// answered a real question before prices existed, and a catalog that could not
// be fetched must cost the user their token counts too.
func (a *App) priceUsage(out *UsageStats) {
	catalog := a.modelCatalog()
	if catalog == nil {
		return
	}
	out.Totals.PricesFetched = catalog.Fetched.Format(time.RFC3339)
	for _, rows := range [][]UsageRow{out.Today, out.Week, out.All} {
		for i := range rows {
			// A plan is not a meter. Codex answers with models OpenAI also
			// sells per token, so pricing those calls by the API rate would
			// bill a subscription twice over and call the result spend.
			if provider.BalanceKindFor(rows[i].Provider) == provider.BalanceSubscription {
				continue
			}
			facts, ok := lookupFacts(catalog, rows[i].Provider, rows[i].Model)
			if !ok || !facts.Price.Priced() {
				continue
			}
			rows[i].Cost = facts.Price.Cost(rows[i].UncachedTokens, rows[i].CachedTokens, rows[i].CompletionTokens)
			rows[i].Priced = true
		}
	}
	// Totals come from the all-time rows, so the headline and the table can
	// never disagree about what was counted.
	for _, r := range out.All {
		if !r.Priced {
			continue
		}
		out.Totals.Cost += r.Cost
		out.Totals.PricedCalls += r.Calls
	}
}

// ModelListing is one row of a provider's model picker: the name it is chosen
// by, and what the catalog knows about it.
//
// Priced is separate from the rates for the same reason UsageRow.Priced is: a
// model the catalog has never heard of must render as a dash. On a list of 411
// OpenRouter models, a zero that reads as "free" is a decision the user would
// act on.
type ModelListing struct {
	Model   string  `json:"model"`
	Input   float64 `json:"input"`
	Output  float64 `json:"output"`
	Priced  bool    `json:"priced"`
	Free    bool    `json:"free"`
	Context int     `json:"context"`
}

// PriceModels annotates a model list the caller already has.
//
// It takes the list rather than fetching its own so the prices cannot end up
// describing a different set of models than the one on screen — the picker's
// list comes from the provider's own endpoint, and a second discovery call
// would be a second answer to "which models are there".
func (a *App) PriceModels(providerName string, models []string) []ModelListing {
	canonical := model.NormalizeProvider(providerName)
	catalog := a.modelCatalog()
	// A local runtime costs nothing to run, so everything it serves is free in
	// the only sense this column means. Nothing about that is in models.dev.
	runsLocally := provider.BalanceKindFor(canonical) == provider.BalanceFree

	out := make([]ModelListing, 0, len(models))
	for _, name := range models {
		row := ModelListing{Model: name, Free: runsLocally}
		// The vendor's own marker is the only trustworthy "free" on a hosted
		// provider. A catalog rate of zero is not one: of 22 OpenRouter models
		// priced at zero, 15 carry this suffix and the other 7 are previews
		// nobody has published a price for yet.
		if strings.HasSuffix(strings.ToLower(name), ":free") {
			row.Free = true
		}
		// The window is asked of ContextWindowTokens rather than read off the
		// catalog row sitting next to the price, so that the picker and the
		// composer's meter cannot answer "how big is this model" differently.
		// They did, and in opposite directions: a Codex model has no catalog
		// row of its own, so this column was blank while the meter drew a
		// 32,000-token window it had made up. Price still comes from the row,
		// because price is exactly what must NOT be inherited from OpenAI on a
		// subscription (priceUsage, and db.go migration 15).
		row.Context = model.ContextWindowTokens(canonical, name)
		if facts, ok := catalog.For(canonical, name); ok && facts.Price.Priced() {
			row.Input, row.Output, row.Priced = facts.Price.Input, facts.Price.Output, true
		}
		out = append(out, row)
	}
	return out
}

// lookupFacts prices a row by provider where the row knows one, and by model
// name where it does not.
//
// The fallback is only for rows written before the provider column existed.
// Those cannot be attributed now and pricing them by name is exactly what the
// stats page already did, so the choice is between the old approximation and no
// figure at all for the history that prompted the feature.
func lookupFacts(catalog *model.ModelCatalog, providerName, modelName string) (model.ModelFacts, bool) {
	if strings.TrimSpace(providerName) != "" {
		return catalog.For(providerName, modelName)
	}
	return catalog.ForModel(modelName)
}

// modelCatalog returns the fetched model facts, reading them off disk once per
// run and installing them so context-window lookups see them too.
//
// Nothing here reaches the network: a chat turn asking for the stats page must
// not wait on models.dev. RefreshModelFacts does the fetching, on its own
// schedule.
func (a *App) modelCatalog() *model.ModelCatalog {
	a.pricesMu.Lock()
	defer a.pricesMu.Unlock()
	if a.pricesLoaded {
		return a.prices
	}
	a.pricesLoaded = true
	root, err := config.DataRoot()
	if err != nil {
		return nil
	}
	catalog, err := model.LoadModelCatalog(root)
	if err != nil {
		debuglog.Msg("model catalog: cache unreadable: %v", err)
		return nil
	}
	a.prices = catalog
	// The context meter reads ContextWindowTokens, which consults whatever is
	// installed — so a cached catalog has to be installed even on the path
	// where no refresh ever succeeds.
	model.SetModelCatalog(catalog)
	return a.prices
}

// RefreshModelFacts fetches the model catalog, caches it, and installs it.
//
// Called once at startup and available to the UI as an explicit retry. Silent
// on failure by the same rule the update check follows: an offline laptop is
// not a broken app, and a red banner about models.dev over a working chat is an
// answer to a question nobody asked.
func (a *App) RefreshModelFacts() {
	root, err := config.DataRoot()
	if err != nil {
		return
	}
	// Installs whatever it ends up with, fresh or cached — see
	// model.RefreshModelCatalog.
	catalog, err := model.RefreshModelCatalog(a.ctx, root)
	if err != nil {
		debuglog.Msg("model catalog: refresh failed: %v", err)
	}
	if catalog == nil {
		return
	}
	a.pricesMu.Lock()
	a.prices, a.pricesLoaded = catalog, true
	a.pricesMu.Unlock()
}

func usageByModel(db *sql.DB, since string) ([]UsageRow, error) {
	// Grouped by model AND provider, because the pair is what a price applies
	// to. The same model id is sold per token by one company and included in a
	// subscription by another, and a row that merged the two could only be
	// priced by guessing which. Old rows carry '' and group together as the
	// unattributed set they are.
	// query-direct: this file aborts on a scan error rather than skipping the row,
	// which is stricter than eachRow. A cost total built from most of the rows is
	// worse than no total, because it looks like an answer.
	rows, err := db.Query(
		`SELECT model,
		        COALESCE(provider, ''),
		        SUM(prompt_tokens),
		        SUM(completion_tokens),
		        COALESCE(SUM(cached_prompt_tokens), 0),
		        COUNT(cached_prompt_tokens),
		        COUNT(*)
		 FROM token_usage WHERE time >= ? GROUP BY model, COALESCE(provider, '')
		 ORDER BY SUM(prompt_tokens)+SUM(completion_tokens) DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []UsageRow
	for rows.Next() {
		var r UsageRow
		if err := rows.Scan(&r.Model, &r.Provider, &r.PromptTokens, &r.CompletionTokens, &r.CachedTokens, &r.CacheRows, &r.Calls); err != nil {
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
	// query-direct: this file aborts on a scan error rather than skipping the row,
	// which is stricter than eachRow. A cost total built from most of the rows is
	// worse than no total, because it looks like an answer.
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
	// query-direct: this file aborts on a scan error rather than skipping the row,
	// which is stricter than eachRow. A cost total built from most of the rows is
	// worse than no total, because it looks like an answer.
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
