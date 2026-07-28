package main

import (
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/oauth"
)

// desktopProviders is an allowlist, so a provider added to the engine catalog
// is invisible in the desktop until someone remembers to name it here. That is
// how four working sign-in providers shipped with a green suite and no way to
// reach them in the UI.
//
// These two tests are the guard: anything the engine can sign into must be
// reachable, and nothing listed here may be a name the engine does not know.

func TestEverySignInProviderIsInTheDesktopPicker(t *testing.T) {
	listed := make(map[string]bool, len(desktopProviders))
	for _, p := range desktopProviders {
		listed[p] = true
	}
	for _, method := range oauth.Methods() {
		if !listed[method.Provider] {
			t.Errorf("%q has a sign-in but is not in desktopProviders — the UI cannot offer it", method.Provider)
		}
	}
}

func TestDesktopPickerNamesOnlyRealProviders(t *testing.T) {
	known := make(map[string]bool)
	for _, p := range model.SupportedProviders() {
		known[p] = true
	}
	for _, p := range desktopProviders {
		if !known[p] {
			t.Errorf("desktopProviders lists %q, which the engine catalog does not have — it would silently never render", p)
		}
		if canonical := model.NormalizeProvider(p); canonical != p {
			t.Errorf("desktopProviders lists %q, whose canonical name is %q — the picker keys rows by this string", p, canonical)
		}
	}
}
