// Package oauth holds every "sign in with your own subscription" flow Aetox
// supports, plus the credential store those flows write to.
//
// It imports nothing else from Aetox on purpose. internal/model resolves
// tokens through this package, and internal/config already imports
// internal/model — any edge back from here into config would close a cycle.
// The cost is that Root below repeats config.DataRoot's rule; the two must
// stay in step, which is what TestRootMatchesConfigDataRoot guards.
package oauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Credential is one provider's stored login.
//
// Two shapes share the struct because two shapes share the store: a flow
// either hands back a token that expires and must be refreshed ("oauth"), or
// it mints a plain API key that never does ("api" — what OpenRouter's PKCE
// flow returns). Keeping them in one file means Token() is the single place
// anything in Aetox asks "what do I send as credentials for this provider".
type Credential struct {
	Type      string `json:"type"` // "oauth" | "api"
	Access    string `json:"access,omitempty"`
	Refresh   string `json:"refresh,omitempty"`
	Key       string `json:"key,omitempty"`        // type "api" only
	ExpiresAt int64  `json:"expires_at,omitempty"` // unix millis; 0 = never expires
	// Account is the caller identity a provider wants echoed back on every
	// request, or the login name we show in the UI.
	Account string `json:"account,omitempty"`
	// Endpoint overrides the catalog base URL when the provider tells us at
	// login time which host this account is served from (Qwen's resource_url).
	Endpoint string `json:"endpoint,omitempty"`
	// Label is what the user sees in Settings ("GitHub Copilot · mike").
	Label string `json:"label,omitempty"`
}

// expiryGrace refreshes a token slightly before it dies. Some providers hand
// out short-lived tokens and a long streaming turn can outlive the tail of
// that window, so the grace is generous enough to cover one turn rather than
// one request.
const expiryGrace = 2 * time.Minute

// Expired reports whether the access token needs refreshing. API keys and
// tokens without a stated expiry never do.
func (c Credential) Expired() bool {
	if c.Type == "api" || c.ExpiresAt == 0 {
		return false
	}
	return time.Now().Add(expiryGrace).UnixMilli() >= c.ExpiresAt
}

// Root mirrors config.DataRoot — see the package comment for why it is copied
// rather than called.
func Root() string {
	if override := strings.TrimSpace(os.Getenv("AETOX_DATA_ROOT")); override != "" {
		return override
	}
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		if home, _ := os.UserHomeDir(); home != "" {
			configDir = filepath.Join(home, ".config")
		} else {
			configDir = os.TempDir()
		}
	}
	return filepath.Join(configDir, "aetox")
}

// StorePath is where logins live. Deliberately NOT model-preference.json:
// that file is copied into bug reports and synced by users between machines,
// and a refresh token in it is a standing account takeover.
func StorePath() string {
	return filepath.Join(Root(), "oauth.json")
}

// storeMu serializes read-modify-write on the store within this process.
// ponytail: one process-wide lock, not a file lock — two Aetox processes
// logging into different providers in the same second can still lose one
// write. Add a lock file if that ever shows up in practice.
var storeMu sync.Mutex

// removedProviders are sign-ins Aetox no longer offers (v0.8.1 dropped the
// Claude Pro/Max, ChatGPT and Copilot flows — they rode other products' OAuth
// clients against consumer plans, a standing account risk for the user).
// Credentials left in the store by an older version are dropped on load so a
// stale token is never sent anywhere, and the next save purges them from disk.
var removedProviders = map[string]bool{
	"anthropic":      true,
	"codex":          true,
	"github-copilot": true,
}

func load() map[string]Credential {
	raw, err := os.ReadFile(StorePath())
	if err != nil {
		return map[string]Credential{}
	}
	out := map[string]Credential{}
	if err := json.Unmarshal(raw, &out); err != nil {
		// A corrupt store reads as logged out rather than as a hard failure:
		// the fix a user can act on is "log in again", and returning an error
		// here would instead block the app from starting at all.
		return map[string]Credential{}
	}
	for name := range out {
		if removedProviders[name] {
			delete(out, name)
		}
	}
	return out
}

func save(store map[string]Credential) error {
	path := StorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	// Write-then-rename for the same reason SaveModelPreference does it: a
	// half-written credential file locks the user out of every provider.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Get returns the stored credential for a canonical provider id.
func Get(provider string) (Credential, bool) {
	storeMu.Lock()
	defer storeMu.Unlock()
	cred, ok := load()[key(provider)]
	return cred, ok
}

// Set stores (or replaces) one provider's credential, leaving every other
// provider's untouched.
func Set(provider string, cred Credential) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	store := load()
	store[key(provider)] = cred
	return save(store)
}

// Delete forgets one provider's login. Deleting an absent provider succeeds.
func Delete(provider string) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	store := load()
	delete(store, key(provider))
	return save(store)
}

// LoggedIn lists the canonical provider ids that currently have a credential,
// so Settings can show them without asking provider by provider.
func LoggedIn() []string {
	storeMu.Lock()
	defer storeMu.Unlock()
	store := load()
	out := make([]string, 0, len(store))
	for name := range store {
		out = append(out, name)
	}
	return out
}

func key(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}
