package main

import (
	"os"
	"regexp"
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

// The other half of the allowlist, which nothing was watching. The two tests
// above guard what is *in* desktopProviders; nothing guarded what is missing
// from it, and a missing name is indistinguishable from a decision — the engine
// gains a provider, the desktop never shows it, and the only record of why is
// that nobody typed it.
//
// So the absences are written down. A provider that is in the catalog and not
// on the desktop must be named here with the reason, which makes adding one to
// the engine a two-line decision instead of a silent omission.
var notOnTheDesktop = map[string]string{}

func TestDesktopPickerHidesOnlyWhatItMeansTo(t *testing.T) {
	listed := make(map[string]bool, len(desktopProviders))
	for _, p := range desktopProviders {
		listed[p] = true
	}
	for _, p := range model.SupportedProviders() {
		if listed[p] {
			if _, claimed := notOnTheDesktop[p]; claimed {
				t.Errorf("%q is in desktopProviders and also listed as not on the desktop — one of the two is stale", p)
			}
			continue
		}
		if reason, claimed := notOnTheDesktop[p]; !claimed {
			t.Errorf("the engine knows %q and the desktop never shows it, with no reason recorded — add it to desktopProviders or say here why not", p)
		} else if reason == "" {
			t.Errorf("%q is kept off the desktop with an empty reason", p)
		}
	}
	for p := range notOnTheDesktop {
		if _, known := model.ProviderInfo(p); !known {
			t.Errorf("notOnTheDesktop names %q, which the engine catalog does not have", p)
		}
	}
}

// A third list nobody was comparing: the brand marks live in TypeScript, keyed
// by the same canonical name this Go slice holds, and ProviderMark.svelte falls
// back to a lettered tile when the key is missing. That fallback is deliberate
// (a gap would break the row's width) but it is silent — `codex` shipped as a
// grey "C" next to fourteen real marks and no test had an opinion.
//
// Reading the .ts from Go rather than asserting in vitest is the point: the
// authority on which rows exist is this slice, and a frontend test would have
// to restate it and could then drift from it. Same shape as
// internal/proc/coverage_test.go, which scans source for the same reason.
func TestEveryDesktopProviderHasABrandMark(t *testing.T) {
	const marksPath = "frontend/src/lib/providerMarks.ts"
	raw, err := os.ReadFile(marksPath)
	if err != nil {
		t.Fatalf("read %s: %v", marksPath, err)
	}
	// Keys sit at one indent level and are followed by a backtick: `  openai: `.
	// The quotes are optional because a canonical name with a hyphen
	// ("ollama-cloud") is not a bare JavaScript identifier and has to be
	// written `'ollama-cloud':` — this test went blind to exactly that row
	// until the pattern learned to see it.
	keyed := regexp.MustCompile("(?m)^\t*  '?([a-z0-9-]+)'?: `")
	marks := make(map[string]bool)
	for _, m := range keyed.FindAllStringSubmatch(string(raw), -1) {
		marks[m[1]] = true
	}
	if len(marks) == 0 {
		t.Fatalf("parsed no marks out of %s — the shape of that file changed and this test is now blind", marksPath)
	}

	for _, p := range desktopProviders {
		// aetox is the documented exception: its mark is Logo.svelte, because it
		// carries the brand colour and an entrance animation no other row gets.
		if p == "aetox" {
			continue
		}
		if !marks[p] {
			t.Errorf("desktopProviders lists %q with no entry in %s — it renders as a lettered tile", p, marksPath)
		}
	}
}
