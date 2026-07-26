package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Both payloads below are verbatim from a real DeepSeek call (it serves both
// wire formats), so these lock in what the API actually sends rather than what
// the adapter wishes it sent.

// The Anthropic format reports input_tokens as the freshly-evaluated part
// ALONE, with cached tokens as separate addends. Reading it as "the prompt
// size" recorded a 4011-token call as 43 tokens, and the better the cache
// worked the more it undercounted.
func TestAnthropicUsageCountsCachedInputInThePromptTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "deepseek-chat",
			"content": []map[string]any{{"type": "text", "text": "hi"}},
			"usage": map[string]any{
				"input_tokens":                43,
				"cache_read_input_tokens":     3968,
				"cache_creation_input_tokens": 0,
				"output_tokens":               1,
			},
		})
	}))
	defer server.Close()

	p, err := NewAnthropicProvider(AnthropicConfig{Model: "deepseek-chat", APIKey: "k", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	resp, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("no usage reported")
	}
	if resp.Usage.PromptTokens != 4011 {
		t.Errorf("PromptTokens = %d, want 4011 (43 fresh + 3968 from cache)", resp.Usage.PromptTokens)
	}
	if resp.Usage.CachedPromptTokens != 3968 {
		t.Errorf("CachedPromptTokens = %d, want 3968", resp.Usage.CachedPromptTokens)
	}
	if got := resp.Usage.UncachedPromptTokens(); got != 43 {
		t.Errorf("UncachedPromptTokens() = %d, want 43", got)
	}
	if !resp.Usage.CacheReported {
		t.Error("CacheReported = false, but this format always reports cache accounting")
	}
}

// Cache creation is input the model evaluated this call and then stored, so it
// belongs on the uncached side of the split even though it is a cache field.
func TestAnthropicUsageCountsCacheCreationAsUncached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "hi"}},
			"usage": map[string]any{
				"input_tokens":                10,
				"cache_read_input_tokens":     0,
				"cache_creation_input_tokens": 990,
				"output_tokens":               1,
			},
		})
	}))
	defer server.Close()

	p, _ := NewAnthropicProvider(AnthropicConfig{Model: "m", APIKey: "k", BaseURL: server.URL})
	resp, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Usage.PromptTokens != 1000 || resp.Usage.CachedPromptTokens != 0 {
		t.Errorf("usage = %+v, want 1000 prompt / 0 cached", *resp.Usage)
	}
}

// The OpenAI-compatible format already reports prompt_tokens as the total, so
// the cached count is a subset and nothing needs recomputing.
func TestOpenAICompatibleUsageReadsCacheHits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage map[string]any
		// want zero values mean "expect no cache accounting"
		wantPrompt, wantCached int
		wantReported           bool
	}{
		{
			name: "deepseek flat spelling",
			usage: map[string]any{
				"prompt_tokens": 4011, "completion_tokens": 1, "total_tokens": 4012,
				"prompt_cache_hit_tokens": 3968, "prompt_cache_miss_tokens": 43,
			},
			wantPrompt: 4011, wantCached: 3968, wantReported: true,
		},
		{
			name: "openai nested spelling",
			usage: map[string]any{
				"prompt_tokens": 4011, "completion_tokens": 1,
				"prompt_tokens_details": map[string]any{"cached_tokens": 3968},
			},
			wantPrompt: 4011, wantCached: 3968, wantReported: true,
		},
		{
			// Measured zero, not absent: the provider does account for cache
			// and nothing hit. Must stay distinguishable from the next case.
			name: "reported zero hits",
			usage: map[string]any{
				"prompt_tokens": 4011, "completion_tokens": 1,
				"prompt_cache_hit_tokens": 0, "prompt_cache_miss_tokens": 4011,
			},
			wantPrompt: 4011, wantCached: 0, wantReported: true,
		},
		{
			name:       "provider does no cache accounting",
			usage:      map[string]any{"prompt_tokens": 4011, "completion_tokens": 1},
			wantPrompt: 4011, wantCached: 0, wantReported: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"model":   "m",
					"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "hi"}}},
					"usage":   tc.usage,
				})
			}))
			defer server.Close()

			p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
				Provider: "deepseek", Model: "m", APIKey: "k", BaseURL: server.URL,
			})
			if err != nil {
				t.Fatalf("provider: %v", err)
			}
			resp, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
			if err != nil {
				t.Fatalf("complete: %v", err)
			}
			if resp.Usage == nil {
				t.Fatal("no usage reported")
			}
			if resp.Usage.PromptTokens != tc.wantPrompt || resp.Usage.CachedPromptTokens != tc.wantCached {
				t.Errorf("usage = %d prompt / %d cached, want %d / %d",
					resp.Usage.PromptTokens, resp.Usage.CachedPromptTokens, tc.wantPrompt, tc.wantCached)
			}
			if resp.Usage.CacheReported != tc.wantReported {
				t.Errorf("CacheReported = %v, want %v — a provider that never reported cache must not "+
					"be shown a real 0%% hit rate", resp.Usage.CacheReported, tc.wantReported)
			}
		})
	}
}

// Ollama has no cache accounting at all, and must not claim any.
func TestOllamaReportsNoCacheAccounting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "tiny", "done": true,
			"message":           map[string]any{"role": "assistant", "content": "hi"},
			"prompt_eval_count": 3005, "eval_count": 12,
		})
	}))
	defer server.Close()

	p, _ := NewOllamaProvider(OllamaConfig{Model: "tiny", BaseURL: server.URL})
	resp, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Usage.PromptTokens != 3005 {
		t.Errorf("PromptTokens = %d, want 3005", resp.Usage.PromptTokens)
	}
	if resp.Usage.CacheReported {
		t.Error("CacheReported = true for Ollama, which reports no cache accounting")
	}
}
