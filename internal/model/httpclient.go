package model

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
//
// endpoint is where this client will be pointed, and it decides the one bound
// that IS set — see firstByteBudget. Passing "" is treated as remote, which is
// the safer of the two guesses: a bound that is too generous only delays an
// error, while one that is too tight cancels work that was going fine.
func newModelHTTPClient(connectTimeout time.Duration, endpoint string) *http.Client {
	if connectTimeout <= 0 {
		connectTimeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	// The backstop that was missing. Until this, connectTimeout was the only
	// bound in the client and it covers only the dial — which is worth nothing
	// against a local server, because dialing localhost always succeeds
	// instantly. Ollama accepts the connection the moment it is asked and then
	// says nothing at all while it loads a model into VRAM, and with no
	// http.Client.Timeout and no ResponseHeaderTimeout there was nothing above
	// it: the turn's context is WithCancel, not WithTimeout (desktop/app.go —
	// cancel belongs to the Stop button, it is not a clock). So a stalled
	// server was an unbounded wait with no error, which is what "it hangs" is.
	// Measured against a server that accepts and never writes a header:
	// http.Client.Timeout 0s, ResponseHeaderTimeout 0s, nothing returned.
	//
	// ResponseHeaderTimeout rather than http.Client.Timeout, deliberately, and
	// for the reason stated above: it stops waiting for the FIRST header and
	// never touches a response already arriving, so a long generation that is
	// working is left alone.
	transport.ResponseHeaderTimeout = firstByteBudget(endpoint)
	return &http.Client{Transport: &retryTransport{base: transport}}
}

// firstByteBudget is how long to wait for a provider to begin answering before
// calling it stuck.
//
// Both numbers are backstops, not latency targets, and they are large on
// purpose. A tight bound is not available here: the streaming path writes its
// header as soon as the first token exists, but the tool loop and the
// summarizer call Complete (cognitive/agent.go), and a non-streaming response
// sends no header until the whole generation is finished. Thirty seconds would
// have been a fine number for the streaming half and would have cancelled a
// perfectly healthy 8k-token tool-loop reply for everyone else — the same
// mistake http.Client.Timeout would make, moved one field over. Splitting the
// two would take a second client per provider, and it is worth doing only if
// these numbers turn out to be reached in practice.
//
// Local gets three times the room because it has a whole extra phase first:
// a hosted endpoint that has said nothing for five minutes is stuck, while a
// local one may still be reading a 70B checkpoint off a cold disk, which is
// slow and completely healthy.
func firstByteBudget(endpoint string) time.Duration {
	const (
		remote = 5 * time.Minute
		local  = 15 * time.Minute
	)
	if isLoopbackEndpoint(endpoint) {
		return local
	}
	return remote
}

// isLoopbackEndpoint reports whether this URL points at the user's own machine.
//
// The host is what is asked, not the provider name, and that is the point: it
// answers correctly for someone who has pointed the openai row at a llama-server
// on their own box, and it does not hand the local budget to `ollama-cloud`,
// which is the ollama name on somebody else's hardware.
func isLoopbackEndpoint(endpoint string) bool {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return false
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// retryTransport retries the answers that mean "ask again shortly", and the
// failures that mean the network was not there for a moment.
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
//
// A request that never reached the provider at all is retried on the same
// budget (see retryableTransportError): a laptop whose Wi-Fi drops for two
// seconds answers "no such host", and losing a whole turn's work to that is
// the failure this was extended for.
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

		// No response at all — the request never left, or died before any
		// header came back. Nothing has reached the caller, so a replay is
		// safe when the cause looks momentary.
		if err != nil {
			if attempt >= attempts || !replayable || !retryableTransportError(req.Context(), err) {
				return nil, err
			}
			if waitErr := sleepBeforeRetry(req, backoffFor(attempt)); waitErr != nil {
				return nil, waitErr
			}
			if bodyErr := rewindBody(req); bodyErr != nil {
				return nil, bodyErr
			}
			continue
		}

		if attempt >= attempts || !replayable || !retryableStatus(resp.StatusCode) || insufficientQuota(resp) {
			return resp, nil
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

		if waitErr := sleepBeforeRetry(req, wait); waitErr != nil {
			return nil, waitErr
		}
		if bodyErr := rewindBody(req); bodyErr != nil {
			return nil, bodyErr
		}
	}
}

// sleepBeforeRetry waits out the backoff, but never past the caller's own
// cancellation — the desktop Stop button has to take effect during a retry
// pause, not only between them.
func sleepBeforeRetry(req *http.Request, wait time.Duration) error {
	select {
	case <-req.Context().Done():
		return req.Context().Err()
	case <-time.After(wait):
		return nil
	}
}

// rewindBody rebuilds a request body that has already been consumed, so the
// replay sends the same bytes rather than an empty request.
func rewindBody(req *http.Request) error {
	if req.GetBody == nil {
		return nil
	}
	body, err := req.GetBody()
	if err != nil {
		return err
	}
	req.Body = body
	return nil
}

// retryableTransportError reports whether a failure to get any response at all
// is worth asking again.
//
// The distinction it draws is "the network was briefly not there" versus "this
// request will fail the same way forever". A wrong hostname and a hostname that
// could not be resolved while the interface was reconnecting are the same error
// value, so DNS is treated as retryable: two extra tries cost three seconds,
// and the alternative is what the owner hit — a whole turn thrown away because
// the Wi-Fi blinked.
func retryableTransportError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	// Stop was pressed, or the caller's own deadline ran out. Neither is the
	// network faltering, and retrying either ignores an explicit instruction.
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// A certificate that does not verify will not verify a second later.
	// Checked before the net.OpError case, which some TLS failures arrive in.
	var certErr *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuthority) || errors.As(err, &hostname) {
		return false
	}
	// "no such host" — the case in the owner's screenshot.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	// Refused, reset, or unreachable. A local provider still starting up
	// (LM Studio, Ollama) reads exactly like this.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	// The connection closed before the response headers arrived.
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
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

// outOfCreditsMarkers are the ways a provider says "this account has no money
// left" inside a 429 — the status it also uses for "you are going too fast".
//
// One list, matched case-insensitively, because two different places need the
// same answer: the transport, to stop retrying something no backoff can fix,
// and the error message, to say what actually happened. It started as the
// single OpenAI token and was wrong the first time another provider was tested:
// Z.ai answers the same 429 with code 1113 and "Insufficient balance or no
// resource package. Please recharge." — no shared word with OpenAI's spelling,
// so Aetox called an empty wallet a rate limit and burned its whole retry
// budget waiting for the balance to refill on its own.
//
// Each entry has to be billing language that a genuine "slow down" would never
// contain, since misreading a real rate limit as a dead end is the failure in
// the other direction.
var outOfCreditsMarkers = [][]byte{
	[]byte("insufficient_quota"),      // OpenAI: error.code and error.type
	[]byte("insufficient_user_quota"), // the same idea, per-user
	[]byte("insufficient balance"),    // Z.ai (1113), and common wording elsewhere
	[]byte("no resource package"),     // Z.ai's other half of that sentence
	[]byte("please recharge"),         // Z.ai's instruction; billing, never pacing
	[]byte("exceeded your current quota"),
}

// waitItOutMarkers are a body's own rebuttal: words that name a wait, which an
// account with no money in it never has a reason to say.
//
// They exist because one entry above is not a provider's phrase but a fragment
// of English, and Google reuses it for the opposite condition. The free tier's
// 429 opens with "You exceeded your current quota, please check your plan and
// billing details" — matching the marker — and then goes on: "* Quota exceeded
// for metric: generate_content_free_tier_requests, limit: 20 ... Please retry
// in 43.324604531s." Read as an empty wallet, that told a user on a free key to
// go top up a card that has nothing to do with it, and stopped the retry that
// would have cleared it in under a minute (measured against gemini-3.7-flash,
// 21 Aug 2026).
//
// A rebuttal beats a marker rather than joining the list, because the two are
// not the same kind of evidence. A marker is a guess from wording; a retry
// window is the provider stating the fix.
var waitItOutMarkers = [][]byte{
	[]byte("please retry in"), // Google, in the sentence it appends to the quota line
	[]byte("retrydelay"),      // the same fact as a field, in Google's RetryInfo detail
}

// providerErrorMessage pulls the human sentence out of the {"error":{"message":
// …}} shape every OpenAI-compatible host uses for failures, so an error can
// quote the provider instead of paraphrasing it. Empty when the body is shaped
// some other way, which is the caller's cue to fall back to the raw text.
func providerErrorMessage(body []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Error.Message)
}

// outOfCreditsError is the single sentence for "this account has no money
// left", wherever it is discovered.
//
// Written once because it is now reached three ways — OpenAI's 429 tagged
// insufficient_quota, Z.ai's 429 with a balance message, DeepSeek's plain 402 —
// and three copies of one sentence is three chances for them to drift into
// giving the same condition different advice.
//
// The status is carried into the text rather than hidden: it is what a bug
// report needs, and the provider's own words after it are what the user acts on.
func outOfCreditsError(providerName string, status int, said string) error {
	return fmt.Errorf(
		"%s says this account is out of credits, so waiting will not help. Top up at the provider's billing page, or switch to another provider. (%d: %s)",
		providerName, status, said,
	)
}

// outOfCredits reports whether a 429 body is a balance of zero rather than a
// rate limit. No backoff clears the first; only the user paying does.
func outOfCredits(body []byte) bool {
	lower := bytes.ToLower(body)
	for _, rebuttal := range waitItOutMarkers {
		if bytes.Contains(lower, rebuttal) {
			return false
		}
	}
	for _, marker := range outOfCreditsMarkers {
		if bytes.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// insufficientQuota is outOfCredits asked of a live response.
//
// The status alone cannot say this, so the transport has to peek at a body that
// belongs to the caller — whatever it reads is stitched back on before the
// response is handed over.
func insufficientQuota(resp *http.Response) bool {
	if resp.StatusCode != http.StatusTooManyRequests || resp.Body == nil {
		return false
	}
	peeked, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(peeked), resp.Body), resp.Body}
	return outOfCredits(peeked)
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
	return backoffFor(attempt)
}

// backoffFor is the wait before attempt+1 when nobody told us one — after a
// retryable status with no Retry-After, and after a transport failure, which
// has no response to carry a header at all.
//
// attempt is the zero-based index of the try that just failed, so it is never
// negative here; it clamps rather than trusting its input because the shift
// below would panic outright.
func backoffFor(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	// Capped at 16s: any longer and the transport's own maxWait would refuse to
	// sleep on it anyway, so a bigger number here could only ever be a surprise.
	if attempt > 4 {
		attempt = 4
	}
	// Stay short enough that the user does not think the app has hung.
	return time.Duration(1<<attempt) * time.Second
}
