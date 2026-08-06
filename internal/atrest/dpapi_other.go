//go:build !windows

package atrest

// No encryption at rest off Windows yet — the credentials file is plaintext,
// protected by its 0600 mode and the user-owned directory it sits in, which is
// what aws, gcloud, npm and gh (without a keyring) all do.
//
// The platform equivalents are the macOS Keychain and the Linux Secret Service,
// and both are a different shape from DPAPI: they are a daemon holding named
// items, not a function that wraps bytes, so they need a real dependency and a
// story for headless machines where no keyring is running. Worth doing when
// those platforms are actually shipped to; not worth a half-built version now
// that would have to be redesigned then.
//
// Named the same as the Windows pair so nothing above this line branches on the
// platform: a caller that had to ask would be a caller that can forget to.
func Protect(plaintext []byte) []byte { return plaintext }

func Unprotect(stored []byte) []byte { return stored }
