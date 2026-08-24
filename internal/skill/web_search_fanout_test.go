package skill

// Fan-out: several wordings of one question, searched together (§179).
//
// The things worth pinning are the ones that fail quietly. A sequential
// implementation is three times slower and looks identical from the outside. A
// merge that forgets to dedupe spends its slots on one popular page. A wording
// that times out taking the whole call down with it turns a rate limit into an
// outage. None of those show up in a passing build.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ddgPage builds a results page the real parser can read.
func ddgPage(items ...[2]string) string {
	var b strings.Builder
	b.WriteString("<html><body>")
	for _, it := range items {
		fmt.Fprintf(&b, `<a class="result__a" href="/l/?uddg=%s">%s</a><a class="result__snippet">about %s</a>`,
			strings.ReplaceAll(it[0], ":", "%3A"), it[1], it[1])
	}
	b.WriteString("</body></html>")
	return b.String()
}

func TestFanOutMergesAndDeduplicates(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "text/html")
		switch {
		case strings.Contains(q, "second"):
			// One page both wordings find, one only this one does.
			_, _ = w.Write([]byte(ddgPage([2]string{"https://shared.test/a", "shared"}, [2]string{"https://only-second.test/b", "second only"})))
		default:
			_, _ = w.Write([]byte(ddgPage([2]string{"https://shared.test/a", "shared"}, [2]string{"https://only-first.test/c", "first only"})))
		}
	}))
	defer server.Close()

	s := &webSearchSkill{endpoint: server.URL}
	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"queries": []any{"the first wording", "the second wording"},
	})
	if err != nil {
		t.Fatalf("fan-out: %v", err)
	}
	if hits != 2 {
		t.Errorf("server was hit %d times, want one per wording", hits)
	}
	if got := strings.Count(out.Content, "shared.test"); got != 1 {
		t.Errorf("the page both wordings found appears %d times, want 1", got)
	}
	for _, want := range []string{"only-first.test", "only-second.test"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("the merge lost %s:\n%s", want, out.Content)
		}
	}
	// It says it merged, so a caller can tell a merge from a single search
	// that happened to return little.
	if !strings.Contains(out.Content, "wordings, merged") {
		t.Errorf("the result does not say it was a merge: %q", headLine(out.Content))
	}
}

// In parallel, and it is not an optimisation: sequential would cost three round
// trips of wall clock for something the caller is waiting on.
func TestFanOutRunsTheWordingsTogether(t *testing.T) {
	const delay = 250 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		_, _ = w.Write([]byte(ddgPage([2]string{"https://x.test/" + r.URL.Query().Get("q"), "x"})))
	}))
	defer server.Close()

	s := &webSearchSkill{endpoint: server.URL}
	start := time.Now()
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"queries": []any{"one", "two", "three"},
	}); err != nil {
		t.Fatalf("fan-out: %v", err)
	}
	// Three sequential requests would be 750ms. Generous ceiling so a slow
	// machine does not fail this, tight enough that sequential cannot pass.
	if took := time.Since(start); took > 2*delay {
		t.Errorf("three wordings took %v, want about %v — they ran one after another", took, delay)
	}
}

// DuckDuckGo rate-limits, and three requests where there was one makes that
// likelier. Two answers out of three is a good answer.
func TestOneFailedWordingDoesNotFailTheCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("q"), "doomed") {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(ddgPage([2]string{"https://good.test/a", "good"})))
	}))
	defer server.Close()

	s := &webSearchSkill{endpoint: server.URL}
	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"queries": []any{"doomed wording", "healthy wording"},
	})
	if err != nil {
		t.Fatalf("one rate-limited wording failed the whole call: %v", err)
	}
	if !strings.Contains(out.Content, "good.test") {
		t.Errorf("the surviving wording's results were lost:\n%s", out.Content)
	}
}

// Every wording failing IS the answer, though.
func TestAllWordingsFailingIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	s := &webSearchSkill{endpoint: server.URL}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"queries": []any{"a", "b"}}); err == nil {
		t.Error("every wording failed and the call reported success")
	}
}

func TestQueriesAreCappedAndDeduplicated(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		_, _ = w.Write([]byte(ddgPage([2]string{"https://x.test/" + r.URL.Query().Get("q"), "x"})))
	}))
	defer server.Close()

	s := &webSearchSkill{endpoint: server.URL}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"queries": []any{"one", "two", "three", "four", "five"},
	}); err != nil {
		t.Fatalf("fan-out: %v", err)
	}
	if hits != webSearchMaxQueries {
		t.Errorf("server was hit %d times, want the cap of %d", hits, webSearchMaxQueries)
	}

	// Repeats collapse BEFORE the cap, so sending the same words twice spends
	// one request rather than buying a duplicate and losing a slot.
	atomic.StoreInt64(&hits, 0)
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"queries": []any{"same", "SAME", " same ", "different"},
	}); err != nil {
		t.Fatalf("fan-out: %v", err)
	}
	if hits != 2 {
		t.Errorf("server was hit %d times for two distinct wordings", hits)
	}
}

// The old spelling keeps working, and sending both means all of them.
func TestSingleQueryStillWorksAndCombinesWithQueries(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		_, _ = w.Write([]byte(ddgPage([2]string{"https://x.test/" + r.URL.Query().Get("q"), "x"})))
	}))
	defer server.Close()

	s := &webSearchSkill{endpoint: server.URL}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"query": "just the one"})
	if err != nil {
		t.Fatalf("single query: %v", err)
	}
	if hits != 1 {
		t.Errorf("a single query made %d requests", hits)
	}
	// A lone wording is not announced as a merge.
	if strings.Contains(out.Content, "merged") {
		t.Errorf("a single query reported itself as a merge: %q", headLine(out.Content))
	}

	atomic.StoreInt64(&hits, 0)
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"query": "the old way", "queries": []any{"the new way"},
	}); err != nil {
		t.Fatalf("both spellings: %v", err)
	}
	if hits != 2 {
		t.Errorf("sending both spellings made %d requests, want both used", hits)
	}
}

func headLine(s string) string {
	if i := strings.IndexByte(s, byte('\n')); i >= 0 {
		return s[:i]
	}
	return s
}
