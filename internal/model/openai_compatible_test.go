package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompatibleProviderOmitsReasoningWhenUnsupported(t *testing.T) {
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
			t.Fatalf("expected reasoning to be omitted, got %#v", payload["reasoning"])
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
	if provider.SupportsReasoning() {
		t.Fatal("expected provider reasoning support to be disabled in phase 1")
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
