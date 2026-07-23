package skill

import (
	"context"
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

func TestWebFetchRejectsNonHTTPSchemes(t *testing.T) {
	s := &webFetchSkill{}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"url": "file:///C:/secrets.txt"}); err == nil {
		t.Fatal("file:// must be rejected")
	}
}
