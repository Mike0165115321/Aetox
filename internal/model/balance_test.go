package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The figures DeepSeek sends are JSON strings. Decoding them into a float64
// field fails silently to zero, which would tell a funded account it is broke.
func TestDeepSeekBalanceReadsStringFigures(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"is_available":true,"balance_infos":[
			{"currency":"USD","total_balance":"12.40","granted_balance":"8.00","topped_up_balance":"4.40"}]}`))
	}))
	defer srv.Close()

	got, err := FetchBalance(context.Background(), "deepseek", srv.URL+"/anthropic/v1", "k")
	if err != nil {
		t.Fatalf("FetchBalance: %v", err)
	}
	// The balance lives at the root, not under the configured chat path.
	if gotPath != "/user/balance" {
		t.Errorf("path = %q; want /user/balance", gotPath)
	}
	if !got.HasAmount || got.Amount != 12.40 {
		t.Errorf("amount = %v (hasAmount %v); want 12.40", got.Amount, got.HasAmount)
	}
	if got.Currency != "USD" {
		t.Errorf("currency = %q; want USD", got.Currency)
	}
	if len(got.Parts) != 2 || got.Parts[0].Label != "granted" || got.Parts[0].Amount != 8.00 {
		t.Errorf("parts = %+v; want granted 8.00 then toppedUp 4.40", got.Parts)
	}
}

// One account can hold both currencies. Aetox shows dollars, so the USD entry
// wins no matter which order the array arrives in.
func TestDeepSeekBalancePrefersUSD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"is_available":true,"balance_infos":[
			{"currency":"CNY","total_balance":"88.00","granted_balance":"0","topped_up_balance":"88.00"},
			{"currency":"USD","total_balance":"12.40","granted_balance":"0","topped_up_balance":"12.40"}]}`))
	}))
	defer srv.Close()

	got, _ := FetchBalance(context.Background(), "deepseek", srv.URL, "k")
	if got.Currency != "USD" || got.Amount != 12.40 {
		t.Fatalf("got %s %v; want USD 12.40", got.Currency, got.Amount)
	}
}

// An account with no USD at all is shown in what it actually holds. Converting
// would mean printing an exchange rate we sourced ourselves.
func TestDeepSeekBalanceKeepsForeignCurrencyRatherThanConverting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"is_available":true,"balance_infos":[
			{"currency":"CNY","total_balance":"88.00","granted_balance":"0","topped_up_balance":"88.00"}]}`))
	}))
	defer srv.Close()

	got, _ := FetchBalance(context.Background(), "deepseek", srv.URL, "k")
	if got.Currency != "CNY" || got.Amount != 88.00 {
		t.Fatalf("got %s %v; want CNY 88.00 unconverted", got.Currency, got.Amount)
	}
}

func TestDeepSeekBalanceReportsInsufficient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"is_available":false,"balance_infos":[
			{"currency":"USD","total_balance":"0.00","granted_balance":"0","topped_up_balance":"0"}]}`))
	}))
	defer srv.Close()

	got, err := FetchBalance(context.Background(), "deepseek", srv.URL, "k")
	if err != nil {
		t.Fatalf("FetchBalance: %v", err)
	}
	if got.Sufficient {
		t.Fatal("sufficient = true; want false when the provider says the account cannot call")
	}
}

// Moonshot answers HTTP 200 with a non-zero code for application-level
// failures. Trusting the status line alone would show whatever zero-valued
// struct came back as a real balance.
func TestKimiBalanceRejectsNonZeroCodeOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":40001,"scode":"0x9c4","status":false,"data":{}}`))
	}))
	defer srv.Close()

	got, err := FetchBalance(context.Background(), "kimi", srv.URL+"/v1", "k")
	if err == nil {
		t.Fatal("want an error for code 40001 on HTTP 200")
	}
	if got.HasAmount {
		t.Error("hasAmount = true on a failed read; want no figure at all")
	}
}

func TestKimiBalanceReadsAvailableBalance(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"code":0,"status":true,"scode":"0x0","data":
			{"available_balance":49.58894,"voucher_balance":46.58893,"cash_balance":3.00001}}`))
	}))
	defer srv.Close()

	got, err := FetchBalance(context.Background(), "kimi", srv.URL+"/v1", "k")
	if err != nil {
		t.Fatalf("FetchBalance: %v", err)
	}
	if gotPath != "/v1/users/me/balance" {
		t.Errorf("path = %q; want /v1/users/me/balance", gotPath)
	}
	if !got.HasAmount || got.Amount != 49.58894 {
		t.Errorf("amount = %v; want 49.58894", got.Amount)
	}
	if len(got.Parts) != 2 {
		t.Errorf("parts = %+v; want voucher and cash", got.Parts)
	}
}

// A capped key reports both halves in one response — the only provider whose
// quota does not have to wait for a real turn.
func TestOpenRouterBalanceReadsKeyEndpointAndQuota(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"data":{"limit":10,"limit_remaining":8.25,"usage":1.75,"is_free_tier":false}}`))
	}))
	defer srv.Close()

	got, err := FetchBalance(context.Background(), "openrouter", srv.URL+"/api/v1", "k")
	if err != nil {
		t.Fatalf("FetchBalance: %v", err)
	}
	// /credits answers 403 for anything but a management key; /key is the one
	// a normal user's key can actually read.
	if gotPath != "/api/v1/key" {
		t.Errorf("path = %q; want /api/v1/key", gotPath)
	}
	if got.Amount != 8.25 {
		t.Errorf("amount = %v; want 8.25", got.Amount)
	}
	if got.Quota == nil {
		t.Fatal("quota = nil; want the key window alongside the credits")
	}
	if got.Quota.RemainingPercent < 82 || got.Quota.RemainingPercent > 83 {
		t.Errorf("remaining = %v%%; want ~82.5", got.Quota.RemainingPercent)
	}
}

// limit: null means the key has no cap — not a cap of zero. Reading it as a
// number would report an uncapped key as empty.
func TestOpenRouterUncappedKeyHasNoAmount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"limit":null,"limit_remaining":null,"usage":1.75,"is_free_tier":false}}`))
	}))
	defer srv.Close()

	got, err := FetchBalance(context.Background(), "openrouter", srv.URL+"/api/v1", "k")
	if err != nil {
		t.Fatalf("FetchBalance: %v", err)
	}
	if got.HasAmount {
		t.Error("hasAmount = true; an uncapped key has no remaining figure to show")
	}
	if !got.Sufficient {
		t.Error("sufficient = false; an uncapped key is usable")
	}
	if len(got.Parts) != 1 || got.Parts[0].Label != "used" {
		t.Errorf("parts = %+v; want the usage figure, the honest half we have", got.Parts)
	}
}

// A local runtime has no wallet. Knocking on localhost for a balance path
// would spend a timeout to learn what the catalog already knows.
func TestLocalAndSubscriptionProvidersNeverCallTheNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL.Path)
	}))
	defer srv.Close()

	for _, tc := range []struct{ name, wantKind string }{
		{"ollama", "free"},
		{"lmstudio", "free"},
		{"codex", "subscription"},
		{"anthropic", "web-only"},
	} {
		got, err := FetchBalance(context.Background(), tc.name, srv.URL, "k")
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
		if got.Kind != tc.wantKind {
			t.Errorf("%s: kind = %q; want %q", tc.name, got.Kind, tc.wantKind)
		}
		if got.HasAmount {
			t.Errorf("%s: reported an amount it cannot have", tc.name)
		}
	}
}

// Without a key there is nothing to authenticate with, and firing the request
// anyway just trades a clear message for a 401.
func TestMoneyProviderWithoutKeyFailsWithoutCalling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL.Path)
	}))
	defer srv.Close()

	if _, err := FetchBalance(context.Background(), "deepseek", srv.URL, "  "); err == nil {
		t.Fatal("want an error when no API key is configured")
	}
}

func TestOriginOfStripsChatPath(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://api.deepseek.com/anthropic/v1", "https://api.deepseek.com"},
		{"https://api.deepseek.com", "https://api.deepseek.com"},
		{"http://localhost:8080/v1/", "http://localhost:8080"},
	} {
		got, err := originOf(tc.in)
		if err != nil {
			t.Errorf("originOf(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("originOf(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
	if _, err := originOf("api.deepseek.com"); err == nil {
		t.Error("want an error for a base URL with no scheme")
	}
}
