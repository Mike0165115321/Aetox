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

func pdfMessage() []Message {
	return []Message{{
		Role:      RoleUser,
		Content:   "what is the number on page 3",
		Documents: []Document{{Name: "report.pdf", MediaType: "application/pdf", Data: []byte("%PDF-1.7 fake")}},
	}}
}

// Capability used to ENABLE rather than only to restrict, which is the half
// Aetox never had. A model that reads a pdf itself was still being handed 220
// lines of extracted text, on 103 of OpenRouter's models, 35 of opencode's and
// 19 of Gemini's.
//
// The shape is the AI SDK's, from the converter most of this ecosystem talks to
// these endpoints with: {type: "file", file: {filename, file_data}}, file_data
// a data: URL rather than bare base64.
func TestOpenAICompatibleSendsThePDFItself(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "opencode", Model: "claude-opus-5", APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.Complete(context.Background(), Request{
		Model: "claude-opus-5", Messages: pdfMessage()}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	parts, _ := body["messages"].([]any)
	if len(parts) != 1 {
		t.Fatalf("messages = %d, want 1", len(parts))
	}
	content, ok := parts[0].(map[string]any)["content"].([]any)
	if !ok {
		t.Fatalf("content did not become a part list: %#v", parts[0])
	}
	var file map[string]any
	for _, c := range content {
		if m, _ := c.(map[string]any); m != nil && m["type"] == "file" {
			file, _ = m["file"].(map[string]any)
		}
	}
	if file == nil {
		t.Fatalf("no file part was sent: %#v", content)
	}
	if file["filename"] != "report.pdf" {
		t.Errorf("filename = %v, want report.pdf — at least one backend rejects the part without it", file["filename"])
	}
	data, _ := file["file_data"].(string)
	if !strings.HasPrefix(data, "data:application/pdf;base64,") {
		t.Errorf("file_data = %.40q; want a data: URL, not bare base64", data)
	}
}

// The blocker document_capabilities.go wrote down for itself: "an unverified
// wire shape here is a 400 on a turn that works fine today, and the fallback it
// would replace is not broken."
//
// This is what answers it. A gateway that will not take the part costs one
// extra call and the user lands back on pdf_read — which is where they were
// before the native path existed.
func TestARefusedDocumentPartIsReplayedWithoutIt(t *testing.T) {
	var attempts int
	var lastHadFile bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		raw, _ := io.ReadAll(r.Body)
		lastHadFile = strings.Contains(string(raw), `"type":"file"`)
		if lastHadFile {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid content type 'file' for this model"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "opencode", Model: "claude-opus-5", APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	resp, err := p.Complete(context.Background(), Request{
		Model: "claude-opus-5", Messages: pdfMessage()})
	if err != nil {
		t.Fatalf("the turn died on a refused attachment: %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("text = %q, want ok", resp.Text)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2 (refused, then replayed without the document)", attempts)
	}
	if lastHadFile {
		t.Error("the replay carried the document again")
	}
}

// The replay must not fire on a 400 that merely happens to mention a file. It
// is gated on having actually sent one, so an unrelated refusal still ends the
// turn honestly instead of costing a second request.
func TestAnUnrelatedBadRequestIsNotReplayed(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"could not read file at path /etc/x"}}`))
	}))
	defer srv.Close()

	p, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Provider: "opencode", Model: "claude-opus-5", APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.Complete(context.Background(), Request{
		Model:    "claude-opus-5",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}); err == nil {
		t.Fatal("an unrelated 400 was swallowed")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 — a 400 with no document sent must not be replayed", attempts)
	}
}

// Anthropic's envelope is its own: a document block with the media type beside
// the bytes, and it goes BEFORE the text. That ordering is Anthropic's own
// instruction rather than a preference — a document placed after the question
// measurably degrades the answer about it.
func TestAnthropicSendsTheDocumentBlockBeforeTheText(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()

	p, err := NewAnthropicProvider(AnthropicConfig{
		Model: "claude-opus-5", APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := p.Complete(context.Background(), Request{
		Model: "claude-opus-5", Messages: pdfMessage()}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	msgs, _ := body["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	blocks, _ := msgs[0].(map[string]any)["content"].([]any)
	if len(blocks) < 2 {
		t.Fatalf("content blocks = %d, want the document and the text", len(blocks))
	}
	first, _ := blocks[0].(map[string]any)
	if first["type"] != "document" {
		t.Fatalf("first block is %v; the document must come before the question", first["type"])
	}
	source, _ := first["source"].(map[string]any)
	if source["type"] != "base64" || source["media_type"] != "application/pdf" {
		t.Errorf("source = %#v; want base64 + application/pdf", source)
	}
	data, _ := source["data"].(string)
	if strings.ContainsAny(data, "\n\r") {
		t.Error("base64 carries newlines; the API rejects that")
	}
	if strings.HasPrefix(data, "data:") {
		t.Error("sent a data: URL; this envelope takes bare base64 with the media type beside it")
	}
	if last, _ := blocks[len(blocks)-1].(map[string]any); last["type"] != "text" {
		t.Errorf("last block is %v, want the question", last["type"])
	}
}
