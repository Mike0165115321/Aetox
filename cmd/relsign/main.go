// relsign signs one release file with the project's ed25519 release key —
// release.yml runs it over checksums.txt so the app's embedded public key
// (internal/update/signing.go) has something to verify.
//
//	AETOX_SIGNING_KEY=<base64 32-byte seed> go run ./cmd/relsign checksums.txt
//
// writes checksums.txt.sig (base64 signature). Fails loudly when the key is
// absent or malformed: a release that ships unsigned is a release every
// auto-updater in the wild silently refuses, which is far harder to notice
// than a red CI run.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "relsign:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: relsign <file>")
	}
	seed, err := base64.StdEncoding.DecodeString(os.Getenv("AETOX_SIGNING_KEY"))
	if err != nil || len(seed) != ed25519.SeedSize {
		return fmt.Errorf("AETOX_SIGNING_KEY is missing or not a base64 %d-byte seed", ed25519.SeedSize)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	key := ed25519.NewKeyFromSeed(seed)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, data))
	out := os.Args[1] + ".sig"
	if err := os.WriteFile(out, []byte(sig+"\n"), 0o644); err != nil {
		return err
	}
	// The public key is printed so a release log is enough to confirm WHICH
	// key signed — it must match internal/update/signing.go verbatim.
	fmt.Printf("signed %s with public key %s\n", os.Args[1],
		base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)))
	return nil
}
