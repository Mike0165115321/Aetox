// Package windmill is Aetox's connection to a Windmill the user runs
// themselves: where it lives, the token to send it, and who that token belongs
// to.
//
// Shaped like internal/n8n, which is shaped like internal/github — same vault,
// same four verbs, and the same rule that the package owning the connection owns
// where it connects to. Two things differ from n8n and both shape the file:
//
//   - Windmill can say who you are. `GET /api/users/whoami` returns the account,
//     so unlike n8n this connection is labelled with a person rather than with a
//     hostname.
//   - Nearly everything else is scoped to a WORKSPACE, and the segment is the
//     workspace's `id` — not its display name. Using the name produces 404s that
//     read like permission errors, which is why the id is what travels.
package windmill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/oauth"
)

const (
	// ConnectionID is this connection's id everywhere: the connect catalog, the
	// credential vault, and the `for:` placement in connections.json.
	ConnectionID = "windmill"

	// APIPath is where the API sits under the origin the user pasted. Windmill
	// owns the root of its host, so unlike n8n there is no configurable prefix
	// to guess at.
	APIPath = "/api"

	userAgent = "Aetox"
)

// Account is who a token belongs to. Email rather than username: the API marks
// email required and username optional, so a client that keyed on the pretty
// one would show a blank label for accounts that never set it.
type Account struct {
	Login string `json:"login"`
	Name  string `json:"name,omitempty"`
}

type Source string

const (
	SourceNone       Source = ""
	SourceConnection Source = "connection"
)

// Status is what a settings page renders. Never the token, and never a network
// call.
type Status struct {
	Connected bool   `json:"connected"`
	Login     string `json:"login,omitempty"`
	Source    Source `json:"source,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
}

// BaseURL is the origin this user's Windmill answers on, without a trailing
// slash, or "" when no address has been given.
func BaseURL() string {
	return strings.TrimRight(strings.TrimSpace(config.ConnectionBaseURL(ConnectionID)), "/")
}

// APIBase is BaseURL with the API prefix appended. An address that already ends
// in it is left alone — people paste what worked in curl.
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

// Token returns the credential to send, or "" when no account is attached.
func Token() string {
	if cred, ok := oauth.Get(ConnectionID); ok {
		return strings.TrimSpace(cred.Key)
	}
	return ""
}

// CurrentStatus reports the connection without asking Windmill anything.
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

// Connect stores a token after proving it reaches the instance the user named
// and finding out whose it is.
func Connect(ctx context.Context, token string) (Account, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Account{}, errors.New("ต้องใส่ token ของ Windmill")
	}
	account, err := whoami(ctx, APIBase(), token)
	if err != nil {
		return Account{}, err
	}
	cred := oauth.Credential{
		Type:    "api",
		Key:     token,
		Account: account.Login,
		Label:   "Windmill · " + account.Login,
	}
	if err := oauth.Set(ConnectionID, cred); err != nil {
		return Account{}, fmt.Errorf("บันทึกการเชื่อมต่อ Windmill ไม่สำเร็จ: %w", err)
	}
	return account, nil
}

// Verify re-checks the stored token against the stored address.
func Verify(ctx context.Context) (Account, error) {
	token := Token()
	if token == "" {
		return Account{}, errors.New("ยังไม่ได้เชื่อม Windmill")
	}
	return whoami(ctx, APIBase(), token)
}

// Disconnect forgets the token and keeps the address — a setting the user
// typed, not a credential.
func Disconnect() error {
	return oauth.Delete(ConnectionID)
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// whoami is GET /api/users/whoami: the smallest call that answers both "does
// this token work" and "whose is it".
func whoami(ctx context.Context, apiBase, token string) (Account, error) {
	if apiBase == "" {
		return Account{}, errors.New("ยังไม่ได้ระบุที่อยู่ของ Windmill")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/users/whoami", nil)
	if err != nil {
		return Account{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return Account{}, fmt.Errorf("ติดต่อ Windmill ไม่ได้ที่ %s — ตรวจว่าเซิร์ฟเวอร์เปิดอยู่และที่อยู่ถูกต้อง", BaseURL())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Account{}, fmt.Errorf("อ่านคำตอบจาก Windmill ไม่สำเร็จ: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return Account{}, errors.New("Windmill ปฏิเสธ token นี้ — ผิด หมดอายุ หรือถูกเพิกถอน")
	case resp.StatusCode == http.StatusForbidden:
		// Almost always the read_only box, which is easy to tick and impossible
		// to guess at from a bare "forbidden".
		return Account{}, errors.New("token นี้สิทธิ์ไม่พอ — ตอนสร้างอย่าติ๊ก read_only และเว้นช่อง scope ไว้ว่าง")
	case resp.StatusCode == http.StatusNotFound:
		return Account{}, fmt.Errorf("ไม่พบ API ของ Windmill ที่ %s — ที่อยู่อาจผิด", apiBase)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return Account{}, fmt.Errorf("Windmill ตอบกลับสถานะ %d", resp.StatusCode)
	}

	var parsed struct {
		Email    string `json:"email"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Account{}, fmt.Errorf("คำตอบจาก Windmill ไม่ใช่ JSON ที่อ่านได้: %w", err)
	}
	// Email is the required field; username is optional and absent on plenty of
	// real accounts, so keying on it would label a working connection blank.
	if strings.TrimSpace(parsed.Email) == "" {
		return Account{}, errors.New("Windmill ตอบกลับบัญชีที่ไม่มีอีเมล")
	}
	return Account{Login: parsed.Email, Name: strings.TrimSpace(parsed.Username)}, nil
}
