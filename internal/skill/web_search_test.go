package skill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWebSearchParsesDuckDuckGoResults(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		encoded := url.QueryEscape("https://example.com/foo-phone-12-review")
		_, _ = w.Write([]byte(`<html><body>
<div class="result">
  <a class="result__a" href="//duckduckgo.com/l/?uddg=` + encoded + `&rut=abc">Foo Phone 12 review</a>
  <a class="result__snippet" href="//duckduckgo.com/l/?uddg=` + encoded + `">Great camera, decent battery, 19900 baht.</a>
</div>
<div class="result">
  <a class="result__a" href="https://plain.example.org/page">Plain link result</a>
</div>
<a class="result__a" href="/internal/ad">Ad without scheme</a>
</body></html>`))
	}))
	defer server.Close()

	s := &webSearchSkill{endpoint: server.URL}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"query": "foo phone 12 รีวิว"})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if gotQuery != "foo phone 12 รีวิว" {
		t.Errorf("query sent = %q", gotQuery)
	}
	content := out.Content
	if !strings.Contains(content, "Foo Phone 12 review") ||
		!strings.Contains(content, "https://example.com/foo-phone-12-review") {
		t.Errorf("missing decoded uddg result, got:\n%s", content)
	}
	if !strings.Contains(content, "Great camera, decent battery") {
		t.Errorf("missing snippet, got:\n%s", content)
	}
	if !strings.Contains(content, "https://plain.example.org/page") {
		t.Errorf("missing plain href result, got:\n%s", content)
	}
	if strings.Contains(content, "/internal/ad") {
		t.Errorf("schemeless internal links must be dropped, got:\n%s", content)
	}
}

func TestWebSearchEmptyQueryFails(t *testing.T) {
	s := &webSearchSkill{}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"query": "  "}); err == nil {
		t.Fatal("empty query must fail")
	}
}
