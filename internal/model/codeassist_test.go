package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/oauth"
)

func codeAssistServer(t *testing.T, body string, capture func(*http.Request, []byte)) *httptest.Server {
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

// newTestCodeAssist mirrors the real base URL shape. The endpoint addresses
// methods as ":verb" appended to a path (".../v1internal:streamGenerateContent"),
// so a base URL that is only host:port produces something url.Parse reads as a
// malformed port — which is exactly what a user typing a bare host into the
// custom-endpoint box would hit.
func newTestCodeAssist(t *testing.T, url, project string) *CodeAssistProvider {
	t.Helper()
	url += "/v1internal"
	p, err := NewCodeAssistProvider(CodeAssistConfig{
		Provider:    "code-assist",
		Model:       "gemini-2.5-flash",
		BaseURL:     url,
		Project:     project,
		TokenSource: func(context.Context) (string, error) { return "ya29.token", nil },
	})
	if err != nil {
		t.Fatalf("NewCodeAssistProvider: %v", err)
	}
	return p
}

func TestCodeAssistWrapsTheGeminiBodyWithAProject(t *testing.T) {
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
				Name:       "read",
				Parameters: json.RawMessage(`{"type":"object"}`),
			},
		}},
	}

	payload, err := buildCodeAssistRequest("gemini-2.5-pro", "proj-123", req)
	if err != nil {
		t.Fatalf("buildCodeAssistRequest: %v", err)
	}

	// The project is the field this endpoint 500s without.
	if payload.Project != "proj-123" || payload.Model != "gemini-2.5-pro" {
		t.Fatalf("envelope = %+v; want the model and project at the top level", payload)
	}
	// The system prompt is its own field with role "user" — there is no system role.
	if payload.Request.SystemInstruction == nil ||
		payload.Request.SystemInstruction.Parts[0].Text != "You are Aetox." ||
		payload.Request.SystemInstruction.Role != "user" {
		t.Fatalf("systemInstruction = %+v", payload.Request.SystemInstruction)
	}

	contents := payload.Request.Contents
	if len(contents) != 3 {
		t.Fatalf("contents = %d; want user + model(functionCall) + user(functionResponse): %+v", len(contents), contents)
	}
	if contents[1].Role != "model" || contents[1].Parts[0].FunctionCall == nil ||
		contents[1].Parts[0].FunctionCall.Name != "read" {
		t.Fatalf("tool call turn = %+v; want a model turn carrying a functionCall", contents[1])
	}
	// The result goes back as a *user* turn: this API has no tool role, and it
	// matches the call by function name rather than by id.
	result := contents[2]
	if result.Role != "user" || result.Parts[0].FunctionResponse == nil {
		t.Fatalf("tool result turn = %+v; want a user turn carrying a functionResponse", result)
	}
	if result.Parts[0].FunctionResponse.Name != "read" {
		t.Fatalf("functionResponse name = %q; want the name of the call it answers",
			result.Parts[0].FunctionResponse.Name)
	}
	if result.Parts[0].FunctionResponse.Response["output"] != "package main" {
		t.Fatalf("functionResponse = %+v; want the output wrapped in an object", result.Parts[0].FunctionResponse)
	}

	// One tool object holding every declaration; the per-tool spelling is a
	// schema error here.
	if len(payload.Request.Tools) != 1 || len(payload.Request.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("tools = %+v; want one entry holding all declarations", payload.Request.Tools)
	}
}

func TestCodeAssistSeparatesThoughtsFromTheAnswer(t *testing.T) {
	body := sseLines(
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"weighing options","thought":true}]}}]}}`,
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"Tokyo."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":13,"candidatesTokenCount":5,"thoughtsTokenCount":20,"totalTokenCount":38,"cachedContentTokenCount":4},"modelVersion":"gemini-2.5-flash"}}`,
	)
	server := codeAssistServer(t, body, nil)
	defer server.Close()

	var answer, thoughts strings.Builder
	p := newTestCodeAssist(t, server.URL, "proj-1")
	resp, err := p.StreamComplete(context.Background(),
		Request{Messages: []Message{{Role: RoleUser, Content: "capital of Japan?"}}},
		func(c string) error { answer.WriteString(c); return nil },
		func(c string) error { thoughts.WriteString(c); return nil },
	)
	if err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}

	// Both arrive in the same `text` field; only `thought:true` tells them
	// apart, and getting it wrong shows the model's private notes as its answer.
	if resp.Text != "Tokyo." || answer.String() != "Tokyo." {
		t.Fatalf("answer = %q / %q; want only the reply", resp.Text, answer.String())
	}
	if resp.ReasoningContent != "weighing options" || thoughts.String() != "weighing options" {
		t.Fatalf("reasoning = %q / %q", resp.ReasoningContent, thoughts.String())
	}

	if resp.Usage == nil {
		t.Fatal("no usage reported")
	}
	// Thinking tokens are billed and counted apart from the visible answer;
	// dropping them under-reports every reasoning turn.
	if resp.Usage.CompletionTokens != 25 {
		t.Fatalf("completion tokens = %d; want candidates(5) + thoughts(20)", resp.Usage.CompletionTokens)
	}
	if resp.Usage.PromptTokens != 13 || resp.Usage.CachedPromptTokens != 4 {
		t.Fatalf("usage = %+v", *resp.Usage)
	}
	if resp.Model != "gemini-2.5-flash" {
		t.Fatalf("model = %q; want the version the server reported", resp.Model)
	}
}

func TestCodeAssistCollectsFunctionCalls(t *testing.T) {
	body := sseLines(
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Bangkok"}}}]},"finishReason":"STOP"}]}}`,
	)
	server := codeAssistServer(t, body, nil)
	defer server.Close()

	p := newTestCodeAssist(t, server.URL, "proj-1")
	resp, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "weather?"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(resp.ToolCalls))
	}
	call := resp.ToolCalls[0]
	if call.Function.Name != "get_weather" || !strings.Contains(call.Function.Arguments, "Bangkok") {
		t.Fatalf("call = %+v", call.Function)
	}
	// This API often issues no call id, but every caller in Aetox keys a tool
	// result by one.
	if call.ID == "" {
		t.Fatal("tool call has no id; the result could not be routed back to it")
	}
}

func TestCodeAssistSendsProjectAndBearer(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody []byte
	server := codeAssistServer(t,
		sseLines(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}}`),
		func(r *http.Request, raw []byte) {
			gotAuth = r.Header.Get("Authorization")
			gotPath = r.URL.RequestURI()
			gotBody = raw
		})
	defer server.Close()

	p := newTestCodeAssist(t, server.URL, "proj-42")
	if _, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if gotAuth != "Bearer ya29.token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	// alt=sse is what turns this into a stream at all.
	if !strings.Contains(gotPath, ":streamGenerateContent") || !strings.Contains(gotPath, "alt=sse") {
		t.Fatalf("path = %q; want the streaming verb with alt=sse", gotPath)
	}
	if !strings.Contains(string(gotBody), `"project":"proj-42"`) {
		t.Fatalf("body did not carry the project: %s", gotBody)
	}
}

// A request without a project id answers 500 with a body that explains
// nothing, so the runtime has to explain it instead.
func TestCodeAssistExplainsTheMissingProject500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":500,"message":"Internal error"}}`))
	}))
	defer server.Close()

	p := newTestCodeAssist(t, server.URL, "")
	_, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "no project id") {
		t.Fatalf("err = %v; want it to name the missing project", err)
	}
}

func TestCodeAssistFactoryUsesSignInProject(t *testing.T) {
	signIn(t, "code-assist", oauth.Credential{
		Type: "oauth", Access: "ya29.x", Account: "proj-from-signin",
		Endpoint: "https://cloudcode-pa.googleapis.com/v1internal",
	})

	p, err := NewProvider(ProviderOptions{Provider: "gemini-code-assist", Model: "gemini-2.5-flash"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ca, ok := p.(*CodeAssistProvider)
	if !ok {
		t.Fatalf("provider type = %T; want the code-assist runtime", p)
	}
	if ca.project != "proj-from-signin" {
		t.Fatalf("project = %q; want the one the sign-in resolved", ca.project)
	}
}

// "gemini" is the public API-key endpoint and must keep meaning that.
func TestGeminiProviderIsNotCodeAssist(t *testing.T) {
	if got := NormalizeProvider("gemini"); got != "gemini" {
		t.Fatalf("NormalizeProvider(gemini) = %q", got)
	}
	if got := NormalizeProvider("gemini-code-assist"); got != "code-assist" {
		t.Fatalf("NormalizeProvider(gemini-code-assist) = %q", got)
	}
}
