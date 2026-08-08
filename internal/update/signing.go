package update

// Release signatures: the difference between "this file arrived intact" and
// "this file came from us".
//
// checksums.txt alone proves integrity, not origin — it sits in the same
// release as the files it describes, so anyone who can replace the files can
// replace it too. The signature breaks that symmetry: checksums.txt.sig is
// made with a private key that exists only in the repo's CI secret (and the
// owner's backup), and this binary carries the public half. An attacker who
// owns the GitHub account can still swap every asset; what they cannot do is
// sign the swap. Auto-update refuses, loudly, and the user is exactly where
// they were.
//
// Only checksums.txt is signed, and that is enough: every downloadable is
// verified against it by hash, so trust flows sig → checksums → artifact.
//
// cmd/relsign is the other half — release.yml runs it with the secret to
// produce the .sig this file demands.

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
)

// releasePublicKey verifies checksums.txt.sig. Compiled in, which is the whole
// security property: replacing it requires replacing the binary, and the
// binary is what the user already trusts. A var (not const) only so the
// package's own tests can pair it with a throwaway key.
//
// Generated 2026-08-09; the private half lives in the repo's AETOX_SIGNING_KEY
// secret and the owner's backup. Rotating it means: new pair, new secret, new
// value here, new release — older builds keep verifying old releases fine,
// they just can't verify ones signed after the rotation.
var releasePublicKey = "BpT/5X1XAnKV6BTJpf9vhWuU6lUct810BdvH5sdlSdo="

const signatureName = checksumsName + ".sig"

// verifySignature checks that sig (base64, as relsign writes it) signs data
// with the release key. Empty releasePublicKey refuses everything: a build
// without a key must fail closed, not skip the check it cannot do.
func verifySignature(data, sig []byte) error {
	pub, err := base64.StdEncoding.DecodeString(releasePublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("build นี้ไม่มีกุญแจตรวจลายเซ็นที่ใช้ได้ — อัปเดตอัตโนมัติถูกปิด")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil {
		return fmt.Errorf("อ่านลายเซ็นไม่ออก: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), data, raw) {
		return fmt.Errorf("ลายเซ็นของ %s ไม่ผ่านการตรวจ — ไฟล์ release อาจถูกสลับ ระบบไม่ติดตั้งต่อ", checksumsName)
	}
	return nil
}
