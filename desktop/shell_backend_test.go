package main

import (
	"strings"
	"testing"
)

// These run against the machine's real distro list, because that is the part of
// the picker no unit test can fake: proc.Distros() shells out to wsl.exe, and
// what comes back is what the menu shows. On a machine with no WSL they still
// assert the one entry every machine has.

// Every entry must carry a label. An option rendering as an empty chip is not a
// cosmetic fault — it is a row in the menu that the user cannot tell apart from
// the one below it, and the setting behind it changes where their commands run.
func TestShellsLabelsEveryOption(t *testing.T) {
	a := &App{}
	options := a.Shells()
	if len(options) == 0 {
		t.Fatal("Shells() offered nothing — the native shell is always available")
	}
	for _, option := range options {
		if strings.TrimSpace(option.Label) == "" {
			t.Errorf("option %q has no label", option.Setting)
		}
		if strings.TrimSpace(option.Setting) == "" {
			t.Errorf("option labelled %q has no setting to store", option.Label)
		}
		t.Logf("%-16s %s", option.Setting, option.Label)
	}
}

// Two rows reading the same is the same failure as a blank one: the user picks
// one of them and cannot know which. Distro names are unique by construction,
// so this really guards the native entry never colliding with a distro called
// "Windows" — which is a name somebody will eventually give one.
func TestShellsLabelsAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, option := range (&App{}).Shells() {
		if other, ok := seen[option.Label]; ok {
			t.Errorf("%q and %q both read as %q in the menu", other, option.Setting, option.Label)
		}
		seen[option.Label] = option.Setting
	}
}

// The chip and the menu have to agree: the chip is drawn from CurrentShell and
// the menu from Shells, and a selected row whose text differs from the chip
// above it reads as two different shells being active at once.
func TestCurrentShellMatchesItsRowInTheMenu(t *testing.T) {
	a := &App{}
	current := a.CurrentShell()
	for _, option := range a.Shells() {
		if option.Setting == current.Setting && option.Label != current.Label {
			t.Errorf("menu shows %q for %s, chip shows %q", option.Label, option.Setting, current.Label)
		}
	}
}
