package account

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"testing"
	"time"
)

// fakeID is enough of the id server to hold the client to its side of the
// contract. It is written from aetox-cloud/auth.go rather than from memory:
// the form fields, the JSON keys and the error codes below are what that
// server actually sends, and a change on either side should break this.
type fakeID struct {
	t *testing.T

	challenge string // captured from the authorize URL the client built
	issued    int    // how many token pairs handed out

	refreshFails  *ServerError // when set, every refresh is refused with this
	spentRefresh  map[string]bool
	signoutCalled bool
	signoutFails  bool
}

func newFakeID(t *testing.T) (*fakeID, *httptest.Server) {
	t.Helper()
	f := &fakeID{t: t, spentRefresh: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	t.Setenv("AETOX_ID_URL", srv.URL)
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	return f, srv
}

func (f *fakeID) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/token":
		f.token(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/me":
		if r.Header.Get("Authorization") == "" {
			f.fail(w, http.StatusUnauthorized, "invalid_token", "no bearer token")
			return
		}
		writeJSONTest(w, http.StatusOK, User{ID: "u1", Name: "Mike", Email: "mike@example.com"})
	case r.Method == http.MethodPost && r.URL.Path == "/signout":
		f.signoutCalled = true
		if f.signoutFails {
			f.fail(w, http.StatusInternalServerError, "server_error", "")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeID) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		f.fail(w, http.StatusBadRequest, "invalid_request", "could not parse the form")
		return
	}
	if got := r.PostForm.Get("client_id"); got != ClientID {
		f.fail(w, http.StatusBadRequest, "invalid_client", "unknown client "+got)
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		// PKCE, checked the way the real server checks it.
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
			f.fail(w, http.StatusBadRequest, "invalid_grant", "code_verifier does not match")
			return
		}
		f.issue(w)
	case "refresh_token":
		presented := r.PostForm.Get("refresh_token")
		if f.refreshFails != nil {
			f.fail(w, f.refreshFails.Status, f.refreshFails.Code, f.refreshFails.Description)
			return
		}
		// Reuse detection: a spent refresh token means a copy leaked, and the
		// whole family dies rather than the second caller being served.
		if f.spentRefresh[presented] {
			f.fail(w, http.StatusBadRequest, "invalid_grant", "this refresh token is not valid")
			return
		}
		f.spentRefresh[presented] = true
		f.issue(w)
	default:
		f.fail(w, http.StatusBadRequest, "unsupported_grant_type", "")
	}
}

func (f *fakeID) issue(w http.ResponseWriter) {
	f.issued++
	writeJSONTest(w, http.StatusOK, map[string]any{
		"access_token":  "access-" + itoa(f.issued),
		"refresh_token": "refresh-" + itoa(f.issued),
		"token_type":    "Bearer",
		"expires_in":    3600,
		"user":          User{ID: "u1", Name: "Mike", Email: "mike@example.com"},
	})
}

func (f *fakeID) fail(w http.ResponseWriter, status int, code, desc string) {
	writeJSONTest(w, status, map[string]string{"error": code, "error_description": desc})
}

func writeJSONTest(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func itoa(n int) string { return string(rune('0' + n)) }

// browse plays the part of the user's browser landing back on the loopback.
func browse(t *testing.T, redirectURI, query string) {
	t.Helper()
	resp, err := http.Get(redirectURI + "?" + query)
	if err != nil {
		t.Fatalf("loopback did not answer: %v", err)
	}
	resp.Body.Close()
}

func startAndCapture(t *testing.T, f *fakeID) *Pending {
	t.Helper()
	p, err := Start("github")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(p.Cancel)
	u, err := url.Parse(p.URL)
	if err != nil {
		t.Fatalf("Start built an unparseable URL: %v", err)
	}
	f.challenge = u.Query().Get("code_challenge")
	return p
}

func TestStartBuildsWhatTheServerWillAccept(t *testing.T) {
	f, srv := newFakeID(t)
	p := startAndCapture(t, f)

	u, _ := url.Parse(p.URL)
	if got := u.Scheme + "://" + u.Host; got != srv.URL {
		t.Errorf("authorize went to %s, want %s", got, srv.URL)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256 — the server rejects anything else", q.Get("code_challenge_method"))
	}
	if q.Get("client_id") != ClientID || q.Get("provider") != "github" {
		t.Errorf("client_id/provider = %q/%q", q.Get("client_id"), q.Get("provider"))
	}
	if f.challenge == "" || q.Get("state") == "" {
		t.Error("challenge and state must both be present")
	}

	// The server matches a native client's redirect by shape: http, loopback
	// host, path /callback, and no query or fragment at all. Getting any part
	// of this wrong fails at the provider with a message that does not say
	// which side is wrong, so it is pinned here.
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		t.Fatalf("redirect_uri unparseable: %v", err)
	}
	if back.Scheme != "http" || back.Path != "/callback" {
		t.Errorf("redirect_uri = %q, want http and /callback", back)
	}
	if back.RawQuery != "" || back.Fragment != "" {
		t.Errorf("redirect_uri carries a query or fragment: %q", back)
	}
	switch back.Hostname() {
	case "127.0.0.1", "::1", "localhost":
	default:
		t.Errorf("redirect_uri host %q is not a loopback address", back.Hostname())
	}
}

func TestStartRejectsADoorTheServerDoesNotHave(t *testing.T) {
	newFakeID(t)
	if _, err := Start("facebook"); err == nil {
		t.Fatal("Start accepted a provider the id server does not offer")
	}
}

func TestSignInStoresTheSession(t *testing.T) {
	f, _ := newFakeID(t)
	p := startAndCapture(t, f)

	u, _ := url.Parse(p.URL)
	browse(t, u.Query().Get("redirect_uri"), url.Values{
		"code":  {"the-code"},
		"state": {u.Query().Get("state")},
	}.Encode())

	sess, err := p.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if sess.Access == "" || sess.Refresh == "" {
		t.Fatal("a completed sign-in must carry both tokens")
	}
	if sess.User.Name != "Mike" {
		t.Errorf("user = %+v", sess.User)
	}
	if sess.ExpiresAt == 0 {
		t.Error("expires_in was not turned into an absolute expiry")
	}

	stored, ok := Current()
	if !ok || stored.ID != "u1" {
		t.Errorf("Current() = %+v, %v — the session did not reach disk", stored, ok)
	}
}

func TestSignInRefusesACodeFromAnotherAttempt(t *testing.T) {
	f, _ := newFakeID(t)
	p := startAndCapture(t, f)

	u, _ := url.Parse(p.URL)
	browse(t, u.Query().Get("redirect_uri"), "code=the-code&state=not-the-state-we-sent")

	if _, err := p.Wait(context.Background()); err == nil {
		t.Fatal("Wait accepted a code carrying somebody else's state")
	}
	if _, ok := Current(); ok {
		t.Error("a refused sign-in must not leave a session behind")
	}
}

func TestTokenRefreshesWhenTheAccessTokenIsNearlyDead(t *testing.T) {
	newFakeID(t)
	must(t, Save(Session{
		Access:    "old-access",
		Refresh:   "old-refresh",
		ExpiresAt: time.Now().Add(time.Minute).UnixMilli(), // inside expiryGrace
		User:      User{ID: "u1", Name: "Mike"},
	}))

	got, err := Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got == "old-access" {
		t.Fatal("Token handed back an access token that expires inside the grace window")
	}
	sess, _ := Load()
	if sess.Refresh == "old-refresh" {
		t.Error("the rotated refresh token was not stored — the next refresh would look like a replay")
	}
	if sess.User.Name != "Mike" {
		t.Errorf("the refresh blanked the user: %+v", sess.User)
	}
}

func TestTokenLeavesAGoodTokenAlone(t *testing.T) {
	f, _ := newFakeID(t)
	must(t, Save(Session{
		Access:    "still-good",
		Refresh:   "r",
		ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
	}))
	got, err := Token(context.Background())
	if err != nil || got != "still-good" {
		t.Fatalf("Token = %q, %v — want the stored token and no network call", got, err)
	}
	if f.issued != 0 {
		t.Error("Token refreshed a token that had not expired")
	}
}

// Being offline is not being signed out. This is the failure mode that would
// hurt most in practice: a laptop on a plane must still be signed in when it
// lands.
func TestAServerThatIsDownDoesNotSignYouOut(t *testing.T) {
	f, srv := newFakeID(t)
	f.refreshFails = &ServerError{Status: http.StatusInternalServerError, Code: "server_error"}
	must(t, Save(Session{Access: "a", Refresh: "r", ExpiresAt: time.Now().UnixMilli()}))

	if _, err := Token(context.Background()); err == nil {
		t.Fatal("Token reported success while the server was refusing")
	}
	if _, ok := Current(); !ok {
		t.Fatal("a 500 cleared the stored session")
	}

	// Same again with the server unreachable rather than merely unhappy.
	srv.Close()
	if _, err := Token(context.Background()); err == nil {
		t.Fatal("Token reported success with nothing listening")
	}
	if _, ok := Current(); !ok {
		t.Error("an unreachable server cleared the stored session")
	}
}

// The other half of that rule: when the server says the grant itself is dead,
// keeping it would mean showing a signed-in name that no longer signs anything.
func TestARevokedRefreshTokenSignsYouOut(t *testing.T) {
	f, _ := newFakeID(t)
	f.refreshFails = &ServerError{
		Status:      http.StatusBadRequest,
		Code:        "invalid_grant",
		Description: "this refresh token is not valid",
	}
	must(t, Save(Session{Access: "a", Refresh: "r", ExpiresAt: time.Now().UnixMilli()}))

	_, err := Token(context.Background())
	if !errors.Is(err, ErrSignedOut) {
		t.Fatalf("Token error = %v, want ErrSignedOut", err)
	}
	if _, ok := Current(); ok {
		t.Error("a revoked session was left on disk")
	}
}

func TestSignOutClearsLocallyEvenWhenTheServerCannotBeTold(t *testing.T) {
	f, _ := newFakeID(t)
	f.signoutFails = true
	must(t, Save(Session{Access: "a", Refresh: "r", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}))

	err := SignOut(context.Background())
	if err == nil {
		t.Error("SignOut reported a clean sign-out that only half happened")
	}
	if !f.signoutCalled {
		t.Error("SignOut never told the server")
	}
	if _, ok := Current(); ok {
		t.Fatal("SignOut left the session on this machine")
	}
}

func TestSignOutOfNothingSucceeds(t *testing.T) {
	newFakeID(t)
	if err := SignOut(context.Background()); err != nil {
		t.Fatalf("SignOut with no session: %v", err)
	}
}

func TestMeUpdatesTheStoredUser(t *testing.T) {
	newFakeID(t)
	must(t, Save(Session{Access: "a", Refresh: "r", ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
		User: User{ID: "u1", Name: "stale"}}))

	u, err := Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if u.Name != "Mike" {
		t.Errorf("Me() = %+v", u)
	}
	if stored, _ := Current(); stored.Name != "Mike" {
		t.Errorf("the fresh answer was not written back: %+v", stored)
	}
}

func TestTheStoreSurvivesARoundTrip(t *testing.T) {
	newFakeID(t)
	want := Session{Access: "a", Refresh: "r", ExpiresAt: 1234, User: User{ID: "u1", Email: "m@example.com"}}
	must(t, Save(want))

	got, ok := Load()
	if !ok || got != want {
		t.Fatalf("Load() = %+v, %v; want %+v", got, ok, want)
	}

	must(t, Clear())
	if _, ok := Load(); ok {
		t.Error("Clear left a session behind")
	}
	if err := Clear(); err != nil {
		t.Errorf("Clear of nothing: %v", err)
	}
}

// The file holds a bearer token whose string is the whole credential, so it is
// held to the same 0600 as oauth.json rather than to whatever the umask says.
func TestTheStoreIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits are not meaningful on windows")
	}
	newFakeID(t)
	must(t, Save(Session{Access: "a"}))
	info, err := os.Stat(StorePath())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("account.json mode = %v; want 0600", perm)
	}
}

func TestACorruptStoreReadsAsSignedOut(t *testing.T) {
	newFakeID(t)
	must(t, Save(Session{Access: "a"}))
	must(t, os.WriteFile(StorePath(), []byte("{not json"), 0o600))
	if _, ok := Load(); ok {
		t.Error("a corrupt account.json must read as signed out, not as a session")
	}
}

func TestDisplayPrefersTheNameThenTheEmail(t *testing.T) {
	for _, tc := range []struct {
		u    User
		want string
	}{
		{User{ID: "u1", Name: "Mike", Email: "m@example.com"}, "Mike"},
		{User{ID: "u1", Email: "m@example.com"}, "m@example.com"},
		{User{ID: "u1"}, "u1"},
	} {
		if got := tc.u.Display(); got != tc.want {
			t.Errorf("Display(%+v) = %q, want %q", tc.u, got, tc.want)
		}
	}
}

func TestBaseURLPrefersTheOverride(t *testing.T) {
	t.Setenv("AETOX_ID_URL", "")
	if BaseURL() != DefaultBaseURL {
		t.Errorf("BaseURL() = %q, want the built-in default", BaseURL())
	}
	t.Setenv("AETOX_ID_URL", "http://localhost:8080/")
	if got := BaseURL(); got != "http://localhost:8080" {
		t.Errorf("BaseURL() = %q — the trailing slash would double up in every path", got)
	}
}

// The switch that keeps the whole feature closed until there is something for
// it to talk to. This test is what should fail on the day DefaultBaseURL is
// filled in, so that opening the feature is a deliberate act with the rest of
// this file re-read rather than a constant edited in passing.
func TestNoServerIsConfiguredInThisBuild(t *testing.T) {
	t.Setenv("AETOX_ID_URL", "")
	if Configured() {
		t.Fatalf("DefaultBaseURL is %q — if the id server is deployed, update this test with the decision that deployed it", DefaultBaseURL)
	}
	if _, err := Start("github"); !errors.Is(err, ErrNotOpen) {
		t.Errorf("Start with no server = %v, want ErrNotOpen — it must not open a listener or a browser", err)
	}
}

func TestAnOverrideOpensIt(t *testing.T) {
	t.Setenv("AETOX_ID_URL", "http://localhost:8080")
	if !Configured() {
		t.Fatal("AETOX_ID_URL is what a checkout uses to run against a local server")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
