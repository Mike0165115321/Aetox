package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Live smoke test against a real provider, skipped unless AETOX_LIVE=1.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveToolCallProgress -v -count=1
//
// It exists because the fakes cannot answer the question that matters: does the
// tool-call progress a UI depends on actually arrive from the provider the user
// is configured against, over the wire format that provider actually uses.
// DeepSeek defaults to the Anthropic wire format, which is exactly where this
// reporting was missing.
func liveProviderKey(t *testing.T, provider string) string {
	t.Helper()
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run live provider tests")
	}
	// Read from the user's own config rather than an env var, so a key never
	// lands in a shell history or a CI log.
	data, err := os.ReadFile(filepath.Join(os.Getenv("APPDATA"), "aetox", "model-preference.json"))
	if err != nil {
		t.Skipf("no model-preference.json: %v", err)
	}
	var pref struct {
		ProviderAPIKeys map[string]string `json:"provider_api_keys"`
	}
	if err := json.Unmarshal(data, &pref); err != nil {
		t.Fatalf("preference file unreadable: %v", err)
	}
	key := strings.TrimSpace(pref.ProviderAPIKeys[provider])
	if key == "" {
		t.Skipf("no %s key configured", provider)
	}
	return key
}

// Cache accounting is read from fields no fake can vouch for: the unit tests
// pin the parsing against recorded payloads, but only the real API can prove
// the field names still exist. Sending the same long prompt twice makes the
// second call a hit by construction.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveCacheAccounting -v
func TestLiveCacheAccounting(t *testing.T) {
	key := liveProviderKey(t, "deepseek")

	// Long enough to be cacheable at all, and identical across both calls.
	var filler strings.Builder
	for i := 0; i < 400; i++ {
		filler.WriteString("The quick brown fox jumps over the lazy dog. ")
	}
	ask := func(p Provider) Usage {
		resp, err := p.Complete(context.Background(), Request{
			MaxTokens: 8,
			Messages: []Message{
				{Role: RoleSystem, Content: filler.String()},
				{Role: RoleUser, Content: "Say hi in one word."},
			},
		})
		if err != nil {
			t.Fatalf("complete: %v", err)
		}
		if resp.Usage == nil {
			t.Fatal("no usage reported at all")
		}
		return *resp.Usage
	}

	// Both wire formats, because each counts input differently and the app can
	// be configured either way.
	for _, format := range []string{"anthropic", "openai-compatible"} {
		t.Run(format, func(t *testing.T) {
			p, err := NewProvider(ProviderOptions{
				Provider: "deepseek", Model: "deepseek-chat", APIKey: key,
				WireFormat: format, Timeout: 2 * time.Minute,
			})
			if err != nil {
				t.Fatalf("provider: %v", err)
			}
			ask(p) // prime the cache
			u := ask(p)
			t.Logf("prompt=%d cached=%d uncached=%d reported=%v",
				u.PromptTokens, u.CachedPromptTokens, u.UncachedPromptTokens(), u.CacheReported)

			if !u.CacheReported {
				t.Fatal("cache accounting went unread — the field names moved")
			}
			if u.CachedPromptTokens == 0 {
				t.Error("no cache hit on an identical repeat; either caching stopped or the field is misread")
			}
			// The bug this replaced: PromptTokens carrying only the uncached
			// remainder, which on a well-cached call is a tiny fraction.
			if u.PromptTokens <= u.CachedPromptTokens {
				t.Errorf("PromptTokens %d does not include the %d cached tokens",
					u.PromptTokens, u.CachedPromptTokens)
			}
			if u.PromptTokens < 3000 {
				t.Errorf("PromptTokens = %d for a ~4000 token prompt — the total is being undercounted",
					u.PromptTokens)
			}
		})
	}
}

func liveWriteTool() ToolDefinition {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Relative destination path"},
			"content": map[string]any{"type": "string", "description": "File content"},
		},
		"required":             []string{"path", "content"},
		"additionalProperties": false,
	})
	return ToolDefinition{
		Type:     "function",
		Function: ToolFunction{Name: "write", Description: "Write content to a file in sandbox root.", Parameters: schema},
	}
}

// Both wire formats are covered, because the reporting is implemented once per
// format and a fake stream cannot prove either one matches what the API really
// sends. DeepSeek exercises the Anthropic format (its catalog default), Gemini
// the OpenAI-compatible one.
func TestLiveToolCallProgress(t *testing.T) {
	for _, tc := range []struct {
		provider, model string
		// streamsArguments records what the API actually does, measured, not
		// assumed: DeepSeek sends a tool call's arguments in hundreds of
		// fragments, so the row's line count climbs the whole way. Gemini sends
		// the finished call in a single delta at the end, so there is nothing to
		// count — the row can only appear, correctly named, once. Asserting one
		// rule for both would either fail on Gemini or stop checking DeepSeek.
		streamsArguments bool
	}{
		{"deepseek", "deepseek-v4-flash", true},
		{"gemini", "gemini-2.5-flash", false},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			liveToolCallProgress(t, tc.provider, tc.model, tc.streamsArguments)
		})
	}
}

func liveToolCallProgress(t *testing.T, provider, modelName string, streamsArguments bool) {
	key := liveProviderKey(t, provider)

	// Exactly how the app builds it: provider name only, catalog decides the
	// runtime and URL.
	p, err := NewProvider(ProviderOptions{
		Provider: provider,
		Model:    modelName,
		APIKey:   key,
		Timeout:  5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	t.Logf("provider concrete type: %T", p)
	streamer, ok := p.(StreamingProvider)
	if !ok {
		t.Fatal("provider does not stream")
	}

	var seen []progressUpdate
	var firstAt, lastAt time.Duration
	start := time.Now()

	resp, err := streamer.StreamComplete(context.Background(), Request{
		Model:       modelName,
		MaxTokens:   8000,
		Temperature: 0.2,
		Tools:       []ToolDefinition{liveWriteTool()},
		Thinking:    &ThinkingConfig{Type: "enabled"},
		Reasoning:   &ReasoningConfig{Effort: "high"},
		Messages: []Message{
			{Role: RoleSystem, Content: "You are a coding agent. Use the write tool. Do not explain."},
			{Role: RoleUser, Content: "Write a single-file landing page to index.html — modern dark theme, inline CSS, at least 120 lines. Call the write tool once."},
		},
		OnToolCallProgress: func(id, name, subject string, lines int) {
			if firstAt == 0 {
				firstAt = time.Since(start)
			}
			lastAt = time.Since(start)
			seen = append(seen, progressUpdate{id, name, subject, lines})
		},
	}, nil, func(string) error { return nil })
	total := time.Since(start)
	if err != nil {
		t.Fatalf("stream failed after %v: %v", total, err)
	}

	if len(resp.ToolCalls) == 0 {
		t.Fatalf("no tool call in %v; text was %q", total, resp.Text)
	}
	args := resp.ToolCalls[0].Function.Arguments
	t.Logf("total=%v  tool=%s  args=%d bytes  id=%q",
		total.Round(time.Millisecond), resp.ToolCalls[0].Function.Name, len(args), resp.ToolCalls[0].ID)
	t.Logf("progress: %d updates, first at %v, last at %v",
		len(seen), firstAt.Round(time.Millisecond), lastAt.Round(time.Millisecond))
	for i, u := range seen {
		if i < 3 || i == len(seen)-1 {
			t.Logf("  [%d] id=%q subject=%q lines=%d", i, u.id, u.subject, u.lines)
		} else if i == 3 {
			t.Logf("  ... %d more ...", len(seen)-4)
		}
	}

	if len(seen) == 0 {
		t.Fatal("no progress updates — a large write is still silent on this provider")
	}
	// The id the UI saw must be the id the finished call carries, on every
	// provider: that is what stops the timeline drawing the same call twice.
	if seen[0].id != resp.ToolCalls[0].ID {
		t.Errorf("streamed id %q != finished id %q — the timeline would draw two rows",
			seen[0].id, resp.ToolCalls[0].ID)
	}
	// And the row must end up naming the file, however few updates it took.
	if last := seen[len(seen)-1]; last.subject == "" {
		t.Errorf("the row never learned the path: %+v", last)
	}

	if !streamsArguments {
		// Nothing more to check: one delta carried the whole call, so a single
		// update at the end is the most this provider can offer.
		return
	}
	// The row must open long before the call finishes, or it is not progress.
	if firstAt > total/2 {
		t.Errorf("first update at %v of a %v stream — too late to be useful", firstAt, total)
	}
	if len(seen) < 5 {
		t.Errorf("%d updates over %v — the counter would look stalled", len(seen), total)
	}
	if last := seen[len(seen)-1]; last.lines < 2 {
		t.Errorf("final line count %d never climbed", last.lines)
	}
}
