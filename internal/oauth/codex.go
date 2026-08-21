package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ChatGPT (Codex) sign-in.
//
// The redirect URI is registered against localhost:1455 exactly, so unlike
// OpenRouter this flow cannot take any free port — if 1455 is busy the sign-in
// cannot proceed, and saying so beats failing with a bind error.
//
// "localhost", not "127.0.0.1": OAuth redirect matching is string equality, and
// the two are different strings even though they are the same machine.
const (
	codexClientID     = "app_EMoamEEZ73f0CkXaXp7hrann" // the Codex CLI's public client
	codexAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	codexTokenURL     = "https://auth.openai.com/oauth/token"
	codexRedirectHost = "localhost"
	codexRedirectPort = 1455
	codexRedirectPath = "/auth/callback"
	codexScope        = "openid profile email offline_access"

	// CodexBaseURL is the ChatGPT backend that serves a subscription. It is not
	// api.openai.com and it does not speak /chat/completions — see the
	// "responses" runtime in internal/model.
	CodexBaseURL = "https://chatgpt.com/backend-api/codex"

	// CodexOriginator identifies Aetox to that backend. The header is required;
	// its value is ours to choose.
	CodexOriginator = "aetox"
)

// StartCodex opens the browser flow and the listener that catches its redirect.
func StartCodex() (*Pending, error) {
	verifier, challenge, err := NewPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomString(32)
	if err != nil {
		return nil, err
	}
	lb, err := StartLoopbackAs(codexRedirectHost, codexRedirectPort, codexRedirectPath)
	if err != nil {
		return nil, fmt.Errorf("ChatGPT sign-in needs port %d, which is in use — close the other Codex sign-in and try again: %w", codexRedirectPort, err)
	}

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", codexClientID)
	q.Set("redirect_uri", lb.RedirectURI)
	q.Set("scope", codexScope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	// Both flags are what the official client sends. Without the first the
	// id_token comes back without the organization claims the account id is
	// read from, which is the one field this whole flow exists to obtain.
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")

	return &Pending{
		URL:      codexAuthorizeURL + "?" + q.Encode(),
		Verifier: verifier,
		State:    state,
		provider: "codex",
		lb:       lb,
	}, nil
}

// FinishCodex waits for the redirect, exchanges the code, and stores the login.
func FinishCodex(ctx context.Context, pending *Pending) error {
	if pending == nil || pending.lb == nil {
		return errors.New("no sign-in in progress")
	}
	defer pending.Cancel()

	code, state, err := pending.lb.Wait(ctx)
	if err != nil {
		return err
	}
	// The state check is the only thing standing between a user and a code
	// planted by another site, so a mismatch is fatal rather than a warning.
	if state != "" && state != pending.State {
		return errors.New("authorization code does not match this sign-in — start again")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", pending.lb.RedirectURI)
	form.Set("client_id", codexClientID)
	form.Set("code_verifier", pending.Verifier)

	tokens, err := codexToken(ctx, form)
	if err != nil {
		return err
	}
	cred, err := codexCredential(tokens)
	if err != nil {
		return err
	}
	return Set("codex", cred)
}

func refreshCodex(ctx context.Context, cred Credential) (Credential, error) {
	if strings.TrimSpace(cred.Refresh) == "" {
		return Credential{}, errors.New("ChatGPT sign-in has no refresh token — sign in again")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cred.Refresh)
	form.Set("client_id", codexClientID)
	form.Set("scope", "openid profile email")

	tokens, err := codexToken(ctx, form)
	if err != nil {
		return Credential{}, err
	}
	next, err := codexCredential(tokens)
	if err != nil {
		return Credential{}, err
	}
	if next.Refresh == "" {
		next.Refresh = cred.Refresh
	}
	if next.Account == "" {
		// A refresh response often omits the id_token; the account id does not
		// change, so keep the one the sign-in established rather than losing
		// the header every request needs.
		next.Account = cred.Account
	}
	return next, nil
}

// codexTokenResponse is the token endpoint's shape. id_token is the field that
// matters beyond the tokens themselves: the ChatGPT account id lives in its
// claims and nowhere else in the response.
type codexTokenResponse struct {
	tokenResponse
	IDToken string `json:"id_token"`
}

func codexToken(ctx context.Context, form url.Values) (codexTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return codexTokenResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return codexTokenResponse{}, err
	}
	var tokens codexTokenResponse
	if err := readJSON(resp, &tokens); err != nil {
		return codexTokenResponse{}, fmt.Errorf("ChatGPT sign-in failed: %w", err)
	}
	if tokens.AccessToken == "" {
		return codexTokenResponse{}, errors.New("ChatGPT sign-in returned no access token")
	}
	return tokens, nil
}

func codexCredential(tokens codexTokenResponse) (Credential, error) {
	cred := Credential{
		Type:      "oauth",
		Access:    tokens.AccessToken,
		Refresh:   tokens.RefreshToken,
		ExpiresAt: tokens.expiresAt(),
		Endpoint:  CodexBaseURL,
		Label:     "ChatGPT",
	}
	account, email := codexAccountFromIDToken(tokens.IDToken)
	cred.Account = account
	if email != "" {
		cred.Label = "ChatGPT · " + email
	}
	return cred, nil
}

// codexAccountFromIDToken digs the ChatGPT account id out of the id_token.
//
// The backend requires it as a header on every request, and it is not in the
// token response body — only in a namespaced claim inside the JWT. The
// signature is deliberately not verified: this token was just received over TLS
// from the issuer we asked, and Aetox is reading a routing id out of it, not
// making an authorization decision on it.
func codexAccountFromIDToken(idToken string) (accountID, email string) {
	parts := strings.Split(strings.TrimSpace(idToken), ".")
	if len(parts) < 2 {
		return "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ""
	}
	var claims struct {
		Email string `json:"email"`
		Auth  struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
			ChatGPTPlanType  string `json:"chatgpt_plan_type"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", ""
	}
	return claims.Auth.ChatGPTAccountID, claims.Email
}

// ---------------------------------------------------------------------------
// Importing an existing Codex CLI session
// ---------------------------------------------------------------------------

// ImportCodexCLI adopts the login the user already has from the Codex CLI,
// instead of making them sign in a second time on the same machine.
//
// Deliberately an explicit action, never automatic: reading another tool's
// credential file is something a user should ask for, not discover.
func ImportCodexCLI() error {
	path := codexCLIAuthPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no Codex CLI session found at %s", path)
		}
		return err
	}
	var file struct {
		Tokens struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("could not read the Codex CLI session at %s: %w", path, err)
	}
	if file.Tokens.AccessToken == "" || file.Tokens.RefreshToken == "" {
		return fmt.Errorf("the Codex CLI session at %s is not signed in", path)
	}

	cred := Credential{
		Type:     "oauth",
		Access:   file.Tokens.AccessToken,
		Refresh:  file.Tokens.RefreshToken,
		Account:  file.Tokens.AccountID,
		Endpoint: CodexBaseURL,
		Label:    "ChatGPT (from Codex CLI)",
		// No expiry recorded in that file. Zero would mean "never expires" and
		// strand the session on a dead token, so treat it as already expired:
		// the first request refreshes, and from then on the real expiry is
		// known.
		ExpiresAt: 1,
	}
	if account, email := codexAccountFromIDToken(file.Tokens.IDToken); account != "" {
		cred.Account = account
		if email != "" {
			cred.Label = "ChatGPT · " + email
		}
	}
	return Set("codex", cred)
}

// CodexCLIAvailable reports whether there is a Codex CLI session to import.
func CodexCLIAvailable() bool {
	_, err := os.Stat(codexCLIAuthPath())
	return err == nil
}

func codexCLIAuthPath() string {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return filepath.Join(home, "auth.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".codex", "auth.json")
	}
	return filepath.Join(home, ".codex", "auth.json")
}
