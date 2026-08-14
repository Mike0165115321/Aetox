package main

// The half of self-update the user should never have to remember: running the
// check. Everything downstream of "a newer Aetox exists" already worked —
// internal/update.Apply downloads the release, verifies its signature, swaps
// the exe and restarts into it — but nothing ever asked the question unless the
// user walked to Settings → About and pressed a button. A one-click update
// nobody knows to look for is an update nobody gets, which is why the app kept
// being reinstalled by hand.
//
// So: one check shortly after the window settles, then once a day for as long
// as the app stays open, and the answer is pushed to the frontend as an event
// it can put in front of the user.
//
// Three rules keep a check nobody asked for from turning into a nuisance:
//
//   - Silent unless there is genuinely something newer. A failed automatic
//     check says nothing at all — the user did not pose the question, so a red
//     banner about GitHub being unreachable is an answer to nothing. The
//     explicit button in Settings still reports its failures; that one WAS
//     asked for, and silence there would look like a dead click.
//   - It never downloads anything. The notice offers, the user decides, and
//     bytes only move after a click. An app that quietly pulls 40 MB on someone
//     else's tethered connection has made a decision that was not its to make.
//   - AETOX_DISABLE_UPDATE_CHECK switches the whole thing off, exactly as it
//     already switched off the manual check.

import (
	"context"
	"time"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/update"
	"github.com/Mike0165115321/Aetox/internal/version"
)

const (
	// Late enough that the check never competes with first paint or the engine
	// bootstrap for the network — the update is not urgent, the window is.
	updateFirstCheckDelay = 20 * time.Second

	// Releases do not happen hourly. A daily question is the most this can be
	// worth asking, and GitHub's ETag makes every repeat a 304 with no body
	// anyway (see internal/update's cache), so the cost is a rounding error
	// either way.
	updateCheckEvery = 24 * time.Hour
)

// updateCheck is the seam this file's tests replace. Nothing else assigns to it.
var updateCheck = update.Check

// watchForUpdates asks once, then daily, for the life of the process. Started
// from startup as a goroutine and never stopped: it outlives nothing, since the
// process exiting is what ends it.
//
// The startup check is deliberately unconditional rather than skipped when
// internal/update's cache says "asked recently". A cached answer still has to
// reach a window that has only just opened — the previous run's check may have
// found a release the user never saw because they closed the app — and the
// conditional request that answers it costs a 304.
func (a *App) watchForUpdates() {
	if update.Disabled() {
		return
	}
	time.Sleep(updateFirstCheckDelay)
	for {
		a.announceUpdate()
		time.Sleep(updateCheckEvery)
	}
}

// announceUpdate runs one check and tells the frontend only if there is
// something to tell. The whole Status rides along: the notice has to offer the
// right action per channel (one click for portable/installer, the Scoop command
// for Scoop, the release page for anything else), and that decision is already
// made — and tested — inside internal/update. Re-deriving it in the UI would be
// a second place answering the same question.
func (a *App) announceUpdate() {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	st, err := updateCheck(ctx, version.Current)
	if err != nil {
		// Offline, rate-limited, switched off: all of it belongs in the log,
		// none of it in the window. Nobody asked.
		debuglog.Msg("automatic update check: %v", err)
		return
	}
	if !st.Available {
		return
	}
	a.emitEvent("update:available", st)
}
