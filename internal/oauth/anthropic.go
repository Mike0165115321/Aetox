package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Claude Pro/Max sign-in. The client id below is Claude Code's own public
// OAuth client — Anthropic publishes no client for third-party apps, so every
// tool that offers this (opencode, plandex, crush) reuses that one. Two
// consequences the UI must not hide: Anthropic can revoke it at any time, and
// a user's consumer plan terms may not permit driving it from another client.
// ProviderRisk reports this as "restricted"; the sign-in screen shows it.
const (
	anthropicClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	anthropicAuthorizeURL = "https://claude.ai/oauth/authorize"
	anthropicTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	anthropicRedirectURI  = "https://console.anthropic.com/oauth/code/callback"
	anthropicScopes       = "org:create_api_key user:profile user:inference"

	// AnthropicBeta must ride on every request made with an OAuth token. Without
	// it the Messages API answers 401 as if the token were invalid.
	AnthropicBeta = "oauth-2025-04-20"

	// AnthropicOAuthSystemPrefix is required by the same endpoint: a request
	// whose first system block is anything else is refused. It is a condition
	// of the credential, not a prompt choice — internal/model prepends it and
	// leaves the rest of Aetox's system prompt after it.
	AnthropicOAuthSystemPrefix = "You are Claude Code, Anthropic's official CLI for Claude."
)

// StartAnthropic builds the authorize URL and the PKCE secret that must come
// back with the code.
//
// Unlike every other flow here this one cannot use a loopback listener:
// Anthropic pins the redirect to its own console page, which displays the code
// for the user to copy. So the caller shows the URL, takes a pasted string,
// and hands it to FinishAnthropic.
func StartAnthropic() (*Pending, error) {
	verifier, challenge, err := newPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomString(32)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("code", "true")
	q.Set("client_id", anthropicClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", anthropicRedirectURI)
	q.Set("scope", anthropicScopes)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)

	return &Pending{
		URL:      anthropicAuthorizeURL + "?" + q.Encode(),
		Verifier: verifier,
		State:    state,
		provider: "anthropic",
	}, nil
}

// FinishAnthropic exchanges the pasted code for tokens and stores them.
//
// The console hands the user "<code>#<state>". Accepting a bare code too would
// mean skipping the state check, which is the only thing standing between a
// user and pasting a code phished from another site, so a missing state is an
// error rather than a shrug.
func FinishAnthropic(ctx context.Context, pending *Pending, pasted string) error {
	if pending == nil {
		return errors.New("no sign-in in progress")
	}
	code, state, ok := splitAnthropicCode(pasted)
	if !ok {
		return errors.New("that does not look like an authorization code — copy the whole line the page shows, including the part after #")
	}
	if state != pending.State {
		return errors.New("authorization code does not match this sign-in — start again")
	}

	body, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"state":         state,
		"code_verifier": pending.Verifier,
		"redirect_uri":  anthropicRedirectURI,
		"client_id":     anthropicClientID,
	})
	if err != nil {
		return err
	}
	tokens, err := anthropicToken(ctx, body)
	if err != nil {
		return err
	}
	return Set("anthropic", Credential{
		Type:      "oauth",
		Access:    tokens.AccessToken,
		Refresh:   tokens.RefreshToken,
		ExpiresAt: tokens.expiresAt(),
		Label:     "Claude Pro/Max",
	})
}

func refreshAnthropic(ctx context.Context, cred Credential) (Credential, error) {
	if strings.TrimSpace(cred.Refresh) == "" {
		return Credential{}, errors.New("claude sign-in has no refresh token — sign in again")
	}
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": cred.Refresh,
		"client_id":     anthropicClientID,
	})
	if err != nil {
		return Credential{}, err
	}
	tokens, err := anthropicToken(ctx, body)
	if err != nil {
		return Credential{}, err
	}
	cred.Access = tokens.AccessToken
	if tokens.RefreshToken != "" {
		// Anthropic rotates the refresh token on some responses and omits it on
		// others; overwriting with an empty string would log the user out on the
		// next refresh.
		cred.Refresh = tokens.RefreshToken
	}
	cred.ExpiresAt = tokens.expiresAt()
	return cred, nil
}

func anthropicToken(ctx context.Context, body []byte) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicTokenURL, bytes.NewReader(body))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", AnthropicBeta)
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	var tokens tokenResponse
	if err := readJSON(resp, &tokens); err != nil {
		return tokenResponse{}, fmt.Errorf("claude sign-in failed: %w", err)
	}
	if tokens.AccessToken == "" {
		return tokenResponse{}, errors.New("claude sign-in returned no access token")
	}
	return tokens, nil
}

func splitCodeState(pasted string) (code, state string, ok bool) {
	pasted = strings.TrimSpace(pasted)
	code, state, found := strings.Cut(pasted, "#")
	code = strings.TrimSpace(code)
	state = strings.TrimSpace(state)
	return code, state, found && code != "" && state != ""
}

// splitAnthropicCode also tolerates the user pasting the whole callback URL
// instead of the code shown on the page — same information, and telling them
// off for it helps nobody.
func splitAnthropicCode(pasted string) (code, state string, ok bool) {
	pasted = strings.TrimSpace(pasted)
	if strings.HasPrefix(pasted, "http://") || strings.HasPrefix(pasted, "https://") {
		if u, err := url.Parse(pasted); err == nil {
			q := u.Query()
			return strings.TrimSpace(q.Get("code")), strings.TrimSpace(q.Get("state")),
				q.Get("code") != "" && q.Get("state") != ""
		}
	}
	return splitCodeState(pasted)
}
