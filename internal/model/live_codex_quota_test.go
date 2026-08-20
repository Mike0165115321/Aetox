package model

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// What windows does the ChatGPT backend actually report, and do they match what
// OpenAI's own client shows the same account?
//
// On 2026-08-20 Aetox drew two bars for a Codex account — "เดือนนี้ 0%" and a
// full green "ลิมิตช่วงนี้ 100%" — while OpenAI's own usage panel for that same
// account showed one row: "Monthly 0%, Sep 11". A second bar reading 100% next
// to an exhausted one tells the user they have capacity they do not have, and
// nothing in the app could say whether that row was a real window, a vestigial
// header, or a parse landing in the wrong place.
//
// This does not need quota to answer, which is the point: NoteQuotas is called
// before the status check precisely because a 429 states the same rate-limit
// headers as a 200 (see StreamComplete). So an exhausted plan is a perfectly
// good subject — it will refuse the turn and report its windows on the way out.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveCodexQuota -v -count=1
func TestLiveCodexQuotaReportsWhatTheProviderReports(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against the real ChatGPT backend")
	}
	// See the note in the reasoning test: the Codex CLI's auth.json is a
	// different client's credential and may be years stale; this asks about the
	// account Aetox itself runs on.
	t.Setenv("CODEX_HOME", t.TempDir())
	signInCodexLive(t)

	modelID := strings.TrimSpace(os.Getenv("AETOX_LIVE_MODEL"))
	if modelID == "" {
		modelID = "gpt-5.6-luna"
	}

	var (
		mu     sync.Mutex
		seen   []Quota
		called bool
	)
	SetQuotaObserver(func(provider string, quotas []Quota) {
		mu.Lock()
		defer mu.Unlock()
		called = true
		seen = append(seen, quotas...)
	})
	t.Cleanup(func() { SetQuotaObserver(nil) })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	p, err := NewProvider(ProviderOptions{Provider: "codex", Model: modelID, Timeout: 60 * time.Second})
	if err != nil {
		t.Fatalf("NewProvider(%s): %v", modelID, err)
	}
	// The answer is irrelevant and the error is expected on an exhausted plan.
	// Only the headers that came back with it are being read.
	_, reqErr := p.Complete(ctx, Request{
		Model:    modelID,
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if reqErr != nil {
		t.Logf("request refused (fine, the headers are what this reads): %v", reqErr)
	}

	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Fatal("NoteQuotas never fired — no response reached the observer at all")
	}
	if len(seen) == 0 {
		t.Fatal("the backend stated no rate-limit windows; the card is right to show nothing")
	}
	now := time.Now()
	for i, q := range seen {
		reset := "not stated"
		if q.HasReset() {
			reset = q.ResetAt.Format("2006-01-02 15:04") +
				" (in " + q.ResetAt.Sub(now).Round(time.Hour).String() + ")"
		}
		t.Logf("window %d: name=%q remaining=%.1f%% resets=%s", i+1, q.Window, q.RemainingPercent, reset)
	}
	t.Logf("windows reported: %d", len(seen))
}
