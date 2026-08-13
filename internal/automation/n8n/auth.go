// Package n8n is Aetox's connection to an n8n the user runs themselves: where
// it lives, the key to send it, and the smallest request that proves both.
//
// It is shaped like internal/github on purpose — same vault, same four verbs,
// same rule that the package owning the connection owns where it connects to.
// One thing is genuinely different and shapes the whole file: GitHub is one
// host for everybody and can state it as a constant, while n8n is software the
// user installed on a machine only they know the address of. That address is a
// setting (internal/config's connections.json), not a secret, and it is read
// back here rather than passed in, so that a tool three layers away can ask
// "where is n8n" without every caller in between having to carry the answer.
package n8n

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/oauth"
	"github.com/Mike0165115321/Aetox/internal/statereport"
)

const (
	// ConnectionID is this connection's id everywhere: the connect catalog, the
	// credential vault, and the `for:` placement in connections.json.
	ConnectionID = "n8n"

	// APIPath is where the public API sits under whatever the user pasted.
	//
	// Both segments are instance-configurable — N8N_PATH mounts the whole app
	// under a prefix and N8N_PUBLIC_API_ENDPOINT renames the `api` segment — so
	// this is a default, not a law. A user whose instance is arranged unusually
	// pastes the full API base instead, which is why BaseURL below leaves an
	// address that already contains this path alone.
	APIPath = "/api/v1"

	// authHeader is a plain header carrying the raw key. Not Bearer, not
	// base64: n8n reads x-n8n-api-key off the request directly, and sending it
	// as an Authorization header authenticates nothing.
	authHeader = "X-N8N-API-KEY"

	userAgent = "Aetox"
)

// Account is who a key belongs to — except that n8n cannot say.
//
// The public API has no whoami: no /me, no identity on any response, and the
// one endpoint that could answer (GET /users) needs instance-owner rights that
// an ordinary key does not have. So Login is the host the key was proved
// against rather than a person's name. That is a real answer to the question
// the UI is asking — "which n8n is this?" — and inventing a username would be
// worse than admitting we do not have one.
type Account struct {
	Login string `json:"login"`
	Name  string `json:"name,omitempty"`
}

// Source mirrors github.Source. There is no environment fallback for n8n — no
// conventional variable exists for it, and inventing one would be a second way
// to configure the same thing — so a connection is the only source there is.
type Source string

const (
	SourceNone       Source = ""
	SourceConnection Source = "connection"
)

// Status is what a settings page renders. Never the key, and never a network
// call: a settings page renders on every keystroke of its search box.
type Status struct {
	Connected bool   `json:"connected"`
	Login     string `json:"login,omitempty"`
	Source    Source `json:"source,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
}

// BaseURL is where this user's n8n lives, without a trailing slash, or "" when
// no address has been given.
func BaseURL() string {
	return strings.TrimRight(strings.TrimSpace(config.ConnectionBaseURL(ConnectionID)), "/")
}

// APIBase is BaseURL with the API prefix appended — the root every request is
// built from.
//
// An address that already ends in the prefix is left alone. People paste what
// worked in curl, and turning `.../api/v1` into `.../api/v1/api/v1` would 404
// with an error message that blames their key.
func APIBase() string {
	base := BaseURL()
	if base == "" {
		return ""
	}
	if strings.HasSuffix(base, APIPath) {
		return base
	}
	return base + APIPath
}

// Token returns the key to send, or "" when no account is attached.
func Token() string {
	if cred, ok := oauth.Get(ConnectionID); ok {
		return strings.TrimSpace(cred.Key)
	}
	return ""
}

// CurrentStatus reports the connection without asking n8n anything.
func CurrentStatus() Status {
	cred, ok := oauth.Get(ConnectionID)
	if !ok || strings.TrimSpace(cred.Key) == "" {
		return Status{BaseURL: BaseURL()}
	}
	return Status{
		Connected: true, Login: cred.Account,
		Source: SourceConnection, BaseURL: BaseURL(),
	}
}

// Connect stores a key after proving it reaches the instance the user named.
//
// The address is read from storage rather than taken as an argument because
// connect.Connect writes it first, for exactly this: a key cannot be validated
// without knowing where to validate it, so the two arrive together or the
// attempt is meaningless.
func Connect(ctx context.Context, key string) (Account, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return Account{}, errors.New("ต้องใส่ API key ของ n8n")
	}
	account, err := probe(ctx, APIBase(), key)
	if err != nil {
		return Account{}, err
	}
	cred := oauth.Credential{
		Type:    "api",
		Key:     key,
		Account: account.Login,
		Label:   "n8n · " + account.Login,
	}
	if err := oauth.Set(ConnectionID, cred); err != nil {
		return Account{}, fmt.Errorf("บันทึกการเชื่อมต่อ n8n ไม่สำเร็จ: %w", err)
	}
	return account, nil
}

// Verify re-checks the stored key against the stored address.
//
// Worth pressing more often here than for a hosted service: an n8n API key
// carries an expiry the user chose when they made it, so one that worked
// yesterday can be dead today with nothing having changed on this side.
func Verify(ctx context.Context) (Account, error) {
	key := Token()
	if key == "" {
		return Account{}, statereport.New("ยังไม่ได้เชื่อม n8n")
	}
	return probe(ctx, APIBase(), key)
}

// Disconnect forgets the key. The address stays: it is a setting the user
// typed, not a credential, and asking them where their own server lives a
// second time because they rotated a key is a small insult.
func Disconnect() error {
	return oauth.Delete(ConnectionID)
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// probe is GET /workflows?limit=1 — the smallest call that answers "does this
// key work, against this address".
//
// It is a list call rather than an identity call because n8n has no identity
// call. Every version has this endpoint, it needs only the ability to list, and
// its three failure modes map cleanly onto the three things a user gets wrong:
// a bad key, an address pointing at something that is not n8n, and an instance
// whose operator switched the public API off.
func probe(ctx context.Context, apiBase, key string) (Account, error) {
	if apiBase == "" {
		return Account{}, statereport.New("ยังไม่ได้ระบุที่อยู่ของ n8n")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/workflows?limit=1", nil)
	if err != nil {
		return Account{}, err
	}
	req.Header.Set(authHeader, key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return Account{}, statereport.Newf("ติดต่อ n8n ไม่ได้ที่ %s — ตรวจว่าเซิร์ฟเวอร์เปิดอยู่และที่อยู่ถูกต้อง", BaseURL())
	}
	defer resp.Body.Close()
	// Drained so the connection can be reused, capped so a wrong address
	// pointing at something that streams cannot hold the call open.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	// All four are the machine's state — a key's validity, an address's truth,
	// a server's answer — not behaviour anyone should learn from (statereport).
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return Account{}, statereport.New("n8n ปฏิเสธ API key นี้ — ผิด หมดอายุ หรือถูกเพิกถอน (คีย์ของ n8n มีวันหมดอายุที่ตั้งไว้ตอนสร้าง)")
	case resp.StatusCode == http.StatusForbidden:
		return Account{}, statereport.New("คีย์นี้ใช้ได้แต่สิทธิ์ไม่พอ — สร้างคีย์ใหม่ที่มีสิทธิ์อ่านและเขียน workflow")
	case resp.StatusCode == http.StatusNotFound:
		// The most common self-host misconfiguration, and the one whose default
		// error message sends the user looking at their key instead.
		return Account{}, statereport.Newf("ไม่พบ API ของ n8n ที่ %s — ที่อยู่อาจผิด หรือผู้ดูแลปิด public API ไว้ (N8N_PUBLIC_API_DISABLED)", apiBase)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return Account{}, statereport.Newf("n8n ตอบกลับสถานะ %d", resp.StatusCode)
	}
	// The host, because it is the true answer to "which n8n is this" and the
	// only one the API can give.
	return Account{Login: hostOf(BaseURL()), Name: "n8n"}, nil
}

func hostOf(raw string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	if slash := strings.IndexByte(trimmed, '/'); slash >= 0 {
		trimmed = trimmed[:slash]
	}
	if trimmed == "" {
		return "n8n"
	}
	return trimmed
}

// decodeJSON is the shared reader for the tool calls in internal/skill: one cap,
// one error sentence, so a 50MB workflow list cannot be read into memory by one
// caller because it forgot.
func decodeJSON(resp *http.Response, into any) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("อ่านคำตอบจาก n8n ไม่สำเร็จ: %w", err)
	}
	if into == nil {
		return nil
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("คำตอบจาก n8n ไม่ใช่ JSON ที่อ่านได้: %w", err)
	}
	return nil
}
