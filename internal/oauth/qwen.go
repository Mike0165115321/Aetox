package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Qwen sign-in — device code, same shape as Copilot's, with two differences
// that matter: it is PKCE-protected (the verifier goes to the token endpoint,
// not the device endpoint), and the account's inference host is handed back at
// login as resource_url rather than being a fixed catalog entry.
const (
	qwenClientID   = "f0304373b74a44d2b584a3fb70ca9e56" // the qwen-code CLI's public client
	qwenDeviceURL  = "https://chat.qwen.ai/api/v1/oauth2/device/code"
	qwenTokenURL   = "https://chat.qwen.ai/api/v1/oauth2/token"
	qwenScope      = "openid profile email model.completion"
	qwenGrantType  = "urn:ietf:params:oauth:grant-type:device_code"
	qwenFallbackEP = "https://dashscope.aliyuncs.com/compatible-mode/v1"
)

// StartQwen asks for a device code. Pending.URL is the "already filled in"
// verification link when Qwen provides one, so most users never type the code.
func StartQwen(ctx context.Context) (*Pending, error) {
	verifier, challenge, err := newPKCE()
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("client_id", qwenClientID)
	form.Set("scope", qwenScope)
	form.Set("code_challenge", challenge)
	form.Set("code_challenge_method", "S256")

	var out struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		CompleteURI     string `json:"verification_uri_complete"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := qwenForm(ctx, qwenDeviceURL, form, &out); err != nil {
		return nil, fmt.Errorf("qwen sign-in failed: %w", err)
	}
	if out.DeviceCode == "" {
		return nil, errors.New("qwen returned no device code")
	}

	link := out.CompleteURI
	if link == "" {
		link = out.VerificationURI
	}
	return &Pending{
		URL:             link,
		UserCode:        out.UserCode,
		VerificationURI: out.VerificationURI,
		Verifier:        verifier,
		deviceCode:      out.DeviceCode,
		interval:        out.Interval,
		expiresIn:       out.ExpiresIn,
		provider:        "qwen",
	}, nil
}

// FinishQwen blocks until the user approves, then stores the login.
func FinishQwen(ctx context.Context, pending *Pending) error {
	if pending == nil || pending.deviceCode == "" {
		return errors.New("no sign-in in progress")
	}
	tokens, err := pollDeviceCode(ctx, pending, qwenTokenURL, url.Values{
		"grant_type":    {qwenGrantType},
		"client_id":     {qwenClientID},
		"device_code":   {pending.deviceCode},
		"code_verifier": {pending.Verifier},
	}, qwenForm)
	if err != nil {
		return err
	}
	return Set("qwen", Credential{
		Type:      "oauth",
		Access:    tokens.AccessToken,
		Refresh:   tokens.RefreshToken,
		ExpiresAt: tokens.expiresAt(),
		Endpoint:  qwenEndpoint(tokens.ResourceURL),
		Label:     "Qwen",
	})
}

func refreshQwen(ctx context.Context, cred Credential) (Credential, error) {
	if strings.TrimSpace(cred.Refresh) == "" {
		return Credential{}, errors.New("qwen sign-in has no refresh token — sign in again")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cred.Refresh)
	form.Set("client_id", qwenClientID)

	var tokens tokenResponse
	if err := qwenForm(ctx, qwenTokenURL, form, &tokens); err != nil {
		return Credential{}, fmt.Errorf("qwen token refresh failed: %w", err)
	}
	if tokens.AccessToken == "" {
		return Credential{}, errors.New("qwen token refresh returned no access token")
	}
	cred.Access = tokens.AccessToken
	if tokens.RefreshToken != "" {
		cred.Refresh = tokens.RefreshToken
	}
	cred.ExpiresAt = tokens.expiresAt()
	if ep := qwenEndpoint(tokens.ResourceURL); ep != "" {
		// The account can be moved between hosts between refreshes; the newest
		// answer wins over whatever login recorded.
		cred.Endpoint = ep
	}
	return cred, nil
}

// qwenEndpoint normalizes resource_url, which arrives as a bare host far more
// often than as a URL, and rarely with the /v1 the OpenAI-compatible client
// needs.
func qwenEndpoint(resourceURL string) string {
	host := strings.TrimSpace(resourceURL)
	if host == "" {
		return qwenFallbackEP
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	host = strings.TrimSuffix(host, "/")
	if !strings.HasSuffix(host, "/v1") {
		host += "/v1"
	}
	return host
}

func qwenForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	return readJSON(resp, out)
}
