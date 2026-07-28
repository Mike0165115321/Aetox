package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestAnthropicProviderMapsMaxTokensStopReasonToLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "claude-haiku-4-5",
			"content": [{"type":"text","text":"partial answ"}],
			"stop_reason": "max_tokens",
			"usage": {"input_tokens": 5, "output_tokens": 8192}
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
		Messages: []Message{{Role: RoleUser, Content: "long answer please"}},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.FinishReason != FinishReasonLength {
		t.Fatalf("expected stop_reason max_tokens mapped to %q, got %q", FinishReasonLength, resp.FinishReason)
	}
}

func TestAnthropicProviderKeepsEndTurnStopReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"model": "claude-haiku-4-5",
			"content": [{"type":"text","text":"done"}],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 5, "output_tokens": 3}
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
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.FinishReason != "end_turn" {
		t.Fatalf("expected end_turn passthrough, got %q", resp.FinishReason)
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

// The Anthropic wire format is what Claude uses — and DeepSeek's default — so
// this is the path most tool calls actually take. It had no progress reporting
// at all: a model writing a large file went silent from the end of its thinking
// until the call was complete, which reads as a freeze.
func TestAnthropicStreamReportsToolCallProgress(t *testing.T) {
	defer func(prev time.Duration) { toolProgressInterval = prev }(toolProgressInterval)
	toolProgressInterval = 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-haiku-4-5\",\"usage\":{\"input_tokens\":4}}}\n\n"))
		// The name arrives before a single byte of input — the row can open at once.
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_01\",\"name\":\"write\"}}\n\n"))
		for _, frag := range []string{
			// \\n here so the JSON-decoded partial_json carries an escaped \n,
			// which is what a real newline inside a JSON string value looks like
			// on the wire — and what ContentLinesSoFar counts.
			`{\"path\": \"lan`,
			`ding.html\", \"content\": \"<h1>a</h1>\\n`,
			`<p>b</p>\\n`,
			`<p>c</p>\\n\"}`,
		} {
			_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"" + frag + "\"}}\n\n"))
		}
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider, err := NewAnthropicProvider(AnthropicConfig{Model: "claude-haiku-4-5", APIKey: "k", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	type upd struct {
		id, name, subject string
		lines             int
	}
	var seen []upd
	resp, err := provider.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "make a page"}},
		OnToolCallProgress: func(id, name, subject string, lines int) {
			seen = append(seen, upd{id, name, subject, lines})
		},
	}, nil, nil)
	if err != nil {
		t.Fatalf("stream complete failed: %v", err)
	}

	if len(seen) == 0 {
		t.Fatal("no progress at all — this is the freeze the tracker exists to prevent")
	}
	// The row opens on the tool name, before any arguments exist.
	if seen[0].name != "write" || seen[0].id != "toolu_01" || seen[0].lines != 0 {
		t.Errorf("first update = %+v, want the row opening on name+id with no content yet", seen[0])
	}
	// It then names itself and the count climbs.
	last := seen[len(seen)-1]
	if last.subject != "landing.html" {
		t.Errorf("final update never carried the path: %+v", last)
	}
	if last.lines != 4 {
		t.Errorf("final line count = %d, want 4", last.lines)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i].lines < seen[i-1].lines {
			t.Errorf("count went backwards at %d: %+v after %+v", i, seen[i], seen[i-1])
		}
		if seen[i].id != "toolu_01" {
			t.Errorf("update %d lost the call id: %+v", i, seen[i])
		}
	}

	// The id the UI saw while streaming must be the id the finished call carries,
	// or the timeline draws the same call twice.
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "toolu_01" {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
	if got := resp.ToolCalls[0].Function.Arguments; got != `{"path": "landing.html", "content": "<h1>a</h1>\n<p>b</p>\n<p>c</p>\n"}` {
		t.Errorf("arguments not stitched back correctly: %q", got)
	}
}

// The picker used to show three Claude names written into the catalog by hand.
// They go stale every few months, and a stale one is a 404 that reads like a
// bug in Aetox — so the provider is asked instead.
func TestDiscoverAnthropicModelsPaginates(t *testing.T) {
	var gotAuth, gotVersion string
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		seen = append(seen, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after_id") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-a"},{"id":"claude-b"}],"has_more":true,"last_id":"claude-b"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-c"}],"has_more":false,"last_id":"claude-c"}`))
	}))
	defer server.Close()

	models, err := DiscoverAnthropicModels(context.Background(), "anthropic", server.URL, "sk-ant-key", nil)
	if err != nil {
		t.Fatalf("DiscoverAnthropicModels: %v", err)
	}
	if strings.Join(models, ",") != "claude-a,claude-b,claude-c" {
		t.Fatalf("models = %v; want every page, in order", models)
	}
	if len(seen) != 2 || !strings.Contains(seen[1], "after_id=claude-b") {
		t.Fatalf("requests = %v; want the second page keyed off last_id", seen)
	}
	if gotAuth != "sk-ant-key" || gotVersion != anthropicAPIVersion {
		t.Fatalf("auth = %q, version = %q", gotAuth, gotVersion)
	}
}

func TestDiscoverAnthropicModelsUsesTheSignInWhenThereIsOne(t *testing.T) {
	var gotAuth, gotAPIKey, gotBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		gotBeta = r.Header.Get("anthropic-beta")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-x"}],"has_more":false}`))
	}))
	defer server.Close()

	models, err := DiscoverAnthropicModels(context.Background(), "anthropic", server.URL, "",
		func(context.Context) (string, error) { return "oat-token", nil })
	if err != nil || len(models) != 1 {
		t.Fatalf("models = %v, err = %v", models, err)
	}
	if gotAuth != "Bearer oat-token" || gotAPIKey != "" || gotBeta == "" {
		t.Fatalf("subscription path sent Authorization=%q x-api-key=%q beta=%q", gotAuth, gotAPIKey, gotBeta)
	}
}

// The desktop pings a provider with a tiny max_tokens to prove it is reachable.
// A model that spends that budget before emitting a visible token is a working
// provider, and reporting it as "response has empty text" told users otherwise.
func TestAnthropicTruncatedPingIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[],"stop_reason":"max_tokens","usage":{"input_tokens":9,"output_tokens":1}}`))
	}))
	defer server.Close()

	p, err := NewAnthropicProvider(AnthropicConfig{Provider: "anthropic", Model: "claude-x", APIKey: "k", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewAnthropicProvider: %v", err)
	}
	resp, err := p.Complete(context.Background(), Request{
		Messages:  []Message{{Role: RoleUser, Content: "ping"}},
		MaxTokens: 1,
	})
	if err != nil {
		t.Fatalf("a truncated ping was reported as a failure: %v", err)
	}
	if resp.FinishReason != FinishReasonLength {
		t.Fatalf("FinishReason = %q; want the truncation marker", resp.FinishReason)
	}
}

// But a genuinely empty answer with no reason to be empty is still an error.
func TestAnthropicEmptyAnswerWithoutTruncationStillFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[],"stop_reason":"end_turn"}`))
	}))
	defer server.Close()

	p, _ := NewAnthropicProvider(AnthropicConfig{Provider: "anthropic", Model: "claude-x", APIKey: "k", BaseURL: server.URL})
	if _, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err == nil {
		t.Fatal("an empty end_turn answer was accepted")
	}
}

// The thinking contract on current Claude models, in one test.
//
// There are exactly two knobs — thinking is adaptive or off, and depth is
// output_config.effort. The obvious-looking third one, thinking.budget_tokens,
// is rejected with a 400 by every current model, so a level must never become a
// token count.
func TestAnthropicThinkingUsesEffortNotBudget(t *testing.T) {
	payload, err := buildAnthropicRequest("anthropic", "claude-sonnet-5", Request{
		Messages:  []Message{{Role: RoleUser, Content: "hi"}},
		Reasoning: &ReasoningConfig{Effort: "xhigh"},
	}, false, false)
	if err != nil {
		t.Fatalf("buildAnthropicRequest: %v", err)
	}
	if payload.Thinking == nil || payload.Thinking.Type != "adaptive" {
		t.Fatalf("thinking = %+v; want adaptive", payload.Thinking)
	}
	// Without this the reasoning arrives as empty strings and the thinking
	// panel stays blank on a model that is visibly thinking.
	if payload.Thinking.Display != "summarized" {
		t.Fatalf("thinking.display = %q; want summarized", payload.Thinking.Display)
	}
	if payload.OutputConfig == nil || payload.OutputConfig.Effort != "xhigh" {
		t.Fatalf("output_config = %+v; want the effort the user chose", payload.OutputConfig)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "budget_tokens") {
		t.Fatalf("request carries budget_tokens, which is a 400 on every current model: %s", body)
	}
}

// Sampling and thinking cannot coexist on Anthropic: the API answers
// "temperature may only be set to 1 when thinking is enabled or in adaptive
// mode", and Aetox sets a temperature on every call — so this rejected every
// Claude request with a think level chosen.
func TestAnthropicOmitsTemperatureButOtherProvidersKeepIt(t *testing.T) {
	req := Request{
		Messages:    []Message{{Role: RoleUser, Content: "hi"}},
		Temperature: 0.2,
		Reasoning:   &ReasoningConfig{Effort: "high"},
	}

	claude, err := buildAnthropicRequest("anthropic", "claude-sonnet-5", req, false, false)
	if err != nil {
		t.Fatalf("buildAnthropicRequest: %v", err)
	}
	// omitempty means zero is absent on the wire, which is the goal.
	if claude.Temperature != 0 {
		t.Fatalf("temperature = %v; want it left off for Anthropic", claude.Temperature)
	}

	// DeepSeek only borrows this wire format and still honors temperature —
	// dropping it there would be a silent behavior change for a provider that
	// never had the problem.
	deepseek, err := buildAnthropicRequest("deepseek", "deepseek-v4-flash", req, false, false)
	if err != nil {
		t.Fatalf("buildAnthropicRequest: %v", err)
	}
	if deepseek.Temperature != 0.2 {
		t.Fatalf("temperature = %v; want the caller's value kept for DeepSeek", deepseek.Temperature)
	}
}

func TestAnthropicThinkingOff(t *testing.T) {
	off := Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Thinking: &ThinkingConfig{Type: "disabled"},
	}

	payload, err := buildAnthropicRequest("anthropic", "claude-sonnet-5", off, false, false)
	if err != nil {
		t.Fatalf("buildAnthropicRequest: %v", err)
	}
	if payload.Thinking == nil || payload.Thinking.Type != "disabled" {
		t.Fatalf("thinking = %+v; want an explicit disable", payload.Thinking)
	}
	if payload.OutputConfig != nil {
		t.Fatalf("output_config = %+v; effort is meaningless with thinking off", payload.OutputConfig)
	}

	// Fable-class models think unconditionally and reject an explicit disable,
	// so "off" there has to mean sending no thinking field at all.
	fable, err := buildAnthropicRequest("anthropic", "claude-fable-5", off, false, false)
	if err != nil {
		t.Fatalf("buildAnthropicRequest: %v", err)
	}
	if fable.Thinking != nil {
		t.Fatalf("thinking = %+v; want the field omitted entirely on a fable model", fable.Thinking)
	}
}

// A Pro/Max sign-in reports its limits as anthropic-ratelimit-unified-reset in
// Unix seconds — not the RFC3339 per-key headers — and the one 429 a real user
// hit showed "try again shortly" because the runtime could read neither. Both
// dialects must resolve to a real duration.
func TestRateLimitWindowReadsBothHeaderDialects(t *testing.T) {
	unified := &http.Response{Header: http.Header{}}
	unified.Header.Set("anthropic-ratelimit-unified-reset",
		strconv.FormatInt(time.Now().Add(90*time.Second).Unix(), 10))
	if wait := rateLimitWindow(unified); wait < 80*time.Second || wait > 100*time.Second {
		t.Fatalf("unified unix reset → %s; want ~90s", wait)
	}

	apiKey := &http.Response{Header: http.Header{}}
	apiKey.Header.Set("anthropic-ratelimit-tokens-reset",
		time.Now().Add(45*time.Second).UTC().Format(time.RFC3339))
	if wait := rateLimitWindow(apiKey); wait < 35*time.Second || wait > 55*time.Second {
		t.Fatalf("rfc3339 reset → %s; want ~45s", wait)
	}

	silent := &http.Response{Header: http.Header{}}
	if wait := rateLimitWindow(silent); wait != 0 {
		t.Fatalf("no headers → %s; want 0", wait)
	}
}
