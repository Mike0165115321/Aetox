package main

// The console and network recorders, and the one thing they must never do:
// present an absence of evidence as evidence of absence.
//
// Both actions answer "nothing here" more often than anything else, and that
// sentence is only worth something if the reader knows what was being listened
// to. Everything below is about the sentences around the list, not the list.

import (
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

func TestFormatBrowserLogRefusesToCallSilenceAnAnswer(t *testing.T) {
	got := formatBrowserLog("the page", browserLogReport{Kind: "console", Armed: false})

	if !strings.Contains(got, "no report at all") {
		t.Errorf("an unarmed recorder must say it has no report, got:\n%s", got)
	}
	// The specific misreading: a page whose recorder never ran looks exactly
	// like a page with a clean console, and a model that reports "no errors"
	// off this has reported on nothing.
	if !strings.Contains(got, "Reload") {
		t.Errorf("an unarmed recorder must name the way to arm it, got:\n%s", got)
	}
}

func TestFormatBrowserLogEmptyAnswersNameTheirOwnScope(t *testing.T) {
	network := formatBrowserLog("the page", browserLogReport{Kind: "network", Armed: true})
	// An empty network list is true of the page's own code and says nothing
	// about the images and scripts the browser fetched for it. Without that
	// line, "no requests" reads as "nothing loaded".
	if !strings.Contains(network, "Images, scripts and stylesheets") {
		t.Errorf("an empty network list must say what it never counted, got:\n%s", network)
	}

	console := formatBrowserLog("the page", browserLogReport{Kind: "console", Armed: true})
	if !strings.Contains(console, "since it loaded") {
		t.Errorf("an empty console must bound its claim to this document, got:\n%s", console)
	}
}

func TestFormatBrowserLogListsConsoleEntries(t *testing.T) {
	rep := browserLogReport{
		Kind:  "console",
		Armed: true,
		Entries: []browserLogEntry{
			{Level: "warn", Text: "deprecated"},
			{Level: "error", Text: "Uncaught TypeError: x is not a function (app.js:12)"},
		},
		Dropped: 40,
	}
	got := formatBrowserLog("the page", rep)

	for _, want := range []string{"[warn] deprecated", "[error] Uncaught TypeError", "40 earlier entries were dropped"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatBrowserLogListsRequestsAndMarksTheOnesThatNeverAnswered(t *testing.T) {
	rep := browserLogReport{
		Kind:  "network",
		Armed: true,
		Entries: []browserLogEntry{
			{Method: "GET", URL: "https://api.example.com/items", Status: 401, MS: 88},
			{Method: "POST", URL: "https://api.example.com/save", Status: 0, MS: 3000, Error: "no response"},
		},
	}
	got := formatBrowserLog("the page", rep)

	if !strings.Contains(got, "GET 401 88ms https://api.example.com/items") {
		t.Errorf("a request line must carry status and duration, got:\n%s", got)
	}
	// 0 is a status the page never received, and printing it as "0" would put a
	// number where there was no answer at all.
	if !strings.Contains(got, "POST - 3000ms") {
		t.Errorf("a request that never came back must not be shown as status 0, got:\n%s", got)
	}
	if !strings.Contains(got, "redacted") {
		t.Errorf("the network list must state that credentials are stripped, got:\n%s", got)
	}
}

func TestLogScriptWatchesWhatAPageNeverLogsForItself(t *testing.T) {
	js := logScript()

	// console.log is the easy half. These three are the reason the tool is
	// worth having: a page that throws does not log its own exception.
	for _, want := range []string{"'error'", "unhandledrejection", "securitypolicyviolation"} {
		if !strings.Contains(js, want) {
			t.Errorf("the console recorder must catch %s, got:\n%s", want, js)
		}
	}
	for _, want := range []string{"window.fetch", "XMLHttpRequest.prototype.open", "XMLHttpRequest.prototype.send"} {
		if !strings.Contains(js, want) {
			t.Errorf("the network recorder must wrap %s, got:\n%s", want, js)
		}
	}
}

func TestLogScriptNeverTakesHeadersOrBodiesAndStripsURLSecrets(t *testing.T) {
	js := logScript()

	// The redaction is the mitigation for the one field that can carry a
	// secret. If this pattern is ever weakened, the tool starts handing tokens
	// to the model out of ordinary query strings.
	for _, want := range []string{"token", "secret", "password", "credential", "<redacted>"} {
		if !strings.Contains(js, want) {
			t.Errorf("URL redaction must cover %q, got:\n%s", want, js)
		}
	}
	// Nothing reads a header or a body, which is why there is no rule about
	// them anywhere else. This is the test that keeps it that way.
	for _, forbidden := range []string{".headers", "requestBody", "responseText", ".body"} {
		if strings.Contains(js, forbidden) {
			t.Errorf("the recorder must not read %s — headers and bodies are deliberately never captured", forbidden)
		}
	}
}

func TestReadLogScriptReportsWhetherItWasArmed(t *testing.T) {
	js := readLogScript("tok", "console")
	if !strings.Contains(js, "armed") {
		t.Errorf("the read must carry whether the recorder existed, got:\n%s", js)
	}
	if !strings.Contains(js, `var kind="console"`) {
		t.Errorf("the kind must be embedded as a quoted literal, got:\n%s", js)
	}
}

func TestConsoleAndNetworkAreSeparateRights(t *testing.T) {
	perms := map[string]string{}
	for _, call := range skill.PackedCalls("browser") {
		perms[call.Action] = call.Permission
	}
	// One tool on the outside, the old browser_<action> names still the
	// vocabulary of permission on the inside. A profile granted "may see this
	// page's errors" must not thereby be granted the map of every service the
	// page calls.
	if perms["console"] != "browser_console" || perms["network"] != "browser_network" {
		t.Errorf("console and network must be grantable separately, got %v", perms)
	}
}

func TestFullCaptureReachesBelowTheFold(t *testing.T) {
	params := fullCaptureParams(1280, 4000)
	// Without this flag the clip is still measured against the viewport and
	// everything below the fold comes back blank — which looks like a broken
	// engine rather than a missing option.
	if !strings.Contains(params, `"captureBeyondViewport":true`) {
		t.Errorf("a full-page capture must address the document, got: %s", params)
	}
	if !strings.Contains(params, `"height":4000`) {
		t.Errorf("the clip must carry the document's real height, got: %s", params)
	}
}
