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

func TestLiveToolCallProgress(t *testing.T) {
	key := liveProviderKey(t, "deepseek")

	// Exactly how the app builds it: provider name only, catalog decides the
	// runtime and URL. For deepseek that is the Anthropic wire format.
	p, err := NewProvider(ProviderOptions{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
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
		Model:       "deepseek-v4-flash",
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
	// The row must open long before the call finishes, or it is not progress.
	if firstAt > total/2 {
		t.Errorf("first update at %v of a %v stream — too late to be useful", firstAt, total)
	}
	// The id the UI saw must be the id the finished call carries.
	if seen[0].id != resp.ToolCalls[0].ID {
		t.Errorf("streamed id %q != finished id %q — the timeline would draw two rows",
			seen[0].id, resp.ToolCalls[0].ID)
	}
	if last := seen[len(seen)-1]; last.lines < 2 {
		t.Errorf("final line count %d never climbed", last.lines)
	}
}
