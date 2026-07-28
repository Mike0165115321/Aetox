package model

import (
	"context"
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

	client := newModelHTTPClient(5 * time.Second)
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

		client := newModelHTTPClient(5 * time.Second)
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

	client := newModelHTTPClient(5 * time.Second)
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

	client := newModelHTTPClient(5 * time.Second)
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
	if _, err := newModelHTTPClient(5 * time.Second).Do(req); err == nil {
		t.Fatal("a cancelled request kept waiting out the retry")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation took %s to take effect", elapsed)
	}
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
