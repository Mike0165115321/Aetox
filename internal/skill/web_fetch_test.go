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

// A page bigger than one window comes back cut, and the cut counts past
// itself.
//
// This is the rule `read` and `capture` already follow, and it is the whole of
// what replaced the summarizer (§177): a cap that stops silently cannot be told
// apart from a page that simply ended, so a caller reads "that is all there is"
// where the truth is "that is all you were given".
func TestWebFetchCutsAtOneWindowAndSaysHowMuchIsLeft(t *testing.T) {
	page := strings.Repeat("filler paragraph about nothing in particular. ", 800)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>" + page + "</p></body></html>"))
	}))
	defer server.Close()

	s := &webFetchSkill{}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(out.Content) > webFetchWindow+400 {
		t.Errorf("one call handed back %d characters, want about %d", len(out.Content), webFetchWindow)
	}
	for _, want := range []string{"showing", "from:", "no download"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("the cut does not say %q:\n%s", want, out.Content[len(out.Content)-300:])
		}
	}
}

// The second call is served from the cache, so continuing a long page costs a
// tool round trip and not a second download. That is the whole reason `from`
// could replace a summarizer without costing time.
func TestWebFetchContinuesFromAnOffsetWithoutFetchingAgain(t *testing.T) {
	page := strings.Repeat("filler paragraph about nothing in particular. ", 800)
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>" + page + "</p><p>ENDMARKER</p></body></html>"))
	}))
	defer server.Close()

	s := &webFetchSkill{}
	first, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	second, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL, "from": webFetchWindow})
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	if hits != 1 {
		t.Errorf("server was hit %d times, want 1 — the continuation must come from the cache", hits)
	}
	if first.Content == second.Content {
		t.Error("continuing returned the same window as the first call")
	}
	if strings.Contains(second.Content, "URL: ") {
		t.Error("the continuation repeats the header, which the first call already paid for")
	}
}

// Reaching the end is a report, not an error. A caller that added the window
// size once too many should be told it is done rather than refused.
func TestWebFetchPastTheEndSaysSoInsteadOfFailing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer": 42}`))
	}))
	defer server.Close()

	s := &webFetchSkill{}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL, "from": 999999})
	if err != nil {
		t.Fatalf("past the end must not fail: %v", err)
	}
	if !strings.Contains(out.Content, "reached the end") {
		t.Errorf("Content = %q, want it to say there is nothing there", out.Content)
	}
}

// A short page is handed over whole and says nothing about windows, because
// nothing was left out. A note on every fetch is a note nobody reads.
func TestAShortPageCarriesNoCutNotice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer": 42}`))
	}))
	defer server.Close()

	s := &webFetchSkill{}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.Contains(out.Content, `{"answer": 42}`) {
		t.Errorf("Content = %q, want the whole page", out.Content)
	}
	if strings.Contains(out.Content, "showing") {
		t.Errorf("a page that fits carried a cut notice: %q", out.Content)
	}
}

// The window cuts on a space when there is one to hand. A window ending
// mid-word reads as corrupted text rather than as a page that continues.
func TestTheWindowCutsOnAWordBoundary(t *testing.T) {
	body := strings.Repeat("word ", webFetchWindow)
	shown, note := windowOf(body, 0)
	if note == "" {
		t.Fatal("a body far longer than the window came back with no notice")
	}
	if strings.HasSuffix(shown, "wor") || strings.HasSuffix(shown, "wo") {
		t.Errorf("the window ended mid-word: %q", shown[len(shown)-12:])
	}
}

func TestWebFetchRejectsNonHTTPSchemes(t *testing.T) {
	s := &webFetchSkill{}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"url": "file:///C:/secrets.txt"}); err == nil {
		t.Fatal("file:// must be rejected")
	}
}
