package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/oauth"
)

// Live round trips against the two subscription backends, skipped unless
// AETOX_LIVE=1.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveSubscription -v -count=1
//
// Everything else in this package proves Aetox builds the request it meant to.
// Only this proves the request is one the provider accepts — which for a
// reverse-engineered wire format is the whole question. It asks a real model a
// real question and requires a real answer.
//
// Each subtest signs in by adopting the session the matching official CLI
// already has on this machine, into an isolated credential store: the test
// never reads or writes the developer's real oauth.json, and it never needs an
// interactive browser flow.

func TestLiveSubscriptionAsksAndAnswers(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against real subscription backends")
	}

	t.Run("codex", func(t *testing.T) {
		t.Setenv("AETOX_DATA_ROOT", t.TempDir())
		if !oauth.CodexCLIAvailable() {
			t.Skip("no Codex CLI session on this machine to adopt")
		}
		if err := oauth.ImportCodexCLI(); err != nil {
			t.Fatalf("ImportCodexCLI: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		p, err := NewProvider(ProviderOptions{
			Provider: "codex",
			Model:    "gpt-5.5",
			Timeout:  90 * time.Second,
		})
		if err != nil {
			t.Fatalf("NewProvider: %v", err)
		}
		assertAnswersLive(ctx, t, p)
	})

	t.Run("code-assist", func(t *testing.T) {
		t.Setenv("AETOX_DATA_ROOT", t.TempDir())
		if !oauth.GeminiCLIAvailable() {
			t.Skip("no Gemini CLI session on this machine to adopt")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := oauth.ImportGeminiCLI(ctx); err != nil {
			t.Fatalf("ImportGeminiCLI: %v", err)
		}
		if oauth.CodeAssistProject() == "" {
			t.Fatal("import succeeded but no Code Assist project was resolved; every request would 500")
		}

		p, err := NewProvider(ProviderOptions{
			Provider: "code-assist",
			Model:    "gemini-2.5-flash",
			Timeout:  90 * time.Second,
		})
		if err != nil {
			t.Fatalf("NewProvider: %v", err)
		}
		assertAnswersLive(ctx, t, p)
	})
}

// assertAnswersLive runs the two things a provider has to do for Aetox to be
// usable on it: answer a question, and call a tool when told to.
func assertAnswersLive(ctx context.Context, t *testing.T, p Provider) {
	t.Helper()

	resp, err := retryPastRateLimit(t, func() (Response, error) {
		return p.Complete(ctx, Request{
			Messages: []Message{
				{Role: RoleSystem, Content: "Answer in one short sentence."},
				{Role: RoleUser, Content: "What is the capital of Japan?"},
			},
			MaxTokens: 2000,
		})
	})
	skipIfOutOfQuota(t, err)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(strings.ToLower(resp.Text), "tokyo") {
		t.Fatalf("answer did not contain the fact asked for: %q", resp.Text)
	}
	t.Logf("answer: %q", strings.TrimSpace(resp.Text))
	if resp.Usage != nil {
		t.Logf("usage: prompt=%d (cached %d) completion=%d",
			resp.Usage.PromptTokens, resp.Usage.CachedPromptTokens, resp.Usage.CompletionTokens)
	} else {
		t.Error("no usage reported — the token counter will read zero for this provider")
	}

	// A provider that answers but cannot call a tool is not usable as an agent,
	// and the tool shape is the part most likely to be wrong per-provider.
	streamer, ok := p.(StreamingProvider)
	if !ok {
		t.Fatal("provider does not stream; the desktop needs streaming")
	}
	var streamed strings.Builder
	weatherTool := []ToolDefinition{{
		Type: "function",
		Function: ToolFunction{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			Parameters:  []byte(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}}
	toolResp, err := retryPastRateLimit(t, func() (Response, error) {
		streamed.Reset()
		return streamer.StreamComplete(ctx, Request{
			Messages: []Message{
				{Role: RoleSystem, Content: "Use the get_weather tool when asked about weather. Do not answer from memory."},
				{Role: RoleUser, Content: "What is the weather in Bangkok right now?"},
			},
			Tools:     weatherTool,
			MaxTokens: 2000,
		}, func(chunk string) error { streamed.WriteString(chunk); return nil }, nil)
	})
	skipIfOutOfQuota(t, err)
	if err != nil {
		t.Fatalf("StreamComplete with tools: %v", err)
	}
	if len(toolResp.ToolCalls) == 0 {
		t.Fatalf("no tool call; the model answered %q instead", toolResp.Text)
	}
	call := toolResp.ToolCalls[0]
	t.Logf("tool call: %s(%s) id=%s", call.Function.Name, call.Function.Arguments, call.ID)
	if call.Function.Name != "get_weather" {
		t.Fatalf("called %q; want get_weather", call.Function.Name)
	}
	if !strings.Contains(strings.ToLower(call.Function.Arguments), "bangkok") {
		t.Fatalf("arguments = %q; want the city from the question", call.Function.Arguments)
	}
	if call.ID == "" {
		t.Fatal("tool call has no id; the result cannot be routed back to it")
	}

	// And the other half of the round trip: hand the result back and check the
	// provider accepts the shape this runtime sends for it.
	final, err := retryPastRateLimit(t, func() (Response, error) {
		return p.Complete(ctx, Request{
			Messages: []Message{
				{Role: RoleSystem, Content: "Use the get_weather tool when asked about weather."},
				{Role: RoleUser, Content: "What is the weather in Bangkok right now?"},
				{Role: RoleAssistant, ToolCalls: toolResp.ToolCalls},
				{Role: RoleTool, ToolCallID: call.ID, Name: call.Function.Name, Content: `{"temp_c":34,"conditions":"hazy sunshine"}`},
			},
			Tools:     weatherTool,
			MaxTokens: 2000,
		})
	})
	skipIfOutOfQuota(t, err)
	if err != nil {
		t.Fatalf("second turn (tool result round trip): %v", err)
	}
	if !strings.Contains(final.Text, "34") {
		t.Fatalf("final answer did not use the tool result: %q", final.Text)
	}
	t.Logf("after tool result: %q", strings.TrimSpace(final.Text))
}

// retryPastRateLimit runs call, and if the provider says "too fast" rather than
// "you are out", waits and tries once more.
//
// The Gemini free tier allows roughly one request a minute per model, so a test
// that makes three calls in a row hits the limiter on the second every time.
// That is pacing, not a defect, and it must not read as one.
func retryPastRateLimit(t *testing.T, call func() (Response, error)) (Response, error) {
	t.Helper()
	resp, err := call()
	if err == nil || !isPerMinuteLimit(err) {
		return resp, err
	}
	t.Logf("rate limited; waiting out the window before retrying: %v", err)
	time.Sleep(65 * time.Second)
	return call()
}

// isPerMinuteLimit distinguishes a short throttling window (wait a minute) from
// a spent plan (wait days). Only the first is worth retrying inside a test.
func isPerMinuteLimit(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "RESOURCE_EXHAUSTED") && strings.Contains(msg, "reset after")
}

// skipIfOutOfQuota separates "this account has nothing left this cycle" from
// "the request was wrong".
//
// A plan-limit 429 is itself evidence the runtime works: the backend only
// reaches quota enforcement after it has accepted the credentials, the headers,
// the model id and the body. Failing the test on it would mean a green suite
// depends on how much of someone's monthly plan is left.
func skipIfOutOfQuota(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "limit is used up"),
		strings.Contains(msg, "usage_limit_reached"),
		strings.Contains(msg, "RESOURCE_EXHAUSTED"),
		strings.Contains(msg, "limit reached"):
		t.Skipf("request accepted but the plan is out of quota — the wire format is proven up to metering: %v", err)
	}
}

// Live check of the Claude thinking contract, skipped unless AETOX_LIVE=1.
//
//	AETOX_LIVE=1 ANTHROPIC_API_KEY=... go test ./internal/model/ -run TestLiveAnthropicEffort -v -count=1
//
// The whole point: thinking depth is output_config.effort, and the older
// thinking.budget_tokens is a 400. Which of those is true cannot be settled by
// reading — the request either is accepted or it is not.
func TestLiveAnthropicEffortLadder(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against the real Anthropic API")
	}
	// TestMain points the whole package at a throwaway credential store; this
	// test is the one that wants the real one, because the credential under
	// test is the machine's actual Claude sign-in.
	if root := strings.TrimSpace(os.Getenv("AETOX_REAL_DATA_ROOT")); root != "" {
		t.Setenv("AETOX_DATA_ROOT", root)
	} else {
		t.Setenv("AETOX_DATA_ROOT", defaultUserDataRoot())
	}

	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" && !oauth.Has("anthropic") {
		t.Skip("no Claude credentials: set ANTHROPIC_API_KEY or sign in with `aetox login anthropic`")
	}

	model := strings.TrimSpace(os.Getenv("AETOX_LIVE_MODEL"))
	if model == "" {
		model = "claude-sonnet-5"
	}

	for _, level := range []string{"off", "low", "medium", "high", "xhigh", "max"} {
		t.Run(level, func(t *testing.T) {
			// Through the factory, so a signed-in plan is resolved exactly as
			// it is for a real turn rather than by a test-only shortcut.
			p, err := NewProvider(ProviderOptions{
				Provider: "anthropic", Model: model, APIKey: apiKey,
				Timeout: 90 * time.Second,
			})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}

			req := Request{
				// A real system prompt rides along on purpose: the OAuth
				// endpoint matches the first system block byte-for-byte and a
				// concatenated prompt is refused as a fake 429 — a contract
				// only a live call can hold us to.
				Messages: []Message{
					{Role: RoleSystem, Content: "You are Aetox, a helpful assistant."},
					{Role: RoleUser, Content: "Reply with the single word: ok"},
				},
				// The temperature Aetox sets on every real call. If the runtime
				// forwarded it, this request would 400 — so every subtest here
				// is also a regression test for that.
				Temperature: 0.2,
				MaxTokens:   2000,
			}
			if level == "off" {
				req.Thinking = &ThinkingConfig{Type: "disabled"}
			} else {
				req.Reasoning = &ReasoningConfig{Effort: level}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			resp, err := retryPastRateLimit(t, func() (Response, error) { return p.Complete(ctx, req) })
			skipIfOutOfQuota(t, err)
			if err != nil {
				t.Fatalf("effort %q rejected: %v", level, err)
			}
			t.Logf("effort %-6s → %q (thinking %d chars)", level,
				strings.TrimSpace(resp.Text), len(resp.ReasoningContent))
		})
	}
}

// defaultUserDataRoot mirrors config.DataRoot for the one test that needs the
// real credential store rather than the package's throwaway one.
func defaultUserDataRoot() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "aetox")
}
