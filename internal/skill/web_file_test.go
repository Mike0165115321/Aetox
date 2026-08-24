package skill

// A file at a URL.
//
// `web_fetch` used to check one thing — "does the content type say html" — and
// hand anything else over as-is. For JSON that is right. For a PDF it put the
// raw bytes of the file into the model's context: forty thousand characters of
// binary, paid for in tokens, carrying nothing.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fetchFrom(t *testing.T, contentType, path string, body []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	s := &webFetchSkill{httpClient: srv.Client()}
	out, err := s.ExecuteTool(t.Context(), map[string]any{"url": srv.URL + path})
	if err != nil {
		t.Fatalf("web_fetch: %v", err)
	}
	return out.Content
}

// The one that was actively costing tokens every time it happened.
func TestABinaryBodyIsNamedRatherThanDumped(t *testing.T) {
	// A body with NULs in it, like every real binary format.
	body := append([]byte("PK\x03\x04"), make([]byte, 4096)...)
	got := fetchFrom(t, "application/zip", "/bundle.zip", body)

	if strings.Contains(got, "\x00") {
		t.Error("the raw bytes went into the reply")
	}
	for _, want := range []string{"bundle.zip", "application/zip", "nothing to read"} {
		if !strings.Contains(got, want) {
			t.Errorf("reply does not say %q:\n%s", want, got)
		}
	}
}

// An image is bytes with no reader here either. Naming it is a complete answer
// to "what is at this URL".
func TestAnImageURLIsDescribed(t *testing.T) {
	got := fetchFrom(t, "image/png", "/diagram.png", []byte("\x89PNG\r\n\x1a\n"+strings.Repeat("x", 200)))

	if !strings.Contains(got, "diagram.png") || !strings.Contains(got, "image/png") {
		t.Errorf("reply does not name the file:\n%s", got)
	}
}

// Servers lie about content type more often than URLs lie about their
// extension — application/octet-stream for a PDF is routine — so the extension
// gets a vote. This one asserts the routing decision, not the PDF reader, which
// needs poppler.
func TestAPDFIsRoutedByExtensionWhenTheServerWillNotSay(t *testing.T) {
	u := mustURL(t, "https://example.invalid/papers/thesis.pdf")
	if kind := fetchedFileKind(u, "application/octet-stream"); kind != ".pdf" {
		t.Errorf("kind = %q, want .pdf", kind)
	}
	if kind := fetchedFileKind(mustURL(t, "https://example.invalid/x"), "application/pdf"); kind != ".pdf" {
		t.Errorf("a server that does say gets ignored: %q", kind)
	}
	for _, ext := range []string{".docx", ".xlsx", ".pptx"} {
		if kind := fetchedFileKind(mustURL(t, "https://example.invalid/file"+ext), ""); kind != ext {
			t.Errorf("%s routed to %q", ext, kind)
		}
	}
}

// JSON and plain text were always right and must stay untouched.
func TestTextStillComesBackWhole(t *testing.T) {
	got := fetchFrom(t, "application/json", "/api/thing", []byte(`{"ok":true,"rows":3}`))

	if !strings.Contains(got, `{"ok":true,"rows":3}`) {
		t.Errorf("json did not survive:\n%s", got)
	}
}

// A body with no content type worth trusting is judged by its bytes — the same
// NUL check `read` uses, and the one signal a server cannot get wrong for you.
func TestBytesDecideWhenTheHeaderSaysNothing(t *testing.T) {
	if !looksBinaryType(mustURL(t, "https://x.invalid/f"), "", "abc\x00def") {
		t.Error("a body with NULs was taken for text")
	}
	if looksBinaryType(mustURL(t, "https://x.invalid/f"), "text/plain", "plain words") {
		t.Error("plain text was taken for binary")
	}
}

func TestASizeIsReadableToAPerson(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{512, "512 bytes"}, {2048, "2 KB"}, {3 << 20, "3.0 MB"}} {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
