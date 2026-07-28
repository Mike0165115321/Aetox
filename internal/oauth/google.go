package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Gemini Code Assist sign-in — a personal Google account driving the free
// Code Assist tier, which is what the Gemini CLI does.
//
// Two things make this the most involved flow here. First, the credentials are
// an *installed-app* OAuth client: Google publishes the id and secret in the
// CLI's own source precisely because a desktop app cannot keep a secret, so the
// secret below is a public constant, not a leak. Second, OAuth alone does not
// buy inference — the account has to be associated with a Code Assist project,
// and that project id must ride on every request afterwards.
const (
	googleClientID     = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	googleClientSecret = "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"
	googleAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL     = "https://oauth2.googleapis.com/token"
	googleUserInfoURL  = "https://www.googleapis.com/oauth2/v2/userinfo"
	googleRedirectPath = "/oauth2callback"

	// userinfo.profile is what the CLI asks for and is not needed here — the
	// email comes back with .email alone, and a smaller consent screen is a
	// better one.
	googleScope = "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email"

	// CodeAssistBaseURL is the private surface the CLI talks to. Custom verbs
	// (":loadCodeAssist", ":streamGenerateContent"), not REST paths.
	CodeAssistBaseURL = "https://cloudcode-pa.googleapis.com/v1internal"

	codeAssistFreeTier = "free-tier"
)

// StartGoogle opens the consent screen and the listener that catches the
// redirect. access_type=offline is what makes a refresh token come back at all;
// without it the sign-in dies an hour later with no way to renew.
func StartGoogle() (*Pending, error) {
	state, err := randomString(32)
	if err != nil {
		return nil, err
	}
	lb, err := startLoopback(0, googleRedirectPath)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", googleClientID)
	q.Set("redirect_uri", lb.RedirectURI)
	q.Set("scope", googleScope)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("state", state)

	return &Pending{
		URL:      googleAuthorizeURL + "?" + q.Encode(),
		State:    state,
		provider: "code-assist",
		lb:       lb,
	}, nil
}

// FinishGoogle exchanges the code, then does the part OAuth alone does not
// cover: finding (or provisioning) the Code Assist project this account is
// served by. Without that id every inference request answers 500.
func FinishGoogle(ctx context.Context, pending *Pending) error {
	if pending == nil || pending.lb == nil {
		return errors.New("no sign-in in progress")
	}
	defer pending.Cancel()

	code, state, err := pending.lb.wait(ctx)
	if err != nil {
		return err
	}
	if state != "" && state != pending.State {
		return errors.New("authorization code does not match this sign-in — start again")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", googleClientID)
	form.Set("client_secret", googleClientSecret)
	form.Set("redirect_uri", pending.lb.RedirectURI)

	tokens, err := googleToken(ctx, form)
	if err != nil {
		return err
	}

	cred := Credential{
		Type:      "oauth",
		Access:    tokens.AccessToken,
		Refresh:   tokens.RefreshToken,
		ExpiresAt: tokens.expiresAt(),
		Endpoint:  CodeAssistBaseURL,
		Label:     "Gemini (Google account)",
	}
	if email := googleEmail(ctx, tokens.AccessToken); email != "" {
		cred.Label = "Gemini · " + email
	}

	project, err := resolveCodeAssistProject(ctx, tokens.AccessToken)
	if err != nil {
		return err
	}
	// Account carries the project id: it is exactly as much a property of the
	// sign-in as the token is, and every request needs it.
	cred.Account = project

	return Set("code-assist", cred)
}

func refreshGoogle(ctx context.Context, cred Credential) (Credential, error) {
	if strings.TrimSpace(cred.Refresh) == "" {
		return Credential{}, errors.New("Google sign-in has no refresh token — sign in again")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cred.Refresh)
	form.Set("client_id", googleClientID)
	form.Set("client_secret", googleClientSecret)

	tokens, err := googleToken(ctx, form)
	if err != nil {
		return Credential{}, err
	}
	cred.Access = tokens.AccessToken
	cred.ExpiresAt = tokens.expiresAt()
	// Google's refresh response carries NO refresh_token. Writing the response
	// through as-is is the classic way to log the user out on the first
	// refresh — the old one has to be carried forward deliberately.
	if tokens.RefreshToken != "" {
		cred.Refresh = tokens.RefreshToken
	}
	return cred, nil
}

func googleToken(ctx context.Context, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	var tokens tokenResponse
	if err := readJSON(resp, &tokens); err != nil {
		return tokenResponse{}, fmt.Errorf("Google sign-in failed: %w", err)
	}
	if tokens.AccessToken == "" {
		return tokenResponse{}, errors.New("Google sign-in returned no access token")
	}
	return tokens, nil
}

// googleEmail is decoration for the Settings row; a failure must not fail the
// sign-in.
func googleEmail(ctx context.Context, accessToken string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	var out struct {
		Email string `json:"email"`
	}
	if err := readJSON(resp, &out); err != nil {
		return ""
	}
	return out.Email
}

// ---------------------------------------------------------------------------
// Code Assist onboarding
// ---------------------------------------------------------------------------

// codeAssistMetadata is sent on both onboarding calls. Every field is a fixed
// enum value; nothing here describes the actual machine.
type codeAssistMetadata struct {
	IDEType    string `json:"ideType"`
	Platform   string `json:"platform"`
	PluginType string `json:"pluginType"`
}

func defaultCodeAssistMetadata() codeAssistMetadata {
	return codeAssistMetadata{
		IDEType:    "IDE_UNSPECIFIED",
		Platform:   "PLATFORM_UNSPECIFIED",
		PluginType: "GEMINI",
	}
}

type loadCodeAssistResponse struct {
	CurrentTier *struct {
		ID string `json:"id"`
	} `json:"currentTier"`
	AllowedTiers []struct {
		ID        string `json:"id"`
		IsDefault bool   `json:"isDefault"`
	} `json:"allowedTiers"`
	CloudAICompanionProject string `json:"cloudaicompanionProject"`
}

type longRunningOperation struct {
	Name     string `json:"name"`
	Done     bool   `json:"done"`
	Response *struct {
		CloudAICompanionProject *struct {
			ID string `json:"id"`
		} `json:"cloudaicompanionProject"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// resolveCodeAssistProject asks which project this account is served by, and
// provisions one if the account has never used Code Assist before.
//
// The provisioning half is the cold-start path only: an account that has ever
// opened Gemini CLI or the web onboarding already has a project and returns it
// from the first call.
func resolveCodeAssistProject(ctx context.Context, accessToken string) (string, error) {
	// Absent fields must be *absent*, not null: the CLI omits them, and a port
	// that serializes nil as null sends a different payload than the one this
	// endpoint is known to accept.
	load := struct {
		Metadata codeAssistMetadata `json:"metadata"`
	}{Metadata: defaultCodeAssistMetadata()}

	var loaded loadCodeAssistResponse
	if err := codeAssistCall(ctx, accessToken, ":loadCodeAssist", load, &loaded); err != nil {
		return "", fmt.Errorf("Google sign-in worked but Code Assist did not answer: %w", err)
	}
	if loaded.CloudAICompanionProject != "" {
		return loaded.CloudAICompanionProject, nil
	}

	tier := codeAssistFreeTier
	for _, allowed := range loaded.AllowedTiers {
		if allowed.IsDefault && allowed.ID != "" {
			tier = allowed.ID
			break
		}
	}

	// Free tier must NOT send cloudaicompanionProject — the server answers
	// Precondition Failed when it is present.
	onboard := struct {
		TierID   string             `json:"tierId"`
		Metadata codeAssistMetadata `json:"metadata"`
	}{TierID: tier, Metadata: defaultCodeAssistMetadata()}

	var op longRunningOperation
	if err := codeAssistCall(ctx, accessToken, ":onboardUser", onboard, &op); err != nil {
		return "", fmt.Errorf("Code Assist onboarding failed: %w", err)
	}

	// Provisioning is a long-running operation. The CLI polls every 5s with no
	// cap; a cap belongs here, because "forever" is not a state a sign-in
	// screen can render.
	deadline := time.Now().Add(2 * time.Minute)
	for !op.Done {
		if time.Now().After(deadline) {
			return "", errors.New("Code Assist is still setting up this account — try signing in again in a minute")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(codeAssistPollInterval):
		}
		if err := codeAssistGet(ctx, accessToken, "/"+op.Name, &op); err != nil {
			return "", fmt.Errorf("Code Assist onboarding failed: %w", err)
		}
	}
	if op.Error != nil && op.Error.Message != "" {
		return "", fmt.Errorf("Code Assist onboarding failed: %s", op.Error.Message)
	}
	if op.Response == nil || op.Response.CloudAICompanionProject == nil || op.Response.CloudAICompanionProject.ID == "" {
		return "", errors.New("Code Assist finished onboarding without naming a project — this account may need setup at codeassist.google.com first")
	}
	return op.Response.CloudAICompanionProject.ID, nil
}

// codeAssistPollInterval is a variable so the tests can drive onboarding
// without waiting out real seconds.
var codeAssistPollInterval = 5 * time.Second

func codeAssistCall(ctx context.Context, accessToken, method string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, CodeAssistBaseURL+method, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	return readJSON(resp, out)
}

func codeAssistGet(ctx context.Context, accessToken, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, CodeAssistBaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	return readJSON(resp, out)
}

// CodeAssistProject reports the project id the sign-in resolved, which the
// runtime must send on every request.
func CodeAssistProject() string {
	cred, ok := Get("code-assist")
	if !ok {
		return ""
	}
	return cred.Account
}

// ---------------------------------------------------------------------------
// Importing an existing Gemini CLI session
// ---------------------------------------------------------------------------

// ImportGeminiCLI adopts the Google login the user already has from the Gemini
// CLI. Same deal as ImportCodexCLI: explicit action, never automatic.
//
// The CLI's file holds tokens but not the project id — that lives in its own
// settings and is re-resolved per run — so this asks Code Assist for it, which
// also proves the imported token actually works before storing anything.
func ImportGeminiCLI(ctx context.Context) error {
	path := geminiCLICredsPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no Gemini CLI session found at %s", path)
		}
		return err
	}
	var file struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiryDate   int64  `json:"expiry_date"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("could not read the Gemini CLI session at %s: %w", path, err)
	}
	if file.AccessToken == "" || file.RefreshToken == "" {
		return fmt.Errorf("the Gemini CLI session at %s is not signed in", path)
	}

	cred := Credential{
		Type:      "oauth",
		Access:    file.AccessToken,
		Refresh:   file.RefreshToken,
		ExpiresAt: file.ExpiryDate,
		Endpoint:  CodeAssistBaseURL,
		Label:     "Gemini (from Gemini CLI)",
	}
	// An imported access token is usually already stale. Refresh first so the
	// project lookup below is not the thing that discovers it.
	if cred.Expired() {
		refreshed, refreshErr := refreshGoogle(ctx, cred)
		if refreshErr != nil {
			return refreshErr
		}
		cred = refreshed
	}
	if email := googleEmail(ctx, cred.Access); email != "" {
		cred.Label = "Gemini · " + email
	}
	project, err := resolveCodeAssistProject(ctx, cred.Access)
	if err != nil {
		return err
	}
	cred.Account = project

	return Set("code-assist", cred)
}

// GeminiCLIAvailable reports whether there is a Gemini CLI session to import.
func GeminiCLIAvailable() bool {
	_, err := os.Stat(geminiCLICredsPath())
	return err == nil
}

func geminiCLICredsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".gemini", "oauth_creds.json")
	}
	return filepath.Join(home, ".gemini", "oauth_creds.json")
}
