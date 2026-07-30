package model

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
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
	return &http.Client{Transport: &retryTransport{base: transport}}
}

// retryTransport retries the answers that mean "ask again shortly".
//
// Every runtime builds its client here, so this is the one place that covers
// all five wire formats — and until now there were none: a single rate-limit
// blip ended the user's turn with a raw JSON blob. (config.MaxRetries has
// existed the whole time and is read by nothing; this is the behavior it
// described.)
//
// Only statuses that are explicitly "temporary, retry" are retried, and only
// when the provider's own Retry-After can be honored or a short backoff is
// safe. A 400, a 401 or a 403 is never retried — those mean the request itself
// is wrong, and repeating it just wastes the user's quota.
type retryTransport struct {
	base http.RoundTripper
	// attempts is the number of extra tries after the first. Two matches what
	// config.MaxRetries has always claimed.
	attempts int
	// maxWait caps how long a provider may tell us to sleep. A rate limit that
	// resets in an hour is not something to block a turn on; it is something to
	// report.
	maxWait time.Duration
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attempts := t.attempts
	if attempts <= 0 {
		attempts = 2
	}
	maxWait := t.maxWait
	if maxWait <= 0 {
		maxWait = 20 * time.Second
	}

	// A request with a body can only be replayed if it can be rebuilt.
	// http.NewRequest sets GetBody for the readers Aetox uses (bytes.Reader),
	// so this is true in practice — but a caller that streams an unrepeatable
	// body must not have it silently half-sent twice.
	replayable := req.Body == nil || req.GetBody != nil

	var resp *http.Response
	var err error
	for attempt := 0; ; attempt++ {
		resp, err = t.base.RoundTrip(req)
		if err != nil || attempt >= attempts || !replayable || !retryableStatus(resp.StatusCode) {
			return resp, err
		}

		wait := retryAfter(resp, attempt)
		if wait > maxWait {
			// Longer than we are willing to hold the turn open: hand the
			// response back so the runtime can report when it resets.
			return resp, nil
		}

		// The body must be drained and closed or the connection is not reused,
		// and its content is about to be superseded anyway.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(wait):
		}

		if req.GetBody != nil {
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				return nil, bodyErr
			}
			req.Body = body
		}
	}
}

// retryableStatus lists the answers that mean "not now, try again".
//
// 529 is Anthropic's "overloaded" and is not in any RFC; it is included because
// it is exactly the case this exists for.
func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError, // 500 — providers use it for transient faults
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
		529:                            // Anthropic: overloaded
		return true
	default:
		return false
	}
}

// humanizeDuration renders a wait window for an error message a person reads.
func humanizeDuration(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d >= 2*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	case d >= 2*time.Minute:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
}

// providerRetryAfter reads only what the provider said, and reports whether it
// said anything at all. Split from the backoff below because two callers want
// different things from a 429: the transport wants a duration to sleep, and an
// error message wants to know whether there is a real reset time worth quoting.
func providerRetryAfter(resp *http.Response) (time.Duration, bool) {
	header := resp.Header.Get("Retry-After")
	if header == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	if when, err := http.ParseTime(header); err == nil {
		if wait := time.Until(when); wait > 0 {
			return wait, true
		}
		return 0, true
	}
	return 0, false
}

// retryAfter is how long to wait before attempt+1.
//
// attempt is the zero-based index of the try that just failed, so it is never
// negative here — but the shift below would panic outright if it were, which is
// exactly what happened when a caller reached in with -1 to mean "no attempt,
// just tell me the window". That caller now uses providerRetryAfter, and this
// one clamps rather than trusting its input.
func retryAfter(resp *http.Response, attempt int) time.Duration {
	if wait, stated := providerRetryAfter(resp); stated {
		return wait
	}
	if attempt < 0 {
		attempt = 0
	}
	// Capped at 16s: any longer and the transport's own maxWait would refuse to
	// sleep on it anyway, so a bigger number here could only ever be a surprise.
	if attempt > 4 {
		attempt = 4
	}
	// No guidance: back off, but stay short enough that the user does not think
	// the app has hung.
	return time.Duration(1<<attempt) * time.Second
}
