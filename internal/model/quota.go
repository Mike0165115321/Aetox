package model

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/provider"
)

// Quota is how much of one rate-limit window is left.
//
// Unlike a balance, a quota is never fetched: providers state it in the
// headers of the turns Aetox was going to run anyway, so this arrives as a
// side effect of chatting and is only ever as fresh as the last reply. The UI
// has to say so — see ObservedAt.
type Quota struct {
	// Window is a stable key the UI translates, never a sentence: "5h",
	// "week", "minute", "day", "key". It comes from the provider's own
	// dialect rather than a constant, because the windows genuinely differ —
	// Groq answers per minute and Codex per week, and one hardcoded label
	// would be wrong for whichever one it was not written for.
	Window string `json:"window"`

	// RemainingPercent is what is left, 0–100. Stored as remaining rather
	// than used because that is the direction the bar and the sentence both
	// read; providers that report the used half are converted on the way in.
	RemainingPercent float64 `json:"remainingPercent"`

	// ResetAt is when the window refills. Zero when the provider stated a
	// remaining amount but not a reset — half an answer is still worth
	// showing, so this is not a reason to drop the quota.
	ResetAt time.Time `json:"resetAt"`

	// ObservedAt is when the response carrying this was received.
	ObservedAt time.Time `json:"observedAt"`
}

// HasReset reports whether ResetAt was actually stated.
func (q Quota) HasReset() bool { return !q.ResetAt.IsZero() }

// quotaObserver receives every window a live response states. It is a package
// global rather than a field on each provider because the clients are built
// per turn and thrown away, while the thing that wants to remember — the
// settings panel — outlives all of them.
var (
	quotaObserverMu sync.RWMutex
	quotaObserver   func(providerName string, quotas []Quota)
)

// SetQuotaObserver installs the sink that remembers quotas across turns.
// Passing nil detaches it. Safe to call at any time.
func SetQuotaObserver(f func(providerName string, quotas []Quota)) {
	quotaObserverMu.Lock()
	defer quotaObserverMu.Unlock()
	quotaObserver = f
}

// NoteQuotas reads whatever windows a response carries and hands them to the
// observer. Providers call it on every response they get back, success or not:
// a 429 states the same headers and is exactly when the number matters most.
//
// It reports nothing and fails at nothing — a provider that says nothing about
// its limits simply leaves the last known value alone.
func NoteQuotas(providerName string, resp *http.Response) {
	source := provider.QuotaSourceFor(providerName)
	if source == provider.QuotaNone || resp == nil {
		return
	}
	// An empty result is still reported. "This provider answered and stated no
	// limits" and "this provider has never answered" look identical from a nil
	// slice, and the UI draws them differently — one says it does not report a
	// quota, the other says the number is not known yet.
	quotas := ReadQuotas(resp, source, time.Now())
	if len(quotas) == 0 {
		// The x-codex-* family is undocumented, so "no quota" is ambiguous:
		// either the account genuinely has no window, or the header was
		// renamed and the parser is looking for a name that no longer exists.
		// Recording which headers did arrive turns the next diagnosis into
		// reading a log instead of guessing again. Names only — a header value
		// can be a token.
		debuglog.Msg("quota: %s stated none; headers present: %s",
			providerName, strings.Join(headerNames(resp), ", "))
	}
	quotaObserverMu.RLock()
	observer := quotaObserver
	quotaObserverMu.RUnlock()
	if observer != nil {
		observer(provider.Normalize(providerName), quotas)
	}
}

// ReadQuotas pulls every rate-limit window a response states, in the dialect
// the provider speaks. Returns nil when the provider says nothing — which is
// a normal outcome, not an error: plenty of OpenAI-compatible hosts send no
// rate-limit headers at all, and the UI shows "does not report one" rather
// than inventing a number.
//
// now is passed in rather than read so the durations stay testable.
func ReadQuotas(resp *http.Response, source provider.QuotaSource, now time.Time) []Quota {
	if resp == nil {
		return nil
	}
	switch source {
	case provider.QuotaCodex:
		return readCodexQuotas(resp.Header, now)
	case provider.QuotaAnthropic:
		return readAnthropicQuotas(resp.Header, now)
	case provider.QuotaOpenAIStd:
		return readOpenAIStdQuotas(resp.Header, now)
	default:
		return nil
	}
}

// readCodexQuotas reads the x-codex-* family the ChatGPT backend sends
// alongside a Codex turn: a 5-hour window ("primary") and a weekly one
// ("secondary"), each stated as the percentage already spent.
//
// None of this is documented. Every field is optional and every parse failure
// is silent, so the day OpenAI renames a header the quota row goes blank
// instead of the app breaking — a missing number is a state the UI already
// draws, and a wrong one is not.
func readCodexQuotas(h http.Header, now time.Time) []Quota {
	windows := []struct{ header, window string }{
		{"primary", "5h"},
		{"secondary", "week"},
	}
	out := make([]Quota, 0, len(windows))
	for _, w := range windows {
		used, ok := parsePercent(h.Get("x-codex-" + w.header + "-used-percent"))
		if !ok {
			continue
		}
		q := Quota{
			Window:           w.window,
			RemainingPercent: clampPercent(100 - used),
			ObservedAt:       now,
		}
		// Two spellings seen in the wild for the same fact; whichever is
		// present wins, and neither being present just means no reset time.
		if secs, err := strconv.ParseFloat(strings.TrimSpace(h.Get("x-codex-"+w.header+"-reset-after-seconds")), 64); err == nil && secs > 0 {
			q.ResetAt = now.Add(time.Duration(secs * float64(time.Second)))
		} else if mins, err := strconv.ParseFloat(strings.TrimSpace(h.Get("x-codex-"+w.header+"-window-minutes")), 64); err == nil && mins > 0 {
			q.ResetAt = now.Add(time.Duration(mins * float64(time.Minute)))
		}
		out = append(out, q)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// readAnthropicQuotas prefers the unified window a Pro/Max sign-in reports,
// because that is the one actually binding on such an account. API keys do not
// send it and get the separate token and request windows instead.
func readAnthropicQuotas(h http.Header, now time.Time) []Quota {
	if q, ok := anthropicWindow(h, "unified", "week", now); ok {
		return []Quota{q}
	}
	out := make([]Quota, 0, 2)
	if q, ok := anthropicWindow(h, "tokens", "minute", now); ok {
		out = append(out, q)
	}
	if q, ok := anthropicWindow(h, "requests", "minute", now); ok {
		out = append(out, q)
	}
	if len(out) == 0 {
		return nil
	}
	// Both windows are per-minute and only the tighter one can actually stop
	// a turn; showing two near-identical bars would be noise.
	return []Quota{tightest(out)}
}

func anthropicWindow(h http.Header, family, window string, now time.Time) (Quota, bool) {
	remaining, okR := parseFloatHeader(h.Get("anthropic-ratelimit-" + family + "-remaining"))
	limit, okL := parseFloatHeader(h.Get("anthropic-ratelimit-" + family + "-limit"))
	if !okR || !okL || limit <= 0 {
		return Quota{}, false
	}
	q := Quota{
		Window:           window,
		RemainingPercent: clampPercent(remaining / limit * 100),
		ObservedAt:       now,
	}
	q.ResetAt = parseResetInstant(h.Get("anthropic-ratelimit-"+family+"-reset"), now)
	return q, true
}

// readOpenAIStdQuotas reads the x-ratelimit-* family most OpenAI-compatible
// hosts send. These windows are short — usually a minute — which is exactly
// why Window is carried as data: labelling this "this week" would be wrong on
// every provider that lands here.
func readOpenAIStdQuotas(h http.Header, now time.Time) []Quota {
	out := make([]Quota, 0, 2)
	for _, family := range []string{"tokens", "requests"} {
		remaining, okR := parseFloatHeader(h.Get("x-ratelimit-remaining-" + family))
		limit, okL := parseFloatHeader(h.Get("x-ratelimit-limit-" + family))
		if !okR || !okL || limit <= 0 {
			continue
		}
		q := Quota{
			Window:           "minute",
			RemainingPercent: clampPercent(remaining / limit * 100),
			ObservedAt:       now,
		}
		q.ResetAt = parseResetInstant(h.Get("x-ratelimit-reset-"+family), now)
		out = append(out, q)
	}
	if len(out) == 0 {
		return nil
	}
	return []Quota{tightest(out)}
}

// tightest picks the window closest to running out — the one that will
// actually stop the next turn.
func tightest(quotas []Quota) Quota {
	best := quotas[0]
	for _, q := range quotas[1:] {
		if q.RemainingPercent < best.RemainingPercent {
			best = q
		}
	}
	return best
}

// parseResetInstant accepts every spelling of "when does this refill" seen
// across the three dialects, because the format is not ours to assume:
// an RFC3339 timestamp, Unix seconds, a bare number of seconds from now, or
// Go-ish duration text like "6m0s" / "1h30m" / "250ms". Returns the zero time
// when it is none of those, which the UI reads as "no reset stated".
func parseResetInstant(value string, now time.Time) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if when, err := time.Parse(time.RFC3339, value); err == nil {
		return when
	}
	if d, err := time.ParseDuration(value); err == nil && d > 0 {
		return now.Add(d)
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}
	}
	// A plain number is ambiguous: Anthropic sends a Unix timestamp here and
	// the OpenAI-compatible hosts send seconds-from-now. Anything large enough
	// to be a real epoch (past 2001) is read as one; the rest is a duration.
	if seconds > 1_000_000_000 {
		return time.Unix(int64(seconds), 0)
	}
	return now.Add(time.Duration(seconds * float64(time.Second)))
}

func parseFloatHeader(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parsePercent accepts "42", "42.5" and "42%" alike.
func parsePercent(value string) (float64, bool) {
	return parseFloatHeader(strings.TrimSuffix(strings.TrimSpace(value), "%"))
}

// headerNames lists the response header names, sorted, with no values: the
// point is to see what the provider is calling its rate-limit fields, and a
// value here could be a session token in a log file.
func headerNames(resp *http.Response) []string {
	out := make([]string, 0, len(resp.Header))
	for name := range resp.Header {
		out = append(out, strings.ToLower(name))
	}
	sort.Strings(out)
	return out
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
