package model

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
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
			"model": "gpt-5.2",
			"choices": [
				{"message": {"role":"assistant", "content":"ok"}}
			]
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "openai",
		Model:    "gpt-5.2", // a model that actually has the knob; gpt-4o-mini does not
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

// The other half of the test above, and the one that was missing: the Groq
// branch is chosen by provider name, but include_reasoning is a field only
// Groq's REASONING models accept. Sent to llama-3.3-70b-versatile — Groq's own
// row in the catalog names it as the fallback — the whole turn dies with
// "400: `include_reasoning` is not supported with this model" before a token
// is generated. Found by running a real key against a real endpoint, which is
// the only way it could have been found: every fake in this file answers 200.
func TestGroqSendsIncludeReasoningOnlyToModelsThatTakeIt(t *testing.T) {
	for _, c := range []struct {
		model string
		want  bool
	}{
		{"llama-3.3-70b-versatile", false}, // the catalog's own groq fallback
		{"llama-3.1-8b-instant", false},
		{"openai/gpt-oss-120b", true},
		{"qwen/qwen3-32b", true},
	} {
		var got map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		}))
		p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider: "groq", Model: c.model, APIKey: "k", BaseURL: server.URL,
		})
		if err != nil {
			t.Fatalf("%s: %v", c.model, err)
		}
		_, err = p.Complete(t.Context(), Request{
			Model:    c.model,
			Messages: []Message{{Role: RoleUser, Content: "hi"}},
		})
		server.Close()
		if err != nil {
			t.Fatalf("%s: %v", c.model, err)
		}
		if _, sent := got["include_reasoning"]; sent != c.want {
			t.Errorf("%s: include_reasoning sent=%v, want %v", c.model, sent, c.want)
		}
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
		// Verbatim, not folded. This assertion used to expect "high" for a
		// requested "low", because this path had its own effort table that
		// collapsed low and medium onto high — three of DeepSeek's six real
		// levels were unreachable on this wire format as a result.
		if got := payload["reasoning_effort"]; got != "low" {
			t.Fatalf("expected deepseek reasoning_effort=low (as asked), got %#v", got)
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

// DeepSeek sometimes leaks DSML tool-call markup into content as plain text
// with no structured tool_calls. Complete must lift the calls out and strip
// the markup so the tool loop executes instead of showing raw DSML to users.
func TestOpenAICompatibleProviderLiftsDSMLFromContent(t *testing.T) {
	content := "กำลังสร้าง Landing Page ให้คุณเลยครับ\\n" +
		"<｜DSML｜tool_calls>\\n" +
		"<｜DSML｜invoke name=\\\"write\\\">\\n" +
		"<｜DSML｜parameter name=\\\"path\\\" string=\\\"true\\\">landing.html</｜DSML｜parameter>\\n" +
		"</｜DSML｜invoke>\\n" +
		"</｜DSML｜tool_calls>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "deepseek-v4-flash",
			"choices": [
				{"message": {"role":"assistant", "content":"` + content + `"}, "finish_reason": "stop"}
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
		Messages: []Message{{Role: RoleUser, Content: "landing page please"}},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Function.Name != "write" {
		t.Fatalf("expected lifted write call, got %#v", response.ToolCalls)
	}
	if strings.Contains(response.Text, "DSML") {
		t.Fatalf("markup must be stripped from text, got %q", response.Text)
	}
}

// The streaming path must apply the same DSML backstop as Complete: a leak
// arriving as content chunks has to be lifted into ToolCalls and stripped from
// the streamed text, or the tool never runs and users see raw markup.
func TestOpenAICompatibleProviderStreamLiftsDSMLFromContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Leak split across chunks, exactly as a real stream delivers it.
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"กำลังสร้างให้ครับ\\n<｜DSML｜tool_calls>\\n<｜DSML｜invoke name=\\\"write\\\">\\n\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"<｜DSML｜parameter name=\\\"path\\\" string=\\\"true\\\">a.html</｜DSML｜parameter>\\n</｜DSML｜invoke>\\n</｜DSML｜tool_calls>\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "deepseek", Model: "deepseek-v4-flash", APIKey: "k", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	response, err := provider.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "write a.html"}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("stream complete failed: %v", err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Function.Name != "write" {
		t.Fatalf("expected lifted write call, got %#v", response.ToolCalls)
	}
	if strings.Contains(response.Text, "DSML") {
		t.Fatalf("markup must be stripped from streamed text, got %q", response.Text)
	}
}

// OpenAI-style streaming splits a tool call across deltas: name/id first, then
// arguments in fragments keyed by index. StreamComplete must stitch them back.
func TestOpenAICompatibleProviderStreamReassemblesStructuredToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"write\",\"arguments\":\"{\\\"pa\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"th\\\":\\\"a.html\\\"}\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	response, err := provider.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "write a.html"}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("stream complete failed: %v", err)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("expected 1 reassembled tool call, got %#v", response.ToolCalls)
	}
	call := response.ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "write" {
		t.Fatalf("unexpected call identity: %#v", call)
	}
	args, err := ParseToolArguments(call.Function.Arguments)
	if err != nil {
		t.Fatalf("reassembled arguments not valid JSON (%q): %v", call.Function.Arguments, err)
	}
	if args["path"] != "a.html" {
		t.Fatalf("unexpected reassembled args: %#v", args)
	}
	if response.FinishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", response.FinishReason)
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

// The sibling of TestOpenAICompatibleProviderAllowsReasoningOnlyResponse for
// the other field name. Ollama's OpenAI-compatible endpoint and llama.cpp
// (LM Studio's runtime) send "reasoning", not DeepSeek's "reasoning_content".
// Reading only the latter dropped the whole answer and reported a healthy
// server as "response has empty text" — measured against Ollama 0.32 +
// ornith:9b, whose first token always goes to the reasoning channel.
func TestOpenAICompatibleProviderReadsPlainReasoningField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "ornith:9b",
			"choices": [
				{"message": {"role":"assistant", "content":"", "reasoning":"The"}, "finish_reason":"length"}
			]
		}`))
	}))
	defer server.Close()

	requireKey := false
	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "lmstudio", Model: "ornith:9b", BaseURL: server.URL, RequireAPIKey: &requireKey,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	response, err := provider.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("a reasoning-only answer must not read as a failure: %v", err)
	}
	if response.ReasoningContent != "The" {
		t.Errorf("reasoning = %q, want %q", response.ReasoningContent, "The")
	}
}

// Same field, streaming — the path chat actually runs on. Chunks must arrive
// through onReasoningChunk with their spacing intact, not trimmed per chunk.
func TestOpenAICompatibleProviderStreamReadsPlainReasoningField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, chunk := range []string{
			`{"choices":[{"delta":{"reasoning":"The user"}}]}`,
			`{"choices":[{"delta":{"reasoning":" sent ping"}}]}`,
			`{"choices":[{"delta":{"content":"pong"},"finish_reason":"stop"}]}`,
		} {
			_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	requireKey := false
	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "lmstudio", Model: "ornith:9b", BaseURL: server.URL, RequireAPIKey: &requireKey,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	var streamed string
	response, err := provider.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	}, nil, func(chunk string) error {
		streamed += chunk
		return nil
	})
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if want := "The user sent ping"; streamed != want {
		t.Errorf("streamed reasoning = %q, want %q", streamed, want)
	}
	if response.ReasoningContent != "The user sent ping" {
		t.Errorf("final reasoning = %q", response.ReasoningContent)
	}
}

// Same class as the Anthropic thinking/temperature clash: OpenAI's reasoning
// models answer 400 "Unsupported value: 'temperature'" when both are sent, and
// Aetox sets a temperature on every call.
func TestOpenAIDropsTemperatureWhenReasoningEffortIsSent(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "openai", Model: "o4-mini", APIKey: "sk-x", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider: %v", err)
	}
	if _, err := p.Complete(context.Background(), Request{
		Messages:    []Message{{Role: RoleUser, Content: "hi"}},
		Temperature: 0.2,
		Reasoning:   &ReasoningConfig{Effort: "medium"},
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, sent := body["temperature"]; sent {
		t.Fatalf("temperature was sent alongside reasoning_effort: %v", body)
	}
	if body["reasoning_effort"] != "medium" {
		t.Fatalf("reasoning_effort = %v; want it kept", body["reasoning_effort"])
	}

	// A provider that accepts both must keep both — this is not a blanket rule.
	body = nil
	groq, _ := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "groq", Model: "llama-3.3-70b-versatile", APIKey: "k", BaseURL: server.URL,
	})
	if _, err := groq.Complete(context.Background(), Request{
		Messages:    []Message{{Role: RoleUser, Content: "hi"}},
		Temperature: 0.2,
		Reasoning:   &ReasoningConfig{Effort: "medium"},
	}); err != nil {
		t.Fatalf("Complete (groq): %v", err)
	}
	if body["temperature"] == nil {
		t.Fatalf("groq lost its temperature: %v", body)
	}
}

// The same 429 means two different things: "slow down" fixes itself, "the
// account is out of credits" fixes only at the billing page. Until this, both
// paths dumped the raw JSON body at the user and left the decoding to them.
func TestOpenAICompatibleStatusErrorsAreActionable(t *testing.T) {
	quotaBody := `{"error":{"message":"You exceeded your current quota, please check your plan and billing details.","type":"insufficient_quota","param":null,"code":"insufficient_quota"}}`
	cases := []struct {
		name       string
		status     int
		retryAfter string
		body       string
		want       string
	}{
		{"out of credits", http.StatusTooManyRequests, "0", quotaBody, "out of credits"},
		// Z.ai spells the same condition with no word in common with OpenAI's
		// version — code 1113, "Insufficient balance or no resource package."
		// Recognising only OpenAI's token called this a rate limit and left the
		// user waiting for a balance that refills only at the billing page.
		{
			"out of credits, said another way", http.StatusTooManyRequests, "0",
			`{"error":{"code":"1113","message":"Insufficient balance or no resource package. Please recharge."}}`,
			"out of credits",
		},
		// A third spelling again: some hosts do not use 429 for this at all and
		// answer 402, whose entire meaning is "pay first" — no body to read.
		{
			"payment required", http.StatusPaymentRequired, "",
			`{"error":{"message":"Insufficient Balance","code":"invalid_request_error"}}`,
			"out of credits",
		},
		// Retry-After longer than the transport will wait, so the response
		// comes straight back for the message instead of being retried.
		{"rate limited", http.StatusTooManyRequests, "3600", `{"error":{"message":"Rate limit reached","type":"tokens","code":"rate_limit_exceeded"}}`, "rate limiting this key"},
		{"bad key", http.StatusUnauthorized, "", `{"error":{"message":"Incorrect API key provided"}}`, "rejected the credentials"},
	}
	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if tc.retryAfter != "" {
				w.Header().Set("Retry-After", tc.retryAfter)
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL,
		})
		if err != nil {
			t.Fatalf("%s: new provider failed: %v", tc.name, err)
		}
		_, err = provider.Complete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: "hi"}},
		})
		server.Close()

		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s gave %v; want it to mention %q", tc.name, err, tc.want)
		}
		// Quoting the provider's own sentence is the point — each one words the
		// instruction differently and that difference is what the user acts on.
		// What must never reach them is the JSON around it.
		if err != nil && strings.Contains(err.Error(), `{"error"`) {
			t.Errorf("%s leaked the raw JSON body: %v", tc.name, err)
		}
	}
}

// The streaming path answers the same 429 before any event arrives, and has to
// say the same thing the non-streaming path does.
func TestOpenAICompatibleStreamTranslatesInsufficientQuota(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"You exceeded your current quota, please check your plan and billing details.","type":"insufficient_quota","code":"insufficient_quota"}}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	start := time.Now()
	_, err = provider.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "out of credits") {
		t.Fatalf("err = %v; want the out-of-credits translation", err)
	}
	// The whole point of the transport half of the fix: an empty balance must
	// not be backoff-retried before it is reported.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("took %s; want no retries against an empty balance", elapsed)
	}
}

// Gemini 3 attaches an encrypted `thought_signature` to every tool call and
// refuses the next turn without it — "Function call is missing a
// thought_signature in functionCall parts", a 400 that ends the turn. Aetox's
// ToolCall had three fields and json.Unmarshal dropped the rest on the floor,
// so every Gemini 3 tool use died on its second turn while Gemini 2.5, which
// sends no such field, worked fine and hid the bug.
//
// Verified against the live endpoint on 2026-08-15: echo the field back and the
// second turn is 200; strip it and the identical request is a 400.
func TestOpenAICompatibleCarriesToolCallExtraContentBothWays(t *testing.T) {
	const signature = `{"google":{"thought_signature":"EqACCp0CARFNMg9ETsCkiTf5tkZncYrl"}}`

	var secondRequest []byte
	turn := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		turn++
		if turn == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[
				{"id":"call_226","type":"function","extra_content":` + signature + `,
				 "function":{"name":"echo","arguments":"{\"text\":\"ping\"}"}}]},
				"finish_reason":"tool_calls"}]}`))
			return
		}
		secondRequest = body
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "gemini", Model: "gemini-3.7-flash", APIKey: "k", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	req := Request{Messages: []Message{{Role: RoleUser, Content: "echo ping"}}}
	first, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if len(first.ToolCalls) != 1 {
		t.Fatalf("turn 1 returned %d tool calls", len(first.ToolCalls))
	}
	// Captured on the way in...
	if got := string(first.ToolCalls[0].ExtraContent); got != signature {
		t.Fatalf("extra_content = %q; want it carried through verbatim", got)
	}

	// ...and handed straight back on the way out.
	req.Messages = append(req.Messages,
		Message{Role: RoleAssistant, ToolCalls: first.ToolCalls},
		Message{Role: RoleTool, ToolCallID: first.ToolCalls[0].ID, Content: "ping"})
	if _, err := p.Complete(context.Background(), req); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if !bytes.Contains(secondRequest, []byte(`"thought_signature"`)) {
		t.Fatalf("the signature never reached the second request:\n%s", secondRequest)
	}
}

// The other half: a provider that sends no extra_content must produce a request
// byte-identical to the one it produced before this field existed. The prefix
// of a conversation is what providers cache, and an extra key on every tool
// call would miss that cache for everyone who is not on Gemini.
func TestToolCallWithoutExtraContentSerializesUnchanged(t *testing.T) {
	payload, err := json.Marshal(ToolCall{
		ID: "call_1", Type: "function",
		Function: FunctionCall{Name: "echo", Arguments: `{"text":"ping"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("extra_content")) {
		t.Fatalf("an absent extra_content still reached the wire: %s", payload)
	}
}

// Streaming assembles tool calls from deltas, and the signature arrives whole
// in one of them rather than in pieces — so it is taken, not concatenated.
func TestStreamAccumulatorKeepsExtraContent(t *testing.T) {
	acc := newStreamToolAccumulator(nil)
	acc.add([]streamToolCallDelta{{Index: 0, ID: "call_1", Type: "function"}})
	acc.add([]streamToolCallDelta{{
		Index: 0, ExtraContent: json.RawMessage(`{"google":{"thought_signature":"abc"}}`),
	}})
	acc.add([]streamToolCallDelta{{Index: 0, Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "echo", Arguments: `{"text":`}}})
	acc.add([]streamToolCallDelta{{Index: 0, Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Arguments: `"ping"}`}}})

	calls := acc.finalize()
	if len(calls) != 1 {
		t.Fatalf("finalize gave %d calls", len(calls))
	}
	if got := string(calls[0].ExtraContent); got != `{"google":{"thought_signature":"abc"}}` {
		t.Errorf("streamed extra_content = %q", got)
	}
	if got := calls[0].Function.Arguments; got != `{"text":"ping"}` {
		t.Errorf("arguments were disturbed: %q", got)
	}
}

// Gemini's compat layer sends each tool call whole, in its own chunk, with its
// own id and no index field — so every one of them decodes to index 0. Keyed by
// index alone the pair became one call carrying both argument objects glued
// together, and sending that back cost the next turn a flat 400
// "Request contains an invalid argument" (gemini-3.7-flash, 21 Aug 2026).
func TestStreamAccumulatorSplitsWholeCallsThatShareAnIndex(t *testing.T) {
	fn := func(name, args string) struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} {
		return struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: name, Arguments: args}
	}
	acc := newStreamToolAccumulator(nil)
	acc.add([]streamToolCallDelta{{
		ID: "call_1", Type: "function", Function: fn("get_weather", `{"city":"Chiang Mai"}`),
		ExtraContent: json.RawMessage(`{"google":{"thought_signature":"sig"}}`),
	}})
	acc.add([]streamToolCallDelta{{
		ID: "call_2", Type: "function", Function: fn("get_time", `{"city":"Chiang Mai"}`),
	}})

	calls := acc.finalize()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %#v", len(calls), calls)
	}
	if calls[0].ID != "call_1" || calls[0].Function.Name != "get_weather" {
		t.Errorf("first call is wrong: %#v", calls[0])
	}
	if calls[1].ID != "call_2" || calls[1].Function.Name != "get_time" {
		t.Errorf("second call is wrong: %#v", calls[1])
	}
	for _, call := range calls {
		if _, err := ParseToolArguments(call.Function.Arguments); err != nil {
			t.Errorf("arguments of %s are not valid JSON (%q): %v", call.ID, call.Function.Arguments, err)
		}
	}
	// The signature belongs to the call it arrived with, not to whichever call
	// happened to be open at index 0.
	if got := string(calls[0].ExtraContent); got != `{"google":{"thought_signature":"sig"}}` {
		t.Errorf("first call lost its signature: %q", got)
	}
	if len(calls[1].ExtraContent) != 0 {
		t.Errorf("second call was handed a signature it never had: %q", string(calls[1].ExtraContent))
	}
}

// The OpenAI shape must keep working: id once on the opening fragment, index on
// every fragment, arguments in pieces — and two calls that differ only by index.
func TestStreamAccumulatorKeepsIndexedFragmentsApart(t *testing.T) {
	fn := func(name, args string) struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} {
		return struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: name, Arguments: args}
	}
	acc := newStreamToolAccumulator(nil)
	acc.add([]streamToolCallDelta{{Index: 0, ID: "call_a", Type: "function", Function: fn("read", `{"path":`)}})
	acc.add([]streamToolCallDelta{{Index: 1, ID: "call_b", Type: "function", Function: fn("read", `{"path":`)}})
	acc.add([]streamToolCallDelta{{Index: 0, Function: fn("", `"a.txt"}`)}})
	acc.add([]streamToolCallDelta{{Index: 1, Function: fn("", `"b.txt"}`)}})

	calls := acc.finalize()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %#v", len(calls), calls)
	}
	if calls[0].ID != "call_a" || calls[0].Function.Arguments != `{"path":"a.txt"}` {
		t.Errorf("first call is wrong: %#v", calls[0])
	}
	if calls[1].ID != "call_b" || calls[1].Function.Arguments != `{"path":"b.txt"}` {
		t.Errorf("second call is wrong: %#v", calls[1])
	}
}

// OpenAI's newer models answer max_tokens with "400: Unsupported parameter:
// 'max_tokens' is not supported with this model. Use 'max_completion_tokens'
// instead", which ends the turn before a token is generated — measured on
// gpt-5.6-sol, 21 Aug 2026. Every other host in the catalog is still on
// max_tokens and rejects a field it has never heard of, so the switch is
// per-provider and has to stay that way.
func TestOpenAISendsMaxCompletionTokensAndNobodyElseDoes(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = nil
		_ = json.NewDecoder(r.Body).Decode(&body)
		if streamed, _ := body["stream"].(bool); streamed {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	openai, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "openai", Model: "gpt-5.6-sol", APIKey: "sk-x", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider: %v", err)
	}
	if _, err := openai.Complete(context.Background(), Request{
		Messages:  []Message{{Role: RoleUser, Content: "hi"}},
		MaxTokens: 4096,
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, sent := body["max_tokens"]; sent {
		t.Errorf("openai was sent max_tokens, the field it rejects: %v", body)
	}
	if body["max_completion_tokens"] != float64(4096) {
		t.Errorf("max_completion_tokens = %v, want 4096", body["max_completion_tokens"])
	}

	// The streamed path is the one every desktop turn takes, so it has to make
	// the same choice.
	if _, err := openai.StreamComplete(context.Background(), Request{
		Messages:  []Message{{Role: RoleUser, Content: "hi"}},
		MaxTokens: 4096,
	}, nil, nil); err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}
	if _, sent := body["max_tokens"]; sent {
		t.Errorf("streamed openai request carried max_tokens: %v", body)
	}
	if body["max_completion_tokens"] != float64(4096) {
		t.Errorf("streamed max_completion_tokens = %v, want 4096", body["max_completion_tokens"])
	}

	other, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "kimi", Model: "kimi-k2", APIKey: "k", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider (kimi): %v", err)
	}
	if _, err := other.Complete(context.Background(), Request{
		Messages:  []Message{{Role: RoleUser, Content: "hi"}},
		MaxTokens: 4096,
	}); err != nil {
		t.Fatalf("Complete (kimi): %v", err)
	}
	if body["max_tokens"] != float64(4096) {
		t.Errorf("kimi lost max_tokens: %v", body)
	}
	if _, sent := body["max_completion_tokens"]; sent {
		t.Errorf("kimi was sent a field only OpenAI takes: %v", body)
	}
}
