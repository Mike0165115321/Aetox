package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestOpenAICompatibleProviderUsesOpenAIReasoningEffortPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if _, ok := payload["reasoning"]; ok {
			t.Fatalf("expected reasoning object to be omitted, got %#v", payload["reasoning"])
		}
		if _, ok := payload["thinking"]; ok {
			t.Fatalf("expected thinking to be omitted, got %#v", payload["thinking"])
		}
		if got := payload["reasoning_effort"]; got != "high" {
			t.Fatalf("expected reasoning_effort=high, got %#v", got)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "gpt-4o-mini",
			"choices": [
				{"message": {"role":"assistant", "content":"ok"}}
			]
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "openai",
		Model:    "gpt-4o-mini",
		APIKey:   "k",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if !provider.SupportsReasoning() {
		t.Fatal("expected provider reasoning support to be enabled")
	}

	_, err = provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "ping"},
		},
		Reasoning: &ReasoningConfig{Effort: "high"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
}

func TestOpenAICompatibleProviderUsesGroqReasoningPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if got := payload["reasoning_effort"]; got != "high" {
			t.Fatalf("expected reasoning_effort=high, got %#v", got)
		}
		if got, ok := payload["include_reasoning"].(bool); !ok || got {
			t.Fatalf("expected include_reasoning=false, got %#v", payload["include_reasoning"])
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "openai/gpt-oss-20b",
			"choices": [
				{"message": {"role":"assistant", "content":"ok"}}
			]
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "groq",
		Model:    "openai/gpt-oss-20b",
		APIKey:   "k",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	_, err = provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "ping"},
		},
		Reasoning: &ReasoningConfig{Effort: "high"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
}

func TestOpenAICompatibleProviderUsesGeminiReasoningPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if got := payload["reasoning_effort"]; got != "low" {
			t.Fatalf("expected reasoning_effort=low, got %#v", got)
		}
		if _, ok := payload["reasoning"]; ok {
			t.Fatalf("expected reasoning object to be omitted, got %#v", payload["reasoning"])
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "gemini-2.5-flash-lite",
			"choices": [
				{"message": {"role":"assistant", "content":"ok"}}
			]
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "gemini",
		Model:    "gemini-2.5-flash-lite",
		APIKey:   "k",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	_, err = provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "ping"},
		},
		Reasoning: &ReasoningConfig{Effort: "low"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
}

func TestOpenAICompatibleProviderUsesDeepSeekThinkingPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if _, ok := payload["reasoning"]; ok {
			t.Fatalf("expected reasoning to be omitted for deepseek, got %#v", payload["reasoning"])
		}
		if got := payload["reasoning_effort"]; got != "high" {
			t.Fatalf("expected deepseek reasoning_effort=high, got %#v", got)
		}
		thinking, ok := payload["thinking"].(map[string]any)
		if !ok {
			t.Fatalf("expected thinking object, got %#v", payload["thinking"])
		}
		if thinking["type"] != "enabled" {
			t.Fatalf("expected thinking.type=enabled, got %#v", thinking["type"])
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "deepseek-v4-flash",
			"choices": [
				{"message": {"role":"assistant", "content":"ok"}}
			]
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   "k",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}
	if !provider.SupportsReasoning() {
		t.Fatal("expected deepseek provider reasoning support to be enabled")
	}

	response, err := provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "ping"},
		},
		Reasoning: &ReasoningConfig{Effort: "low"},
		Thinking:  &ThinkingConfig{Type: "enabled"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if response.Text != "ok" {
		t.Fatalf("expected ok response, got %q", response.Text)
	}
}

func TestOpenAICompatibleProviderAllowsReasoningOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "deepseek-v4-flash",
			"choices": [
				{"message": {"role":"assistant", "content":"", "reasoning_content":"internal"}}
			]
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   "k",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	response, err := provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "ping"},
		},
		Thinking: &ThinkingConfig{Type: "enabled"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if response.Text != "" {
		t.Fatalf("expected empty final text, got %q", response.Text)
	}
	if response.ReasoningContent != "internal" {
		t.Fatalf("expected reasoning content to be preserved, got %q", response.ReasoningContent)
	}
}

func TestOpenAICompatibleProviderReportsLengthFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// truncated tool call: finish_reason "length", arguments cut mid-JSON
		_, _ = w.Write([]byte(`{
			"model": "deepseek-v4-flash",
			"choices": [
				{
					"message": {"role":"assistant", "content":"", "tool_calls":[
						{"id":"call_1","type":"function","function":{"name":"write","arguments":"{\"path\": \"landing.html\", \"content\": \"<!DOCTYPE html>"}}
					]},
					"finish_reason": "length"
				}
			]
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   "k",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	response, err := provider.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "make a landing page"}},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if response.FinishReason != FinishReasonLength {
		t.Fatalf("expected finish reason %q, got %q", FinishReasonLength, response.FinishReason)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("expected the truncated tool call to be preserved, got %d calls", len(response.ToolCalls))
	}
}

func TestOpenAICompatibleProviderNormalStopFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "deepseek-v4-flash",
			"choices": [
				{"message": {"role":"assistant", "content":"done"}, "finish_reason": "stop"}
			]
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   "k",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	response, err := provider.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if response.FinishReason != "stop" {
		t.Fatalf("expected finish reason stop, got %q", response.FinishReason)
	}
}

func TestOpenAICompatibleProviderStreamCollectsReasoningAndContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"คิด\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"สวัสดี\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   "k",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	var chunks []string
	response, err := provider.StreamComplete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "ping"},
		},
		Thinking: &ThinkingConfig{Type: "enabled"},
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("stream complete failed: %v", err)
	}
	if response.Text != "สวัสดี" {
		t.Fatalf("expected final content, got %q", response.Text)
	}
	if response.ReasoningContent != "คิด" {
		t.Fatalf("expected reasoning content, got %q", response.ReasoningContent)
	}
	if !reflect.DeepEqual(chunks, []string{"สวัสดี"}) {
		t.Fatalf("unexpected stream chunks: %#v", chunks)
	}
	wantUsage := &Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}
	if !reflect.DeepEqual(response.Usage, wantUsage) {
		t.Fatalf("unexpected usage: got %+v want %+v", response.Usage, wantUsage)
	}
}
