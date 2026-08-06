// Package atrest wraps a secret for storage on disk, using whatever the
// platform provides — DPAPI on Windows, plaintext everywhere else for now.
//
// Its own package, below both internal/config and internal/oauth, because both
// of them hold credentials and internal/config already imports internal/oauth:
// putting the pair in config would have made the token store unable to reach
// it, and copying it into oauth would be the same rule kept in two places, one
// of which would eventually be the one nobody updated.
//
// The two functions are deliberately total — neither returns an error. A
// failure to encrypt returns the plaintext, and a failure to decrypt returns
// what it was given. Refusing to save a user's API key because an OS call
// failed trades a real, immediate loss of function for a hypothetical gain, and
// there is no better move to offer the caller at that point. What each one does
// on failure is documented at the function.
package atrest
