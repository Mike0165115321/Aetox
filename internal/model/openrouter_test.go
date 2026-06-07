package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenRouterProviderComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got == "" {
			t.Fatal("expected authorization header")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var payload struct {
			Model    string    `json:"model"`
			Messages []Message `json:"messages"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("bad json payload: %v", err)
		}
		if payload.Model == "" {
			t.Fatal("expected model in payload")
		}
		if len(payload.Messages) != 1 {
			t.Fatal("expected one message")
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "remote/model",
			"choices": [
				{"message": {"role":"assistant", "content":"hello from model"}}
			],
			"usage": {
				"prompt_tokens": 22,
				"completion_tokens": 8,
				"total_tokens": 30
			}
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenRouterProvider(OpenRouterConfig{
		Model:   "my-model",
		APIKey:  "k",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	response, err := provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "ping"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if response.Provider != "openrouter" {
		t.Fatalf("expected provider openrouter, got %s", response.Provider)
	}
	if response.Model != "remote/model" {
		t.Fatalf("expected remote/model, got %s", response.Model)
	}
	if response.Text != "hello from model" {
		t.Fatalf("unexpected text: %s", response.Text)
	}
	if response.Usage == nil || response.Usage.TotalTokens != 30 {
		t.Fatalf("expected usage, got %+v", response.Usage)
	}
}

func TestOpenRouterProviderCompleteHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer server.Close()

	provider, err := NewOpenRouterProvider(OpenRouterConfig{
		Model:   "my-model",
		APIKey:  "k",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	_, err = provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "ping"},
		},
	})
	if err == nil {
		t.Fatal("expected error from non-200 response")
	}
	if !strings.Contains(err.Error(), "openrouter request failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenRouterProviderCompleteToolCallWithoutText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "remote/model",
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "",
						"tool_calls": [
							{
								"id": "call_time_1",
								"type": "function",
								"function": {
									"name": "time",
									"arguments": "{}"
								}
							}
						]
					}
				}
			],
			"usage": {
				"prompt_tokens": 12,
				"completion_tokens": 16,
				"total_tokens": 28
			}
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenRouterProvider(OpenRouterConfig{
		Model:   "my-model",
		APIKey:  "k",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	response, err := provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "what time"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(response.ToolCalls))
	}
	if response.ToolCalls[0].Function.Name != "time" {
		t.Fatalf("unexpected tool name: %s", response.ToolCalls[0].Function.Name)
	}
	if response.Text != "" {
		t.Fatalf("expected empty text for pure tool call, got %q", response.Text)
	}
}
