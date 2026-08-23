package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// What models.dev tells Aetox about a model, and what Aetox does with it.
//
// Two facts are wanted from the same document, for the same reason: both were
// previously answered by tables written into Go, and a table of model facts is
// stale the week after it is written.
//
//   - Price. token_usage has held prompt/cached/completion counts per call
//     since the usage page shipped; what was missing was a rate to multiply
//     them by, so "why did my balance drain?" could only be answered in tokens.
//   - Context window. context_window.go curates one per provider with a
//     fallback, and the fallback is what most models actually get: the Gemini
//     entry knows one prefix and the account's own discovery lists 37 models,
//     so every 3.x model was being measured against a guess — and that guess
//     drives the percentage on the composer that the user reads all day.
//
// Three rules keep a third-party catalog from becoming a liability of its own:
//
//   - Nothing is invented. A model the catalog does not know has no price and
//     no stated window; callers fall back to what they did before, and the
//     usage page shows a dash rather than a zero that reads as "free".
//   - The network is never required. Aetox renders offline by design; the
//     distilled table is cached on disk and the app runs on the cache. A failed
//     refresh leaves the last good table in place.
//   - What is cached is the handful of numbers we use, not the 3.7 MB upstream
//     document. There is no reason to keep 185 providers' descriptions,
//     modalities and knowledge cutoffs on a user's disk.

// ModelPrice is what a model charges, in USD per million tokens.
//
// CacheRead is its own field rather than a discount factor because the gap is
// not a rounding detail: DeepSeek V4 Flash reads cache at $0.0028 against $0.14
// for fresh input, fifty times cheaper. Aetox measures cache hits already
// (93-98% on the owner's own history), so pricing that splits the two is the
// difference between a believable figure and one off by an order of magnitude.
type ModelPrice struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// Cost prices one call. uncachedInput and cachedInput must already be split —
// Usage.UncachedPromptTokens does that — because charging the cached half at
// the fresh-input rate is exactly the mistake this type exists to prevent.
//
// A provider that publishes no cache_read rate charges cache reads as ordinary
// input, which is what the fallback below says.
func (p ModelPrice) Cost(uncachedInput, cachedInput, output int64) float64 {
	cacheRate := p.CacheRead
	if cacheRate == 0 {
		cacheRate = p.Input
	}
	return float64(uncachedInput)/1e6*p.Input +
		float64(cachedInput)/1e6*cacheRate +
		float64(output)/1e6*p.Output
}

// Priced reports whether this is a real rate rather than an absent one. A row
// quoting nothing for both input and output is a row the catalog declined to
// price, and treating it as free would invent a fact.
func (p ModelPrice) Priced() bool { return p.Input != 0 || p.Output != 0 }

// ModelFacts is everything Aetox keeps about one model.
//
// Context is 0 when the catalog states no window, which is a different claim
// from "small window" and must stay distinguishable — a caller that reads 0 as
// a real limit would show a full context meter on an empty conversation.
type ModelFacts struct {
	Price     ModelPrice `json:"price"`
	Context   int        `json:"context"`
	MaxOutput int        `json:"maxOutput"`
	// Input and Output are the modalities this model takes and produces, in
	// the catalog's own vocabulary: text, image, pdf, audio, video.
	//
	// Lists rather than a boolean per modality, which is the shape opencode
	// settled on and the reason to copy it: audio, video and pdf are already in
	// the table (591, 1,007 and 1,608 models), so every boolean added here
	// would have been a fifth ResolveX function to write, a fifth fallback
	// philosophy to invent, and a fifth place for the answer to drift.
	//
	// This replaced `ImageIn bool` and `TextOut bool`, which were the same fact
	// spelled twice in one struct.
	//
	// It was the last thing in this struct's neighbourhood to be answered by a
	// hand-written list. visionModelMarkers matched substrings of the model id
	// ("gpt-4o", "claude-3", "sonnet", "vision"), which works right up until a
	// company names a sighted model something the list never anticipated:
	// measured 2026-08-23, that list called 13 of opencode-go's 28 models blind
	// when they can see, 18 of 93 on opencode and 99 of 360 on openrouter. The
	// owner met it as a screenshot going to image_ocr instead of to qwen3.7-plus,
	// which takes text, image and video.
	//
	// The galling part is that this was never missing data. `modalities` was
	// already parsed here and only its Output half was read.
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
	// ToolCall is carried, with Output above, so DefaultFor can tell a chat
	// model from the rest of a provider's shelf. Without them the rule picked
	// Groq's llama-prompt-guard (a safety classifier) and Qwen's asr-flash
	// (speech recognition), because both are cheap and neither can hold a
	// conversation. An agent needs tools; a model that cannot call one is not a
	// candidate.
	ToolCall bool `json:"toolCall"`
	// Released is the vendor's date for the model, used to prefer something
	// current over something merely cheap. Empty when the catalog omits it.
	Released string `json:"released"`
	// NoTemperature is set only where the catalog states outright that the
	// model refuses a temperature. Written in the negative on purpose: 475 of
	// the 7,248 models it describes omit the field entirely, and a plain
	// `Temperature bool` would read every one of those absences as a refusal
	// and strip a setting those models accept. Absent stays false here, the
	// temperature is sent, and temperatureRefused is what catches a model the
	// catalog was silent or wrong about.
	NoTemperature bool `json:"noTemperature"`
	// ReasoningToggle is whether the thinking can be switched off, and
	// ReasoningLevels are the effort rungs the model accepts, shallowest first.
	//
	// Separate fields because a model can have either, both or neither:
	// deepseek-v4-flash carries a toggle AND [low, high, max], claude-opus-5
	// carries the rungs with no way to stop it, kimi-k2.6 is a plain on/off.
	// One field would have to invent a rung for the toggle-only case and lose
	// the off position on the models that have both.
	//
	// Only openrouter reads these today. The other nine providers still answer
	// from the prefix tables in thinking_capabilities.go, on purpose: those are
	// one company per row, where a table ages slowly, and each carries measured
	// decisions worth moving one at a time rather than in one sweep.
	ReasoningToggle bool     `json:"reasoningToggle"`
	ReasoningLevels []string `json:"reasoningLevels,omitempty"`
	// Reasoning is whether the model has a thinking dial at all.
	//
	// Read per model rather than per provider because of the gateways: one
	// OpenCode base URL serves 64 ids from nine vendors, and 88 of the 93 the
	// catalog describes reason while the rest do not. The alternative is the
	// shape `openrouter` still uses — a list of model ids compiled by hand,
	// which for a gateway is nine companies' release schedules copied into
	// this repo and wrong the week any of them ships.
	//
	// False on a catalog file written before this field existed, which reads
	// as "no dial" until the next refresh. That is the safe direction: a
	// missing menu is recoverable, a menu whose levels go nowhere is not.
	Reasoning bool `json:"reasoning"`
}

// ModelCatalog is the distilled table plus the moment it was fetched.
//
// Fetched is not decoration. Every figure derived from the prices is an
// estimate against a third party's published list, and a UI that shows money
// without saying how old the rates are invites the reader to treat it as an
// invoice. It is not one.
type ModelCatalog struct {
	Fetched time.Time             `json:"fetched"`
	Source  string                `json:"source"`
	Models  map[string]ModelFacts `json:"models"`
}

// modelsDevAPI is the published catalog. One document, no key, no pagination.
const modelsDevAPI = "https://models.dev/api.json"

// modelCatalogFile is the distilled cache inside the app's own data root.
const modelCatalogFile = "model-catalog.json"

// modelsDevProvider maps an Aetox canonical provider onto the name models.dev
// files it under.
//
// Four of them disagree, and the disagreements are not typos to be normalized
// away — they are two projects naming the same company differently. Aetox names
// the product a user picks ("gemini", "kimi"); models.dev names the vendor
// ("google", "moonshotai"). Anything not listed is already identical.
func modelsDevProvider(canonical string) string {
	switch canonical {
	case "gemini":
		return "google"
	case "kimi":
		return "moonshotai"
	case "together":
		return "togetherai"
	case "qwen":
		// DashScope serves Alibaba's own models under Alibaba's catalog entry.
		return "alibaba"
	default:
		return canonical
	}
}

// catalogKey is how one model is addressed in the table.
func catalogKey(provider, modelName string) string {
	return modelsDevProvider(NormalizeProvider(provider)) + "/" +
		strings.ToLower(strings.TrimSpace(modelName))
}

// For looks up one model. The second return is false when the catalog has never
// heard of it, which callers must render as "unknown" and never as zero.
func (c *ModelCatalog) For(provider, modelName string) (ModelFacts, bool) {
	if c == nil || len(c.Models) == 0 {
		return ModelFacts{}, false
	}
	facts, ok := c.Models[catalogKey(provider, modelName)]
	if !ok {
		return ModelFacts{}, false
	}
	return facts, true
}

// ForModel looks a model up by name when the provider was not recorded.
//
// token_usage has never stored which provider served a call, and adding the
// column would only answer for rows written after it — the fortnight of history
// that prompted all this would stay unpriced. Model ids are distinctive enough
// to look up on their own ("deepseek-v4-flash", "glm-4.5"), so this scans for
// the name across the catalog.
//
// Where several providers serve the same id it only answers if they agree.
// Two hosts reselling one open model at different rates is a real case, and
// picking one at random would put a number on screen that is wrong in a way
// nobody could see. Disagreement means unknown.
func (c *ModelCatalog) ForModel(modelName string) (ModelFacts, bool) {
	if c == nil || len(c.Models) == 0 {
		return ModelFacts{}, false
	}
	suffix := "/" + strings.ToLower(strings.TrimSpace(modelName))
	var found ModelFacts
	seen := false
	for key, facts := range c.Models {
		if !strings.HasSuffix(key, suffix) {
			continue
		}
		if !seen {
			found, seen = facts, true
			continue
		}
		if !sameFacts(facts, found) {
			return ModelFacts{}, false // several providers, several answers
		}
	}
	return found, seen
}

// sameFacts is == written out, because ModelFacts carries a slice now and Go
// will not compare a struct that does. Spelled out rather than reflect.DeepEqual
// so that adding a field is a compile error here instead of a silent change to
// the "several providers disagree" rule above.
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameFacts(a, b ModelFacts) bool {
	if a.Price != b.Price || a.Context != b.Context || a.MaxOutput != b.MaxOutput ||
		a.ToolCall != b.ToolCall || a.Released != b.Released ||
		a.NoTemperature != b.NoTemperature || a.Reasoning != b.Reasoning ||
		a.ReasoningToggle != b.ReasoningToggle {
		return false
	}
	return sameStrings(a.ReasoningLevels, b.ReasoningLevels) &&
		sameStrings(a.Input, b.Input) && sameStrings(a.Output, b.Output)
}

// DefaultFor picks a provider's cold-start model out of the catalog.
//
// It exists so that no model name has to be written into Go. Every name that
// ever was has rotted: the Gemini fallback pointed at a model Google had
// withdrawn from new accounts, and a sweep on 2026-08-15 found four more dead
// on arrival — Perplexity's still named the llama-sonar line, retired long
// enough ago that the catalog lists four models and none of them is it. A name
// in source is a fact with an expiry date and no alarm on it.
//
// The rule, in order, is chosen so that its output cannot be a surprise:
//
//   - It must call tools and answer in text. Aetox is an agent; a model that
//     cannot call a tool cannot do the job. Cheapness alone picked Groq's
//     llama-prompt-guard, a safety classifier, and Qwen's asr-flash, a speech
//     recognizer — both real entries on those providers' shelves, neither able
//     to hold a conversation.
//   - A real per-token price on both sides, and a stated context window. This
//     drops embedding models and the preview rows a catalog carries at zero,
//     and a model Aetox cannot measure is one whose meter would read nothing.
//   - Of what remains, only the current generation: models released within a
//     year of that provider's newest. This is the half of the rule that keeps
//     it honest in both directions. Sorting by price alone reaches for a
//     vendor's oldest leftovers; sorting by date alone reaches for their
//     flagship, and picking the flagship by default is how a first run opens on
//     the most expensive model a company sells — the same deepseek-v4-pro that
//     took 42% of a fortnight's spend in 22 minutes.
//   - Then the cheapest of that generation. That is the small fast tier every
//     vendor ships beside its flagship, and it is exactly what the hand-written
//     names used to say: haiku, flash, mini, small. The judgment those encoded
//     was never the name, it was "the cheap current workhorse" — which is a
//     rule, and rules do not rot.
//   - Then a vendor-published moving alias, then alphabetical, so the answer is
//     stable across runs rather than map-order roulette.
//
// Returns "" when the catalog knows nothing usable, which leaves the caller on
// whatever it did before — the live model list it already asked for, and only
// then the static name.
func (c *ModelCatalog) DefaultFor(providerName string) string {
	if c == nil || len(c.Models) == 0 {
		return ""
	}
	prefix := modelsDevProvider(NormalizeProvider(providerName)) + "/"

	type candidate struct {
		id    string
		facts ModelFacts
	}
	var pool []candidate
	newest := ""
	for key, facts := range c.Models {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if !facts.Produces("text") || facts.Price.Input <= 0 || facts.Price.Output <= 0 || facts.Context <= 0 {
			continue
		}
		pool = append(pool, candidate{strings.TrimPrefix(key, prefix), facts})
		if facts.Released > newest {
			newest = facts.Released
		}
	}
	if len(pool) == 0 {
		return ""
	}

	// Tool calling is required where the provider offers it at all. Where none
	// of its models claim it — Perplexity's search models, for one — insisting
	// would return nothing and leave the caller on a name already known to be
	// dead, which is the worse of the two answers.
	withTools := pool[:0:0]
	for _, c := range pool {
		if c.facts.ToolCall {
			withTools = append(withTools, c)
		}
	}
	if len(withTools) > 0 {
		pool = withTools
	}

	cutoff := generationCutoff(newest)
	current := pool[:0:0]
	for _, c := range pool {
		if c.facts.Released >= cutoff {
			current = append(current, c)
		}
	}
	if len(current) > 0 {
		pool = current
	}

	best := pool[0]
	for _, c := range pool[1:] {
		if betterDefault(c.id, c.facts, best.id, best.facts) {
			best = c
		}
	}
	return best.id
}

// generationCutoff is one year before the provider's newest release, as an ISO
// date string. An empty or unparseable newest disables the window rather than
// excluding everything, since a catalog that states no dates should narrow
// nothing.
func generationCutoff(newest string) string {
	when, err := time.Parse("2006-01-02", newest)
	if err != nil {
		return ""
	}
	return when.AddDate(-1, 0, 0).Format("2006-01-02")
}

// betterDefault reports whether (id, facts) should displace (curID, cur) as the
// cold-start pick, among candidates already filtered to one generation.
func betterDefault(id string, facts ModelFacts, curID string, cur ModelFacts) bool {
	if facts.Price.Input != cur.Price.Input {
		return facts.Price.Input < cur.Price.Input
	}
	if alias, curAlias := strings.HasSuffix(id, "-latest"), strings.HasSuffix(curID, "-latest"); alias != curAlias {
		return alias
	}
	return id < curID
}

// Age says how stale the table is, for the label next to any figure built on it.
func (c *ModelCatalog) Age() time.Duration {
	if c == nil || c.Fetched.IsZero() {
		return 0
	}
	return time.Since(c.Fetched)
}

// installedCatalog is the table ContextWindowTokens consults.
//
// A package global with a setter, the same shape as SetQuotaObserver above it,
// and for the same reason: ContextWindowTokens is a pure lookup called from
// three entry points on every turn, and neither it nor its callers should learn
// to read a file. Whoever owns the process installs the catalog once.
var (
	installedCatalogMu sync.RWMutex
	installedCatalog   *ModelCatalog
)

// SetModelCatalog installs the fetched catalog for context-window lookups.
// Passing nil detaches it, which returns ContextWindowTokens to the curated
// tables alone. Safe to call at any time.
func SetModelCatalog(c *ModelCatalog) {
	installedCatalogMu.Lock()
	defer installedCatalogMu.Unlock()
	installedCatalog = c
}

// catalogRefusesTemperature reports whether the installed catalog states that
// this model rejects a temperature.
//
// It exists so the first attempt is already correct on a model that is known to
// refuse one, rather than spending a round trip to be told. The replay in
// openai_compatible.go stays regardless: this table is a third party's, and the
// `nvidia` row is the standing proof that it can be wrong about an endpoint
// (43 of 100 ids). Measured refusal outranks it; this only saves the first hit.
func catalogRefusesTemperature(provider, modelName string) bool {
	installedCatalogMu.RLock()
	c := installedCatalog
	installedCatalogMu.RUnlock()
	facts, ok := c.For(provider, modelName)
	return ok && facts.NoTemperature
}

// catalogContextWindow reports the window the installed catalog states, or 0.
func catalogContextWindow(provider, modelName string) int {
	installedCatalogMu.RLock()
	c := installedCatalog
	installedCatalogMu.RUnlock()
	facts, ok := c.For(provider, modelName)
	if !ok || facts.Context <= 0 {
		return 0
	}
	return facts.Context
}

// modelsDevDocument is only the part of the upstream JSON we keep.
type modelsDevDocument map[string]struct {
	Models map[string]struct {
		Cost *struct {
			Input      float64 `json:"input"`
			Output     float64 `json:"output"`
			CacheRead  float64 `json:"cache_read"`
			CacheWrite float64 `json:"cache_write"`
		} `json:"cost"`
		Limit *struct {
			Context int `json:"context"`
			Output  int `json:"output"`
		} `json:"limit"`
		ToolCall         bool  `json:"tool_call"`
		Reasoning        bool  `json:"reasoning"`
		Temperature      *bool `json:"temperature"`
		ReasoningOptions []struct {
			Type   string   `json:"type"`
			Values []string `json:"values"`
		} `json:"reasoning_options"`
		Modalities *struct {
			Input  []string `json:"input"`
			Output []string `json:"output"`
		} `json:"modalities"`
		ReleaseDate string `json:"release_date"`
	} `json:"models"`
}

// distill reduces the upstream document to the table Aetox stores.
//
// Split out from the fetch so the parsing is testable against a recorded
// payload without a network round trip, the same way the provider catalog's
// discovery parsers are.
func distill(raw []byte, now time.Time) (*ModelCatalog, error) {
	var doc modelsDevDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("models.dev response parse failed: %w", err)
	}
	catalog := &ModelCatalog{Fetched: now, Source: modelsDevAPI, Models: map[string]ModelFacts{}}
	for provider, entry := range doc {
		for modelID, m := range entry.Models {
			facts := ModelFacts{}
			if m.Cost != nil {
				facts.Price = ModelPrice{
					Input:      m.Cost.Input,
					Output:     m.Cost.Output,
					CacheRead:  m.Cost.CacheRead,
					CacheWrite: m.Cost.CacheWrite,
				}
			}
			if m.Limit != nil {
				facts.Context, facts.MaxOutput = m.Limit.Context, m.Limit.Output
			}
			facts.ToolCall, facts.Released = m.ToolCall, m.ReleaseDate
			facts.Reasoning = m.Reasoning
			facts.NoTemperature = m.Temperature != nil && !*m.Temperature
			for _, opt := range m.ReasoningOptions {
				switch opt.Type {
				case "toggle":
					facts.ReasoningToggle = true
				case "effort":
					for _, v := range opt.Values {
						if knownThinkingLevel(v) {
							facts.ReasoningLevels = append(facts.ReasoningLevels, v)
						}
					}
				}
				// "budget_tokens" is read and dropped: no runtime here can put
				// a token budget on the wire, so storing it would be storing a
				// control nothing can draw.
			}
			if m.Modalities != nil {
				facts.Input = knownModalities(m.Modalities.Input)
				facts.Output = knownModalities(m.Modalities.Output)
			}
			// A row stating neither a price nor a window says nothing worth
			// storing, and keeping it would make "known to the catalog" mean
			// less than it should.
			if !facts.Price.Priced() && facts.Context <= 0 {
				continue
			}
			catalog.Models[strings.ToLower(provider)+"/"+strings.ToLower(modelID)] = facts
		}
	}
	if len(catalog.Models) == 0 {
		return nil, fmt.Errorf("models.dev returned nothing usable")
	}
	return catalog, nil
}

// LoadModelCatalog reads the cached table. A missing file is not an error — it
// is a fresh install, or one that has never been online since this shipped.
func LoadModelCatalog(dataRoot string) (*ModelCatalog, error) {
	raw, err := os.ReadFile(filepath.Join(dataRoot, modelCatalogFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var catalog ModelCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		// A corrupt cache is not worth failing over: the next refresh replaces
		// it, and until then the caller falls back rather than breaking.
		return nil, nil
	}
	return &catalog, nil
}

// SaveModelCatalog writes the distilled table, atomically so an interrupted
// write cannot leave a half-file the next start reads as corrupt.
func SaveModelCatalog(dataRoot string, catalog *ModelCatalog) error {
	if catalog == nil {
		return nil
	}
	payload, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dataRoot, modelCatalogFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// FetchModelCatalog downloads the catalog and distills it. It does not write
// anything — the caller decides whether a refresh is worth persisting, which
// keeps this callable from a test without a temp directory.
//
// endpoint is overridable for tests only; empty means the real one.
func FetchModelCatalog(ctx context.Context, endpoint string) (*ModelCatalog, error) {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = modelsDevAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// The shared model client: same retry and timeout behaviour every other
	// outbound call in this package gets, rather than a second HTTP policy.
	resp, err := newModelHTTPClient(2*time.Minute, endpoint).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("models.dev returned status %d", resp.StatusCode)
	}
	// Bounded: this is a document off the public internet, and an unbounded
	// ReadAll on one is a memory limit set by whoever is serving it.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	return distill(raw, time.Now())
}

// InstallCachedCatalog reads the cached table off disk and installs it, so that
// everything asking what a model can do has an answer before the first turn.
//
// It exists because that was not true and nobody noticed until the CLI went
// red. Capabilities are resolved from this catalog now — thinking depths,
// vision, documents, tool calling — and with none installed every one of them
// is "unknown", which is the honest answer and the wrong one to ship: the
// desktop happened to install a catalog only when its usage panel ran, and the
// CLI never installed one at all, so a user's thinking picker existed or did
// not depending on which screen they had opened.
//
// Deliberately silent and networkless. A missing cache is a first run, not a
// failure, and the fetch belongs to RefreshModelCatalog on its own schedule —
// startup must not wait on models.dev.
//
// One function rather than four lines at each entry point, because two entry
// points spelling the same startup step differently is how they drift.
func InstallCachedCatalog(dataRoot string) {
	if strings.TrimSpace(dataRoot) == "" {
		return
	}
	catalog, err := LoadModelCatalog(dataRoot)
	if err != nil || catalog == nil || len(catalog.Models) == 0 {
		return
	}
	SetModelCatalog(catalog)
}

// RefreshModelCatalog fetches, stores, and installs, returning what is now
// current.
//
// Failure is deliberately soft: it returns whatever was already cached, because
// an offline laptop should keep yesterday's numbers rather than lose them. The
// error comes back too, so a caller that wants to say "could not refresh" can.
func RefreshModelCatalog(ctx context.Context, dataRoot string) (*ModelCatalog, error) {
	fresh, err := FetchModelCatalog(ctx, "")
	if err != nil {
		cached, _ := LoadModelCatalog(dataRoot)
		SetModelCatalog(cached)
		return cached, err
	}
	SetModelCatalog(fresh)
	if saveErr := SaveModelCatalog(dataRoot, fresh); saveErr != nil {
		// The table is good even if the disk is not; use it for this run.
		return fresh, saveErr
	}
	return fresh, nil
}
