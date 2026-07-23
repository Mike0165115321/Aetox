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

// Request.MaxTokens must reach Ollama as options.num_predict — without it,
// Ollama's own default (as low as 128 on some models) silently truncates
// tool-call generation.
func TestOllamaComplete_SendsNumPredictFromMaxTokens(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "tiny",
			"done":    true,
			"message": map[string]any{"role": "assistant", "content": "ok"},
		})
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaConfig{Model: "tiny", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	if _, err := provider.Complete(context.Background(), Request{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8192,
	}); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if !strings.Contains(gotBody, `"num_predict":8192`) {
		t.Fatalf("expected options.num_predict 8192 in request body, got: %s", gotBody)
	}

	// MaxTokens 0 must omit options entirely, not send num_predict 0.
	if _, err := provider.Complete(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if strings.Contains(gotBody, "num_predict") {
		t.Fatalf("MaxTokens 0 must not send num_predict, got: %s", gotBody)
	}
}

// Ollama reports the num_predict cap as done_reason "length" — that must
// surface as Response.FinishReason so the tool-loop truncation guard fires
// for local models too.
func TestOllamaComplete_SurfacesLengthDoneReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":       "tiny",
			"done":        true,
			"done_reason": "length",
			"message":     map[string]any{"role": "assistant", "content": "<!DOCTYPE html><ht"},
		})
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaConfig{Model: "tiny", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	resp, err := provider.Complete(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.FinishReason != FinishReasonLength {
		t.Fatalf("expected done_reason length to surface as %q, got %q", FinishReasonLength, resp.FinishReason)
	}
}

// Reasoning tokens must arrive via onReasoningChunk as each streamed line
// comes in, not only bundled into the final Response — that's what lets the
// desktop UI show live "thinking" text instead of a static spinner.
func TestOllamaStreamComplete_EmitsReasoningChunksLive(t *testing.T) {
	lines := []string{
		`{"model":"tiny","done":false,"message":{"role":"assistant","reasoning_content":"first "}}`,
		`{"model":"tiny","done":false,"message":{"role":"assistant","reasoning_content":"second"}}`,
		`{"model":"tiny","done":false,"message":{"role":"assistant","content":"answer"}}`,
		`{"model":"tiny","done":true,"message":{"role":"assistant"}}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, l := range lines {
			_, _ = w.Write([]byte(l + "\n"))
		}
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaConfig{Model: "tiny", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	var reasoningChunks, contentChunks []string
	resp, err := provider.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk string) error {
		contentChunks = append(contentChunks, chunk)
		return nil
	}, func(chunk string) error {
		reasoningChunks = append(reasoningChunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("stream complete: %v", err)
	}
	if got := strings.Join(reasoningChunks, "|"); got != "first|second" {
		t.Fatalf("expected reasoning chunks delivered live, got %q", got)
	}
	if got := strings.Join(contentChunks, "|"); got != "answer" {
		t.Fatalf("expected content chunk delivered live, got %q", got)
	}
	if resp.ReasoningContent != "firstsecond" {
		t.Fatalf("expected accumulated reasoning in final response, got %q", resp.ReasoningContent)
	}
}
