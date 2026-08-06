package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseLines joins event payloads into a stream body. The blank line between
// events is part of the framing, not decoration.
func sseLines(events ...string) string {
	return strings.Join(events, "\n\n") + "\n\n"
}

func responsesServer(t *testing.T, body string, capture func(*http.Request, []byte)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(raw)
		if capture != nil {
			capture(r, raw)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
}

func TestResponsesBuildsTypedInputAndFlatTools(t *testing.T) {
	req := Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are Aetox."},
			{Role: RoleUser, Content: "read the file"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{
				ID: "call_1", Type: "function",
				Function: FunctionCall{Name: "read", Arguments: `{"path":"a.go"}`},
			}}},
			{Role: RoleTool, ToolCallID: "call_1", Content: "package main"},
		},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunction{
				Name:        "read",
				Description: "read a file",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		}},
	}

	payload, err := buildResponsesRequest("codex", "gpt-5.1-codex", req)
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}

	// The system prompt is a top-level field here, not an item.
	if payload.Instructions != "You are Aetox." {
		t.Fatalf("instructions = %q; want the system message", payload.Instructions)
	}
	for _, item := range payload.Input {
		if item.Role == "system" {
			t.Fatal("a system item was sent as input; this endpoint ignores those")
		}
	}

	if len(payload.Input) != 3 {
		t.Fatalf("input items = %d; want user + function_call + function_call_output, got %+v", len(payload.Input), payload.Input)
	}
	if payload.Input[0].Type != "message" || payload.Input[0].Role != "user" ||
		payload.Input[0].Content[0].Type != "input_text" {
		t.Fatalf("user item = %+v; want a message with input_text", payload.Input[0])
	}
	// The assistant's call is an item of its own, not a field on a message.
	call := payload.Input[1]
	if call.Type != "function_call" || call.CallID != "call_1" || call.Name != "read" {
		t.Fatalf("tool call item = %+v; want a function_call carrying call_1", call)
	}
	// And its result comes back keyed by the same call_id.
	result := payload.Input[2]
	if result.Type != "function_call_output" || result.CallID != "call_1" || result.Output != "package main" {
		t.Fatalf("tool result item = %+v; want a function_call_output for call_1", result)
	}

	if len(payload.Tools) != 1 {
		t.Fatalf("tools = %d; want 1", len(payload.Tools))
	}
	// Flat, not nested under "function" — the nested form is a 400 that names
	// no field.
	if payload.Tools[0].Name != "read" || payload.Tools[0].Type != "function" {
		t.Fatalf("tool = %+v; want name and type on the tool itself", payload.Tools[0])
	}

	if payload.Store {
		t.Fatal("store = true; the ChatGPT backend refuses to persist third-party turns")
	}
	if !payload.Stream {
		t.Fatal("stream = false; this endpoint only answers text/event-stream")
	}
}

func TestResponsesStreamAssemblesTextToolCallAndUsage(t *testing.T) {
	body := sseLines(
		`data: {"type":"response.created","response":{"model":"gpt-5.1-codex"}}`,
		`data: {"type":"response.reasoning_summary_text.delta","delta":"thinking…"}`,
		`data: {"type":"response.output_text.delta","delta":"one moment"}`,
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_1","call_id":"call_abc","name":"read","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"path\":"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"\"a.go\"}"}`,
		`data: {"type":"response.output_item.done","item":{"type":"function_call","id":"item_1","call_id":"call_abc","name":"read","arguments":"{\"path\":\"a.go\"}"}}`,
		`data: {"type":"response.completed","response":{"model":"gpt-5.1-codex","usage":{"input_tokens":1200,"output_tokens":48,"total_tokens":1248,"input_tokens_details":{"cached_tokens":1024}}}}`,
	)
	server := responsesServer(t, body, nil)
	defer server.Close()

	p, err := NewResponsesProvider(ResponsesConfig{
		Provider: "codex", Model: "gpt-5.1-codex", BaseURL: server.URL, APIKey: "tok",
	})
	if err != nil {
		t.Fatalf("NewResponsesProvider: %v", err)
	}

	var streamed, reasoned strings.Builder
	resp, err := p.StreamComplete(context.Background(),
		Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}},
		func(chunk string) error { streamed.WriteString(chunk); return nil },
		func(chunk string) error { reasoned.WriteString(chunk); return nil },
	)
	if err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}

	if resp.Text != "one moment" {
		t.Fatalf("text = %q", resp.Text)
	}
	if streamed.String() != "one moment" {
		t.Fatalf("streamed = %q; want the same text through the callback", streamed.String())
	}
	if resp.ReasoningContent != "thinking…" || reasoned.String() != "thinking…" {
		t.Fatalf("reasoning = %q / %q; want the summary deltas", resp.ReasoningContent, reasoned.String())
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d; want 1", len(resp.ToolCalls))
	}
	call := resp.ToolCalls[0]
	// call_id, not the item id: that is what function_call_output must echo.
	if call.ID != "call_abc" {
		t.Fatalf("tool call id = %q; want the call_id the endpoint will match on", call.ID)
	}
	if call.Function.Name != "read" || call.Function.Arguments != `{"path":"a.go"}` {
		t.Fatalf("tool call = %+v", call.Function)
	}

	if resp.Usage == nil {
		t.Fatal("usage was not reported")
	}
	if resp.Usage.PromptTokens != 1200 || resp.Usage.CachedPromptTokens != 1024 || resp.Usage.CompletionTokens != 48 {
		t.Fatalf("usage = %+v; want input 1200 (1024 cached), output 48", *resp.Usage)
	}
	if !resp.Usage.CacheReported {
		t.Fatal("cache accounting was not marked as reported")
	}
}

// A dropped or reordered arguments delta produces invalid JSON that only fails
// later, at tool-dispatch time, far from the cause. The done event carries the
// complete arguments, so it wins.
func TestResponsesDoneEventOverridesPartialArguments(t *testing.T) {
	body := sseLines(
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"write","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"path\":\"trunc"}`,
		`data: {"type":"response.output_item.done","item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"write","arguments":"{\"path\":\"whole.go\"}"}}`,
		`data: {"type":"response.completed","response":{}}`,
	)
	server := responsesServer(t, body, nil)
	defer server.Close()

	p, _ := NewResponsesProvider(ResponsesConfig{Provider: "codex", Model: "m", BaseURL: server.URL, APIKey: "tok"})
	resp, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := resp.ToolCalls[0].Function.Arguments; got != `{"path":"whole.go"}` {
		t.Fatalf("arguments = %q; want the complete set from the done event", got)
	}
}

func TestResponsesSendsSubscriptionHeaders(t *testing.T) {
	var gotAuth, gotAccount, gotOriginator, gotBeta, gotAccept string
	server := responsesServer(t,
		sseLines(`data: {"type":"response.output_text.delta","delta":"hi"}`, `data: {"type":"response.completed","response":{}}`),
		func(r *http.Request, _ []byte) {
			gotAuth = r.Header.Get("Authorization")
			gotAccount = r.Header.Get("chatgpt-account-id")
			gotOriginator = r.Header.Get("originator")
			gotBeta = r.Header.Get("OpenAI-Beta")
			gotAccept = r.Header.Get("Accept")
		})
	defer server.Close()

	calls := 0
	p, _ := NewResponsesProvider(ResponsesConfig{
		Provider: "codex", Model: "m", BaseURL: server.URL,
		TokenSource: func(context.Context) (string, error) { calls++; return "live-token", nil },
		Headers:     map[string]string{"chatgpt-account-id": "acct_123", "originator": "aetox"},
	})
	if _, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if gotAuth != "Bearer live-token" || calls != 1 {
		t.Fatalf("auth = %q after %d token lookups", gotAuth, calls)
	}
	// Without the account id the backend cannot route to the user's plan.
	if gotAccount != "acct_123" {
		t.Fatalf("chatgpt-account-id = %q", gotAccount)
	}
	if gotOriginator != "aetox" || gotBeta != "responses=experimental" || gotAccept != "text/event-stream" {
		t.Fatalf("originator=%q beta=%q accept=%q", gotOriginator, gotBeta, gotAccept)
	}
}

// 401, 403 and 429 mean three different things here and send the user to three
// different fixes; a bare status code sends them to the wrong one.
func TestResponsesStatusErrorsAreActionable(t *testing.T) {
	cases := map[int]string{
		http.StatusUnauthorized:    "sign in again",
		http.StatusForbidden:       "plan may not include it",
		http.StatusTooManyRequests: "plan limit reached",
	}
	for status, want := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
		}))
		p, _ := NewResponsesProvider(ResponsesConfig{Provider: "codex", Model: "m", BaseURL: server.URL, APIKey: "tok"})
		_, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
		server.Close()

		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("status %d gave %v; want it to mention %q", status, err, want)
		}
	}
}

func TestResponsesSurfacesFailedEvent(t *testing.T) {
	body := sseLines(
		`data: {"type":"response.output_text.delta","delta":"partial"}`,
		`data: {"type":"response.failed","response":{"error":{"message":"model overloaded"}}}`,
	)
	server := responsesServer(t, body, nil)
	defer server.Close()

	p, _ := NewResponsesProvider(ResponsesConfig{Provider: "codex", Model: "m", BaseURL: server.URL, APIKey: "tok"})
	_, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "model overloaded") {
		t.Fatalf("err = %v; want the provider's own message", err)
	}
}

// The endpoint adds event types over time. Erroring on an unrecognized one
// breaks Aetox on someone else's release day.
func TestResponsesIgnoresUnknownEvents(t *testing.T) {
	body := sseLines(
		`data: {"type":"response.some_future_thing","payload":{"whatever":1}}`,
		`data: not json at all`,
		`data: {"type":"response.output_text.delta","delta":"still here"}`,
		`data: {"type":"response.completed","response":{}}`,
	)
	server := responsesServer(t, body, nil)
	defer server.Close()

	p, _ := NewResponsesProvider(ResponsesConfig{Provider: "codex", Model: "m", BaseURL: server.URL, APIKey: "tok"})
	resp, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "still here" {
		t.Fatalf("text = %q", resp.Text)
	}
}

func TestResponsesReportsToolProgressWhileArgumentsStream(t *testing.T) {
	body := sseLines(
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"write","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"path\":\"a.go\","}`,
		`data: {"type":"response.output_item.done","item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"write","arguments":"{\"path\":\"a.go\"}"}}`,
		`data: {"type":"response.completed","response":{}}`,
	)
	server := responsesServer(t, body, nil)
	defer server.Close()

	var names []string
	p, _ := NewResponsesProvider(ResponsesConfig{Provider: "codex", Model: "m", BaseURL: server.URL, APIKey: "tok"})
	_, err := p.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		OnToolCallProgress: func(_, name, _ string, _ int) {
			names = append(names, name)
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	// The name arrives before any argument, so the UI can draw the row at once
	// instead of sitting silent through a long write.
	if len(names) == 0 || names[0] != "write" {
		t.Fatalf("progress names = %v; want the tool named on first sighting", names)
	}
}

func TestResponsesFactoryBuildsCodexFromSignIn(t *testing.T) {
	signIn(t, "codex", oauthCredential("tok", "acct_9"))

	p, err := NewProvider(ProviderOptions{Provider: "chatgpt-codex", Model: "gpt-5.1-codex"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	responses, ok := p.(*ResponsesProvider)
	if !ok {
		t.Fatalf("provider type = %T; want the responses runtime", p)
	}
	if responses.headers["chatgpt-account-id"] != "acct_9" {
		t.Fatalf("headers = %v; want the account id from the sign-in", responses.headers)
	}
	if responses.baseURL != "https://chatgpt.com/backend-api/codex" {
		t.Fatalf("baseURL = %q", responses.baseURL)
	}
}

// "chatgpt" has meant the API-key OpenAI provider since before this runtime
// existed. Moving it would silently repoint saved preferences at a different
// account and a different bill.
func TestChatGPTAliasStillMeansOpenAI(t *testing.T) {
	if got := NormalizeProvider("chatgpt"); got != "openai" {
		t.Fatalf("NormalizeProvider(chatgpt) = %q; want openai", got)
	}
	if got := NormalizeProvider("codex"); got != "codex" {
		t.Fatalf("NormalizeProvider(codex) = %q; want codex", got)
	}
}
