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
	// Install the captured catalog for the whole package.
	//
	// Capability answers come from it now, and with none installed every one of
	// them is "unknown" — correct, and it would mean no test in this package
	// ever exercised a real capability again. This was learned the expensive
	// way: a first attempt at migrating the resolvers ran against an empty
	// catalog, twenty tests went red for that reason alone, and the whole
	// change was backed out.
	//
	// A test that wants the no-catalog world says so by calling
	// SetModelCatalog(nil) itself, and every test that swaps the catalog must
	// restore what was HERE rather than nil — restoring nil silently leaves
	// every later test in the world this line exists to avoid.
	SetModelCatalog(&ModelCatalog{
		Source: "models.dev (captured 2026-08-23)",
		Models: capabilityMatrixRows,
	})

	// os.Exit skips defers, so the cleanup has to happen before it.
	code := m.Run()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}
