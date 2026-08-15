package model

import (
	"net/http"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/provider"
)

func responseWith(headers map[string]string) *http.Response {
	resp := &http.Response{Header: http.Header{}}
	for k, v := range headers {
		resp.Header.Set(k, v)
	}
	return resp
}

var quotaNow = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

// Codex is the case the whole feature was asked for: two windows at once, and
// the weekly one running out while the five-hour one still looks fine. Both
// names come from the stated length, not from the header slot.
func TestCodexQuotasReadBothWindows(t *testing.T) {
	resp := responseWith(map[string]string{
		"x-codex-primary-used-percent":          "34",
		"x-codex-primary-window-minutes":        "300",
		"x-codex-primary-reset-after-seconds":   "7200",
		"x-codex-secondary-used-percent":        "88.0",
		"x-codex-secondary-window-minutes":      "10080",
		"x-codex-secondary-reset-after-seconds": "259200",
	})
	got := ReadQuotas(resp, provider.QuotaCodex, quotaNow)
	if len(got) != 2 {
		t.Fatalf("got %d windows; want 2", len(got))
	}
	if got[0].Window != "5h" || got[0].RemainingPercent != 66 {
		t.Errorf("primary = %+v; want 5h at 66%% remaining", got[0])
	}
	if got[1].Window != "week" || got[1].RemainingPercent != 12 {
		t.Errorf("secondary = %+v; want week at 12%% remaining", got[1])
	}
	if want := quotaNow.Add(2 * time.Hour); !got[0].ResetAt.Equal(want) {
		t.Errorf("primary reset = %v; want %v", got[0].ResetAt, want)
	}
}

// The bug this was all found through: the primary slot carrying a window that
// is no longer five hours. The row used to say "this 5 hours · resets in 26
// days" because the name was a constant and only the number came from the
// wire. Whatever the length turns out to be, the label may not outrank it.
func TestCodexWindowIsNamedByItsStatedLength(t *testing.T) {
	got := ReadQuotas(responseWith(map[string]string{
		"x-codex-primary-used-percent":        "94",
		"x-codex-primary-window-minutes":      "37440", // 26 days: no name for this
		"x-codex-primary-reset-after-seconds": "2246400",
	}), provider.QuotaCodex, quotaNow)
	if len(got) != 1 {
		t.Fatalf("got %d windows; want 1", len(got))
	}
	if got[0].Window != "other" {
		t.Errorf("Window = %q; want other — a 26-day window is not five hours and has no name here", got[0].Window)
	}
	if got[0].RemainingPercent != 6 {
		t.Errorf("remaining = %v%%; want 6", got[0].RemainingPercent)
	}
	// A month-long window in the primary slot is still named for its length.
	got = ReadQuotas(responseWith(map[string]string{
		"x-codex-primary-used-percent":   "10",
		"x-codex-primary-window-minutes": "43200",
	}), provider.QuotaCodex, quotaNow)
	if len(got) != 1 || got[0].Window != "month" {
		t.Errorf("got %+v; want a single month window", got)
	}
}

// A window that says it refills later than it is long is describing two
// different things, and there is no way to tell from here which half is stale.
// The percentage survives; the impossible half does not reach the screen.
func TestCodexResetBeyondItsOwnWindowIsDropped(t *testing.T) {
	got := ReadQuotas(responseWith(map[string]string{
		"x-codex-primary-used-percent":        "50",
		"x-codex-primary-window-minutes":      "300",
		"x-codex-primary-reset-after-seconds": "2246400", // 26 days into a 5-hour window
	}), provider.QuotaCodex, quotaNow)
	if len(got) != 1 {
		t.Fatalf("got %d windows; want 1", len(got))
	}
	if got[0].Window != "5h" || got[0].RemainingPercent != 50 {
		t.Errorf("got %+v; want the 5h window at 50%%", got[0])
	}
	if got[0].HasReset() {
		t.Errorf("ResetAt = %v; want none — 26 days is not a moment inside a 5-hour window", got[0].ResetAt)
	}
}

// window-minutes is how long the window runs. It was being read as how long
// until the window refills, which is a different fact and always the larger
// one: every quota with no reset header used to draw a reset a whole window
// away, and the two were never the same number.
func TestCodexWindowLengthIsNotAResetTime(t *testing.T) {
	got := ReadQuotas(responseWith(map[string]string{
		"x-codex-primary-used-percent":   "20",
		"x-codex-primary-window-minutes": "300",
	}), provider.QuotaCodex, quotaNow)
	if len(got) != 1 {
		t.Fatalf("got %d windows; want 1", len(got))
	}
	if got[0].HasReset() {
		t.Errorf("ResetAt = %v; want none — the backend stated a length, not a refill time", got[0].ResetAt)
	}
}

// The x-codex-* family is undocumented. When OpenAI renames it the row must go
// blank, not report a window that no longer exists.
func TestCodexQuotasVanishWhenHeadersDo(t *testing.T) {
	if got := ReadQuotas(responseWith(nil), provider.QuotaCodex, quotaNow); got != nil {
		t.Fatalf("got %+v; want nil when the headers are absent", got)
	}
}

// A used-percent with no reset header is still worth showing — half an answer
// beats none, so the quota survives with a zero ResetAt.
func TestCodexQuotaSurvivesMissingResetHeader(t *testing.T) {
	got := ReadQuotas(responseWith(map[string]string{
		"x-codex-primary-used-percent": "10",
	}), provider.QuotaCodex, quotaNow)
	if len(got) != 1 {
		t.Fatalf("got %d windows; want 1", len(got))
	}
	if got[0].HasReset() {
		t.Error("HasReset() = true; want false so the UI omits the reset clause")
	}
}

// A Pro/Max sign-in reports the window that actually binds it; the per-minute
// token and request counters are what an API key gets instead.
func TestAnthropicQuotaPrefersUnifiedWindow(t *testing.T) {
	got := ReadQuotas(responseWith(map[string]string{
		"anthropic-ratelimit-unified-remaining": "30",
		"anthropic-ratelimit-unified-limit":     "100",
		"anthropic-ratelimit-tokens-remaining":  "9000",
		"anthropic-ratelimit-tokens-limit":      "10000",
	}), provider.QuotaAnthropic, quotaNow)
	if len(got) != 1 {
		t.Fatalf("got %d windows; want 1", len(got))
	}
	// Named for whose limit it is. The family states a reset instant and never
	// a length, so "this week" was a guess about a plan, not a reading.
	if got[0].Window != "plan" || got[0].RemainingPercent != 30 {
		t.Errorf("got %+v; want the unified plan window at 30%%", got[0])
	}
}

// Two per-minute bars side by side is noise; only the tighter one can stop the
// next turn.
func TestAnthropicQuotaKeepsTheTighterWindow(t *testing.T) {
	got := ReadQuotas(responseWith(map[string]string{
		"anthropic-ratelimit-tokens-remaining":   "9000",
		"anthropic-ratelimit-tokens-limit":       "10000",
		"anthropic-ratelimit-requests-remaining": "1",
		"anthropic-ratelimit-requests-limit":     "50",
	}), provider.QuotaAnthropic, quotaNow)
	if len(got) != 1 {
		t.Fatalf("got %d windows; want 1", len(got))
	}
	if got[0].RemainingPercent != 2 {
		t.Errorf("remaining = %v%%; want the 2%% request window, not the roomy token one", got[0].RemainingPercent)
	}
}

func TestOpenAIStdQuotaReadsRateLimitHeaders(t *testing.T) {
	got := ReadQuotas(responseWith(map[string]string{
		"x-ratelimit-remaining-tokens": "7100",
		"x-ratelimit-limit-tokens":     "10000",
		"x-ratelimit-reset-tokens":     "43s",
	}), provider.QuotaOpenAIStd, quotaNow)
	if len(got) != 1 {
		t.Fatalf("got %d windows; want 1", len(got))
	}
	// The window is a minute here. Labelling every quota "this week" would be
	// wrong on every provider that lands in this dialect.
	if got[0].Window != "minute" || got[0].RemainingPercent != 71 {
		t.Errorf("got %+v; want minute at 71%%", got[0])
	}
	if want := quotaNow.Add(43 * time.Second); !got[0].ResetAt.Equal(want) {
		t.Errorf("reset = %v; want %v", got[0].ResetAt, want)
	}
}

// The minute in this dialect is OpenAI's and is inherited by every host that
// copied the header names, several of which meter by the day. Nothing here can
// confirm the minute — but a window still refilling two hours from now
// disproves it, and that much must reach the label.
func TestOpenAIStdWindowStopsClaimingAMinuteWhenTheResetSaysOtherwise(t *testing.T) {
	got := ReadQuotas(responseWith(map[string]string{
		"x-ratelimit-remaining-requests": "400",
		"x-ratelimit-limit-requests":     "1000",
		"x-ratelimit-reset-requests":     "2h",
	}), provider.QuotaOpenAIStd, quotaNow)
	if len(got) != 1 {
		t.Fatalf("got %d windows; want 1", len(got))
	}
	if got[0].Window != "other" {
		t.Errorf("Window = %q; want other — a window refilling in 2h is not a minute", got[0].Window)
	}
}

// A host that sends no rate-limit headers reports no quota. That is a normal
// outcome the UI draws as "does not report one" — not an error, and not a
// silent 100%.
func TestSilentHostReportsNoQuota(t *testing.T) {
	if got := ReadQuotas(responseWith(nil), provider.QuotaOpenAIStd, quotaNow); got != nil {
		t.Fatalf("got %+v; want nil", got)
	}
	if got := ReadQuotas(responseWith(map[string]string{
		"x-ratelimit-remaining-tokens": "7100",
	}), provider.QuotaOpenAIStd, quotaNow); got != nil {
		t.Fatalf("got %+v; want nil when the limit is missing and no percentage can be formed", got)
	}
}

// A limit of zero would divide by zero rather than mean anything.
func TestZeroLimitIsNotAQuota(t *testing.T) {
	if got := ReadQuotas(responseWith(map[string]string{
		"x-ratelimit-remaining-tokens": "0",
		"x-ratelimit-limit-tokens":     "0",
	}), provider.QuotaOpenAIStd, quotaNow); got != nil {
		t.Fatalf("got %+v; want nil", got)
	}
}

// The three dialects spell "when does this refill" four different ways, and
// the format is not ours to assume.
func TestParseResetInstantAcceptsEveryDialect(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  time.Time
	}{
		{"rfc3339", "2026-08-14T12:01:30Z", quotaNow.Add(90 * time.Second)},
		{"unix seconds", "1786708890", quotaNow.Add(90 * time.Second)},
		{"go duration", "1m30s", quotaNow.Add(90 * time.Second)},
		{"seconds from now", "90", quotaNow.Add(90 * time.Second)},
		{"milliseconds", "250ms", quotaNow.Add(250 * time.Millisecond)},
	} {
		got := parseResetInstant(tc.value, quotaNow)
		if !got.Equal(tc.want) {
			t.Errorf("%s: parseResetInstant(%q) = %v; want %v", tc.name, tc.value, got.UTC(), tc.want)
		}
	}
	for _, bad := range []string{"", "   ", "never", "0", "-5"} {
		if got := parseResetInstant(bad, quotaNow); !got.IsZero() {
			t.Errorf("parseResetInstant(%q) = %v; want the zero time", bad, got)
		}
	}
}

// A provider whose catalog entry says QuotaNone is not asked at all, even when
// the response happens to carry headers from some proxy in between.
func TestQuotaNoneReadsNothing(t *testing.T) {
	got := ReadQuotas(responseWith(map[string]string{
		"x-ratelimit-remaining-tokens": "10",
		"x-ratelimit-limit-tokens":     "100",
	}), provider.QuotaNone, quotaNow)
	if got != nil {
		t.Fatalf("got %+v; want nil", got)
	}
}
