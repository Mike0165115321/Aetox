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
	"strings"
	"sync"

	"github.com/Mikedev115/Aetox/internal/capability"
	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/debuglog"
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

	// The editor's download and its MCP entry are two halves of one press —
	// see connectVideoEditor for the whole argument. Failing to write the
	// entry does not fail the download that succeeded: the shelf preset in
	// Settings is still there, and the agent's card still says what is missing.
	for _, c := range missing {
		if c.ID == "kinocut" {
			if err := a.connectVideoEditor(); err != nil {
				debuglog.Msg("capabilities: connect video editor: %v", err)
			}
			break
		}
	}

	a.emitEvent("capabilities:done", capabilityDone{OK: true})
}

// ToolPart is one program inside a capability, as the install report lists it.
type ToolPart struct {
	ID string `json:"id"`
	// Title is the name and version to print; Includes is what else rides in
	// the same archive. Both fall back to nothing rather than to the id: an
	// install report that says "kinocut" where it means "kinocut 1.15.0" is
	// answering a narrower question than it was asked.
	Title       string `json:"title,omitempty"`
	Includes    string `json:"includes,omitempty"`
	License     string `json:"license,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	ApproxBytes int64  `json:"approxBytes"`
}

// ToolInstallPlan is what a person is shown before they press install: what
// arrives, whose it is, how big, and where it lands.
//
// It exists because the agent lock says "install this or you cannot use me",
// and a button that says that owes an answer to "install what, exactly". The
// four assurances beside it on screen are not written here — they are properties
// of internal/capability that hold for every entry (checksum before unpack, no
// elevation, no PATH, one folder to delete), so they belong in the locale files
// as one sentence each rather than being repeated per tool.
type ToolInstallPlan struct {
	// Capability is the id asked about, echoed back so a late answer cannot be
	// drawn against the wrong card.
	Capability string `json:"capability"`
	// Parts is what is still missing. Empty means nothing to install, which is
	// a real answer: the capability is already there, or this build offers it
	// not at all (an unpinned entry, or a platform with no manifest).
	Parts []ToolPart `json:"parts"`
	// TotalBytes is the sum of what is still missing, never of the whole
	// capability — quoting the full size of something half present overcharges
	// for a download that will not happen.
	TotalBytes int64 `json:"totalBytes"`
	// Dest is the folder the user can delete to undo all of it, absolute and
	// theirs to read.
	Dest string `json:"dest"`
}

// ToolInstallPlan answers the report the lock owes before it is pressed.
func (a *App) ToolInstallPlan(name string) ToolInstallPlan {
	plan := ToolInstallPlan{Capability: name, Parts: []ToolPart{}} // never nil: §34
	if root, err := config.DataRoot(); err == nil {
		plan.Dest = root
	}
	for _, c := range capability.MissingFor([]string{name}) {
		plan.Parts = append(plan.Parts, ToolPart{
			ID:          c.ID,
			Title:       c.Title,
			Includes:    c.Includes,
			License:     c.License,
			Homepage:    c.Homepage,
			ApproxBytes: c.ApproxBytes,
		})
		plan.TotalBytes += c.ApproxBytes
	}
	return plan
}

// CapabilityForServer maps an MCP server an agent declares in `needs:` to the
// capability that installs it, or "" when Aetox installs nothing for it.
//
// One row today, and the table is the point rather than the length: the agent
// says what it needs, internal/capability says what can be fetched, and neither
// of them can be asked "does pressing install on this agent download anything".
// Answering that anywhere else would mean the lock on a card guessing, and a
// lock that offers to install a service it cannot install is worse than one
// that sends the user to the connections page.
func (a *App) CapabilityForServer(server string) string {
	switch strings.ToLower(strings.TrimSpace(server)) {
	case VideoEditorServer:
		// The download that actually contains the server. It answered "video"
		// (the shared ffmpeg) until the kinocut bundle existed, because that
		// was the only thing this app could fetch for it — which meant the
		// lock on the editor's card offered a 90MB download that did not
		// contain the editor. Installing this one also writes the entry and
		// the placement (connectVideoEditor), so the press finishes what it
		// says; ffmpeg then surfaces through the gate's own "incomplete" turn,
		// named for what it is.
		return VideoEditCapability
	}
	return ""
}
