package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Pending is an authorization in flight: the URL the user has to visit, and
// the PKCE secret that proves the code they come back with belongs to this
// machine. It has to survive the round trip through the UI, so it is a value
// the caller holds rather than package state — two windows can start two
// logins without stepping on each other.
type Pending struct {
	URL      string `json:"url"`
	Verifier string `json:"-"`
	State    string `json:"-"`
	// UserCode and VerificationURI are set by device-code flows only: there
	// the user types a short code into a page instead of following a URL that
	// already carries the request.
	UserCode        string `json:"user_code,omitempty"`
	VerificationURI string `json:"verification_uri,omitempty"`

	interval   int
	expiresIn  int
	provider   string
	// lb is the local listener waiting for a redirect, for the flows that get
	// to choose their own redirect URI. Nil for device-code and paste flows.
	lb *loopback
}

// Provider is the canonical id this sign-in will store under.
func (p *Pending) Provider() string { return p.provider }

// Cancel releases anything the sign-in is holding — safe on any Pending, and
// safe to call after Finish.
func (p *Pending) Cancel() {
	if p != nil && p.lb != nil {
		p.lb.close()
		p.lb = nil
	}
}

// newPKCE returns a fresh verifier and its S256 challenge.
func newPKCE() (verifier, challenge string, err error) {
	verifier, err = randomString(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func randomString(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// httpClient is shared by every flow. The timeout is per request, not per
// flow: device-code polling makes many short requests over several minutes.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// clientUserAgent identifies Aetox to authorization servers. Not cosmetic:
// some edges answer Go's default "Go-http-client/1.1" with an HTML block page,
// which surfaces as "invalid character '<'" on an endpoint that works fine
// from curl. Anything that names a client gets through.
const clientUserAgent = "Aetox"

// readJSON decodes a successful response, or turns a non-2xx into an error
// carrying the body — a bare "401" from a token endpoint is unactionable,
// the body usually says exactly which parameter was rejected.
func readJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// tokenResponse is the shape every one of these flows returns, give or take
// the fields each provider adds.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func (t tokenResponse) expiresAt() int64 {
	if t.ExpiresIn <= 0 {
		return 0
	}
	return time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).UnixMilli()
}

// newGetRequest is a tiny helper the live sign-in test uses to check that an
// authorize page is still served.
func newGetRequest(ctx context.Context, url string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
}
