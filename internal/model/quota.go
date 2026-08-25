package model

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/provider"
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
// alongside a Codex turn. Each window states three separate things: how much
// of it is spent, how long the window runs, and how long until it refills.
//
// The window's name comes from the length the backend states, never from which
// header slot it arrived in. The first version of this hardcoded primary="5h"
// and secondary="week" — true about the plans in force the day it was written,
// and still printed word for word after those plans changed underneath it. A
// row could read "this 5 hours · resets in 26 days" and nothing in the code
// was in a position to notice, because the only field that could have said
// otherwise, window-minutes, was being spent as a reset time. A header slot is
// an address. It is not a fact about the account.
//
// None of this is documented. Every field is optional and every parse failure
// is silent, so the day OpenAI renames a header the quota row goes blank
// instead of the app breaking — a missing number is a state the UI already
// draws, and a wrong one is not.
func readCodexQuotas(h http.Header, now time.Time) []Quota {
	out := make([]Quota, 0, 2)
	for _, family := range []string{"primary", "secondary"} {
		used, ok := parsePercent(h.Get("x-codex-" + family + "-used-percent"))
		if !ok {
			continue
		}
		minutes, _ := parseFloatHeader(h.Get("x-codex-" + family + "-window-minutes"))
		span := time.Duration(minutes * float64(time.Minute))
		q := Quota{
			Window:           windowName(span),
			RemainingPercent: clampPercent(100 - used),
			ObservedAt:       now,
		}
		if secs, ok := parseFloatHeader(h.Get("x-codex-" + family + "-reset-after-seconds")); ok && secs > 0 {
			// A window cannot refill later than its own length. When it says
			// otherwise the two halves are describing different things and
			// there is no way to tell from here which half is the stale one,
			// so the percentage is kept and the reset is dropped. Half a row
			// is what the missing-reset case already draws.
			if until := time.Duration(secs * float64(time.Second)); span <= 0 || until <= span {
				q.ResetAt = now.Add(until)
			} else {
				// The one place worth a log line. This family is undocumented,
				// its values are never recorded anywhere, and the last time it
				// disagreed with itself the only evidence available was a
				// screenshot of the row it produced. Lengths and durations
				// only; nothing in x-codex-* identifies an account.
				debuglog.Msg("quota: codex %s says it is %v long and refills in %v; keeping the percentage, dropping the reset",
					family, span, until)
			}
		}
		// A percentage with neither a name nor a reset is not a window, and
		// drawing it is worse than drawing nothing.
		//
		// Measured against the live backend on 2026-08-20, on a free plan with
		// the monthly allowance spent. Two families came back:
		//
		//	month  0% remaining, resets 2026-09-11
		//	other  100% remaining, no reset stated
		//
		// The first is exactly what OpenAI's own usage panel showed that
		// account, to the day. The second had no window-minutes to be named by
		// and no reset-after-seconds to count down, so it reached the card as a
		// full green bar labelled "this window" sitting beside an exhausted one
		// — reading, to the person who owns that account, as capacity they did
		// not have. OpenAI's client does not show it at all.
		//
		// Deliberately narrow: a window that states a length keeps its row even
		// with no reset (half an answer is still an answer, and the comment on
		// ResetAt says so), and one that states a reset keeps its row even
		// under the vague name. Only the pair together — nothing to call it,
		// nothing to count toward — leaves a number that cannot be read, and
		// those are the ones that stop here.
		if q.Window == windowUnnamed && !q.HasReset() {
			continue
		}
		out = append(out, q)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// windowName turns a stated window length into the stable key the UI
// translates. Only a length with a name in the UI's vocabulary gets one;
// everything else — including a provider that stated no length at all — is
// "other", which draws as "this window" beside the number. That is vaguer
// than "this week" and, unlike it, cannot be wrong.
func windowName(d time.Duration) string {
	for _, known := range []struct {
		span time.Duration
		name string
	}{
		{time.Minute, "minute"},
		{time.Hour, "hour"},
		{5 * time.Hour, "5h"},
		{24 * time.Hour, "day"},
		{7 * 24 * time.Hour, "week"},
		{30 * 24 * time.Hour, "month"},
	} {
		// A tenth either way. The backends round their own minutes, and a
		// month is a fixed number of days in no calendar but this one.
		if d >= known.span*9/10 && d <= known.span*11/10 {
			return known.name
		}
	}
	return windowUnnamed
}

// windowUnnamed is what a length with no name in the UI's vocabulary gets. The
// UI draws it as "this window", which is vaguer than "this week" and, unlike
// it, cannot be wrong.
const windowUnnamed = "other"

// readAnthropicQuotas prefers the unified window a Pro/Max sign-in reports,
// because that is the one actually binding on such an account. API keys do not
// send it and get the separate token and request windows instead.
//
// The unified family states a reset instant and never a length, so the row is
// named for whose limit it is rather than for how long it runs. It used to say
// "this week", which was a guess about a plan rather than anything the header
// carried — the same guess that went stale on Codex.
func readAnthropicQuotas(h http.Header, now time.Time) []Quota {
	if q, ok := anthropicWindow(h, "unified", "plan", now); ok {
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
		// The minute is OpenAI's, and most hosts that copied the header names
		// copied the minute with them — but not all, and several of these
		// providers meter by the day. This cannot confirm the minute; it can
		// only catch the window that disproves it, because a window still
		// refilling an hour from now was never a minute long. The rest keep
		// the assumption they have always had, unproven, as §108 left it.
		if !q.ResetAt.IsZero() && q.ResetAt.Sub(now) > 90*time.Second {
			q.Window = "other"
		}
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
