package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The 64 KB cliff is not the Responses API's alone. Every wire format has a
// case that puts a whole turn on one line — a tool call's arguments arriving in
// a single delta, or any gateway that buffers upstream and forwards the
// finished answer in one go — and each stream reader used to die there with
// bufio.Scanner's "token too long" after the model had already done, and
// billed, the work.
func TestProviderStreamsSurviveLinesLargerThan64KB(t *testing.T) {
	huge := strings.Repeat("x", 300*1024)
	quoted, err := json.Marshal(huge)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	big := string(quoted)

	// Every line the fake providers below send is one line: the separator is
	// written here, never inside a payload.
	lines := func(payloads ...string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, payload := range payloads {
				_, _ = w.Write([]byte(payload))
				_, _ = w.Write([]byte("\n\n"))
			}
		}
	}

	t.Run("anthropic", func(t *testing.T) {
		server := httptest.NewServer(lines(
			`data: {"type":"message_start","message":{"model":"claude-haiku-4-5","usage":{"input_tokens":4}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+big+`}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_stop"}`,
		))
		defer server.Close()

		provider, err := NewAnthropicProvider(AnthropicConfig{
			Model: "claude-haiku-4-5", APIKey: "k", BaseURL: server.URL,
		})
		if err != nil {
			t.Fatalf("new provider failed: %v", err)
		}
		resp, err := provider.StreamComplete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: "ping"}},
		}, nil, nil)
		if err != nil {
			t.Fatalf("stream complete failed: %v", err)
		}
		if len(resp.Text) != len(huge) {
			t.Fatalf("text length = %d; want %d", len(resp.Text), len(huge))
		}
	})

	t.Run("openai-compatible", func(t *testing.T) {
		server := httptest.NewServer(lines(
			`data: {"choices":[{"delta":{"content":`+big+`}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		))
		defer server.Close()

		provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL,
		})
		if err != nil {
			t.Fatalf("new provider failed: %v", err)
		}
		resp, err := provider.StreamComplete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: "ping"}},
		}, nil, nil)
		if err != nil {
			t.Fatalf("stream complete failed: %v", err)
		}
		if len(resp.Text) != len(huge) {
			t.Fatalf("text length = %d; want %d", len(resp.Text), len(huge))
		}
	})

	// Ollama is NDJSON rather than SSE, and it is the format most likely to hit
	// this for real: it sends a tool call's whole arguments object in a single
	// message instead of streaming the fragments.
	t.Run("ollama", func(t *testing.T) {
		server := httptest.NewServer(lines(
			`{"model":"tiny","done":false,"message":{"role":"assistant","content":`+big+`}}`,
			`{"model":"tiny","done":true,"message":{"role":"assistant"}}`,
		))
		defer server.Close()

		provider, err := NewOllamaProvider(OllamaConfig{Model: "tiny", BaseURL: server.URL})
		if err != nil {
			t.Fatalf("new provider failed: %v", err)
		}
		resp, err := provider.StreamComplete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: "ping"}},
		}, nil, nil)
		if err != nil {
			t.Fatalf("stream complete failed: %v", err)
		}
		if len(resp.Text) != len(huge) {
			t.Fatalf("text length = %d; want %d", len(resp.Text), len(huge))
		}
	})
}
