package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// Which shell a project's commands run in is a fact about the project, so the
// lookup has to survive the things a project folder's spelling actually does on
// Windows: arrive capitalised differently, arrive with a trailing separator,
// arrive from a different caller than the one that saved it. A setting that
// quietly stops applying is worse than one that was never made, because the
// commands keep running — somewhere else.

func TestShellForPrefersTheProjectsOwnChoice(t *testing.T) {
	root := `D:\GitHub\proj`
	var pref ModelPreference
	pref.SetShellFor(root, proc.WSL("mikedev"))
	pref.SetShellFor("", proc.Native())

	if got := proc.FormatBackend(pref.ShellFor(root)); got != "wsl:mikedev" {
		t.Errorf("ShellFor(%q) = %q, want wsl:mikedev", root, got)
	}
	if got := proc.FormatBackend(pref.ShellFor(`D:\GitHub\other`)); got != "native" {
		t.Errorf("an unchosen folder = %q, want the default native", got)
	}
}

func TestShellForMatchesTheFolderHoweverItIsSpelled(t *testing.T) {
	var pref ModelPreference
	pref.SetShellFor(`D:\GitHub\proj`, proc.WSL("mikedev"))

	for _, spelling := range []string{
		`d:\github\proj`,
		`D:\GitHub\proj\`,
		`D:\GitHub\sub\..\proj`,
	} {
		if got := proc.FormatBackend(pref.ShellFor(spelling)); got != "wsl:mikedev" {
			t.Errorf("ShellFor(%q) = %q, want wsl:mikedev — the setting stopped applying to its own folder", spelling, got)
		}
	}
}

// One folder, one answer. Saving under a second spelling must replace the first
// rather than sit beside it, or the map holds two answers and which one wins is
// map iteration order.
func TestSetShellForReplacesAnEarlierSpellingOfTheSameFolder(t *testing.T) {
	var pref ModelPreference
	pref.SetShellFor(`D:\GitHub\proj`, proc.WSL("mikedev"))
	pref.SetShellFor(`d:\github\proj\`, proc.Native())

	if len(pref.Shells) != 1 {
		t.Errorf("Shells holds %d entries for one folder: %v", len(pref.Shells), pref.Shells)
	}
	if got := proc.FormatBackend(pref.ShellFor(`D:\GitHub\proj`)); got != "native" {
		t.Errorf("ShellFor after re-picking = %q, want native", got)
	}
}

// "Use WSL everywhere" is said once, under the "" key, and applies to every
// folder that has not been chosen for.
func TestShellForFallsBackToTheGlobalChoice(t *testing.T) {
	var pref ModelPreference
	pref.SetShellFor("", proc.WSL("mikedev"))

	if got := proc.FormatBackend(pref.ShellFor(`D:\GitHub\anything`)); got != "wsl:mikedev" {
		t.Errorf("ShellFor with only a global choice = %q, want wsl:mikedev", got)
	}
}

// A project that lives inside a distro is built by that distro's toolchain, and
// the native shell can read its files without being able to run any of it.
// Nobody should have to discover the picker to get that right.
func TestShellForDefaultsToTheDistroAProjectLivesIn(t *testing.T) {
	var pref ModelPreference

	for _, root := range []string{
		`\\wsl.localhost\mikedev\home\mike\proj`,
		`\\wsl$\mikedev\home\mike\proj`,
	} {
		if got := proc.FormatBackend(pref.ShellFor(root)); got != "wsl:mikedev" {
			t.Errorf("ShellFor(%q) = %q, want wsl:mikedev", root, got)
		}
	}
	if got := proc.FormatBackend(pref.ShellFor(`D:\GitHub\proj`)); got != "native" {
		t.Errorf("a project on a Windows drive defaulted to %q, want native", got)
	}
}

// An explicit choice outranks the folder's own implication: someone who keeps a
// project under \\wsl.localhost but builds it with the Windows toolchain has
// said so, and the guess must not keep overriding them.
func TestAnExplicitChoiceBeatsTheFolderDefault(t *testing.T) {
	root := `\\wsl.localhost\mikedev\home\mike\proj`
	var pref ModelPreference
	pref.SetShellFor(root, proc.Native())

	if got := proc.FormatBackend(pref.ShellFor(root)); got != "native" {
		t.Errorf("ShellFor(%q) = %q, want the native shell the user picked", root, got)
	}
}

// "The default distro" is an instruction, not an answer: stored as-is it would
// follow the user's default around and one day run the project's commands in a
// distro it was never tested in. Skipped when the machine has no WSL, where
// there is no name to resolve to.
func TestSetShellForStoresAResolvedDistroName(t *testing.T) {
	if len(proc.Distros()) == 0 {
		t.Skip("no WSL distro on this machine")
	}
	var pref ModelPreference
	pref.SetShellFor(`D:\GitHub\proj`, proc.ParseBackend("wsl"))

	stored := pref.Shells[filepath.Clean(`D:\GitHub\proj`)]
	if !strings.HasPrefix(stored, "wsl:") {
		t.Errorf("stored %q, want a wsl:<name> with the distro resolved", stored)
	}
}
