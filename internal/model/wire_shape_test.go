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

// What actually goes on the wire, per dialect.
//
// Sixteen of the twenty-two rows speak openai-compatible, two speak anthropic,
// two speak responses and one speaks ollama — so a dialect getting this wrong
// is not one provider broken, it is most of them. These tests hold a real HTTP
// server up to each client and read the JSON it sends, rather than asserting on
// the Go structs on the way in: a struct tag typo, a missing omitempty or a
// field renamed by a refactor all look fine from inside and are a 400 outside.
//
// One request shape for all of them — text, an image, a document, a tool and a
// thinking level — because the interesting failures are in how the parts are
// combined, not in any one part alone.

func kitchenSinkRequest(model string) Request {
	return Request{
		Model:     model,
		MaxTokens: 1024,
		Messages: []Message{{
			Role:      RoleUser,
			Content:   "what is on page 3 and in this picture",
			Images:    []Image{{MediaType: "image/png", Data: []byte("\x89PNG fake")}},
			Documents: []Document{{Name: "report.pdf", MediaType: "application/pdf", Data: []byte("%PDF fake")}},
		}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunction{
				Name:        "read",
				Description: "read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		}},
		Reasoning: &ReasoningConfig{Effort: "high"},
	}
}

// responsesSSE is a minimal but complete event stream for the responses
// dialect, which always streams — a single JSON body reaches it as an empty
// answer rather than as a parse error, which is a confusing way to learn that.
const responsesSSE = `data: {"type":"response.created","response":{"model":"m"}}

data: {"type":"response.output_text.delta","delta":"ok"}

data: {"type":"response.completed","response":{"model":"m"}}

`

// captureBody answers one request with `reply` and hands back what was sent.
func captureBody(t *testing.T, reply string) (*httptest.Server, func() map[string]any) {
	t.Helper()
	var seen map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &seen); err != nil {
			t.Errorf("the client sent something that is not JSON: %v", err)
		}
		if strings.HasPrefix(reply, "data:") {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = w.Write([]byte(reply))
	}))
	return srv, func() map[string]any { return seen }
}

func firstUserContent(t *testing.T, body map[string]any, key string) []any {
	t.Helper()
	msgs, _ := body[key].([]any)
	if len(msgs) == 0 {
		t.Fatalf("%s carried no messages: %#v", key, body)
	}
	content, ok := msgs[0].(map[string]any)["content"].([]any)
	if !ok {
		t.Fatalf("the user message did not become a part list: %#v", msgs[0])
	}
	return content
}

func partTypes(content []any) []string {
	var out []string
	for _, c := range content {
		if m, _ := c.(map[string]any); m != nil {
			if s, _ := m["type"].(string); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func hasPart(content []any, want string) bool {
	for _, got := range partTypes(content) {
		if got == want {
			return true
		}
	}
	return false
}

// openai-compatible: sixteen rows ride on this one.
func TestWireShapeOpenAICompatible(t *testing.T) {
	prev := installedCatalog
	t.Cleanup(func() { SetModelCatalog(prev) })
	SetModelCatalog(&ModelCatalog{Models: map[string]ModelFacts{
		"opencode/claude-opus-5": {
			Context: 1000000, ToolCall: true, Reasoning: true,
			Input: []string{"text", "image", "pdf"}, Output: []string{"text"},
		},
	}})

	srv, body := captureBody(t, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	defer srv.Close()

	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "opencode", Model: "claude-opus-5", APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.Complete(context.Background(), kitchenSinkRequest("claude-opus-5")); err != nil {
		t.Fatalf("complete: %v", err)
	}

	sent := body()
	content := firstUserContent(t, sent, "messages")
	for _, want := range []string{"text", "image_url", "file"} {
		if !hasPart(content, want) {
			t.Errorf("no %q part; got %v", want, partTypes(content))
		}
	}
	// Text first: the question reads as a caption under the picture otherwise,
	// and several providers weight the last part most heavily.
	if got := partTypes(content); len(got) == 0 || got[0] != "text" {
		t.Errorf("part order = %v; the question must come first", got)
	}
	if _, ok := sent["tools"].([]any); !ok {
		t.Errorf("no tools were sent to a model the catalog says calls them: %#v", sent["tools"])
	}
	if sent["stream"] == true {
		t.Error("Complete asked for a stream")
	}
}

// anthropic: its own envelope for every part, and the document goes BEFORE the
// text, which is Anthropic's own instruction rather than a preference.
func TestWireShapeAnthropic(t *testing.T) {
	srv, body := captureBody(t, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`)
	defer srv.Close()

	p, err := NewAnthropicProvider(AnthropicConfig{
		Model: "claude-opus-5", APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.Complete(context.Background(), kitchenSinkRequest("claude-opus-5")); err != nil {
		t.Fatalf("complete: %v", err)
	}

	sent := body()
	content := firstUserContent(t, sent, "messages")
	order := partTypes(content)
	if len(order) < 3 {
		t.Fatalf("blocks = %v; want document, text and image", order)
	}
	if order[0] != "document" {
		t.Errorf("first block is %q; the document must precede the question", order[0])
	}
	if !hasPart(content, "image") || !hasPart(content, "text") {
		t.Errorf("blocks = %v; want an image and the question too", order)
	}
	// This dialect takes bare base64 with the media type beside it, never the
	// data: URL the openai-compatible one takes. Sending the wrong one is a 400
	// that reads like a schema complaint.
	for _, c := range content {
		m, _ := c.(map[string]any)
		if m == nil || (m["type"] != "document" && m["type"] != "image") {
			continue
		}
		src, _ := m["source"].(map[string]any)
		if src == nil {
			t.Fatalf("%v block has no source object", m["type"])
		}
		if src["type"] != "base64" || src["media_type"] == "" {
			t.Errorf("%v source = %#v; want base64 with a media_type", m["type"], src)
		}
		data, _ := src["data"].(string)
		if strings.HasPrefix(data, "data:") {
			t.Errorf("%v carried a data: URL; this dialect takes bare base64", m["type"])
		}
		if strings.ContainsAny(data, "\r\n") {
			t.Errorf("%v base64 carries newlines; the API rejects that", m["type"])
		}
	}
	if _, ok := sent["tools"].([]any); !ok {
		t.Error("no tools were sent")
	}
}

// responses: the dialect a ChatGPT subscription speaks. Different part names
// again, and the only one verified end to end against a real backend.
func TestWireShapeResponses(t *testing.T) {
	srv, body := captureBody(t, responsesSSE)
	defer srv.Close()

	p, err := NewResponsesProvider(ResponsesConfig{
		Provider: "codex", Model: "gpt-5.5", APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.Complete(context.Background(), kitchenSinkRequest("gpt-5.5")); err != nil {
		t.Fatalf("complete: %v", err)
	}

	sent := body()
	content := firstUserContent(t, sent, "input")
	for _, want := range []string{"input_text", "input_image", "input_file"} {
		if !hasPart(content, want) {
			t.Errorf("no %q part; got %v", want, partTypes(content))
		}
	}
	for _, c := range content {
		m, _ := c.(map[string]any)
		if m == nil || m["type"] != "input_file" {
			continue
		}
		if m["filename"] == "" || m["filename"] == nil {
			t.Error("input_file has no filename; at least one backend rejects the part without it")
		}
		if data, _ := m["file_data"].(string); !strings.HasPrefix(data, "data:") {
			t.Errorf("file_data = %.30q; this dialect takes a data: URL", data)
		}
	}
}

// ollama: the local runtime, and the one dialect with no file part at all. What
// matters here is that nothing tries to invent one.
func TestWireShapeOllamaCarriesNoDocument(t *testing.T) {
	srv, body := captureBody(t, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
	defer srv.Close()

	p, err := NewOllamaProvider(OllamaConfig{Model: "llava:13b", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.Complete(context.Background(), kitchenSinkRequest("llava:13b")); err != nil {
		t.Fatalf("complete: %v", err)
	}

	raw, _ := json.Marshal(body())
	for _, forbidden := range []string{"input_file", "file_data", `"type":"file"`, `"type":"document"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("the ollama request carried %q; its API has no such part", forbidden)
		}
	}
	// It does take images, and by its own spelling: a bare base64 array.
	if !strings.Contains(string(raw), "images") {
		t.Error("the image did not reach the ollama request")
	}
	if ResolveDocuments("ollama", "llava:13b") {
		t.Error("ResolveDocuments said yes for a runtime with nowhere to put one")
	}
}

// Every dialect must survive a request with nothing attached. The part-list
// branches only run when there is something to attach, so this is the path most
// turns actually take and the one a refactor is likeliest to break silently.
func TestEveryDialectSendsAPlainTurn(t *testing.T) {
	plain := Request{
		Model:     "m",
		MaxTokens: 512,
		Messages:  []Message{{Role: RoleUser, Content: "hello"}},
	}

	t.Run("openai-compatible", func(t *testing.T) {
		srv, body := captureBody(t, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
		defer srv.Close()
		p, _ := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
			Provider: "opencode", Model: "m", APIKey: "k", BaseURL: srv.URL})
		if _, err := p.Complete(context.Background(), plain); err != nil {
			t.Fatalf("complete: %v", err)
		}
		msgs, _ := body()["messages"].([]any)
		if len(msgs) != 1 {
			t.Fatalf("messages = %d", len(msgs))
		}
		if got, _ := msgs[0].(map[string]any)["content"].(string); got != "hello" {
			t.Errorf("content = %#v; a plain turn must stay a plain string", msgs[0].(map[string]any)["content"])
		}
	})

	t.Run("anthropic", func(t *testing.T) {
		srv, body := captureBody(t, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`)
		defer srv.Close()
		p, _ := NewAnthropicProvider(AnthropicConfig{Model: "m", APIKey: "k", BaseURL: srv.URL})
		if _, err := p.Complete(context.Background(), plain); err != nil {
			t.Fatalf("complete: %v", err)
		}
		content := firstUserContent(t, body(), "messages")
		if got := partTypes(content); len(got) != 1 || got[0] != "text" {
			t.Errorf("blocks = %v; a plain turn is one text block", got)
		}
	})

	t.Run("responses", func(t *testing.T) {
		srv, body := captureBody(t, responsesSSE)
		defer srv.Close()
		p, _ := NewResponsesProvider(ResponsesConfig{
			Provider: "codex", Model: "m", APIKey: "k", BaseURL: srv.URL})
		if _, err := p.Complete(context.Background(), plain); err != nil {
			t.Fatalf("complete: %v", err)
		}
		if content := firstUserContent(t, body(), "input"); !hasPart(content, "input_text") {
			t.Errorf("parts = %v; want input_text", partTypes(content))
		}
	})
}
