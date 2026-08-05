package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serve runs one request through the file host, with `next` recording whether
// the request was passed through instead of claimed.
func serve(t *testing.T, root, target string) (*http.Response, string, bool) {
	t.Helper()
	passed := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { passed = true })
	a := &App{}
	a.cfg.SandboxRoot = root

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	a.fileHost(next).ServeHTTP(rec, req)
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	return res, string(body), passed
}

// The rule the whole app depends on: anything not addressed to the prefix must
// leave untouched. Claiming it would take the app's own HTML with it and the
// window would come up blank.
func TestFileHostPassesThroughForeignPaths(t *testing.T) {
	for _, path := range []string{"/", "/index.html", "/assets/app.js", "/aetox-files/x"} {
		_, _, passed := serve(t, t.TempDir(), path)
		if !passed {
			t.Errorf("%s: claimed by the file host, want pass-through", path)
		}
	}
}

func TestFileHostServesProjectFile(t *testing.T) {
	root := t.TempDir()
	want := "<svg xmlns=\"http://www.w3.org/2000/svg\"/>"
	if err := os.WriteFile(filepath.Join(root, "mark.svg"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	res, body, passed := serve(t, root, "/aetox-file/mark.svg")
	if passed {
		t.Fatal("passed through, want served")
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	// Pinned rather than sniffed: on Windows mime.TypeByExtension asks the
	// registry, where another program's install can leave .svg answering
	// something that turns the pane into a download prompt.
	if ct := res.Header.Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("Content-Type = %q, want image/svg+xml", ct)
	}
	// Scrubbing a video without downloading it first is the whole point of
	// serving over HTTP rather than as a data: URL.
	if ar := res.Header.Get("Accept-Ranges"); ar != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes", ar)
	}
}

// A file tab re-reads on every open because the agent rewrites the same path
// constantly. A cached response would hand back the previous turn's bytes under
// the right filename — the one failure the pane exists to prevent.
func TestFileHostDoesNotCache(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _, _ := serve(t, root, "/aetox-file/a.png")
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestFileHostServesRange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.cfg.SandboxRoot = root
	req := httptest.NewRequest(http.MethodGet, "/aetox-file/clip.mp4", nil)
	req.Header.Set("Range", "bytes=4-6")
	rec := httptest.NewRecorder()
	a.fileHost(http.NotFoundHandler()).ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if string(body) != "456" {
		t.Errorf("body = %q, want %q", body, "456")
	}
}

// This transport adds a way to reach files, not a right to reach them. Every
// request resolves through the same safeSandboxPath as every binding.
func TestFileHostRefusesOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(outside, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(outside) })

	for _, target := range []string{
		"/aetox-file/../secret.txt",
		"/aetox-file/sub/../../secret.txt",
	} {
		res, body, _ := serve(t, root, target)
		if res.StatusCode == http.StatusOK {
			t.Errorf("%s: status 200, want refusal", target)
		}
		if strings.Contains(body, "private") {
			t.Errorf("%s: served a file outside the project", target)
		}
	}
}

// A path already decoded by net/http must not be decoded a second time, or
// %252e%252e arrives as ".." after the guard has already looked at it.
func TestFileHostDoesNotDoubleDecode(t *testing.T) {
	root := t.TempDir()
	res, body, _ := serve(t, root, "/aetox-file/%252e%252e/secret.txt")
	if res.StatusCode == http.StatusOK {
		t.Errorf("status 200, want refusal; body %q", body)
	}
}

func TestFileHostWithoutProject(t *testing.T) {
	res, _, passed := serve(t, "", "/aetox-file/a.png")
	if passed {
		t.Fatal("passed through, want claimed")
	}
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFileHostRefusesDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, _, _ := serve(t, root, "/aetox-file/docs")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}
