// Package account is the Aetox account — one row on Aetox's own id server,
// opened through GitHub or Google but owned by neither.
//
// The app works fully signed out. Nothing here gates a tool, a model or a
// desk; the account exists so that a later store can know who bought what, and
// so the same person is the same person on the app and on the website.
//
// It is deliberately not part of internal/oauth. That package answers "what do
// I send as credentials for this model provider" and internal/model reads it to
// build a request. What it lends here is transport only — the loopback
// listener and PKCE — because the id server speaks the same RFC 8252 shape.
package account

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Mike0165115321/Aetox/internal/oauth"
)

// ClientID is what this build calls itself to the id server. The server holds
// the matching row and marks it a native app, which is what lets the redirect
// below be a loopback address on a port picked at run time.
const ClientID = "aetox-desktop"

// DefaultBaseURL is the deployed id server, and it is **deliberately empty**.
//
// Nothing is hosted yet. An empty base URL is what closes the whole feature:
// Configured reports false, the desktop leaves the account page out of Settings
// and the sign-in out of the sidebar, and the CLI says so rather than dialling
// a host that does not exist. A button that reaches nothing is the placeholder
// that lies, which the 1.0.0 bar forbids by name.
//
// Opening it is one line: put the deployed origin here. Until then AETOX_ID_URL
// turns it on for a checkout running against a local server.
const DefaultBaseURL = ""

// Configured reports whether this build has an id server to talk to at all.
// Every surface that could show a sign-in asks this first.
func Configured() bool { return BaseURL() != "" }

// ErrSignedOut means there is no usable session — either none was stored, or
// the server refused the one that was. Callers show a sign-in button; they do
// not retry.
var ErrSignedOut = errors.New("ยังไม่ได้เข้าสู่ระบบ Aetox")

// ErrNotOpen means this build has no id server. It is not a failure to sign in;
// there is nothing to sign in to yet.
var ErrNotOpen = errors.New("ระบบบัญชี Aetox ยังไม่เปิดใช้งานในรุ่นนี้")

// expiryGrace refreshes an access token before it dies rather than after. The
// server issues them for an hour; a long turn started at minute 58 should not
// discover the expiry halfway through.
const expiryGrace = 5 * time.Minute

var httpClient = &http.Client{Timeout: 30 * time.Second}

// BaseURL is the id server this build talks to.
func BaseURL() string {
	if v := strings.TrimSpace(os.Getenv("AETOX_ID_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return DefaultBaseURL
}

// Providers are the doors the id server offers. They are doors, not owners:
// the token each one returns is used once to ask who knocked and is then
// dropped, so signing in through GitHub today and Google tomorrow lands on the
// same Aetox account as long as the address is one the provider verified.
func Providers() []string { return []string{"github", "google"} }

// ServerError is a refusal the id server stated in its own words. Status and
// Code are what callers branch on; Description is written for a person.
type ServerError struct {
	Status      int
	Code        string
	Description string
}

func (e *ServerError) Error() string {
	if e.Description != "" {
		return e.Description
	}
	if e.Code != "" {
		return e.Code
	}
	return fmt.Sprintf("id server returned %d", e.Status)
}

// fatal reports whether this refusal means the stored session is dead rather
// than that the request went wrong. Only these clear the session — a timeout,
// a DNS failure or a 500 must leave a signed-in laptop signed in, because
// "you were offline for an afternoon" is not "you were signed out".
func (e *ServerError) fatal() bool {
	switch e.Code {
	case "invalid_grant", "invalid_token", "access_denied", "invalid_client":
		return true
	}
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

// Pending is a sign-in in flight: the URL the browser has to visit, and the
// PKCE secret proving the code that comes back belongs to this attempt. It is
// a value the caller holds rather than package state, so two windows can each
// start one.
type Pending struct {
	URL      string
	provider string
	verifier string
	state    string
	lb       *oauth.Loopback
}

// Provider names the door this attempt went through.
func (p *Pending) Provider() string { return p.provider }

// Cancel releases the listener. Safe on nil, safe after Wait.
func (p *Pending) Cancel() {
	if p != nil && p.lb != nil {
		p.lb.Close()
		p.lb = nil
	}
}

// Start opens a sign-in and returns the URL to put in front of the user.
func Start(provider string) (*Pending, error) {
	if !Configured() {
		return nil, ErrNotOpen
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !slices.Contains(Providers(), provider) {
		return nil, fmt.Errorf("ไม่รู้จักช่องทางเข้าสู่ระบบ %q", provider)
	}
	verifier, challenge, err := oauth.NewPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomState()
	if err != nil {
		return nil, err
	}
	// Port 0 and the bare path /callback are not arbitrary: they are the shape
	// the server's loopback client matches on, and it accepts no query or
	// fragment attached to it.
	lb, err := oauth.StartLoopback(0, "/callback")
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("client_id", ClientID)
	q.Set("provider", provider)
	q.Set("redirect_uri", lb.RedirectURI)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")

	return &Pending{
		URL:      BaseURL() + "/authorize?" + q.Encode(),
		provider: provider,
		verifier: verifier,
		state:    state,
		lb:       lb,
	}, nil
}

// Wait blocks until the browser comes back, exchanges the code, and stores the
// session. Cancel ctx to give up.
func (p *Pending) Wait(ctx context.Context) (Session, error) {
	defer p.Cancel()
	code, state, err := p.lb.Wait(ctx)
	if err != nil {
		return Session{}, err
	}
	// The state check is what stops something else on this machine from firing
	// a code of its own at the listener while a real sign-in is open.
	if state != p.state {
		return Session{}, errors.New("การเข้าสู่ระบบนี้ไม่ตรงกับที่เริ่มไว้")
	}
	sess, err := postToken(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ClientID},
		"code":          {code},
		"code_verifier": {p.verifier},
		"redirect_uri":  {p.lb.RedirectURI},
	})
	if err != nil {
		return Session{}, err
	}
	if err := Save(sess); err != nil {
		return Session{}, err
	}
	return sess, nil
}

// refreshMu makes refreshing single-flight within this process, and it is load
// bearing rather than tidy. The server rotates the refresh token on every use
// and treats a second presentation of a spent one as a leaked copy, revoking
// the whole family. Two turns refreshing at the same instant would send the
// same token twice and sign the user out everywhere.
var refreshMu sync.Mutex

// Token returns an access token good for the next few minutes, refreshing it
// first if it is close to expiry. ErrSignedOut means there is nothing to
// refresh and the caller should offer a sign-in.
func Token(ctx context.Context) (string, error) {
	refreshMu.Lock()
	defer refreshMu.Unlock()

	sess, ok := Load()
	if !ok {
		return "", ErrSignedOut
	}
	if sess.ExpiresAt == 0 || time.Now().Add(expiryGrace).UnixMilli() < sess.ExpiresAt {
		return sess.Access, nil
	}
	if sess.Refresh == "" {
		return "", ErrSignedOut
	}
	next, err := postToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {ClientID},
		"refresh_token": {sess.Refresh},
	})
	if err != nil {
		var se *ServerError
		if errors.As(err, &se) && se.fatal() {
			_ = Clear()
			return "", fmt.Errorf("%w: %s", ErrSignedOut, se.Error())
		}
		return "", err
	}
	// A refresh does not always resend the user. Keep the one already on disk
	// rather than blanking the name in Settings.
	if next.User.ID == "" {
		next.User = sess.User
	}
	if err := Save(next); err != nil {
		return "", err
	}
	return next.Access, nil
}

// Me asks the server who this token belongs to and updates what is on disk.
// Current() is the offline answer; this one is for "is that still true".
func Me(ctx context.Context) (User, error) {
	token, err := Token(ctx)
	if err != nil {
		return User{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL()+"/me", nil)
	if err != nil {
		return User{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return User{}, err
	}
	var u User
	if err := readJSON(resp, &u); err != nil {
		var se *ServerError
		if errors.As(err, &se) && se.fatal() {
			_ = Clear()
			return User{}, fmt.Errorf("%w: %s", ErrSignedOut, se.Error())
		}
		return User{}, err
	}
	if sess, ok := Load(); ok && u.ID != "" {
		sess.User = u
		_ = Save(sess)
	}
	return u, nil
}

// SignOut throws the session away here and, if it can be reached, on the
// server too.
//
// The local half always happens. A returned error means the server was not
// told, so the refresh token stays live there until it expires — worth saying
// out loud rather than reporting a clean sign-out that only half happened.
func SignOut(ctx context.Context) error {
	sess, ok := Load()
	if err := Clear(); err != nil {
		return err
	}
	if !ok {
		return nil
	}
	token := sess.Refresh
	if token == "" {
		token = sess.Access
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL()+"/signout",
		strings.NewReader(url.Values{"token": {token}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	return readJSON(resp, nil)
}

const userAgent = "Aetox"

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	User         User   `json:"user"`
}

func postToken(ctx context.Context, form url.Values) (Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL()+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Session{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return Session{}, err
	}
	var out tokenResponse
	if err := readJSON(resp, &out); err != nil {
		return Session{}, err
	}
	if out.AccessToken == "" {
		return Session{}, errors.New("id server ตอบกลับมาโดยไม่มีโทเคน")
	}
	sess := Session{Access: out.AccessToken, Refresh: out.RefreshToken, User: out.User}
	if out.ExpiresIn > 0 {
		sess.ExpiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second).UnixMilli()
	}
	return sess, nil
}

// readJSON decodes a success, or turns a refusal into a ServerError carrying
// what the server actually said. A bare "400" from a token endpoint is
// unactionable; the body names the parameter that was rejected.
func readJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		se := &ServerError{Status: resp.StatusCode}
		var wire struct {
			Error string `json:"error"`
			Desc  string `json:"error_description"`
		}
		if json.Unmarshal(body, &wire) == nil {
			se.Code, se.Description = wire.Error, wire.Desc
		}
		if se.Description == "" && se.Code == "" {
			se.Description = strings.TrimSpace(string(body))
		}
		return se
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

func randomState() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
