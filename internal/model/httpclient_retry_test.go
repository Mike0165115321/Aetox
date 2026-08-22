package model

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Until this existed there was no retry anywhere in Aetox: one rate-limit blip
// from a provider ended the user's turn. config.MaxRetries has claimed
// otherwise since the beginning and is read by nothing.

func TestRetryTransportRetriesRateLimitAndSucceeds(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"message":"Error"}}`))
			return
		}
		// The body has to survive the replay, or the retry sends an empty
		// request that fails for a different reason.
		if !strings.Contains(string(body), "hello") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := newModelHTTPClient(5*time.Second, "")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want the retry to have succeeded", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d; want one failure and one retry", got)
	}
}

// A wrong request is not a temporary one. Retrying it just spends the user's
// quota on the same rejection.
func TestRetryTransportDoesNotRetryClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(status)
		}))

		client := newModelHTTPClient(5*time.Second, "")
		resp, err := client.Do(mustPost(t, server.URL))
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		resp.Body.Close()
		server.Close()

		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("status %d was retried %d times; want none", status, got-1)
		}
	}
}

// An account with no credit left answers 429 too, but no amount of waiting
// changes a balance of zero. The transport hands it straight back — with the
// body intact, because the caller still has to read the reason.
func TestRetryTransportDoesNotRetryInsufficientQuota(t *testing.T) {
	quotaBody := `{"error":{"message":"You exceeded your current quota, please check your plan and billing details.","type":"insufficient_quota","param":null,"code":"insufficient_quota"}}`
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(quotaBody))
	}))
	defer server.Close()

	client := newModelHTTPClient(5*time.Second, "")
	resp, err := client.Do(mustPost(t, server.URL))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		t.Fatalf("reading the body back: %v", readErr)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d; want no retries against an empty balance", got)
	}
	if string(body) != quotaBody {
		t.Fatalf("body = %q; want the peeked body handed back whole", body)
	}
}

// Not every provider says "insufficient_quota". Z.ai answers the identical
// condition with code 1113 and a sentence sharing no word with OpenAI's, and
// while the transport matched that one token it retried an empty wallet through
// the whole backoff budget before reporting a rate limit that was never one.
func TestRetryTransportDoesNotRetryABalanceOfZeroInAnyDialect(t *testing.T) {
	for _, body := range []string{
		`{"error":{"code":"1113","message":"Insufficient balance or no resource package. Please recharge."}}`,
		// Case is not ours to assume either — the match is case-insensitive.
		`{"error":{"message":"INSUFFICIENT BALANCE"}}`,
	} {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(body))
		}))

		client := newModelHTTPClient(5*time.Second, "")
		resp, err := client.Do(mustPost(t, server.URL))
		if err != nil {
			server.Close()
			t.Fatalf("Do: %v", err)
		}
		got, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		server.Close()
		if readErr != nil {
			t.Fatalf("reading the body back: %v", readErr)
		}

		if n := atomic.LoadInt32(&calls); n != 1 {
			t.Errorf("calls = %d for %s; want no retries against an empty balance", n, body)
		}
		if string(got) != body {
			t.Errorf("body = %q; want the peeked body handed back whole", got)
		}
	}
}

// The other direction, which matters just as much: a real rate limit must stay
// retryable. A marker list that swallowed "slow down" would turn a wait of a
// few seconds into a dead turn.
func TestRetryTransportStillRetriesARealRateLimit(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"Rate limit reached for gpt-4o","code":"rate_limit_exceeded"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newModelHTTPClient(5*time.Second, "")
	resp, err := client.Do(mustPost(t, server.URL))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d; want the rate limit retried once and then succeeding", got)
	}
}

// A limit that resets in an hour is not something to block a turn on. The
// response comes straight back so the runtime can say when it resets.
func TestRetryTransportGivesUpOnLongWindows(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", strconv.Itoa(3600))
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newModelHTTPClient(5*time.Second, "")
	start := time.Now()
	resp, err := client.Do(mustPost(t, server.URL))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("waited %s on an hour-long window", elapsed)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d; want it to give up immediately", got)
	}
}

func TestRetryTransportStopsAtTheAttemptCap(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(529) // Anthropic: overloaded
	}))
	defer server.Close()

	client := newModelHTTPClient(5*time.Second, "")
	resp, err := client.Do(mustPost(t, server.URL))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("calls = %d; want the first plus two retries", got)
	}
}

func TestRetryTransportHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader("x"))

	start := time.Now()
	if _, err := newModelHTTPClient(5*time.Second, "").Do(req); err == nil {
		t.Fatal("a cancelled request kept waiting out the retry")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation took %s to take effect", elapsed)
	}
}

// The owner's own screenshot: DeepSeek's host failed to resolve while the
// laptop's Wi-Fi was reconnecting, and the whole turn was lost to it. A request
// that never reached the provider has delivered nothing to the caller, so it is
// safe to send again.
func TestRetryTransportRetriesATransportFailureAndSucceeds(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "hello") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	// Fails the first dial the way an unresolvable host does, then lets the
	// real transport through — the retry has to reach the live server.
	flaky := &flakyTransport{
		base: http.DefaultTransport,
		fail: func() error {
			if atomic.AddInt32(&calls, 1) == 1 {
				return &net.DNSError{Err: "no such host", Name: "api.deepseek.com", IsNotFound: true}
			}
			return nil
		},
	}
	client := &http.Client{Transport: &retryTransport{base: flaky}}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v; want the retry to have carried the turn", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want the replayed body to have arrived intact", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d; want one dial failure and one retry", got)
	}
}

// Stop has to mean stop during a retry pause, not only between them.
func TestRetryTransportDoesNotRetryAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int32
	flaky := &flakyTransport{
		base: http.DefaultTransport,
		fail: func() error {
			atomic.AddInt32(&calls, 1)
			cancel() // the user presses Stop while the dial is failing
			return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		},
	}
	client := &http.Client{Transport: &retryTransport{base: flaky}}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:1/x", strings.NewReader("x"))
	if _, err := client.Do(req); err == nil {
		t.Fatal("a cancelled request was retried into a success")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d; want the cancellation to stop the retries", got)
	}
}

// A certificate that does not verify will not verify a second later, and a
// caller that hung up asked for nothing more. Neither is worth a replay.
func TestRetryableTransportErrorRefusesPermanentFailures(t *testing.T) {
	permanent := []error{
		context.Canceled,
		context.DeadlineExceeded,
		x509.UnknownAuthorityError{},
		x509.HostnameError{Host: "api.example.com"},
		&tls.CertificateVerificationError{Err: errors.New("expired")},
	}
	for _, err := range permanent {
		if retryableTransportError(context.Background(), err) {
			t.Errorf("retryableTransportError(%T) = true; want it left alone", err)
		}
	}

	transient := []error{
		&net.DNSError{Err: "no such host", IsNotFound: true},
		&net.OpError{Op: "dial", Err: errors.New("connection refused")},
		io.EOF,
		io.ErrUnexpectedEOF,
	}
	for _, err := range transient {
		if !retryableTransportError(context.Background(), err) {
			t.Errorf("retryableTransportError(%T) = false; want a retry", err)
		}
	}

	// A cancelled context outranks an otherwise-retryable cause: the dial only
	// failed because Stop tore it down.
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	if retryableTransportError(stopped, &net.DNSError{Err: "no such host"}) {
		t.Error("a dial torn down by Stop was retried")
	}
}

// flakyTransport injects a transport-level failure ahead of the real one, which
// is the only way to exercise a dial that never reaches a server.
type flakyTransport struct {
	base http.RoundTripper
	fail func() error
}

func (f *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := f.fail(); err != nil {
		return nil, err
	}
	return f.base.RoundTrip(req)
}

func mustPost(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader("x"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return req
}

// retryAfter's backoff shifts by the attempt index, so a caller reaching in
// with a negative "there is no attempt" sentinel panicked the whole turn with
// "negative shift amount". The function no longer trusts its input.
func TestRetryAfterNeverPanicsOnOutOfRangeAttempts(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	for _, attempt := range []int{-99, -1, 0, 3, 64, 1 << 20} {
		wait := retryAfter(resp, attempt)
		if wait <= 0 || wait > time.Minute {
			t.Fatalf("retryAfter(attempt=%d) = %s; want a sane bounded backoff", attempt, wait)
		}
	}
}

func TestProviderRetryAfterReportsWhetherTheProviderSaid(t *testing.T) {
	silent := &http.Response{Header: http.Header{}}
	if _, stated := providerRetryAfter(silent); stated {
		t.Fatal("a response with no Retry-After was reported as stating one")
	}

	spoke := &http.Response{Header: http.Header{"Retry-After": []string{"12"}}}
	wait, stated := providerRetryAfter(spoke)
	if !stated || wait != 12*time.Second {
		t.Fatalf("providerRetryAfter = %s, %v; want 12s stated", wait, stated)
	}
}

// Google says "you exceeded your current quota, please check your plan and
// billing details" for a limit that clears in forty seconds, and the marker list
// matched it — so a free key was told to go top up, and the retry that would
// have fixed it never ran. The body's own "Please retry in 43.3s" is the
// rebuttal. Measured against gemini-3.7-flash, 21 Aug 2026.
func TestRetryTransportRetriesAQuotaThatNamesItsOwnWait(t *testing.T) {
	const body = `{"error":{"code":429,"message":"You exceeded your current quota, please check your plan and billing details. For more information on this error, head to: https://ai.google.dev/gemini-api/docs/rate-limits.\n* Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_requests, limit: 20, model: gemini-3.7-flash\nPlease retry in 43.324604531s.","status":"RESOURCE_EXHAUSTED"}}`

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newModelHTTPClient(5*time.Second, "")
	resp, err := client.Do(mustPost(t, server.URL))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d; want the wait honoured and the turn saved", got)
	}
	if outOfCredits([]byte(body)) {
		t.Error("a 429 that names a retry window was read as an empty wallet")
	}
	// The rebuttal must not reach across bodies: a wallet that really is empty
	// says nothing about waiting, and still has to stop the retries.
	empty := `{"error":{"message":"You exceeded your current quota, please check your plan and billing details.","type":"insufficient_quota"}}`
	if !outOfCredits([]byte(empty)) {
		t.Error("insufficient_quota stopped being read as an empty wallet")
	}
}
