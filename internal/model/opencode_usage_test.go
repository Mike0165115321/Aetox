package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The body is not a sample somebody captured. It is what the route builds, from
// packages/console/app/src/routes/zen/go/v1/usage.ts in opencode's own repo:
// formatUsage returns {status, percent, resetsAt} and the handler nests the
// three windows under "usage". Writing the test against the contract rather
// than against one observed call is why this can be trusted without a key.
func TestOpencodeGoUsageIsReadAsThreeWindows(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{
			"rolling":{"status":"ok","percent":12,"resetsAt":"2030-01-01T05:00:00.000Z"},
			"weekly":{"status":"ok","percent":40,"resetsAt":"2030-01-02T00:00:00.000Z"},
			"monthly":{"status":"ok","percent":0,"resetsAt":"2030-01-30T00:00:00.000Z"}}}`))
	}))
	defer srv.Close()

	got, err := FetchBalance(context.Background(), "opencode-go", srv.URL, "k")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotPath != "/usage" {
		t.Errorf("path = %q, want /usage beside the chat route, not off the origin", gotPath)
	}
	if gotAuth != "Bearer k" {
		t.Errorf("auth = %q, want a bearer token", gotAuth)
	}
	// A subscription has no figure, and inventing one out of a percentage is
	// exactly what this must not do.
	if got.HasAmount {
		t.Error("a subscription reported an amount; there is no money in this response")
	}
	if !got.Sufficient {
		t.Error("every window was ok and the card was told the key cannot spend")
	}

	// Shortest window first: the one about to bite is the one to read first,
	// and a map would have shuffled these between refreshes.
	wantWindows := []string{"5h", "week", "month"}
	if len(got.Quotas) != len(wantWindows) {
		t.Fatalf("quotas = %d, want %d", len(got.Quotas), len(wantWindows))
	}
	for i, want := range wantWindows {
		if got.Quotas[i].Window != want {
			t.Errorf("quota %d window = %q, want %q", i, got.Quotas[i].Window, want)
		}
	}
	// percent is the share USED; the bar reads what is LEFT.
	if got.Quotas[0].RemainingPercent != 88 {
		t.Errorf("rolling remaining = %v, want 88 (100 - 12 used)", got.Quotas[0].RemainingPercent)
	}
	if got.Quotas[2].RemainingPercent != 100 {
		t.Errorf("monthly remaining = %v, want 100", got.Quotas[2].RemainingPercent)
	}
	if !got.Quotas[0].HasReset() {
		t.Error("resetsAt was stated and did not survive parsing")
	}
}

// "rate-limited" is the plan saying this key cannot spend right now. The card
// reads Sufficient to say so, and a ceiling that is hit must reach it.
func TestOpencodeGoRateLimitedWindowMarksTheKeyUnusable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"usage":{
			"rolling":{"status":"rate-limited","percent":100,"resetsAt":"2030-01-01T05:00:00.000Z"},
			"weekly":{"status":"ok","percent":40,"resetsAt":"2030-01-02T00:00:00.000Z"},
			"monthly":{"status":"ok","percent":9,"resetsAt":"2030-01-30T00:00:00.000Z"}}}`))
	}))
	defer srv.Close()

	got, err := FetchBalance(context.Background(), "opencode-go", srv.URL, "k")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got.Sufficient {
		t.Error("a rate-limited window still reported the key as able to spend")
	}
	if got.Quotas[0].RemainingPercent != 0 {
		t.Errorf("rolling remaining = %v, want 0", got.Quotas[0].RemainingPercent)
	}
}

// The wallet side is a different row and a different answer. Every balance path
// under /zen/v1 is a 404 and the route folder holds only chat, messages, models
// and responses — so `opencode` must not reach for an endpoint at all.
func TestOpencodeWalletFetchesNothing(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	got, err := FetchBalance(context.Background(), "opencode", srv.URL, "k")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if called {
		t.Error("the wallet row knocked on an endpoint that does not exist")
	}
	if got.Kind != "web-only" {
		t.Errorf("kind = %q, want web-only — the figure is only on their dashboard", got.Kind)
	}
	if len(got.Quotas) != 0 {
		t.Error("the wallet row invented a quota")
	}
}

// A good key with no Go plan behind it gets 403 EntitlementError, and that
// sentence IS the answer — "subscribe to Go" is a different action from "your
// key is wrong". It used to reach the card wrapped in its JSON envelope.
func TestOpencodeGoWithoutASubscriptionSaysSo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"EntitlementError","message":"OpenCode Go subscription required."}}`))
	}))
	defer srv.Close()

	_, err := FetchBalance(context.Background(), "opencode-go", srv.URL, "k")
	if err == nil {
		t.Fatal("a 403 was swallowed")
	}
	if !strings.Contains(err.Error(), "OpenCode Go subscription required.") {
		t.Errorf("error = %q; want the provider's own sentence", err)
	}
	if strings.Contains(err.Error(), "EntitlementError") {
		t.Errorf("error = %q; the JSON envelope reached the card", err)
	}
}

// The thinking dial on a gateway, which is the case every other resolver in
// thinking_capabilities.go is allowed to answer with a prefix and this one is
// not: nine vendors down one base URL, so "does this model reason" is a fact
// about the model and nothing else.
//
// That the field is reasoning_effort at all is not inferred from a successful
// call. parseOpenAiVariant in opencode's own repo (routes/zen/util/variant.ts)
// reads body.reasoningEffort ?? body.reasoning_effort ?? body.reasoning?.effort.
func TestOpencodeThinkingIsAnsweredPerModel(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"opencode/claude-opus-5":  {Context: 200000, Reasoning: true, ToolCall: true},
		"opencode/gpt-5-nano":     {Context: 400000, Reasoning: false, ToolCall: true},
		"opencode-go/qwen3.8-max": {Context: 1000000, Reasoning: true, ToolCall: true},
	}})

	for _, tc := range []struct {
		provider, model string
		want            bool
		why             string
	}{
		{"opencode", "claude-opus-5", true, "the catalog says it reasons"},
		{"opencode", "gpt-5-nano", false, "the catalog says it does not"},
		{"opencode-go", "qwen3.8-max", true, "the Go row reads the same way"},
		{"opencode", "big-pickle", false, "an id no catalog has heard of must not get a menu"},
	} {
		caps := ResolveThinkingCapabilities(tc.provider, tc.model)
		if caps.Supported != tc.want {
			t.Errorf("%s/%s supported = %v, want %v (%s)", tc.provider, tc.model, caps.Supported, tc.want, tc.why)
		}
		if tc.want && caps.Runtime != ThinkingRuntimeReasoningEffort {
			t.Errorf("%s/%s runtime = %q, want the top-level reasoning_effort field",
				tc.provider, tc.model, caps.Runtime)
		}
	}
}

// A level the picker offers must reach the wire, and a model with no dial must
// put nothing there. This is the pairing the catalog comment warns about:
// providerReasoningCapability decides what the menu shows and the request path
// decides what is sent, and they used to be able to disagree.
func TestOpencodeEffortReachesTheWireOnlyWhenTheModelHasOne(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"opencode/thinks": {Context: 100000, Reasoning: true, ToolCall: true},
		"opencode/plain":  {Context: 100000, Reasoning: false, ToolCall: true},
	}})

	for _, tc := range []struct {
		model string
		want  string
	}{{"thinks", "high"}, {"plain", ""}} {
		var sent struct {
			ReasoningEffort string `json:"reasoning_effort"`
			Reasoning       any    `json:"reasoning"`
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &sent)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		}))

		p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider: "opencode", Model: tc.model, APIKey: "k", BaseURL: srv.URL})
		if err != nil {
			t.Fatalf("provider: %v", err)
		}
		if _, err := p.Complete(context.Background(), Request{
			Model:     tc.model,
			Messages:  []Message{{Role: RoleUser, Content: "hi"}},
			Reasoning: &ReasoningConfig{Effort: "high"},
		}); err != nil {
			t.Fatalf("%s: %v", tc.model, err)
		}
		srv.Close()

		if sent.ReasoningEffort != tc.want {
			t.Errorf("%s sent reasoning_effort=%q, want %q", tc.model, sent.ReasoningEffort, tc.want)
		}
		// The nested object is OpenRouter's dialect. Sending both would be two
		// spellings of one setting on a gateway that reads the flat one first.
		if sent.Reasoning != nil {
			t.Errorf("%s also sent a nested reasoning object: %v", tc.model, sent.Reasoning)
		}
	}
}
