//go:build windows

package atrest

import (
	"encoding/base64"
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPI (CryptProtectData) encrypts the credentials file at rest, bound to this
// Windows account. It is the platform's answer to "an app has to hold a secret
// and reuse it without asking every time", and it is what the OS itself keeps
// the Credential Manager's contents under.
//
// What it buys and what it does not, stated plainly so nobody over-trusts it:
// the key is derived from the user's logon, so the ciphertext is useless on
// another machine or in another account — which covers the leaks that actually
// happen to a desktop app: a backup, a folder synced to cloud storage, a copied
// profile, a support bundle, a screenshot of a config file. It does NOT stop
// malware running as this user, because DPAPI decrypts for this user on
// request. Anyone selling it as protection against that is selling the wrong
// thing.
//
// The stored form is base64 of the blob rather than raw bytes, so the file
// stays a text file that survives being opened, diffed and copied by hand.
const dpapiPrefix = "dpapi:"

// protect returns plaintext wrapped for storage. A failure returns the
// plaintext unchanged rather than an error: refusing to save the user's API key
// because an OS call failed would trade a real, immediate loss of function for
// a hypothetical gain, and the caller has no better move to offer them.
func Protect(plaintext []byte) []byte {
	if len(plaintext) == 0 {
		return plaintext
	}
	out, err := cryptProtect(plaintext)
	if err != nil {
		return plaintext
	}
	return []byte(dpapiPrefix + base64.StdEncoding.EncodeToString(out))
}

// unprotect reverses protect, and passes through anything that was not written
// by it — a file from before this existed, or one written on a machine where
// the call failed. Both are ordinary states, not corruption.
func Unprotect(stored []byte) []byte {
	text := strings.TrimSpace(string(stored))
	if !strings.HasPrefix(text, dpapiPrefix) {
		return stored
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(text, dpapiPrefix))
	if err != nil {
		return stored
	}
	out, err := cryptUnprotect(raw)
	if err != nil {
		// Wrong account, restored from another machine, or a corrupt blob.
		// Returning the ciphertext makes the JSON fail to parse, which the
		// caller already handles as "no credentials yet" — the user re-enters
		// the key. Returning a partial or invented value would be worse.
		return stored
	}
	return out
}

func cryptProtect(in []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blobOf(in), nil, nil, 0, nil, 0, &out); err != nil {
		return nil, err
	}
	return takeBlob(&out)
}

func cryptUnprotect(in []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blobOf(in), nil, nil, 0, nil, 0, &out); err != nil {
		return nil, err
	}
	return takeBlob(&out)
}

func blobOf(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// takeBlob copies what the API allocated and frees it. LocalFree is not
// optional: DPAPI hands back memory from the process heap, and skipping the
// free leaks it on every save and load for as long as the app runs.
func takeBlob(b *windows.DataBlob) ([]byte, error) {
	if b.Data == nil {
		return nil, errors.New("dpapi returned no data")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	return append([]byte(nil), unsafe.Slice(b.Data, b.Size)...), nil
}
