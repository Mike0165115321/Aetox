package oauth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// isolateStore points the credential store at a temp dir so a test never reads
// or writes the developer's real logins.
func isolateStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dir)
	return dir
}

func TestStoreRoundTrip(t *testing.T) {
	dir := isolateStore(t)

	if _, ok := Get("openrouter"); ok {
		t.Fatal("empty store reported a credential")
	}

	want := Credential{Type: "oauth", Access: "at", Refresh: "rt", ExpiresAt: 999, Account: "mike"}
	if err := Set("openrouter", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// A second provider must not disturb the first. The store is keyed by a
	// plain string and does not consult the sign-in registry, so a stand-in
	// name is the honest way to say "some other provider" now that OpenRouter
	// is the only real one.
	if err := Set("example-provider", Credential{Type: "api", Key: "other"}); err != nil {
		t.Fatalf("Set example-provider: %v", err)
	}

	got, ok := Get("openrouter")
	if !ok || got != want {
		t.Fatalf("Get = %+v, %v; want %+v", got, ok, want)
	}
	if len(LoggedIn()) != 2 {
		t.Fatalf("LoggedIn = %v; want 2 entries", LoggedIn())
	}

	if err := Delete("openrouter"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := Get("openrouter"); ok {
		t.Fatal("credential survived Delete")
	}
	if _, ok := Get("example-provider"); !ok {
		t.Fatal("Delete removed the wrong provider")
	}

	if got, want := StorePath(), filepath.Join(dir, "oauth.json"); got != want {
		t.Fatalf("StorePath = %q; want %q", got, want)
	}
}

// The store holds refresh tokens: anything readable by other users on the
// machine is an account takeover waiting to happen.
func TestStoreFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits are not meaningful on windows")
	}
	isolateStore(t)
	if err := Set("openrouter", Credential{Type: "oauth", Access: "at"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	info, err := os.Stat(StorePath())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("store mode = %v; want 0600", perm)
	}
}

func TestCorruptStoreReadsAsSignedOut(t *testing.T) {
	dir := isolateStore(t)
	if err := os.WriteFile(filepath.Join(dir, "oauth.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, ok := Get("openrouter"); ok {
		t.Fatal("corrupt store reported a credential")
	}
	// And signing in again must still work rather than inheriting the damage.
	if err := Set("openrouter", Credential{Type: "oauth", Access: "at"}); err != nil {
		t.Fatalf("Set over corrupt store: %v", err)
	}
	if _, ok := Get("openrouter"); !ok {
		t.Fatal("credential did not survive a rewrite over a corrupt store")
	}
}

func TestCredentialExpired(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		cred Credential
		want bool
	}{
		{"api key never expires", Credential{Type: "api", Key: "sk-x"}, false},
		{"no expiry recorded", Credential{Type: "oauth", Access: "a"}, false},
		{"fresh", Credential{Type: "oauth", ExpiresAt: now.Add(time.Hour).UnixMilli()}, false},
		{"already past", Credential{Type: "oauth", ExpiresAt: now.Add(-time.Minute).UnixMilli()}, true},
		// The grace window is the point: a token with 30 seconds left would die
		// mid-turn, so it counts as expired now.
		{"inside grace window", Credential{Type: "oauth", ExpiresAt: now.Add(30 * time.Second).UnixMilli()}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cred.Expired(); got != tc.want {
				t.Fatalf("Expired = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestTokenRefreshesAndPersists(t *testing.T) {
	isolateStore(t)

	calls := 0
	refreshers["test-refreshable"] = func(_ context.Context, cred Credential) (Credential, error) {
		calls++
		cred.Access = "fresh-token"
		cred.ExpiresAt = time.Now().Add(time.Hour).UnixMilli()
		return cred, nil
	}
	t.Cleanup(func() { delete(refreshers, "test-refreshable") })

	stale := Credential{Type: "oauth", Access: "stale", Refresh: "rt", ExpiresAt: time.Now().Add(-time.Minute).UnixMilli()}
	if err := Set("test-refreshable", stale); err != nil {
		t.Fatalf("Set: %v", err)
	}

	token, err := Token(context.Background(), "test-refreshable")
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "fresh-token" {
		t.Fatalf("Token = %q; want the refreshed one", token)
	}

	// Second call must reuse the stored result — re-minting on every turn is
	// how you get rate-limited by your own auth server.
	if _, err := Token(context.Background(), "test-refreshable"); err != nil {
		t.Fatalf("Token (second): %v", err)
	}
	if calls != 1 {
		t.Fatalf("refresher ran %d times; want 1", calls)
	}
	if cred, _ := Get("test-refreshable"); cred.Access != "fresh-token" {
		t.Fatalf("refreshed token was not persisted: %+v", cred)
	}
}

func TestTokenAPIKeyCredentialNeedsNoRefresher(t *testing.T) {
	isolateStore(t)
	if err := Set("openrouter", Credential{Type: "api", Key: "sk-or-v1-x"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := Token(context.Background(), "openrouter")
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "sk-or-v1-x" {
		t.Fatalf("Token = %q; want the stored key", got)
	}
}

func TestTokenSourceNilWhenSignedOut(t *testing.T) {
	isolateStore(t)
	if TokenSource("openrouter") != nil {
		t.Fatal("TokenSource returned a source for a provider nobody signed into")
	}
	if err := Set("openrouter", Credential{Type: "oauth", Access: "a", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	src := TokenSource("open-router") // alias, not the canonical id
	if src == nil {
		t.Fatal("TokenSource returned nil for a signed-in provider")
	}
	got, err := src(context.Background())
	if err != nil || got != "a" {
		t.Fatalf("source = %q, %v; want the stored token", got, err)
	}
}

func TestStatusForNeverLeaksTokens(t *testing.T) {
	isolateStore(t)
	if err := Set("openrouter", Credential{
		Type: "oauth", Access: "secret-access", Refresh: "secret-refresh",
		Account: "mike", Label: "OpenRouter · mike",
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	status := StatusFor("open-router")
	if !status.SignedIn || status.Account != "mike" {
		t.Fatalf("StatusFor = %+v; want a signed-in status for mike", status)
	}
	if status.Provider != "openrouter" {
		t.Fatalf("StatusFor provider = %q; want the canonical id", status.Provider)
	}
}

// Credentials for the four sign-ins that stayed removed — Claude Pro/Max,
// Copilot, Qwen and Gemini Code Assist — may still sit in an oauth.json written
// by an older version. They must read as signed out: never refreshed, never
// sent.
//
// ChatGPT is in the same file and is the control: §69 brought it back, so a
// credential §64 would have dropped is read again rather than discarded.
func TestRemovedProviderCredentialsAreDropped(t *testing.T) {
	dir := isolateStore(t)
	raw := `{
  "anthropic":      {"type": "oauth", "access": "a", "refresh": "r"},
  "codex":          {"type": "oauth", "access": "a", "refresh": "r"},
  "github-copilot": {"type": "oauth", "access": "a", "refresh": "r"},
  "qwen":           {"type": "oauth", "access": "a", "refresh": "r"},
  "code-assist":    {"type": "oauth", "access": "a", "refresh": "r"},
  "openrouter":     {"type": "api",   "key":    "keep-me"}
}`
	if err := os.WriteFile(filepath.Join(dir, "oauth.json"), []byte(raw), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, provider := range []string{"anthropic", "github-copilot", "qwen", "code-assist"} {
		if _, ok := Get(provider); ok {
			t.Fatalf("removed provider %q still reads as signed in", provider)
		}
		if TokenSource(provider) != nil {
			t.Fatalf("removed provider %q still yields a token source", provider)
		}
	}
	for _, provider := range []string{"openrouter", "codex"} {
		if _, ok := Get(provider); !ok {
			t.Fatalf("surviving provider %q was dropped along with the removed ones", provider)
		}
	}
	if got := LoggedIn(); len(got) != 2 {
		t.Fatalf("LoggedIn = %v; want openrouter and codex", got)
	}
}

// Every removed sign-in must also be gone from the registry — a Method left
// behind would draw a button that starts a flow Start() no longer knows.
func TestRemovedProvidersHaveNoSignIn(t *testing.T) {
	for provider := range removedProviders {
		if _, ok := methods[provider]; ok {
			t.Fatalf("removed provider %q still offers a sign-in method", provider)
		}
		if _, ok := refreshers[provider]; ok {
			t.Fatalf("removed provider %q still has a refresher", provider)
		}
		if _, err := Start(context.Background(), provider); err == nil {
			t.Fatalf("Start accepted removed provider %q", provider)
		}
	}
}

// fastPoll shrinks the poll interval so the state machine can be driven in
// milliseconds instead of the 20 real seconds three ticks would otherwise take.
func fastPoll(t *testing.T) {
	t.Helper()
	original := pollFloor
	pollFloor = 5 * time.Millisecond
	t.Cleanup(func() { pollFloor = original })
}

func TestPollDeviceCodeHandlesPendingAndSlowDown(t *testing.T) {
	fastPoll(t)
	pending := &Pending{expiresIn: 60}

	replies := []tokenResponse{
		{Error: "authorization_pending"},
		{Error: "slow_down"},
		{AccessToken: "gho_token"},
	}
	calls := 0
	post := func(_ context.Context, _ string, _ url.Values, out any) error {
		reply := replies[min(calls, len(replies)-1)]
		calls++
		*(out.(*tokenResponse)) = reply
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := pollDeviceCode(ctx, pending, "https://example.invalid/token", url.Values{}, post)
	if err != nil {
		t.Fatalf("pollDeviceCode: %v", err)
	}
	if got.AccessToken != "gho_token" {
		t.Fatalf("token = %q; want gho_token", got.AccessToken)
	}
	if calls != 3 {
		t.Fatalf("post called %d times; want 3 (pending, slow_down, success)", calls)
	}
}

func TestPollDeviceCodeSurfacesDenial(t *testing.T) {
	fastPoll(t)
	pending := &Pending{expiresIn: 30}
	post := func(_ context.Context, _ string, _ url.Values, out any) error {
		*(out.(*tokenResponse)) = tokenResponse{Error: "access_denied"}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := pollDeviceCode(ctx, pending, "https://example.invalid/token", url.Values{}, post); err == nil {
		t.Fatal("a denied sign-in reported success")
	}
}

func TestPollDeviceCodeIgnoresTransportBlips(t *testing.T) {
	fastPoll(t)
	pending := &Pending{expiresIn: 60}
	calls := 0
	post := func(_ context.Context, _ string, _ url.Values, out any) error {
		calls++
		if calls == 1 {
			// A dropped connection must not throw away a login the user has
			// already approved.
			return errors.New("connection reset")
		}
		*(out.(*tokenResponse)) = tokenResponse{AccessToken: "ok"}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := pollDeviceCode(ctx, pending, "https://example.invalid/token", url.Values{}, post)
	if err != nil {
		t.Fatalf("pollDeviceCode: %v", err)
	}
	if got.AccessToken != "ok" {
		t.Fatalf("token = %q; want ok", got.AccessToken)
	}
}

func TestMethodsCoverEveryRefresher(t *testing.T) {
	// A refresher without a sign-in is dead code; a sign-in whose credential
	// expires without a refresher strands the user on a dead token.
	for provider := range refreshers {
		if _, ok := methods[provider]; !ok {
			t.Fatalf("refresher %q has no sign-in method", provider)
		}
	}
	for provider, method := range methods {
		if method.Provider != provider {
			t.Fatalf("method %q carries provider %q", provider, method.Provider)
		}
		switch method.Kind {
		case "device", "browser", "paste":
		default:
			t.Fatalf("method %q has unknown kind %q", provider, method.Kind)
		}
	}
}

func TestStartRejectsUnknownProvider(t *testing.T) {
	if _, err := Start(context.Background(), "ollama"); err == nil {
		t.Fatal("Start accepted a provider with no sign-in")
	}
}

// The ChatGPT backend routes on an account id that appears nowhere in the token
// response body — only in a namespaced claim inside the id_token. Losing it
// means every request 401s, so the extraction is pinned here rather than left
// to the live test.
func TestCodexAccountFromIDToken(t *testing.T) {
	claims := `{"email":"mike@example.com","https://api.openai.com/auth":{"chatgpt_account_id":"acct_42","chatgpt_plan_type":"plus"}}`
	idToken := "ignored." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".sig"

	account, email := codexAccountFromIDToken(idToken)
	if account != "acct_42" {
		t.Fatalf("account = %q; want acct_42", account)
	}
	if email != "mike@example.com" {
		t.Fatalf("email = %q; want the address for the Settings label", email)
	}

	// A malformed token must degrade to empty rather than panic — a refresh
	// response often omits id_token entirely.
	if account, _ := codexAccountFromIDToken("not-a-jwt"); account != "" {
		t.Fatalf("account = %q on a malformed token; want empty", account)
	}
}

// Headers is what carries that account id onto the wire. Only codex has any:
// every other provider must send nothing extra.
func TestHeadersOnlyForCodex(t *testing.T) {
	isolateStore(t)
	if got := Headers("openrouter"); got != nil {
		t.Fatalf("Headers(openrouter) = %v; want nil", got)
	}

	if err := Set("codex", Credential{Type: "oauth", Access: "a", Account: "acct_42"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := Headers("chatgpt-codex")
	if got["chatgpt-account-id"] != "acct_42" {
		t.Fatalf("Headers(codex) = %v; want the account id from the sign-in", got)
	}
	if got["originator"] != CodexOriginator {
		t.Fatalf("Headers(codex) = %v; want the originator the backend requires", got)
	}
}
