// Package github is Aetox's connection to a GitHub account: where the token
// comes from, who it belongs to, and the two base URLs everything else asks it
// for.
//
// It is not a model provider. The credential lives in the same vault as the
// model logins — internal/oauth's store, encrypted at rest by internal/atrest —
// because a GitHub token is the same class of secret and holding two classes to
// two standards is a difference nobody decided on. But it is deliberately
// absent from oauth.Methods(), which is the catalog Settings draws the *model*
// sign-in list from: a GitHub connection extends what the agent can reach, the
// way an MCP server does, and putting it on the Providers page would say it is
// something you can run a turn on.
//
// The dependency runs one way — this package imports oauth, never the reverse —
// so the vault stays a leaf and this stays the only place that knows what a
// GitHub connection is.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/oauth"
)

const (
	// APIBaseURL and RawBaseURL are the two hosts GitHub serves from, and this
	// is the only place either is written down. internal/skill's repo tools
	// read them from here rather than keeping their own copies — one question,
	// one answer, even when the question is as small as a hostname.
	APIBaseURL = "https://api.github.com"
	RawBaseURL = "https://raw.githubusercontent.com"

	// storeKey names this connection inside the credential store. It shares the
	// file with the model logins and nothing else: "github-copilot" was a model
	// provider and is a different key, retired in §64.
	storeKey = "github"

	// apiVersion is what GitHub asks third-party clients to pin so a future
	// breaking change to the REST API does not arrive unannounced.
	apiVersion = "2022-11-28"

	userAgent = "Aetox"
)

// Account is who a token belongs to, as GitHub reports it.
type Account struct {
	Login string `json:"login"`
	Name  string `json:"name,omitempty"`
	// Scopes is what a classic personal access token was granted. Fine-grained
	// tokens and GitHub App installation tokens carry their permissions
	// elsewhere and report nothing here, so an empty list means "not stated",
	// never "no access".
	Scopes []string `json:"scopes,omitempty"`
}

// Source says where the token in use came from, because "disconnect" and
// "still works" have to be explainable together: an env var set outside Aetox
// keeps working after the stored connection is removed, and a user who is not
// told that will report it as a bug.
type Source string

const (
	SourceNone        Source = ""
	SourceConnection  Source = "connection"
	SourceEnvironment Source = "environment"
)

// Status is what Settings renders. It never carries the token, and it never
// touches the network — opening a settings page should not spend a request or
// block on a timeout. Verify is the deliberate check.
type Status struct {
	Connected bool   `json:"connected"`
	Login     string `json:"login,omitempty"`
	Source    Source `json:"source,omitempty"`
	// EnvOverride reports that GITHUB_TOKEN or GH_TOKEN is set as well. Both can
	// be true at once, and the connection wins — see Token.
	EnvOverride bool `json:"env_override"`
}

// envKeys are the environment variables honoured as a fallback, in order.
// Kept because they are how the tools worked before there was anywhere to
// connect an account: CI, a shell that already exports one, and anyone who
// would rather Aetox never hold the secret at all.
var envKeys = []string{"GITHUB_TOKEN", "GH_TOKEN"}

// Token returns the credential to send to GitHub right now, or "" when there is
// none — every caller must still work unauthenticated, since the read-only
// repo tools do, at a lower rate limit.
//
// A stored connection beats the environment. The user connected an account
// inside the app; an env var they may have exported months ago in a shell
// profile should not silently outrank the thing they just clicked.
func Token() string {
	if cred, ok := oauth.Get(storeKey); ok {
		if key := strings.TrimSpace(cred.Key); key != "" {
			return key
		}
	}
	return envToken()
}

func envToken() string {
	for _, key := range envKeys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// CurrentStatus reports the connection without asking GitHub anything.
func CurrentStatus() Status {
	env := envToken() != ""
	cred, ok := oauth.Get(storeKey)
	if ok && strings.TrimSpace(cred.Key) != "" {
		return Status{Connected: true, Login: cred.Account, Source: SourceConnection, EnvOverride: env}
	}
	if env {
		return Status{Connected: true, Source: SourceEnvironment, EnvOverride: true}
	}
	return Status{}
}

// Connect stores a token after proving it works and finding out whose it is.
//
// Validation is not politeness. A token that is wrong, expired or revoked fails
// identically to a repository that does not exist once it is buried inside a
// tool call, and the user is left reading "not found" about a repo they are
// looking at in a browser tab. One request at connect time turns that into a
// sentence on the screen they are already on.
func Connect(ctx context.Context, token string) (Account, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Account{}, errors.New("token is required")
	}
	account, err := fetchAccount(ctx, apiBase, token)
	if err != nil {
		return Account{}, err
	}
	cred := oauth.Credential{
		Type:    "api",
		Key:     token,
		Account: account.Login,
		Label:   "GitHub · " + account.Login,
	}
	if err := oauth.Set(storeKey, cred); err != nil {
		return Account{}, fmt.Errorf("save GitHub connection: %w", err)
	}
	return account, nil
}

// Verify re-checks whichever token is in play and reports who it belongs to.
// Separate from CurrentStatus so that the network call happens when a user asks
// for it, not every time a page renders.
func Verify(ctx context.Context) (Account, error) {
	token := Token()
	if token == "" {
		return Account{}, errors.New("no GitHub token: connect an account or set GITHUB_TOKEN")
	}
	return fetchAccount(ctx, apiBase, token)
}

// apiBase is the seam the tests point at an httptest server. Production never
// assigns it — APIBaseURL above stays the single stated answer for where GitHub
// lives, and this is only how a test avoids talking to the real one.
var apiBase = APIBaseURL

// Disconnect forgets the stored connection. It cannot unset an environment
// variable, and does not pretend to — CurrentStatus reports what is left.
func Disconnect() error {
	return oauth.Delete(storeKey)
}

// httpClient is shared so connect and verify cannot drift on timeout.
var httpClient = &http.Client{Timeout: 20 * time.Second}

// fetchAccount is GET /user: the smallest call that answers both "does this
// token work" and "whose is it".
func fetchAccount(ctx context.Context, baseURL, token string) (Account, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/user", nil)
	if err != nil {
		return Account{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return Account{}, fmt.Errorf("reach GitHub: %w", err)
	}
	defer resp.Body.Close()

	// A GitHub error body is small; the cap is here so a wrong base URL
	// pointing at something that streams cannot hold the call open.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Account{}, fmt.Errorf("read GitHub response: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return Account{}, errors.New("GitHub rejected this token — it is wrong, expired, or was revoked")
	case resp.StatusCode == http.StatusForbidden:
		// 403 here is nearly always the rate limit rather than a permission
		// problem, and saying "forbidden" sends the user looking for a scope to
		// tick that would not have helped.
		return Account{}, errors.New("GitHub refused the request — usually the rate limit; wait a minute and try again")
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return Account{}, fmt.Errorf("GitHub returned status %d", resp.StatusCode)
	}

	var parsed struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Account{}, fmt.Errorf("parse GitHub account: %w", err)
	}
	if strings.TrimSpace(parsed.Login) == "" {
		return Account{}, errors.New("GitHub returned an account with no login name")
	}
	return Account{
		Login:  parsed.Login,
		Name:   strings.TrimSpace(parsed.Name),
		Scopes: parseScopes(resp.Header.Get("X-OAuth-Scopes")),
	}, nil
}

// parseScopes reads the header a classic token's grants arrive in. Fine-grained
// tokens send it empty, which is why the field is documented as "not stated"
// rather than treated as a permission list to check against.
func parseScopes(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if scope := strings.TrimSpace(part); scope != "" {
			out = append(out, scope)
		}
	}
	return out
}
