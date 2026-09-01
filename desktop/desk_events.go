package main

// The desk's one door on the Go side (§187.3).
//
// §187 was the leak — a background chat's file landing on whichever desk was
// on screen — and the fix was to stamp the session onto one event. What made
// the leak POSSIBLE was that every tool emitted its own `workbench:*` event
// with whatever payload it thought of, so the question "whose desk is this
// for" was asked nowhere. This file is where it is asked now, every time: a
// workbench event that does not pass through here does not compile against
// the gate test (desk_event_gate_test.go), so the next desk surface added to
// the app has to answer the ownership question on the day it is written, not
// after its first leak.

// deskEvent tells the window the agent did something to a desk.
//
// sessionID is whose desk — required, and "" is an ANSWER, not an omission:
// it means this surface has no per-session owner yet. Today that is the
// engine-log terminal alone (§187.2); the window draws an ownerless event on
// the desk on screen, which is the pre-§187 behaviour made explicit instead
// of accidental. The browser left the "" club on 1 ก.ย.: its three doors
// (open, tabs select, capture's raise) stamp the conversation that acted, and
// the window routes a background chat's page to that chat's own desk — the
// exact move this comment used to promise. close-browser stays "": the window
// finds the tab by id wherever it lives, so the close needs no owner.
func (a *App) deskEvent(sessionID, event string, payload map[string]string) {
	if payload == nil {
		payload = map[string]string{}
	}
	payload["sessionId"] = sessionID
	a.emitEvent("workbench:"+event, payload)
}

// deskFilesChanged is the second door, for the one workbench event whose
// payload is a list rather than strings: the file cards under a reply asking
// to be re-read. It was born session-stamped (sessionEvent) — the shape the
// rest of the desk is only now catching up to — and lives here so the gate
// holds one rule: workbench:* leaves this file or it does not leave.
func (a *App) deskFilesChanged(conv *conversation, paths []string) {
	a.emitEvent("workbench:files-changed", sessionEvent[[]string]{SessionID: conv.id, Data: paths})
}

// deskMediaOpened is the third door, for a file the editor produced that opens
// itself (video_desk.go).
//
// It carries a struct rather than strings for the same reason the one above
// does — the origin line under the player is a handful of numbers, not a path —
// and it is one event rather than two because opening the tab and saying where
// the file came from are one act. Two events would be two answers to "what just
// arrived", and the day they disagree the line would describe a different clip
// from the one on screen.
//
// Session-stamped like everything else here: an editor working in a background
// chat parks its clip on that chat's desk, and the user finds it there.
func (a *App) deskMediaOpened(conv *conversation, origin MediaOrigin) {
	a.emitEvent("workbench:open-media", sessionEvent[MediaOrigin]{SessionID: conv.id, Data: origin})
}
