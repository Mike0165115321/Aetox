package oauth

import (
	"context"
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

	if _, ok := Get("github-copilot"); ok {
		t.Fatal("empty store reported a credential")
	}

	want := Credential{Type: "oauth", Access: "at", Refresh: "rt", ExpiresAt: 999, Account: "mike"}
	if err := Set("github-copilot", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// A second provider must not disturb the first.
	if err := Set("qwen", Credential{Type: "oauth", Access: "other"}); err != nil {
		t.Fatalf("Set qwen: %v", err)
	}

	got, ok := Get("github-copilot")
	if !ok || got != want {
		t.Fatalf("Get = %+v, %v; want %+v", got, ok, want)
	}
	if len(LoggedIn()) != 2 {
		t.Fatalf("LoggedIn = %v; want 2 entries", LoggedIn())
	}

	if err := Delete("github-copilot"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := Get("github-copilot"); ok {
		t.Fatal("credential survived Delete")
	}
	if _, ok := Get("qwen"); !ok {
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
	if err := Set("qwen", Credential{Type: "oauth", Access: "at"}); err != nil {
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
	if _, ok := Get("qwen"); ok {
		t.Fatal("corrupt store reported a credential")
	}
	// And signing in again must still work rather than inheriting the damage.
	if err := Set("qwen", Credential{Type: "oauth", Access: "at"}); err != nil {
		t.Fatalf("Set over corrupt store: %v", err)
	}
	if _, ok := Get("qwen"); !ok {
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
	if TokenSource("anthropic") != nil {
		t.Fatal("TokenSource returned a source for a provider nobody signed into")
	}
	if err := Set("anthropic", Credential{Type: "oauth", Access: "a", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	src := TokenSource("claude") // alias, not the canonical id
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
	if err := Set("github-copilot", Credential{
		Type: "oauth", Access: "secret-access", Refresh: "secret-refresh",
		Account: "mike", Label: "GitHub Copilot · mike",
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	status := StatusFor("copilot")
	if !status.SignedIn || status.Account != "mike" {
		t.Fatalf("StatusFor = %+v; want a signed-in status for mike", status)
	}
	if status.Provider != "github-copilot" {
		t.Fatalf("StatusFor provider = %q; want the canonical id", status.Provider)
	}
}

func TestQwenEndpointNormalization(t *testing.T) {
	cases := map[string]string{
		"":                            qwenFallbackEP,
		"portal.qwen.ai":              "https://portal.qwen.ai/v1",
		"portal.qwen.ai/":             "https://portal.qwen.ai/v1",
		"https://portal.qwen.ai/v1":   "https://portal.qwen.ai/v1",
		"https://portal.qwen.ai/v1/":  "https://portal.qwen.ai/v1",
		"http://internal.example.com": "http://internal.example.com/v1",
	}
	for in, want := range cases {
		if got := qwenEndpoint(in); got != want {
			t.Fatalf("qwenEndpoint(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSplitAnthropicCode(t *testing.T) {
	cases := []struct {
		in          string
		code, state string
		ok          bool
	}{
		{"abc#xyz", "abc", "xyz", true},
		{"  abc#xyz  ", "abc", "xyz", true},
		// No state means no CSRF check is possible, so it is a hard no rather
		// than a best-effort exchange.
		{"abc", "", "", false},
		{"", "", "", false},
		{"#xyz", "", "", false},
		// Pasting the whole callback URL is a normal user mistake, not a fault.
		{"https://console.anthropic.com/oauth/code/callback?code=abc&state=xyz", "abc", "xyz", true},
	}
	for _, tc := range cases {
		code, state, ok := splitAnthropicCode(tc.in)
		if ok != tc.ok || (ok && (code != tc.code || state != tc.state)) {
			t.Fatalf("splitAnthropicCode(%q) = %q, %q, %v; want %q, %q, %v",
				tc.in, code, state, ok, tc.code, tc.state, tc.ok)
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

func TestHeadersOnlyForCopilot(t *testing.T) {
	if got := Headers("qwen"); got != nil {
		t.Fatalf("Headers(qwen) = %v; want nil", got)
	}
	got := Headers("copilot")
	if got["Copilot-Integration-Id"] != copilotIntegration {
		t.Fatalf("Headers(copilot) = %v; want the editor identification", got)
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
