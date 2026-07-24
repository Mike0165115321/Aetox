package edge

import (
	"errors"
	"testing"
)

// AETOX PATCH verification. This is a real proof, not a mock: if the patch
// regressed and errorCallback still os.Exit(1)'d when a custom callback is
// installed, this test would take the whole `go test` process down with it —
// a failing test, loudly. Surviving to the assertions IS the guarantee that a
// browser tab's WebView2 error can no longer crash the app.
func TestCustomErrorCallbackDoesNotExit(t *testing.T) {
	e := NewChromium()

	var got error
	e.SetErrorCallback(func(err error) { got = err })
	if !e.customErrorCallback {
		t.Fatal("SetErrorCallback must flag customErrorCallback")
	}

	sentinel := errors.New("transient webview failure")
	e.errorCallback(sentinel) // upstream would os.Exit(1) here

	if got != sentinel {
		t.Fatalf("callback got %v, want the passed error", got)
	}
}
