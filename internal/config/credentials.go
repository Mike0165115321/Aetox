package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Mikedev115/Aetox/internal/atrest"
	"github.com/Mikedev115/Aetox/internal/debuglog"
)

// Credentials are the secrets that used to live in model-preference.json, in
// their own file (owner's call, 2026-08-06).
//
// The reason is not that a preference file cannot be given tight permissions —
// it already had 0600 and an atomic write. It is that secrets and settings have
// different *handling*, and one file cannot have two. `ui_locale`, `last_desk`,
// `user_name` and `speech_model_path` are things people open the file to check,
// paste into a bug report, and screenshot. The API keys sat in the same object,
// so every one of those ordinary acts leaked them — which is exactly how a key
// ended up in a debugging transcript on the day this was written.
//
// Splitting them means the ordinary file can stay ordinary: readable, quotable,
// safe to show. Everything that is genuinely a secret is in one place, with one
// set of rules — encrypted at rest where the platform offers it (secret_*.go),
// refused by every file tool (skill.ownSecretFiles), and scrubbed from the
// debug log (debuglog.Redact).
type Credentials struct {
	// ModelAPIKeys is keyed by provider, the same shape the preference file
	// held, so nothing above this layer had to learn a new one.
	ModelAPIKeys map[string]string `json:"provider_api_keys,omitempty"`
}

// CredentialsPath is <DataRoot>/credentials.json.
func CredentialsPath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "credentials.json"), nil
}

// LoadCredentials reads the secrets file. A missing file is not an error — it
// is a fresh install, or one whose keys have not been split out yet.
//
// Every key that comes back is registered with debuglog, which is the one place
// that can guarantee it never appears in a log line: doing it at the read means
// no caller can forget, and a key that was never loaded cannot leak.
func LoadCredentials() (Credentials, error) {
	var creds Credentials
	path, err := CredentialsPath()
	if err != nil {
		return creds, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return creds, nil
		}
		return creds, err
	}
	if err := json.Unmarshal(atrest.Unprotect(raw), &creds); err != nil {
		return creds, err
	}
	for _, key := range creds.ModelAPIKeys {
		debuglog.Redact(key)
	}
	return creds, nil
}

// SaveCredentials writes the secrets file: encrypted at rest where the platform
// offers it, 0600, atomically.
//
// Write-then-rename for the same reason the preference file uses it — a
// truncate that dies mid-write loses every key at once — and the temp file is
// created 0600 too, since a secret briefly world-readable is still a secret
// that was world-readable.
func SaveCredentials(creds Credentials) error {
	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	for _, key := range creds.ModelAPIKeys {
		debuglog.Redact(key)
	}
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

// migrateCredentialsOutOfPreferences moves keys still sitting in
// model-preference.json into the credentials file and rewrites the preference
// file without them.
//
// Ordered so a crash cannot lose a key: the credentials file is written first
// and only a successful write leads to stripping the original. The worst
// interruption leaves the key in both files, which the next run resolves — the
// opposite order would leave a user unable to reach their own provider.
//
// Keys already in the credentials file win. That file is the one the app writes
// to now, so anything in the preference file is by definition the older copy.
func migrateCredentialsOutOfPreferences(pref *ModelPreference) {
	if len(pref.ModelAPIKeys) == 0 {
		return
	}
	creds, err := LoadCredentials()
	if err != nil {
		return // unreadable secrets file: leave the old copy where it is
	}
	if creds.ModelAPIKeys == nil {
		creds.ModelAPIKeys = map[string]string{}
	}
	moved := false
	for provider, key := range pref.ModelAPIKeys {
		if _, taken := creds.ModelAPIKeys[provider]; taken {
			continue
		}
		creds.ModelAPIKeys[provider] = key
		moved = true
	}
	if moved {
		if err := SaveCredentials(creds); err != nil {
			return // could not store them safely: do not remove the only copy
		}
	}
	pref.ModelAPIKeys = nil
	_ = SaveModelPreference(*pref)
	pref.ModelAPIKeys = creds.ModelAPIKeys
}
