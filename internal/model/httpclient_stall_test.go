package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A server that accepts the connection and then says nothing is what Ollama
// looks like while it loads a model into VRAM, and it used to be an unbounded
// wait: the client's only timeout was on the dial, dialing localhost always
// succeeds instantly, and the turn's context is WithCancel rather than
// WithTimeout. Nothing above it was a clock, so "it hangs on the code page" was
// literally true — the only way out was the Stop button.
//
// The bound is deliberately far too long to sit through in a test, so this
// asserts the field rather than the wall clock. Measuring the real thing would
// mean waiting five minutes to learn one duration.
func TestEveryModelClientBoundsTheFirstByte(t *testing.T) {
	for _, tc := range []struct {
		name     string
		endpoint string
		want     time.Duration
	}{
		{"ollama on this machine", "http://localhost:11434", 15 * time.Minute},
		{"lm studio by address", "http://127.0.0.1:1234/v1", 15 * time.Minute},
		{"ipv6 loopback", "http://[::1]:8080/v1", 15 * time.Minute},
		// Same company, someone else's hardware: nothing is loading off this
		// user's disk, so it gets the remote budget.
		{"ollama cloud", "https://ollama.com/v1", 5 * time.Minute},
		{"a hosted provider", "https://api.openai.com/v1", 5 * time.Minute},
		// Unknown is treated as remote on purpose: too generous only delays an
		// error, too tight cancels work that was going fine.
		{"no endpoint given", "", 5 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newModelHTTPClient(20*time.Second, tc.endpoint)
			tr, ok := c.Transport.(*retryTransport)
			if !ok {
				t.Fatalf("transport is %T, not the retrying one — the shape this test reads changed", c.Transport)
			}
			base, ok := tr.base.(*http.Transport)
			if !ok {
				t.Fatalf("inner transport is %T", tr.base)
			}
			if base.ResponseHeaderTimeout != tc.want {
				t.Errorf("first-byte budget is %v, want %v — a stalled %s would wait this long",
					base.ResponseHeaderTimeout, tc.want, tc.name)
			}
			// The other half of the contract, and the reason this is
			// ResponseHeaderTimeout and not the obvious field: Client.Timeout
			// caps the whole exchange, so setting it would cancel a long
			// generation that is arriving perfectly well.
			if c.Timeout != 0 {
				t.Errorf("http.Client.Timeout is %v; it must stay unset or it bounds generation itself", c.Timeout)
			}
		})
	}
}

// The bound must actually fire, not merely be configured. A short one is
// installed by hand here so the test costs a second rather than five minutes.
func TestAStalledServerEventuallyErrors(t *testing.T) {
	stall := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-stall
	}))
	defer func() { close(stall); srv.Close() }()

	c := newModelHTTPClient(20*time.Second, srv.URL)
	base := c.Transport.(*retryTransport).base.(*http.Transport)
	base.ResponseHeaderTimeout = 300 * time.Millisecond

	// No deadline on the context, exactly like a chat turn: if anything stops
	// this, it is the client and not the caller.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, nil)

	done := make(chan error, 1)
	go func() { _, err := c.Do(req); done <- err }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a server that never answered returned success")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("nothing bounded the request — this is the hang, still unfixed")
	}
}
