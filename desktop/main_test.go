package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// TestMain points every test in this package at a throwaway data root.
//
// This is not tidiness. applyConfig persists the model preference — that is how
// the desktop app and the CLI stay on one choice — so every test that
// bootstraps the engine wrote straight into the developer's own
// %APPDATA%\aetox\model-preference.json. Running the suite left the real
// preference set to whatever model the last such test happened to use, and the
// app then opened on a test model with nobody having chosen it. Measured: a
// full `go test ./...` rewrote that file to aetox/aetox-think:test (think=low,
// approval=ask) — field for field the state seen in the wild.
//
// One override here covers every test in the package, including ones not
// written yet, which is the point: the trap was invisible at each call site. A
// test that needs its own root still sets AETOX_DATA_ROOT via t.Setenv, and
// that restores to this temp root afterwards rather than to the real one.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "aetox-desktop-test-")
	if err != nil {
		panic("test data root: " + err.Error())
	}
	if err := os.Setenv("AETOX_DATA_ROOT", root); err != nil {
		panic("test data root: " + err.Error())
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}

// The guard for the guard: if TestMain above is ever removed, or stops setting
// AETOX_DATA_ROOT, every test in this package quietly goes back to writing the
// developer's real preference file — and nothing in any individual test would
// look wrong. Fail here instead, with the reason attached.
func TestDataRootIsAThrowawayDirectory(t *testing.T) {
	root, err := config.DataRoot()
	if err != nil {
		t.Fatalf("DataRoot: %v", err)
	}
	tmp := filepath.Clean(os.TempDir())
	if !strings.HasPrefix(strings.ToLower(filepath.Clean(root)), strings.ToLower(tmp)) {
		t.Fatalf("tests resolve DataRoot to %q, outside %q — applyConfig persists the model preference, so that is the real one this machine runs on (see TestMain)", root, tmp)
	}
}
