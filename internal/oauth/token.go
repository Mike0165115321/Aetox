package oauth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	pvdr "github.com/Mike0165115321/Aetox/internal/provider"
)

// Risk says what a user is agreeing to by signing in, and it is shown on the
// sign-in screen rather than buried in docs.
//
// The distinction is real, not legal hedging: RiskOpen flows are published by
// the provider for third-party apps to use, and RiskRestricted flows work by
// presenting another product's OAuth client. The second kind can be switched
// off by the provider at any time, and a consumer plan's terms may not permit
// driving it from Aetox — the user's account, so the user's call.
type Risk string

const (
	RiskOpen       Risk = "open"
	RiskRestricted Risk = "restricted"
)

// Method describes one sign-in Aetox can offer.
type Method struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	// Kind is how the user completes it: "device" (type a short code into a
	// page), "browser" (redirect comes back to a local listener), or "paste"
	// (copy a code out of the provider's page). The UI needs it to know which
	// screen to draw before the flow starts.
	Kind string `json:"kind"`
	Risk Risk   `json:"risk"`
	Note string `json:"note"`
}

// methods is the registry of working sign-ins. A provider absent from here has
// no OAuth path — Settings offers it an API key field and nothing else.
var methods = map[string]Method{
	"openrouter": {
		Provider: "openrouter", Label: "OpenRouter", Kind: "browser", Risk: RiskOpen,
		Note: "Published OAuth flow. Mints an API key you own and can revoke on your OpenRouter dashboard.",
	},
	"github-copilot": {
		Provider: "github-copilot", Label: "GitHub Copilot", Kind: "device", Risk: RiskRestricted,
		Note: "Signs in through the editor plugin's OAuth client. Needs an active Copilot subscription.",
	},
	"qwen": {
		Provider: "qwen", Label: "Qwen", Kind: "device", Risk: RiskRestricted,
		Note: "Signs in through the qwen-code CLI's OAuth client, against your free Qwen quota.",
	},
	"anthropic": {
		Provider: "anthropic", Label: "Claude Pro/Max", Kind: "paste", Risk: RiskRestricted,
		Note: "Signs in through Claude Code's OAuth client. Anthropic's consumer terms may not allow driving your plan from another app — your account, your call.",
	},
	"code-assist": {
		Provider: "code-assist", Label: "Gemini (Google account)", Kind: "browser", Risk: RiskRestricted,
		Note: "Signs in with a Google account through the Gemini CLI's OAuth client, against the free Code Assist tier.",
	},
	"codex": {
		Provider: "codex", Label: "ChatGPT", Kind: "browser", Risk: RiskRestricted,
		Note: "Signs in through the Codex CLI's OAuth client and runs on your ChatGPT plan. Needs port 1455 free. OpenAI's consumer terms may not allow driving your plan from another app — your account, your call.",
	},
}

// refreshers maps a provider to how its access token is renewed. Credentials
// of type "api" never appear here: nothing about them expires.
var refreshers = map[string]func(context.Context, Credential) (Credential, error){
	"anthropic":      refreshAnthropic,
	"github-copilot": refreshCopilot,
	"qwen":           refreshQwen,
	"codex":          refreshCodex,
	"code-assist":    refreshGoogle,
}

// Methods lists every sign-in Aetox offers, in a stable order.
func Methods() []Method {
	out := make([]Method, 0, len(methods))
	for _, m := range methods {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

// MethodFor reports the sign-in for one provider, if it has one.
func MethodFor(provider string) (Method, bool) {
	m, ok := methods[pvdr.Normalize(provider)]
	return m, ok
}

// Has reports whether this provider is signed in. It does not validate the
// token — an expired one still counts, because the next Token call refreshes
// it rather than asking the user to sign in again.
func Has(provider string) bool {
	_, ok := Get(pvdr.Normalize(provider))
	return ok
}

// Status is what Settings renders per provider.
type Status struct {
	Provider  string `json:"provider"`
	SignedIn  bool   `json:"signed_in"`
	Label     string `json:"label,omitempty"`
	Account   string `json:"account,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

// StatusFor reports one provider's sign-in state without exposing any token.
func StatusFor(provider string) Status {
	canonical := pvdr.Normalize(provider)
	cred, ok := Get(canonical)
	if !ok {
		return Status{Provider: canonical}
	}
	return Status{
		Provider:  canonical,
		SignedIn:  true,
		Label:     cred.Label,
		Account:   cred.Account,
		ExpiresAt: cred.ExpiresAt,
	}
}

// refreshMu serializes token refreshes.
// ponytail: one lock for every provider, so a Copilot refresh briefly blocks a
// Qwen one. Refreshes are seconds apart at most; split it per provider only if
// that ever shows up in a profile.
var refreshMu sync.Mutex

// Token returns the credential to send for this provider right now, refreshing
// it first if it is close to expiry. The refreshed token is written back to the
// store, so a long session does not re-mint on every turn.
func Token(ctx context.Context, provider string) (string, error) {
	canonical := pvdr.Normalize(provider)

	refreshMu.Lock()
	defer refreshMu.Unlock()

	cred, ok := Get(canonical)
	if !ok {
		return "", fmt.Errorf("not signed in to %s", canonical)
	}
	if cred.Type == "api" {
		return cred.Key, nil
	}
	if !cred.Expired() {
		return cred.Access, nil
	}

	refresh, ok := refreshers[canonical]
	if !ok {
		return "", fmt.Errorf("%s sign-in expired and cannot be renewed — sign in again", canonical)
	}
	next, err := refresh(ctx, cred)
	if err != nil {
		return "", err
	}
	if err := Set(canonical, next); err != nil {
		// The token is good even if persisting it failed; using it now and
		// re-minting next time beats failing the user's turn over a disk error.
		return next.Access, nil
	}
	return next.Access, nil
}

// TokenSource returns a function that yields a live token on every call, or
// nil when this provider is not signed in. internal/model treats nil as "use
// the API key path", which is what keeps every existing provider unchanged.
func TokenSource(provider string) func(context.Context) (string, error) {
	canonical := pvdr.Normalize(provider)
	if !Has(canonical) {
		return nil
	}
	return func(ctx context.Context) (string, error) { return Token(ctx, canonical) }
}

// Endpoint reports a base URL the sign-in itself pinned to this account, or ""
// to use the catalog default.
func Endpoint(provider string) string {
	cred, ok := Get(pvdr.Normalize(provider))
	if !ok {
		return ""
	}
	return cred.Endpoint
}

// Headers reports extra headers this provider's credentials require. Empty for
// most; Copilot refuses requests without its editor identification, and the
// ChatGPT backend routes on an account id that is a property of the sign-in
// rather than of the request.
func Headers(provider string) map[string]string {
	switch pvdr.Normalize(provider) {
	case "github-copilot":
		return CopilotHeaders()
	case "codex":
		headers := map[string]string{
			"originator": CodexOriginator,
			"User-Agent": clientUserAgent,
		}
		if cred, ok := Get("codex"); ok && cred.Account != "" {
			headers["chatgpt-account-id"] = cred.Account
		}
		return headers
	default:
		return nil
	}
}

// Start begins a sign-in. What comes back tells the caller what to show: a URL
// to open, and for device flows the short code to type.
func Start(ctx context.Context, provider string) (*Pending, error) {
	switch pvdr.Normalize(provider) {
	case "openrouter":
		return StartOpenRouter()
	case "github-copilot":
		return StartCopilot(ctx)
	case "qwen":
		return StartQwen(ctx)
	case "anthropic":
		return StartAnthropic()
	case "codex":
		return StartCodex()
	case "code-assist":
		return StartGoogle()
	default:
		return nil, fmt.Errorf("%s has no sign-in — add an API key instead", pvdr.Normalize(provider))
	}
}

// Finish completes a sign-in and stores the credential. It blocks for the two
// flows that wait on the user (device polling, browser redirect); pasted is
// used only by the paste flows and ignored otherwise.
func Finish(ctx context.Context, pending *Pending, pasted string) error {
	if pending == nil {
		return errors.New("no sign-in in progress")
	}
	switch pending.provider {
	case "openrouter":
		return FinishOpenRouter(ctx, pending)
	case "github-copilot":
		return FinishCopilot(ctx, pending)
	case "qwen":
		return FinishQwen(ctx, pending)
	case "anthropic":
		return FinishAnthropic(ctx, pending, pasted)
	case "codex":
		return FinishCodex(ctx, pending)
	case "code-assist":
		return FinishGoogle(ctx, pending)
	default:
		return fmt.Errorf("unknown sign-in: %q", pending.provider)
	}
}

// Logout forgets a provider's credential.
func Logout(provider string) error {
	return Delete(pvdr.Normalize(provider))
}
