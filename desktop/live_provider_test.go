package main

import (
	"context"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/safety"
)

// The whole provider chain against a real OpenAI-compatible local server,
// driven through the same App methods the Settings panel calls — not a unit
// test of the pieces, the pieces wired together and actually dialed:
//
//	AETOX_LIVE=1 AETOX_LIVE_BASE_URL=http://127.0.0.1:54998/v1 \
//	  go test ./desktop/ -run TestLiveLMStudio -v -count=1
//
// AETOX_LIVE_BASE_URL points at whatever port the server is really on —
// LM Studio's is configurable, and its llama.cpp runtime picks an ephemeral
// one per loaded model, so hardcoding 1234 would skip on a machine that has a
// server running.
func liveBaseURL(t *testing.T) string {
	t.Helper()
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run live provider tests")
	}
	raw := strings.TrimSpace(os.Getenv("AETOX_LIVE_BASE_URL"))
	if raw == "" {
		raw = model.DefaultBaseURL("lmstudio")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("AETOX_LIVE_BASE_URL is not a URL: %v", err)
	}
	conn, err := net.DialTimeout("tcp", parsed.Host, 2*time.Second)
	if err != nil {
		t.Skipf("nothing listening on %s: %v", parsed.Host, err)
	}
	_ = conn.Close()
	return raw
}

func liveApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // never touch the real preference file
	a := &App{}
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})
	return a
}

// The reported bug, reproduced against the real world: with nothing on the
// catalog default port, switching to lmstudio must come back saying so rather
// than looking like it worked.
func TestLiveLMStudioUnreachableDefaultPortWarns(t *testing.T) {
	liveBaseURL(t) // gate only — this case deliberately uses the catalog default
	a := liveApp(t)
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:1234", time.Second); err == nil {
		_ = conn.Close()
		t.Skip("something IS on :1234 — this case needs the default port dead")
	}

	info, err := a.SwitchProvider("lmstudio")
	if err != nil {
		t.Fatalf("the fallback must keep the app usable: %v", err)
	}
	if info.Warning == "" {
		t.Fatal("switched to a dead endpoint with no warning — the original bug")
	}
	t.Logf("warning: %s", info.Warning)
}

// The same switch once the endpoint points at the server that is really there:
// a model name discovered from the server, no warning, and a real completion
// through the client chat uses.
func TestLiveLMStudioCustomBaseURLConnects(t *testing.T) {
	baseURL := liveBaseURL(t)
	a := liveApp(t)

	if _, err := a.SetProviderBaseURL("lmstudio", baseURL); err != nil {
		t.Fatalf("SetProviderBaseURL: %v", err)
	}
	info, err := a.SwitchProvider("lmstudio")
	if err != nil {
		t.Fatalf("SwitchProvider: %v", err)
	}
	if info.Warning != "" {
		t.Fatalf("warning against a live server: %s", info.Warning)
	}
	if info.ModelName == "" {
		t.Fatal("no model name — discovery never reached the custom endpoint")
	}
	t.Logf("resolved model: %s", info.ModelName)

	// Discovery only proves GET /v1/models. This is a real completion: the
	// endpoint, the wire format, and the model id all have to be right.
	label, err := a.TestProviderConnection("lmstudio", info.ModelName)
	if err != nil {
		t.Fatalf("live completion failed: %v", err)
	}
	t.Logf("completion ok: %s", label)
}

// The reported click, exactly: no override, no custom anything — pick lmstudio
// and expect a working engine. Then a real generation, because a 1-token ping
// on a reasoning model only ever proves the reasoning channel arrived.
func TestLiveLMStudioDefaultEndpointChats(t *testing.T) {
	liveBaseURL(t)
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:1234", 2*time.Second); err != nil {
		t.Skipf("LM Studio's API server is not on :1234: %v", err)
	} else {
		_ = conn.Close()
	}
	a := liveApp(t)

	info, err := a.SwitchProvider("lmstudio")
	if err != nil {
		t.Fatalf("SwitchProvider: %v", err)
	}
	if info.Warning != "" {
		t.Fatalf("warning against a running server: %s", info.Warning)
	}
	if info.ModelName == "" {
		t.Fatal("no model resolved from the running server")
	}

	p, err := model.NewProvider(model.ProviderOptions{
		Provider: "lmstudio",
		Model:    info.ModelName,
		BaseURL:  resolveBaseURLForProvider("lmstudio"),
		Timeout:  120 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resp, err := p.Complete(ctx, model.Request{
		Model:     info.ModelName,
		Messages:  []model.Message{{Role: model.RoleUser, Content: "Reply with exactly: pong"}},
		MaxTokens: 512,
	})
	if err != nil {
		t.Fatalf("real completion failed: %v", err)
	}
	if strings.TrimSpace(resp.Text) == "" {
		t.Fatalf("no answer text (reasoning was %d chars) — this is what chat would show", len(resp.ReasoningContent))
	}
	t.Logf("model %s answered: %q (reasoning %d chars)", info.ModelName, resp.Text, len(resp.ReasoningContent))
}

// The stale-warning bug: switch while the server is down, start the server
// afterwards, and the engine must be able to get off the fallback without the
// user re-selecting the provider by hand. Reported as red text still nagging
// next to a model list that had just loaded from the endpoint it called dead.
func TestLiveLMStudioRecoversAfterServerComesUp(t *testing.T) {
	liveBaseURL(t)
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:1234", 2*time.Second); err != nil {
		t.Skipf("LM Studio's API server is not on :1234: %v", err)
	} else {
		_ = conn.Close()
	}
	a := liveApp(t)

	// Switch against a dead endpoint: this is the state the user was stuck in.
	if _, err := a.SetProviderBaseURL("lmstudio", "http://127.0.0.1:1/v1"); err != nil {
		t.Fatalf("SetProviderBaseURL: %v", err)
	}
	stuck, err := a.SwitchProvider("lmstudio")
	if err != nil {
		t.Fatalf("SwitchProvider: %v", err)
	}
	if stuck.Warning == "" {
		t.Fatal("setup did not reach the fallback state")
	}
	// Retrying while it is still down must stay honest rather than clear the
	// banner on wishful thinking.
	if again := a.RetryActiveProvider(); again.Warning == "" {
		t.Fatal("warning cleared while the endpoint was still dead")
	}

	// The server "comes up". Written straight to the preference file rather
	// than through SetProviderBaseURL, which re-bootstraps on its own and would
	// have done the recovering for us — this has to prove RetryActiveProvider
	// itself does the work, exactly like a server started outside the app.
	pref, _, _ := config.LoadModelPreference()
	pref.SetBaseURLForProvider("lmstudio", "") // back to :1234, which is live
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatalf("save preference: %v", err)
	}
	if a.GetModelInfo().Warning == "" {
		t.Fatal("setup broke: the engine must still be stuck at this point")
	}

	recovered := a.RetryActiveProvider()
	if recovered.Warning != "" {
		t.Fatalf("still warning against a live server: %s", recovered.Warning)
	}
	if recovered.ModelName == "" {
		t.Fatal("recovered with no model — the empty name from the failed bootstrap survived")
	}
	t.Logf("recovered onto %s/%s", recovered.Provider, recovered.ModelName)
}
