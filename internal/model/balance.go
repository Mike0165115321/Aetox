package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/provider"
)

// balanceTimeout is short on purpose: this runs when a settings panel opens,
// and a provider that cannot answer in a few seconds should leave the row
// blank rather than hold the panel.
const balanceTimeout = 8 * time.Second

// preferredCurrency is the only currency Aetox displays. DeepSeek reports an
// array because one account can hold both CNY and USD, and picking is the
// whole reason the field is a list. An account holding no USD at all is shown
// in whatever it does hold — converting would mean sourcing an exchange rate
// and printing a figure that is ours, not the provider's.
const preferredCurrency = "USD"

// Balance is what a provider can say about money right now.
//
// Kind is always set, including on failure, because "why is this blank" is the
// question the UI has to answer even when there is no figure — a local runtime,
// a subscription, and a provider that simply went down are three different
// blanks and must not look alike.
type Balance struct {
	// Kind mirrors provider.BalanceKind: money, free, subscription, web-only.
	Kind string `json:"kind"`

	// HasAmount is false whenever there is no figure to show — every kind but
	// money, and money that failed to fetch or that the provider left open
	// (an OpenRouter key with no cap has no remaining figure, only usage).
	HasAmount bool    `json:"hasAmount"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`

	// Parts breaks Amount down. Label is a stable key the frontend
	// translates — "granted", "toppedUp", "voucher", "cash", "used" — never a
	// sentence, so Thai and English both come from the locale files.
	Parts []BalancePart `json:"parts"`

	// Sufficient is false only when the provider states outright that the
	// account can no longer make calls. It is not the same as Amount being
	// zero: a subscription has no amount and is perfectly usable.
	Sufficient bool `json:"sufficient"`

	// Quota rides along when the same request also answered the window.
	// Only OpenRouter does; everyone else reports quota through headers on
	// real turns. Nil when there is none.
	Quota *Quota `json:"quota,omitempty"`

	FetchedAt time.Time `json:"fetchedAt"`
}

// BalancePart is one named slice of a balance.
type BalancePart struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
}

// FetchBalance asks a provider what is left in the account.
//
// baseURL is the endpoint the user actually configured, not the catalog
// default, and the balance request is aimed at that same host. Pointing it at
// the vendor's real host instead would send a key the user issued for their
// own proxy to a company they never chose to talk to; a proxy that does not
// serve the balance path simply fails, which the UI already draws.
//
// Providers with nothing to fetch return immediately without touching the
// network — a local runtime has no wallet to knock on.
func FetchBalance(ctx context.Context, providerName, baseURL, apiKey string) (Balance, error) {
	canonical := provider.Normalize(providerName)
	kind := provider.BalanceKindFor(canonical)
	base := Balance{Kind: string(kind), Sufficient: true, FetchedAt: time.Now()}

	if kind != provider.BalanceMoney {
		return base, nil
	}
	if strings.TrimSpace(apiKey) == "" {
		return base, fmt.Errorf("%s: no API key configured", canonical)
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = provider.DefaultBaseURL(canonical)
	}

	ctx, cancel := context.WithTimeout(ctx, balanceTimeout)
	defer cancel()

	switch canonical {
	case "deepseek":
		return fetchDeepSeekBalance(ctx, base, baseURL, apiKey)
	case "kimi":
		return fetchKimiBalance(ctx, base, baseURL, apiKey)
	case "openrouter":
		return fetchOpenRouterBalance(ctx, base, baseURL, apiKey)
	default:
		// The catalog says this provider has money to report but nobody here
		// knows how to ask. Better to say so than to return a confident zero.
		return base, fmt.Errorf("%s: balance reading is not implemented", canonical)
	}
}

// getJSON runs the one request shape all three balance endpoints share.
func getJSON(ctx context.Context, endpoint, apiKey string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := (&http.Client{Timeout: balanceTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("balance endpoint failed with status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, into)
}

// originOf reduces a configured base URL to scheme://host.
//
// DeepSeek serves the balance at the root, not under the chat path: the
// configured endpoint ends in /anthropic/v1 (or /v1 on the alternate wire
// format), and appending to either would ask for a path that does not exist.
func originOf(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("cannot read an origin from %q", raw)
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// ---------------------------------------------------------------------------
// DeepSeek
// ---------------------------------------------------------------------------

type deepSeekBalance struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		// Every figure here is a JSON string, not a number — parsing it as a
		// float64 fails silently to zero, which would read as "you are out of
		// credit" on a funded account.
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

func fetchDeepSeekBalance(ctx context.Context, out Balance, baseURL, apiKey string) (Balance, error) {
	origin, err := originOf(baseURL)
	if err != nil {
		return out, err
	}
	var payload deepSeekBalance
	if err := getJSON(ctx, origin+"/user/balance", apiKey, &payload); err != nil {
		return out, fmt.Errorf("deepseek: %w", err)
	}
	if len(payload.BalanceInfos) == 0 {
		return out, fmt.Errorf("deepseek: no balance reported")
	}

	info := payload.BalanceInfos[0]
	for _, candidate := range payload.BalanceInfos {
		if strings.EqualFold(candidate.Currency, preferredCurrency) {
			info = candidate
			break
		}
	}

	out.HasAmount = true
	out.Currency = strings.ToUpper(strings.TrimSpace(info.Currency))
	out.Amount = parseMoney(info.TotalBalance)
	out.Sufficient = payload.IsAvailable
	out.Parts = nonZeroParts(
		BalancePart{Label: "granted", Amount: parseMoney(info.GrantedBalance)},
		BalancePart{Label: "toppedUp", Amount: parseMoney(info.ToppedUpBalance)},
	)
	out.FetchedAt = time.Now()
	return out, nil
}

// ---------------------------------------------------------------------------
// Kimi (Moonshot)
// ---------------------------------------------------------------------------

type kimiBalance struct {
	// Code is 0 on success. Moonshot answers HTTP 200 with a non-zero code
	// for application-level failures, so the status line alone does not say
	// whether the numbers below mean anything.
	Code   int    `json:"code"`
	Status bool   `json:"status"`
	SCode  string `json:"scode"`
	Data   struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	} `json:"data"`
}

func fetchKimiBalance(ctx context.Context, out Balance, baseURL, apiKey string) (Balance, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/users/me/balance"
	var payload kimiBalance
	if err := getJSON(ctx, endpoint, apiKey, &payload); err != nil {
		return out, fmt.Errorf("kimi: %w", err)
	}
	if payload.Code != 0 {
		return out, fmt.Errorf("kimi: balance endpoint returned code %d (%s)", payload.Code, payload.SCode)
	}

	out.HasAmount = true
	out.Currency = "USD"
	out.Amount = payload.Data.AvailableBalance
	// Moonshot stops serving requests at or below zero, and says so only by
	// failing the next call — the figure is the whole warning there is.
	out.Sufficient = payload.Data.AvailableBalance > 0
	out.Parts = nonZeroParts(
		BalancePart{Label: "voucher", Amount: payload.Data.VoucherBalance},
		BalancePart{Label: "cash", Amount: payload.Data.CashBalance},
	)
	out.FetchedAt = time.Now()
	return out, nil
}

// ---------------------------------------------------------------------------
// OpenRouter
// ---------------------------------------------------------------------------

type openRouterKey struct {
	Data struct {
		// Limit is null for a key with no cap of its own, which is why these
		// are pointers: 0 and "no limit set" are opposite facts.
		Limit          *float64 `json:"limit"`
		LimitRemaining *float64 `json:"limit_remaining"`
		LimitReset     string   `json:"limit_reset"`
		Usage          float64  `json:"usage"`
		IsFreeTier     bool     `json:"is_free_tier"`
	} `json:"data"`
}

// fetchOpenRouterBalance reads GET /key rather than GET /credits: the credits
// endpoint answers 403 for every key that is not a management key, so the one
// that works for a normal user is this one — and it carries the quota window
// in the same response, making OpenRouter the only provider whose quota can be
// had without chatting first.
func fetchOpenRouterBalance(ctx context.Context, out Balance, baseURL, apiKey string) (Balance, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/key"
	var payload openRouterKey
	if err := getJSON(ctx, endpoint, apiKey, &payload); err != nil {
		return out, fmt.Errorf("openrouter: %w", err)
	}

	out.Currency = "USD"
	out.Parts = nonZeroParts(BalancePart{Label: "used", Amount: payload.Data.Usage})
	out.FetchedAt = time.Now()

	if payload.Data.LimitRemaining != nil {
		out.HasAmount = true
		out.Amount = *payload.Data.LimitRemaining
		out.Sufficient = *payload.Data.LimitRemaining > 0
	} else {
		// An uncapped key: there is a real account balance behind it, but only
		// a management key can read it. Usage is the honest half we have.
		out.Sufficient = true
	}

	if limit := payload.Data.Limit; limit != nil && *limit > 0 && payload.Data.LimitRemaining != nil {
		now := time.Now()
		out.Quota = &Quota{
			Window:           "key",
			RemainingPercent: clampPercent(*payload.Data.LimitRemaining / *limit * 100),
			ResetAt:          parseResetInstant(payload.Data.LimitReset, now),
			ObservedAt:       now,
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseMoney reads a figure a provider sent as a string. An unparseable value
// yields zero, which is why callers only ever use it for figures the endpoint
// has already committed to sending.
func parseMoney(value string) float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return n
}

// nonZeroParts drops the empty slices of a breakdown, so an account with no
// vouchers does not get a "voucher $0.00" clause explaining nothing.
func nonZeroParts(parts ...BalancePart) []BalancePart {
	out := make([]BalancePart, 0, len(parts))
	for _, p := range parts {
		if p.Amount != 0 {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
