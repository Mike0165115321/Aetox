package model

import (
	"net"
	"net/http"
	"time"
)

// newModelHTTPClient builds the HTTP client every model provider uses.
//
// It deliberately sets NO http.Client.Timeout: that field caps the whole
// exchange including reading the response body, so on a non-streaming call it
// bounds the entire generation — and a reasoning model (thinking=high) or a
// long file write routinely runs past any fixed cap, which then cancels the
// request and triggers a wasteful retry. LLM turns are bounded by context
// cancellation instead (the desktop Stop button → App.CancelTurn), which is the
// architecture's documented brake.
//
// connectTimeout bounds only establishing the TCP connection, so an unreachable
// API still fails fast without capping generation.
func newModelHTTPClient(connectTimeout time.Duration) *http.Client {
	if connectTimeout <= 0 {
		connectTimeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	return &http.Client{Transport: transport}
}
