package main

import (
	"strings"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// Which shell this workspace's commands run in.
//
// The choice is per project folder and lives in the preference file
// (config.ModelPreference.Shells, resolved by config.ShellChoice), so it
// survives a restart and so a person who works on a Windows repo and a repo
// inside a distro does not re-pick every time they switch.
//
// What the UI shows is a chip beside the composer rather than a row on the
// Settings page: the shell decides what a command *means* — the same line is a
// different program to cmd and to bash — and the moment that matters is when
// the user is about to approve one. A setting you have to go looking for is one
// you find out about by having a command fail.

// shellBackend is what the engine is handed. A func, not a value: the user can
// change the shell between two calls of the same session, and the tool
// description the model reads has to change with it.
func (a *App) shellBackend() proc.Backend {
	return a.shells.For(a.cur().cfg.SandboxRoot)
}

// ShellOption is one entry in the picker.
type ShellOption struct {
	// Setting is the stored spelling, and what SetShell takes back.
	Setting string `json:"setting"`
	// Label is what the chip and the menu show — the machine, not the shell
	// binary (proc.Label, and the reason the two differ).
	Label string `json:"label"`
	// Selected marks the one in use for the focused project.
	Selected bool `json:"selected"`
}

// Shells lists what this machine can offer, best first.
//
// The native shell is always present. Distros are whatever wsl.exe reports,
// minus the ones that exist to serve Docker — so on a machine with no WSL this
// returns exactly one entry, which is how the UI knows there is no choice here
// to put in front of anyone.
func (a *App) Shells() []ShellOption {
	current := proc.FormatBackend(a.shellBackend())
	native := proc.Native()
	options := []ShellOption{{
		Setting: proc.FormatBackend(native),
		Label:   proc.Label(native),
	}}
	for _, distro := range proc.Distros() {
		backend := proc.WSL(distro)
		options = append(options, ShellOption{
			Setting: proc.FormatBackend(backend),
			Label:   proc.Label(backend),
		})
	}
	for i := range options {
		options[i].Selected = strings.EqualFold(options[i].Setting, current)
	}
	return options
}

// CurrentShell is what the chip shows without opening the menu.
func (a *App) CurrentShell() ShellOption {
	backend := a.shellBackend()
	return ShellOption{
		Setting:  proc.FormatBackend(backend),
		Label:    proc.Label(backend),
		Selected: true,
	}
}

// SetShell records the choice for the focused project. It takes effect on the
// next command and needs no re-bootstrap, because nothing else about the engine
// changes.
//
// Not applied to a command already running: that one was guarded as one shell's
// syntax, and moving it under a different shell would mean the check that let
// it through described a different program than the one executing. The next
// call picks it up, which is a wait of seconds and the difference between a
// gate and a suggestion.
func (a *App) SetShell(setting string) error {
	return a.shells.Set(strings.TrimSpace(a.cur().cfg.SandboxRoot), setting)
}
