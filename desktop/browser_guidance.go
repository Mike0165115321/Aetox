package main

// What the browser tool used to say on every message, and now says once.
//
// Every paragraph below was in the tool block until 2026-08-18, re-sent with
// each request for the life of the conversation. None of it was wrong — most of
// it was written the day before, and it is the reason `wait` gets used at the
// right moment and `capture` does not get used at every moment. What was wrong
// is when it travelled: the model needs each of these a single time.
//
// Keyed per action, so a session that opens a page and reads it is told those
// two things and not the other seven. Measured on this machine, sessions that
// touch the browser at all almost always use `open` and `read`, sometimes
// `click`, and nothing else — so most of what is below never ships in most
// sessions that use the browser.
//
// What deliberately did NOT move here, and stayed in the block:
//
//   - How refs work and whose tabs are whose. Shared by every action, so keying
//     it per action would send it again and again.
//   - Never type a password into a page. A safety rule, and guidance rides in
//     the message stream where a summarised conversation can lose it silently.
//     That is a fine price for "read before you photograph" and not for this.
//
// See internal/skill/guidance.go for the standard this follows.

import "strings"

func (s *browserSkill) Guidance(args map[string]any) string {
	action := strings.ToLower(strings.TrimSpace(str(args["action"])))
	return browserGuidance[action]
}

var browserGuidance = map[string]string{
	// The condition that generalizes is not "is this page important" — a model
	// cannot answer that about a page it has not read. It is "is the page I am
	// on still an input to my work", which is a question about its own task.
	"open": "newTab=true keeps the page you are on and opens an extra one. Use it only when you will come back here — a list of results you are working through, a page you are comparing against. Otherwise let it replace the page, and use `back` if you were wrong: re-opening a URL is not the same as going back, because a form you filled and a scroll position do not survive it.\n" +
		"A source file (.go, .ts, .py) is a download rather than a page; `read` it instead.",

	// Says WHEN, because the failure it prevents does not look like a failure.
	"wait": "Most pages fetch their real content after the document loads. A `read` straight after `open` or `click` therefore SUCCEEDS and comes back empty, and there is nothing in that answer to suggest waiting — so the honest reading of it is \"there are no results\" when the truth is \"not yet\".\n" +
		"`wait` is the only action that can tell those two apart. Use it whenever you expect something that is not there yet. If it times out, that is a report and not an error: the page may still be loading, or it may genuinely never say that. Read the page before deciding which.",

	// Says when, and names the shape of page that qualifies rather than listing
	// cases: a model holding a camera and no rule for it photographs everything.
	"capture": "Use this only when `read` cannot answer, because the answer was never in the text: a chart, a canvas, a map, a rendered document, or a layout you suspect is wrong. Read first, photograph second — a picture costs far more than the read it would have duplicated.\n" +
		"It sees what is on screen, not the whole page. The file is kept under output/<session> so the user can open it too.",

	"back": "Returns to the previous page in this tab with what you had typed and scrolled still there. Re-opening its URL is a different thing and loses all of it, and a page that came from a POST cannot be re-opened at all.\n" +
		"A tab with nothing behind it does not fail — it says there is nowhere back to.",

	// The one action whose default is a safety position, so the reason for the
	// default travels with it.
	"dialog": "alert/confirm/prompt never block this browser: the page is answered immediately and told what it said. But the answer is CANCEL unless you set accept=true first, and that default is deliberate — a confirm() sitting in front of a deletion is the commonest kind there is, and agreeing to one on the user's behalf is not something a browser tool should do by itself.\n" +
		"So set accept=true BEFORE the click that raises the dialog you mean to agree to, and know that it resets on every new page.",

	"tabs": "Every tab here is one you opened. The user has their own and you cannot reach them, which is why they are not listed — if they want you on a page they have open, they will hand it over.\n" +
		"`select` changes which tab every other action works, and it invalidates your refs exactly like a navigation does: read again before you click.\n" +
		"Close what you are done with. A tab you keep is a window on somebody's screen.",

	// The three below carry the ref rule, which used to sit in the block where
	// every message paid for it. It lives with the actions that SPEND refs, so a
	// session that only opens a page never hears it and never needs to.
	"read": "The [n] refs this hands back belong to THIS page as it is now. They go stale the moment it changes or you select another tab, so the loop is read, act, read again — never read once and work from the list.",

	"click": "A ref belongs to the page it was read from and goes stale the moment that page changes, which a click often does. Read, act, read again.\n" +
		"A click can navigate, raise a dialog, or do nothing visible; read afterwards rather than assuming which.",

	"type": "A ref belongs to the page it was read from. If the page has changed since, read it again before typing into a number that may now mean something else.\n" +
		"For a select element the text must match one of the options `read` listed. enter=true submits, which is how a search box with no button is used.",
}
