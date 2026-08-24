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
	//
	// The closing paragraph is here for the reason `wait`'s trigger sits on
	// `read`, and it is the same failure: a model that never opens a second tab
	// never calls `tabs`, so the tabs guidance — which has said "close what you
	// are done with" all along — arrives for nobody. In a real run the model
	// gathered four sources by replacing the page four times and closed nothing,
	// because reuse is the default (§130) and nothing on the path it took ever
	// mentioned the alternative. The trigger has to ride on `open`, which every
	// session that touches the browser calls.
	"open": "newTab=true keeps the page you are on and opens an extra one. The question is whether you are still going to need this page: gathering several sources to write from, or holding two side by side to compare, means keep them — open each in its own tab. A page you have already taken what you need from is finished, so let the next open replace it, and use `back` if you were wrong: re-opening a URL is not the same as going back, because a form you filled and a scroll position do not survive it.\n" +
		"Tabs you keep are windows on the user's screen. Close each one with `tabs` (act: close) as soon as the work that needed it is done — finishing a task with five of them still open leaves five for somebody else to close.\n" +
		"A source file (.go, .ts, .py) is a download rather than a page; `read` it instead. A video link is words rather than a page: `web_fetch` reads its captions, which is what you want when the question is what was SAID in it — open it here only when the user should watch it.",

	// Says WHEN, because the failure it prevents does not look like a failure.
	"wait": "Most pages fetch their real content after the document loads. A `read` straight after `open` or `click` therefore SUCCEEDS and comes back empty, and there is nothing in that answer to suggest waiting — so the honest reading of it is \"there are no results\" when the truth is \"not yet\".\n" +
		"`wait` is the only action that can tell those two apart. Use it whenever you expect something that is not there yet. If it times out, that is a report and not an error: the page may still be loading, or it may genuinely never say that. Read the page before deciding which.",

	// Says when, and names the shape of page that qualifies rather than listing
	// cases: a model holding a camera and no rule for it photographs everything.
	"capture": "Use this only when `read` cannot answer, because the answer was never in the text: a chart, a canvas, a map, a rendered document, or a layout you suspect is wrong. Read first, photograph second — a picture costs far more than the read it would have duplicated.\n" +
		"By default it sees what is on screen. `full=true` photographs the whole document instead, which is what you want for a long form or a report and is wasted on a page that already fits — it is the same answer in several times the bytes. If a page is too long even for that, the result says where it was cut; nothing else about the picture will tell you.\n" +
		"The file is kept under output/<session> so the user can open it too.",

	// Says WHEN, for the same reason `wait` does: the failure it prevents does
	// not look like a failure. A feed read without scrolling comes back short
	// and successful, and nothing in that answer suggests there was more.
	"scroll": "A page that loads as you go — a feed, a result list, a channel — is one screen deep in the document until something scrolls it. So a short `read` on a page you expected to be long is usually not the whole page: scroll, read again, and repeat until the reading stops growing.\n" +
		"`to` is down, up, top or bottom. ALWAYS send `screens` with down and up: it is how far to travel in this one call, 1 to 10, and deciding it before you call is the whole point. Ten screens of a feed is ten calls at one screen each or one call at ten — same page, ten times the round trip. Send 1 when you mean to read what comes next; send a bigger number when you are reaching for content that has not loaded yet.\n" +
		"It presses once per screen with a pause between, so content that arrives as you go still arrives — a single jump of the same distance would skip it. A page shorter than what you asked for stops at its end rather than failing, so a read after a long scroll that looks short may mean you were already at the bottom. Every scroll invalidates your refs, exactly like a navigation: read again before you click.",

	"back": "Returns to the previous page in this tab with what you had typed and scrolled still there. Re-opening its URL is a different thing and loses all of it, and a page that came from a POST cannot be re-opened at all.\n" +
		"A tab with nothing behind it does not fail — it says there is nowhere back to.",

	// The one action whose default is a safety position, so the reason for the
	// default travels with it.
	"dialog": "alert/confirm/prompt never block this browser: the page is answered immediately and told what it said. But the answer is CANCEL unless you set accept=true first, and that default is deliberate — a confirm() sitting in front of a deletion is the commonest kind there is, and agreeing to one on the user's behalf is not something a browser tool should do by itself.\n" +
		"So set accept=true BEFORE the click that raises the dialog you mean to agree to, and know that it resets on every new page.",

	"tabs": "Every tab here is one you opened. The user has their own and you cannot reach them, which is why they are not listed — if they want you on a page they have open, they will hand it over.\n" +
		"`select` changes which tab every other action works, and it invalidates your refs exactly like a navigation does: read again before you click.\n" +
		"Close what you are done with. A tab you keep is a window on somebody's screen — `close` with no id shuts the one you are working, which is nearly always the one you mean.",

	// The three below carry the ref rule, which used to sit in the block where
	// every message paid for it. It lives with the actions that SPEND refs, so a
	// session that only opens a page never hears it and never needs to.
	// `read` carries the trigger for `wait`, and that is not tidiness — it is
	// the one place guidance-on-first-use does not work.
	//
	// Everything else here is advice about an action you already reached for, so
	// arriving with its first result is early enough. "Reach for `wait`" is the
	// opposite: the failure it prevents is never calling `wait` at all, and
	// guidance attached to the first `wait` arrives only for a model that
	// already knew. The trigger has to ride on an action that gets used
	// constantly, and reading an empty page is exactly the moment it matters.
	"read": "The [n] refs this hands back belong to THIS page as it is now, and to THIS read: every read renumbers them from 1 and strips the ones before it, so a ref from an earlier read is dead even on a page that never changed. They also go stale the moment the page changes or you select another tab, so the loop is read, act, read again — never read once and work from the list.\n" +
		"If a page comes back with less on it than you expected — no results, an empty list, a shell with nothing in it — that is usually not the answer. Most pages fetch their real content after loading, and this read SUCCEEDED on a page that is not finished. Use `wait` for the text you expect, then read again, before you report that something is not there.\n" +
		// The counterweight to the paragraph above, and it has to travel with
		// it. That one teaches "less than you expected means wait", which is
		// right for a page still loading and wrong for the two cases below —
		// and a model holding only the first rule waits out a limit that
		// waiting cannot move.
		"Two things a read says about itself, and both mean something other than \"wait\". The element list stops at 150 and then tells you how many more there were: reach those with `filter`, which lists only the elements whose text contains what you name — re-reading returns the same first 150. And a frame from another site cannot be read from here at all, ever; when the read says so, what is inside it is not coming, and the way to it is `open` on its own URL if you have one.",

	// Both of these say what they CANNOT see, and say it first. A tool that
	// reports on absence has to, because "nothing here" is its most common
	// answer and its most misreadable one: an empty console is evidence only if
	// the reader knows what the recorder was listening to.
	"console": "Reach for this when a page does not do what it should and `read` cannot say why: a blank screen, a button that does nothing, a form that will not submit. It carries console.log/warn/error, uncaught exceptions, unhandled promise rejections, and resources the page's own CSP blocked — the last three of which a page never logs for itself.\n" +
		"It starts when the document is created, so it covers a page you opened, and a page that was already open before this build has no recorder at all. When the answer says the recorder is not running, that is NOT a report that nothing went wrong: reload the page and ask again.",

	"network": "The fetch and XMLHttpRequest calls the page's own code made, oldest first, with status and duration. Use it when a page renders but its data does not: this is what tells a 401 from a 500 from a request that was never made at all.\n" +
		"Three limits worth knowing before you draw a conclusion. Images, scripts and stylesheets are NOT here — those are the browser's own fetches, not the page's, so their absence from this list says nothing about whether they loaded. A status shown as `-` means the request never came back. And anything in a query string that looks like a credential arrives as <redacted>, which is the tool hiding it and not the page sending it that way.",

	"click": "A ref belongs to the page it was read from AND to the read that produced it: the next read renumbers everything, and a filtered read tags only its own matches, so a short list carries ref 1 upward whatever the full page holds. It also goes stale the moment the page changes, which a click often does. Read, act, read again.\n" +
		"A click can navigate, raise a dialog, or do nothing visible; read afterwards rather than assuming which.",

	"type": "A ref belongs to the page it was read from and to the read that produced it: a later read, filtered or not, renumbers them all. If anything has read this page since, read it again before typing into a number that may now mean something else.\n" +
		"For a select element the text must match one of the options `read` listed. enter=true submits, which is how a search box with no button is used.",
}
