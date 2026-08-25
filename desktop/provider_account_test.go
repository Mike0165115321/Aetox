package main

import (
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
)

// "Never answered" and "answered, states no limits" both arrive as an empty
// slice and must not collapse into one state on screen: the first is "not
// known yet, chat once and it appears", the second is "this provider does not
// report a quota". Only the presence of the key tells them apart.
func TestQuotaKnownSeparatesSilenceFromAbsence(t *testing.T) {
	app := &App{}

	unheard := app.providerAccount("groq")
	if unheard.QuotaKnown {
		t.Error("QuotaKnown = true for a provider that has never answered a turn")
	}

	app.rememberQuotas("groq", nil)
	answered := app.providerAccount("groq")
	if !answered.QuotaKnown {
		t.Error("QuotaKnown = false after the provider answered; want true with no windows")
	}
	if len(answered.Quotas) != 0 {
		t.Errorf("Quotas = %+v; want none", answered.Quotas)
	}
}

func TestRememberQuotasNormalizesTheProviderName(t *testing.T) {
	app := &App{}
	q := model.Quota{Window: "week", RemainingPercent: 12, ObservedAt: time.Now()}
	// The clients report whatever name they were built with; the panel looks
	// up canonical names.
	app.rememberQuotas("claude", []model.Quota{q})

	got := app.providerAccount("anthropic")
	if !got.QuotaKnown || len(got.Quotas) != 1 || got.Quotas[0].RemainingPercent != 12 {
		t.Fatalf("anthropic account = %+v; want the window filed under the alias", got)
	}
}

// A local runtime has no wallet and no window, and must not be reported as an
// error just because there was nothing to fetch.
func TestLocalProviderAccountIsNotAnError(t *testing.T) {
	app := &App{}
	got := app.providerAccount("ollama")
	if got.Error != "" {
		t.Errorf("Error = %q; want empty — there was nothing to fetch", got.Error)
	}
	if got.Balance.Kind != "free" {
		t.Errorf("Kind = %q; want free", got.Balance.Kind)
	}
	if got.Balance.HasAmount {
		t.Error("a local runtime reported an amount")
	}
}

// Switching accounts must not leave the previous one's windows on the card.
//
// A quota describes the credential the turn ran on, and nothing refreshes it
// until another turn runs. Without this, signing into a second ChatGPT plan
// drew the first plan's bars under the new account's name — and when the first
// plan was the exhausted one, the switch looked like it had failed. Asserting
// on QuotaKnown rather than on an empty slice is the whole point: the card has
// three states and this must land on "not known yet", never on "answered, no
// limits".
func TestCredentialChangeForgetsTheOldAccountsQuota(t *testing.T) {
	app := &App{}
	app.rememberQuotas("codex", []model.Quota{{
		Window: "month", RemainingPercent: 0, ObservedAt: time.Now(),
	}})
	if got := app.providerAccount("codex"); !got.QuotaKnown {
		t.Fatal("the fixture did not take; nothing is being measured")
	}

	app.forgetQuotas("codex")

	got := app.providerAccount("codex")
	if got.QuotaKnown {
		t.Errorf("QuotaKnown = true after the credential changed; the card still claims %+v", got.Quotas)
	}
	if len(got.Quotas) != 0 {
		t.Errorf("Quotas = %+v after the credential changed; want none", got.Quotas)
	}
}

// The alias handling has to match rememberQuotas', or a window filed under the
// canonical name would survive a sign-out issued under the client's own name.
func TestForgettingQuotasNormalizesTheProviderName(t *testing.T) {
	app := &App{}
	app.rememberQuotas("anthropic", []model.Quota{{Window: "week", RemainingPercent: 40}})
	app.forgetQuotas("claude")
	if got := app.providerAccount("anthropic"); got.QuotaKnown {
		t.Errorf("the alias did not reach the stored window: %+v", got.Quotas)
	}
}
