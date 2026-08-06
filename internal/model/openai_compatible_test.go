package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
