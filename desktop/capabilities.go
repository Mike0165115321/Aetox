package main

// The window's end of internal/capability: what the agent can read today, and
// a way to go and fetch the rest.
//
// Install returns immediately rather than blocking until 150MB has arrived.
// That is the whole point of where this sits in the first-run flow — pressing
// the button lands the user in the app, and the download reports itself from
// the background strip rather than holding the last onboarding screen hostage
// to a progress bar (docs/architecture/capability-install-2026-08-21.md).

import (
	"context"
	"sync"

	"github.com/Mike0165115321/Aetox/internal/capability"
)

// capabilityProgress is one update on the wire to the window. Percent is -1
// when the server sent no Content-Length, which the strip should draw as
// "working" rather than as a bar stuck at zero.
type capabilityProgress struct {
	ID      string `json:"id"`
	Index   int    `json:"index"`
	Of      int    `json:"of"`
	Percent int    `json:"percent"`
}

// capabilityDone is the final word, success or not.
type capabilityDone struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// capabilityInstall is the one install allowed to be in flight. Without it,
// two presses of the same button race two downloads into the same directory,
// and RemoveAll from one lands in the middle of the other's unpack.
type capabilityInstall struct {
	mu      sync.Mutex
	running bool
}

// CapabilityStatuses lists every capability Aetox can install and whether it
// is usable right now. Empty on any platform without a manifest, which every
// caller should read as "nothing to offer" rather than "nothing installed".
func (a *App) CapabilityStatuses() []capability.Status {
	return capability.Statuses()
}

// CapabilitiesInstalling reports whether a download is already in flight, so a
// window reopened mid-install draws the strip instead of the offer.
func (a *App) CapabilitiesInstalling() bool {
	a.capabilities.mu.Lock()
	defer a.capabilities.mu.Unlock()
	return a.capabilities.running
}

// InstallCapabilities starts fetching the missing parts of the capabilities
// named, and returns at once. Progress arrives as "capabilities:progress"
// events and the outcome as "capabilities:done".
//
// The window sends capability names ("pdf", "speech"), never component ids:
// which downloads stand behind one tick is this side's business, and speech
// needing two of them is exactly the kind of detail a screen should not have
// to know. An empty list installs nothing, which is what an untouched screen
// asks for.
//
// Returns true if this call started the work. False means nothing to do, or
// one already running — the second press of a button should be a no-op, not a
// second download.
func (a *App) InstallCapabilities(capabilities []string) bool {
	if capabilities == nil {
		capabilities = []string{}
	}
	missing := capability.MissingFor(capabilities)
	if len(missing) == 0 {
		a.emitEvent("capabilities:done", capabilityDone{OK: true})
		return false
	}

	a.capabilities.mu.Lock()
	if a.capabilities.running {
		a.capabilities.mu.Unlock()
		return false
	}
	a.capabilities.running = true
	a.capabilities.mu.Unlock()

	go a.runCapabilityInstall(missing)
	return true
}

func (a *App) runCapabilityInstall(missing []capability.Component) {
	defer func() {
		a.capabilities.mu.Lock()
		a.capabilities.running = false
		a.capabilities.mu.Unlock()
	}()

	// No deadline on the whole run and none per component. The manifest is
	// ~150MB and the people this is for are not all on fast connections; a
	// timeout generous enough to be fair to them is not a timeout. What ends a
	// stalled download instead is the process exiting, and what the user has
	// meanwhile is an app that works for everything except the tool they have
	// not installed yet.
	ctx := context.Background()

	// Every chunk read would otherwise be an event. At ~32KB per read that is
	// thousands of messages a second into a bar with about 200 pixels to say
	// it with, so the wire carries whole percent changes only.
	last := map[string]int{}
	err := capability.Install(ctx, missing, func(p capability.Progress) {
		percent := p.Percent()
		if seen, ok := last[p.ID]; ok && seen == percent {
			return
		}
		last[p.ID] = percent
		a.emitEvent("capabilities:progress", capabilityProgress{
			ID:      p.ID,
			Index:   p.Index,
			Of:      p.Of,
			Percent: percent,
		})
	})

	if err != nil {
		a.emitEvent("capabilities:done", capabilityDone{OK: false, Error: err.Error()})
		return
	}
	a.emitEvent("capabilities:done", capabilityDone{OK: true})
}
