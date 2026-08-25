package oauth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	pvdr "github.com/Mikedev115/Aetox/internal/provider"
)

// Risk says what a user is agreeing to by signing in, and it is shown on the
// sign-in screen rather than buried in docs.
//
// RiskRestricted means Aetox knows of a specific reason this sign-in could cost
// the user their account: an enforcement the provider has announced or acted on.
// RiskOpen is everything else — either the provider publishes the flow for
// third-party apps (OpenRouter), or the plan it reaches is one the provider
// serves through this client and has not moved against (ChatGPT, §70).
//
// The bar deliberately sits at *evidence*, not at *shape*: §64 warned on shape
// alone — a borrowed OAuth client — and the warning it produced was speculation
// dressed as a finding, on a flow that has worked throughout. A warning that
// fires on everything is one users learn to click past, which costs exactly the
// case it exists for. No sign-in Aetox currently ships is RiskRestricted; the
// level stays because Antigravity (§66) is the worked example of one that would
// be, and the next candidate gets measured against it.
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
//
// Two entries. §66 cut this to OpenRouter alone on the rule "only flows the
// provider publishes for third-party apps"; §69 put ChatGPT back and §70 dropped
// the warning that came with it — see the Risk comment above for where the bar
// sits now. The Note still says what the sign-in does, because that is
// information rather than a caveat: which client it presents, and whose quota
// the turns come out of.
var methods = map[string]Method{
	"openrouter": {
		Provider: "openrouter", Label: "OpenRouter", Kind: "browser", Risk: RiskOpen,
		Note: "Published OAuth flow. Mints an API key you own and can revoke on your OpenRouter dashboard.",
	},
	"codex": {
		Provider: "codex", Label: "ChatGPT", Kind: "browser", Risk: RiskOpen,
		Note: "Signs in through the Codex CLI's OAuth client and runs on your ChatGPT plan — the same quota Codex spends. Needs port 1455 free.",
	},
}

// refreshers maps a provider to how its access token is renewed. Credentials
// of type "api" never appear here: nothing about them expires, which is why
// OpenRouter — whose flow mints exactly such a key — is absent. A flow that
// hands back an expiring token registers here, or Token() strands the user on a
// dead credential.
var refreshers = map[string]func(context.Context, Credential) (Credential, error){
	"codex": refreshCodex,
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
// ponytail: one lock for every provider, so one provider's refresh briefly
// blocks another's. Refreshes are seconds apart at most; split it per provider
// only if that ever shows up in a profile.
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
// most; the ChatGPT backend routes on an account id that is a property of the
// sign-in rather than of the request.
func Headers(provider string) map[string]string {
	switch pvdr.Normalize(provider) {
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
	case "codex":
		return StartCodex()
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
	case "codex":
		return FinishCodex(ctx, pending)
	default:
		return fmt.Errorf("unknown sign-in: %q", pending.provider)
	}
}

// Logout forgets a provider's credential.
func Logout(provider string) error {
	return Delete(pvdr.Normalize(provider))
}
