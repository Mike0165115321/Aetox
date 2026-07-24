package model

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The whole-request cap must be gone: a response that takes far longer than the
// connect timeout (as a reasoning model's generation does) must still succeed.
func TestModelHTTPClientDoesNotCapGeneration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond) // generation slower than the connect timeout
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := newModelHTTPClient(20 * time.Millisecond)
	if client.Timeout != 0 {
		t.Fatalf("model client must not set a whole-request timeout, got %v", client.Timeout)
	}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("slow response must not be cut off by the connect timeout: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("unexpected body %q", body)
	}
}
