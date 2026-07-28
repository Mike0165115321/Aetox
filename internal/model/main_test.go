package model

import (
	"os"
	"testing"
)

// TestMain points the OAuth credential store at a throwaway directory for the
// whole package.
//
// internal/model now consults internal/oauth when it builds a provider, so
// without this a unit test's result depends on whether the developer happens to
// be signed into that provider on this machine — which is exactly how
// TestNewProviderAnthropicMissingAPIKey started failing on a laptop where
// somebody had signed into Claude. Tests that want a credential seed one
// explicitly (see signIn in oauth_wiring_test.go); the live tests override this
// with their own temp dir.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "aetox-model-tests-")
	if err == nil {
		_ = os.Setenv("AETOX_DATA_ROOT", dir)
	}
	// os.Exit skips defers, so the cleanup has to happen before it.
	code := m.Run()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}
