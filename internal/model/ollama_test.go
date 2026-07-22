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

// A model that rejects tools must still complete the turn: the provider
// retries the same request as plain chat (ARCHITECTURE.md §17 backstop).
func TestOllamaComplete_RetriesWithoutToolsWhenModelRejectsThem(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"tools"`) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"registry.ollama.ai/library/tiny does not support tools"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "tiny",
			"done":    true,
			"message": map[string]any{"role": "assistant", "content": "plain answer"},
		})
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaConfig{Model: "tiny", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	resp, err := provider.Complete(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools:    []ToolDefinition{{Type: "function", Function: ToolFunction{Name: "write"}}},
	})
	if err != nil {
		t.Fatalf("expected chat-only retry to succeed, got error: %v", err)
	}
	if resp.Text != "plain answer" {
		t.Fatalf("unexpected reply: %q", resp.Text)
	}
	if requests != 2 {
		t.Fatalf("expected exactly 2 requests (tools, then retry without), got %d", requests)
	}
}
