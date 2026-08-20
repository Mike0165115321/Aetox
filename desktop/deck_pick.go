package main

// ชี้ให้เอเจนดู, pointed at a slide instead of at a web page.
//
// The browser tabs have had this since browser_pick.go: the user outlines
// something on the page and what lands in the composer is the node — its
// selector, its box, the colours it actually renders in. Asked for on the deck
// on 2026-08-20, and it is the same request with a better ending: a page is
// somebody else's, so the most the agent can do about it is talk. A deck is a
// file on this machine that the agent wrote, so pointing at a heading that is
// too big is one grep away from the rule that made it too big.
//
// Which is why the chip names the FILE rather than a URL (deckPickHead): the
// path is the whole difference between "here is what you are looking at" and
// "here is what to edit".
//
// Nothing new is drawn here. A deck renders in an <iframe> inside the app's own
// webview rather than in a native window beside it, so the overlay does not
// have to be injected through an engine bridge and its answer does not have to
// come back through one — the frame is same-origin, and it calls the parent.
// The script is still built here rather than in the frontend, because there is
// one overlay and this is where it lives.

// DeckPickScript is the overlay, addressed to a deck's iframe.
//
// token is minted by the caller and checked by the caller when the answer
// arrives, because the answer never passes through Go on this route. It buys
// the same thing it buys for a tab: a document that posts a pick nobody asked
// for is answering a question that was never put, and the frontend can tell.
//
// opts is the same JSON the browser toolbar builds — the accent off the live
// theme and the hint in the user's language, because the overlay is painted
// inside a document that can reach neither.
func (a *App) DeckPickScript(token, opts string) string {
	return pickScriptTo(token, opts, "window.parent.__aetoxDeckPick")
}

// DeckStopPickScript takes the overlay down: the button pressed again, the
// panel going away, or the deck being reloaded under a live mode.
//
// Handed over rather than reimplemented in the frontend for the same reason the
// script itself is: `window.__aetoxPick` is the overlay's own handle, and a
// second file that knows its name is a second file to fix when it changes.
func (a *App) DeckStopPickScript() string {
	return stopPickScript
}
