package oauth

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live smoke test against the real authorization servers, skipped unless
// AETOX_LIVE=1.
//
//	AETOX_LIVE=1 go test ./internal/oauth/ -run TestLiveSignInStarts -v -count=1
//
// Unit tests can prove the state machine and prove nothing at all about the
// part most likely to break: whether the client ids and endpoints these flows
// are pinned to still exist. They belong to other companies' products and can
// be retired without notice, and the symptom on a user's machine is a sign-in
// button that fails with a bare 404.
//
// Only the opening request of each flow is made — it asks for a device code
// and never approves it, so nothing is authorized and no account is touched.
// The codes expire on their own.
func TestLiveSignInStarts(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against the real authorization servers")
	}
	isolateStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("qwen", func(t *testing.T) {
		pending, err := StartQwen(ctx)
		if err != nil {
			t.Fatalf("StartQwen: %v", err)
		}
		if pending.UserCode == "" || pending.deviceCode == "" {
			t.Fatalf("device code response was empty: %+v", pending)
		}
		t.Logf("qwen device code issued (user code %s)", pending.UserCode)
	})

	// OpenRouter builds its authorize URL locally, so the live question for it
	// is whether that URL is still served at all. A GET that is not a redirect
	// to a login page means the flow moved.
	t.Run("openrouter authorize page", func(t *testing.T) {
		pending, err := StartOpenRouter()
		if err != nil {
			t.Fatalf("StartOpenRouter: %v", err)
		}
		// Releases the local listener the flow opened for the redirect.
		defer pending.Cancel()
		assertReachable(ctx, t, pending.URL)
	})
}

func assertReachable(ctx context.Context, t *testing.T, url string) {
	t.Helper()
	req, err := newGetRequest(ctx, url)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 || resp.StatusCode == 404 {
		t.Fatalf("GET %s answered %s — the flow may have moved", url, resp.Status)
	}
	t.Logf("%s answered %s", url, resp.Status)
}
