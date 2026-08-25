package account

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/Mikedev115/Aetox/internal/atrest"
	"github.com/Mikedev115/Aetox/internal/oauth"
)

// User is the person the id server says is signed in. It mirrors the server's
// users row and nothing else: Aetox never stores the GitHub or Google token
// that opened the door, because the server never hands one out.
type User struct {
	ID        string `json:"id"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
}

// Display is what Settings and the CLI put on screen. Name first because it is
// what the person recognises; the email is the fallback because an account
// opened through GitHub may have no display name at all.
func (u User) Display() string {
	if u.Name != "" {
		return u.Name
	}
	if u.Email != "" {
		return u.Email
	}
	return u.ID
}

// Session is one signed-in account on this machine.
type Session struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	// ExpiresAt is unix millis for the access token only. The refresh token
	// carries its own, longer life that only the server knows.
	ExpiresAt int64 `json:"expires_at,omitempty"`
	User      User  `json:"user"`
}

// StorePath is where the account session lives. Its own file, not oauth.json:
// that file answers "what credential does this model provider take" and
// internal/model reads it to build a request. An account token buys nothing on
// any inference API, and a row in that map would be a row the provider picker
// then has to learn to hide.
func StorePath() string {
	return filepath.Join(oauth.Root(), "account.json")
}

// storeMu serializes read-modify-write within this process. The same caveat as
// oauth.storeMu applies: two Aetox processes are not serialized against each
// other. It matters more here than there — see the note on refresh rotation in
// Token.
var storeMu sync.Mutex

// Load returns the stored session, if there is one.
func Load() (Session, bool) {
	storeMu.Lock()
	defer storeMu.Unlock()
	return load()
}

func load() (Session, bool) {
	raw, err := os.ReadFile(StorePath())
	if err != nil {
		return Session{}, false
	}
	var s Session
	if err := json.Unmarshal(atrest.Unprotect(raw), &s); err != nil {
		// A corrupt file reads as signed out. The fix a person can act on is
		// "sign in again", and an error here would instead be a startup that
		// fails over an optional feature.
		return Session{}, false
	}
	if s.Access == "" {
		return Session{}, false
	}
	return s, true
}

// Save replaces the stored session.
func Save(s Session) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	return save(s)
}

func save(s Session) error {
	path := StorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// Wrapped at rest where the platform offers it, 0600, write-then-rename —
	// the same three rules oauth.save follows, for the same reason: this is a
	// bearer token whose string is the whole credential.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, atrest.Protect(payload), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Clear signs out on this machine. Clearing when already signed out succeeds.
func Clear() error {
	storeMu.Lock()
	defer storeMu.Unlock()
	return clear()
}

func clear() error {
	if err := os.Remove(StorePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Current is who is signed in, answered from disk without a network call. The
// UI asks this on every render; it must never reach the server to answer.
func Current() (User, bool) {
	s, ok := Load()
	return s.User, ok
}
