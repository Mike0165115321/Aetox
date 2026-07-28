package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// OpenRouter sign-in.
//
// The odd one out, and the good kind of odd: OpenRouter documents this flow
// publicly, needs no client id, and what comes back is a real API key the user
// owns and can revoke on their dashboard. Nothing expires, so there is no
// refresher for it — the credential is stored with type "api".
const (
	openRouterAuthorizeURL = "https://openrouter.ai/auth"
	openRouterKeysURL      = "https://openrouter.ai/api/v1/auth/keys"
)

// StartOpenRouter opens a local listener for the redirect and returns the URL
// to send the user to. Call FinishOpenRouter (or Pending.Cancel) after —
// otherwise the listener stays up for the life of the process.
func StartOpenRouter() (*Pending, error) {
	verifier, challenge, err := newPKCE()
	if err != nil {
		return nil, err
	}
	lb, err := startLoopback(0, "/callback")
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("callback_url", lb.RedirectURI)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")

	return &Pending{
		URL:      openRouterAuthorizeURL + "?" + q.Encode(),
		Verifier: verifier,
		provider: "openrouter",
		lb:       lb,
	}, nil
}

// FinishOpenRouter blocks until the browser comes back, exchanges the code for
// an API key, and stores it.
func FinishOpenRouter(ctx context.Context, pending *Pending) error {
	if pending == nil || pending.lb == nil {
		return errors.New("no sign-in in progress")
	}
	defer pending.Cancel()

	code, _, err := pending.lb.wait(ctx)
	if err != nil {
		return err
	}

	body, err := json.Marshal(map[string]string{
		"code":                  code,
		"code_verifier":         pending.Verifier,
		"code_challenge_method": "S256",
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterKeysURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	var out struct {
		Key string `json:"key"`
	}
	if err := readJSON(resp, &out); err != nil {
		return fmt.Errorf("openrouter sign-in failed: %w", err)
	}
	if out.Key == "" {
		return errors.New("openrouter returned no key")
	}
	return Set("openrouter", Credential{
		Type:  "api",
		Key:   out.Key,
		Label: "OpenRouter",
	})
}
