package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAnthropicProviderRequiresModelAndKey(t *testing.T) {
	if _, err := NewAnthropicProvider(AnthropicConfig{APIKey: "k"}); err != ErrMissingModel {
		t.Fatalf("expected ErrMissingModel, got %v", err)
	}
	if _, err := NewAnthropicProvider(AnthropicConfig{Model: "claude-haiku-4-5"}); err != ErrMissingAPIKey {
		t.Fatalf("expected ErrMissingAPIKey, got %v", err)
	}
}

func TestAnthropicProviderCompleteSendsExpectedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "k" {
			t.Fatalf("expected x-api-key header, got %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicAPIVersion {
			t.Fatalf("expected anthropic-version header, got %q", got)
		}
		if r.URL.Path != "/messages" {
			t.Fatalf("expected /messages path, got %q", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if payload["system"] != "be terse" {
			t.Fatalf("expected system prompt, got %#v", payload["system"])
		}
		msgs, ok := payload["messages"].([]any)
		if !ok || len(msgs) != 1 {
			t.Fatalf("expected 1 merged message, got %#v", payload["messages"])
		}
		if got := payload["max_tokens"]; got != float64(defaultAnthropicMaxTokens) {
			t.Fatalf("expected default max_tokens, got %#v", got)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "claude-haiku-4-5",
			"content": [{"type":"text","text":"hi there"}],
			"usage": {"input_tokens": 10, "output_tokens": 3}
		}`))
	}))
	defer server.Close()

	provider, err := NewAnthropicProvider(AnthropicConfig{
		Model:   "claude-haiku-4-5",
		APIKey:  "k",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if provider.Name() != "anthropic" {
		t.Fatalf("expected provider name anthropic, got %q", provider.Name())
	}

	resp, err := provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "be terse"},
			{Role: RoleUser, Content: "ping"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Text != "hi there" {
		t.Fatalf("expected response text, got %q", resp.Text)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 3 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
}

func TestAnthropicProviderCompleteParsesToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "claude-haiku-4-5",
			"content": [
				{"type":"text","text":"let me check"},
				{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Bangkok"}}
			],
			"usage": {"input_tokens": 5, "output_tokens": 8}
		}`))
	}))
	defer server.Close()

	provider, err := NewAnthropicProvider(AnthropicConfig{
		Model:   "claude-haiku-4-5",
		APIKey:  "k",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	resp, err := provider.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "weather?"}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_weather",
				Description: "get weather",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			},
		}},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_1" || tc.Function.Name != "get_weather" {
		t.Fatalf("unexpected tool call: %#v", tc)
	}
	if tc.Function.Arguments != `{"city":"Bangkok"}` {
		t.Fatalf("unexpected tool call arguments: %q", tc.Function.Arguments)
	}
}

func TestAnthropicProviderStreamCollectsTextAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-haiku-4-5\",\"usage\":{\"input_tokens\":4}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"สวัสดี\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider, err := NewAnthropicProvider(AnthropicConfig{
		Model:   "claude-haiku-4-5",
		APIKey:  "k",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	var chunks []string
	resp, err := provider.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("stream complete failed: %v", err)
	}
	if resp.Text != "สวัสดี" {
		t.Fatalf("expected streamed text, got %q", resp.Text)
	}
	if len(chunks) != 1 || chunks[0] != "สวัสดี" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 4 || resp.Usage.CompletionTokens != 2 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
}

func TestConvertMessagesToAnthropicMergesConsecutiveToolResults(t *testing.T) {
	system, msgs := convertMessagesToAnthropic([]Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_1", Function: FunctionCall{Name: "a", Arguments: "{}"}},
			{ID: "call_2", Function: FunctionCall{Name: "b", Arguments: "{}"}},
		}},
		{Role: RoleTool, ToolCallID: "call_1", Content: "result-a"},
		{Role: RoleTool, ToolCallID: "call_2", Content: "result-b"},
	})
	if system != "sys" {
		t.Fatalf("expected system prompt, got %q", system)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 merged turns, got %d: %#v", len(msgs), msgs)
	}
	if msgs[2].Role != "user" || len(msgs[2].Content) != 2 {
		t.Fatalf("expected merged tool_result turn with 2 blocks, got %#v", msgs[2])
	}
}
