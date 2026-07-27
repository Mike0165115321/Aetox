package skill

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetchExtractsTextImagesAndLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Phone Review 2026</title><style>body{color:red}</style></head>
<body>
<script>evil()</script>
<h1>Best phones</h1>
<p>The Foo Phone 12 has a great camera.</p>
<img src="/img/foo12.jpg" alt="Foo Phone 12">
<img src="data:image/png;base64,xx" alt="inline junk">
<a href="/reviews/foo12">Full review</a>
<a href="javascript:void(0)">Ignore me</a>
</body></html>`))
	}))
	defer server.Close()

	s := &webFetchSkill{}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	content := out.Content
	if !strings.Contains(content, "Phone Review 2026") {
		t.Errorf("missing title, got:\n%s", content)
	}
	if !strings.Contains(content, "The Foo Phone 12 has a great camera.") {
		t.Errorf("missing body text, got:\n%s", content)
	}
	if strings.Contains(content, "evil()") || strings.Contains(content, "color:red") {
		t.Errorf("script/style must be stripped, got:\n%s", content)
	}
	if !strings.Contains(content, server.URL+"/img/foo12.jpg") {
		t.Errorf("missing absolute image URL, got:\n%s", content)
	}
	if strings.Contains(content, "data:image") {
		t.Errorf("data: images must be skipped, got:\n%s", content)
	}
	if !strings.Contains(content, server.URL+"/reviews/foo12") {
		t.Errorf("missing absolute link, got:\n%s", content)
	}
	if strings.Contains(content, "javascript:") {
		t.Errorf("javascript: links must be dropped, got:\n%s", content)
	}
}

func TestWebFetchNonHTMLPassesThrough(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"price": 19900}`))
	}))
	defer server.Close()

	s := &webFetchSkill{}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if !strings.Contains(out.Content, `{"price": 19900}`) {
		t.Errorf("JSON body must pass through, got:\n%s", out.Content)
	}
}

// The second read of a page inside one research turn must not be a second
// download. Counted at the server, because "it returned the same text" would
// also be true of a cache that never worked.
func TestWebFetchCachesWithinTheTTL(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"v": 1}`))
	}))
	defer server.Close()

	s := &webFetchSkill{}
	for i := 0; i < 3; i++ {
		out, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL})
		if err != nil {
			t.Fatalf("fetch %d: %v", i, err)
		}
		if !strings.Contains(out.Content, `{"v": 1}`) {
			t.Fatalf("fetch %d returned %q", i, out.Content)
		}
	}
	if hits != 1 {
		t.Errorf("server was hit %d times, want 1", hits)
	}
}

func TestWebFetchDigestsWhenAsked(t *testing.T) {
	page := strings.Repeat("filler paragraph about nothing in particular. ", 400)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>" + page + "</p><p>The retry option is called MaxAttempts.</p></body></html>"))
	}))
	defer server.Close()

	var askedQuestion, sawPage string
	s := &webFetchSkill{digest: func(_ context.Context, question, text string) (string, error) {
		askedQuestion, sawPage = question, text
		return "MaxAttempts", nil
	}}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"url":    server.URL,
		"prompt": "what is the retry option called?",
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if out.Content != "MaxAttempts" {
		t.Errorf("Content = %q, want just the answer", out.Content)
	}
	if askedQuestion != "what is the retry option called?" {
		t.Errorf("digester got question %q", askedQuestion)
	}
	if !strings.Contains(sawPage, "MaxAttempts") {
		t.Error("the digester was handed a page without the answer in it")
	}
}

// A digester that fails must cost the caller nothing: the page is already in
// hand, and the question was only ever an optimization.
func TestWebFetchFallsBackToTheWholePageWhenTheDigesterFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer": 42}`))
	}))
	defer server.Close()

	s := &webFetchSkill{digest: func(context.Context, string, string) (string, error) {
		return "", errors.New("provider is down")
	}}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL, "prompt": "anything"})
	if err != nil {
		t.Fatalf("a failed digest must not fail the fetch: %v", err)
	}
	if !strings.Contains(out.Content, `{"answer": 42}`) {
		t.Errorf("Content = %q, want the full page back", out.Content)
	}
}

// No digester configured — the CLI's case — is not an error either.
func TestWebFetchIgnoresPromptWithoutADigester(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer": 42}`))
	}))
	defer server.Close()

	s := &webFetchSkill{}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL, "prompt": "anything"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.Contains(out.Content, `{"answer": 42}`) {
		t.Errorf("Content = %q, want the full page", out.Content)
	}
}

func TestWebFetchRejectsNonHTTPSchemes(t *testing.T) {
	s := &webFetchSkill{}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"url": "file:///C:/secrets.txt"}); err == nil {
		t.Fatal("file:// must be rejected")
	}
}
