package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// isolate points the credential store at a temp directory and clears the
// environment fallback, so a test never reads or writes the developer's own
// GitHub connection.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	for _, key := range envKeys {
		t.Setenv(key, "")
	}
}

// stubGitHub stands in for api.github.com and reports what it was asked.
func stubGitHub(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previous := apiBase
	apiBase = server.URL
	t.Cleanup(func() { apiBase = previous })
}

func okUser(login, scopes string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if scopes != "" {
			w.Header().Set("X-OAuth-Scopes", scopes)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"` + login + `","name":"Mike"}`))
	}
}

func TestConnectStoresTheAccountAndTokenIsReadable(t *testing.T) {
	isolate(t)
	stubGitHub(t, okUser("mike", "repo, read:org"))

	account, err := Connect(context.Background(), "  ghp_example  ")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if account.Login != "mike" {
		t.Fatalf("login = %q, want mike", account.Login)
	}
	if len(account.Scopes) != 2 || account.Scopes[0] != "repo" || account.Scopes[1] != "read:org" {
		t.Fatalf("scopes = %v, want [repo read:org]", account.Scopes)
	}
	// The token is stored trimmed: a value pasted with a trailing newline must
	// not be sent to GitHub with one.
	if got := Token(); got != "ghp_example" {
		t.Fatalf("Token() = %q, want ghp_example", got)
	}
	status := CurrentStatus()
	if !status.Connected || status.Login != "mike" || status.Source != SourceConnection {
		t.Fatalf("status = %+v, want connected as mike via connection", status)
	}
}

func TestConnectSendsBearerAuth(t *testing.T) {
	isolate(t)
	var seen string
	stubGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		okUser("mike", "")(w, r)
	})

	if _, err := Connect(context.Background(), "ghp_example"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if seen != "Bearer ghp_example" {
		t.Fatalf("Authorization = %q, want Bearer ghp_example", seen)
	}
}

// A token that does not work must not reach the store. Otherwise Settings shows
// a connection that fails on every later call, and the user has no reason to
// suspect the thing that says "connected".
func TestConnectRejectsABadTokenWithoutStoringIt(t *testing.T) {
	isolate(t)
	stubGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	if _, err := Connect(context.Background(), "ghp_wrong"); err == nil {
		t.Fatal("Connect accepted a token GitHub rejected")
	} else if !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("error = %q, want it to name the likely causes", err)
	}
	if Token() != "" {
		t.Fatal("a rejected token was stored anyway")
	}
	if CurrentStatus().Connected {
		t.Fatal("status reports connected after a failed connect")
	}
}

func TestConnectRequiresAToken(t *testing.T) {
	isolate(t)
	if _, err := Connect(context.Background(), "   "); err == nil {
		t.Fatal("Connect accepted an empty token")
	}
}

// The env var stays a working fallback — it is how the repo tools were
// authenticated before there was anywhere to connect an account.
func TestEnvironmentTokenIsUsedWhenNothingIsConnected(t *testing.T) {
	isolate(t)
	t.Setenv("GITHUB_TOKEN", "from_env")

	if got := Token(); got != "from_env" {
		t.Fatalf("Token() = %q, want from_env", got)
	}
	status := CurrentStatus()
	if !status.Connected || status.Source != SourceEnvironment || !status.EnvOverride {
		t.Fatalf("status = %+v, want connected via environment", status)
	}
	if status.Login != "" {
		t.Fatalf("login = %q, want empty — an env var says nothing about whose it is", status.Login)
	}
}

func TestGHTokenIsHonouredToo(t *testing.T) {
	isolate(t)
	t.Setenv("GH_TOKEN", "from_gh")
	if got := Token(); got != "from_gh" {
		t.Fatalf("Token() = %q, want from_gh", got)
	}
}

// The account the user connected in the app wins over one exported in a shell
// profile months ago.
func TestConnectionBeatsTheEnvironment(t *testing.T) {
	isolate(t)
	stubGitHub(t, okUser("mike", ""))
	t.Setenv("GITHUB_TOKEN", "from_env")

	if _, err := Connect(context.Background(), "connected"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if got := Token(); got != "connected" {
		t.Fatalf("Token() = %q, want the connected token to win", got)
	}
	if !CurrentStatus().EnvOverride {
		t.Fatal("status hides that an env var is also set")
	}
}

// Disconnecting cannot unset an env var, and must not claim to have.
func TestDisconnectLeavesTheEnvironmentFallbackStanding(t *testing.T) {
	isolate(t)
	stubGitHub(t, okUser("mike", ""))
	if _, err := Connect(context.Background(), "connected"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Setenv("GITHUB_TOKEN", "from_env")

	if err := Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if got := Token(); got != "from_env" {
		t.Fatalf("Token() = %q, want the environment to take over", got)
	}
	status := CurrentStatus()
	if status.Source != SourceEnvironment {
		t.Fatalf("source = %q, want environment after disconnect", status.Source)
	}
}

func TestDisconnectWithNothingConnectedIsNotAnError(t *testing.T) {
	isolate(t)
	if err := Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if CurrentStatus().Connected {
		t.Fatal("status reports connected with an empty store")
	}
}

func TestVerifyNeedsSomethingToVerify(t *testing.T) {
	isolate(t)
	if _, err := Verify(context.Background()); err == nil {
		t.Fatal("Verify succeeded with no token at all")
	}
}

func TestVerifyChecksTheTokenInPlay(t *testing.T) {
	isolate(t)
	stubGitHub(t, okUser("mike", "repo"))
	t.Setenv("GITHUB_TOKEN", "from_env")

	account, err := Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if account.Login != "mike" {
		t.Fatalf("login = %q, want mike", account.Login)
	}
}

// A rate-limited 403 must not read as a permissions problem, or the user goes
// hunting for a scope to tick that would not have helped.
func TestRateLimitDoesNotReadAsAPermissionProblem(t *testing.T) {
	isolate(t)
	stubGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := Connect(context.Background(), "ghp_example")
	if err == nil {
		t.Fatal("Connect succeeded on a 403")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("error = %q, want it to name the rate limit", err)
	}
}

// Fine-grained tokens send no scope header. That is "not stated", and must not
// fail the connection.
func TestFineGrainedTokenWithNoScopeHeaderStillConnects(t *testing.T) {
	isolate(t)
	stubGitHub(t, okUser("mike", ""))

	account, err := Connect(context.Background(), "github_pat_example")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if len(account.Scopes) != 0 {
		t.Fatalf("scopes = %v, want none stated", account.Scopes)
	}
	if !CurrentStatus().Connected {
		t.Fatal("a fine-grained token did not register as connected")
	}
}

// GitHub answering with something that is not an account is a failure, not an
// empty connection stored under a blank name.
func TestGarbageResponseIsRejected(t *testing.T) {
	isolate(t)
	stubGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"login":""}`))
	})

	if _, err := Connect(context.Background(), "ghp_example"); err == nil {
		t.Fatal("Connect stored an account with no login")
	}
	if Token() != "" {
		t.Fatal("token stored despite an unusable response")
	}
}
