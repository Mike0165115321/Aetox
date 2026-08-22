package main

// What a read says about what it did NOT return.
//
// These cover the reporting rather than the reading, and that split is the
// reason formatBrowserRead exists as its own function: reading needs a live app
// window and a real webview (tool_coverage_test.go marks browser_read "never"
// available for exactly that reason), while what the model is told about the
// page is a pure function of one snapshot and can be pinned here.
//
// The failure they exist to stop: until 2026-08-22 a page with 400 buttons and
// a page with 150 produced the same output, so a model that could not find its
// control had nothing to distinguish "not on this page" from "past the cap" —
// and re-reading returned the same first 150 in the same DOM order forever.

import (
	"strings"
	"testing"
)

func TestFormatBrowserReadReportsWhatItLeftOut(t *testing.T) {
	snap := browserSnapshot{
		Elements:      []browserElement{{Ref: 1, Tag: "button", Text: "ตกลง"}},
		ElementsTotal: 250,
		Images:        []browserImage{{Src: "https://example.com/a.png", Alt: "a"}},
		ImagesTotal:   6,
		BlockedFrames: 2,
	}
	got := formatBrowserRead("Title", "https://example.com", "", "page text", snap)

	if !strings.Contains(got, "and 249 more") {
		t.Errorf("a cut element list must say how many it cut, got:\n%s", got)
	}
	// And it must say so BEFORE the list, not only after it. Two test passes
	// read a 150-item list, missed the line underneath, and each concluded the
	// output had been truncated — it had not. A fact that arrives after the
	// wall is a fact that does not arrive.
	header, notice := strings.Index(got, "1 of 250 listed"), strings.Index(got, "and 249 more")
	if header < 0 || header > notice {
		t.Errorf("the count must be in the list header, above the list, got:\n%s", got)
	}
	// Saying the number without naming the way past it would leave the model
	// with a limit and no door, which is only a slower kind of no.
	if !strings.Contains(got, "filter=") {
		t.Errorf("the cut notice must name filter as the way to reach the rest, got:\n%s", got)
	}
	if !strings.Contains(got, "and 5 more images") {
		t.Errorf("a cut image list must say how many it cut, got:\n%s", got)
	}
	if !strings.Contains(got, "2 frame(s)") {
		t.Errorf("cross-origin frames must be reported, got:\n%s", got)
	}
	// The specific wrong turn this line prevents: read's guidance teaches that
	// a thinner-than-expected page means "not loaded yet, use wait", and no
	// amount of waiting opens a cross-origin frame.
	if !strings.Contains(got, "waiting will not bring it") {
		t.Errorf("the frame notice must rule out waiting, got:\n%s", got)
	}
}

func TestFormatBrowserReadStaysQuietWhenNothingWasCut(t *testing.T) {
	snap := browserSnapshot{
		Elements:      []browserElement{{Ref: 1, Tag: "button", Text: "ตกลง"}},
		ElementsTotal: 1,
		Images:        []browserImage{{Src: "https://example.com/a.png"}},
		ImagesTotal:   1,
	}
	got := formatBrowserRead("Title", "https://example.com", "", "page text", snap)

	for _, unwanted := range []string{"more not listed", "more images not listed", "frame(s)"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("a complete read must not warn about anything; found %q in:\n%s", unwanted, got)
		}
	}
}

func TestFormatBrowserReadEmptyFilterIsAboutTheFilter(t *testing.T) {
	snap := browserSnapshot{ElementsTotal: 0}
	got := formatBrowserRead("Title", "https://example.com", "ส่ง", "page text", snap)

	// An empty list under a filter is the one case where "nothing here" would
	// be a lie about the page rather than a fact about it.
	if !strings.Contains(got, "ส่ง") {
		t.Errorf("the empty-filter answer must name the filter that produced it, got:\n%s", got)
	}
	if !strings.Contains(got, "without one") {
		t.Errorf("the empty-filter answer must point back to an unfiltered read, got:\n%s", got)
	}
}

func TestFormatBrowserReadNamesTheFilterItApplied(t *testing.T) {
	snap := browserSnapshot{
		Elements:      []browserElement{{Ref: 1, Tag: "button", Text: "ส่งข้อความ"}},
		ElementsTotal: 1,
	}
	got := formatBrowserRead("Title", "https://example.com", "ส่ง", "page text", snap)
	if !strings.Contains(got, "contains") || !strings.Contains(got, "ส่ง") {
		t.Errorf("a filtered list must say it is filtered, or it reads as the whole page, got:\n%s", got)
	}
}

func TestTextScriptCountsPastItsOwnCaps(t *testing.T) {
	js := textScript("tok", "")

	// The totals are only true if counting happens before the cap is applied.
	// Order is the whole assertion: elTotal++ after the cap check would report
	// the cap back as the total and the notice would always read "and 0 more".
	elCount := strings.Index(js, "elTotal++")
	elCap := strings.Index(js, "out.length>=150")
	if elCount < 0 || elCap < 0 || elCount > elCap {
		t.Errorf("elements must be counted before the cap is applied, got:\n%s", js)
	}
	imgCount := strings.Index(js, "imgTotal++")
	imgCap := strings.Index(js, "imgs.length>=20")
	if imgCount < 0 || imgCap < 0 || imgCount > imgCap {
		t.Errorf("images must be counted before the cap is applied, got:\n%s", js)
	}
	for _, want := range []string{"elementsTotal:elTotal", "imagesTotal:imgTotal", "frames:scan.blocked"} {
		if !strings.Contains(js, want) {
			t.Errorf("the snapshot must carry %q back, got:\n%s", want, js)
		}
	}
}

func TestPageScriptsReachEveryRoot(t *testing.T) {
	js := textScript("tok", "")
	for _, want := range []string{"el.shadowRoot", "el.contentDocument", "blocked++"} {
		if !strings.Contains(js, want) {
			t.Errorf("textScript must walk shadow roots and same-origin frames and count the rest; missing %q", want)
		}
	}
	// A ref handed out by a read that this tool then cannot act on would be
	// worse than not handing it out, so click and type search the same roots.
	for name, script := range map[string]string{"click": clickScript("tok", 3), "type": typeScript("tok", 3, "x", false)} {
		if !strings.Contains(script, "aetoxFind(") {
			t.Errorf("%sScript must resolve refs through aetoxFind, got:\n%s", name, script)
		}
		if strings.Contains(script, `document.querySelector('[data-aetox-ref=`) {
			t.Errorf("%sScript still searches document only, which misses shadow roots and frames", name)
		}
	}
}

func TestTextScriptClearsStaleRefsFirst(t *testing.T) {
	js := textScript("tok", "")
	clear := strings.Index(js, "removeAttribute('data-aetox-ref')")
	tag := strings.Index(js, "setAttribute('data-aetox-ref'")
	// A filtered read tags far fewer nodes than an unfiltered one, so without
	// the sweep a ref could still resolve to whatever the previous read tagged.
	if clear < 0 || tag < 0 || clear > tag {
		t.Errorf("stale refs must be cleared before new ones are assigned, got:\n%s", js)
	}
}

func TestTextScriptFilterCannotBeCode(t *testing.T) {
	js := textScript("tok", `"); alert(1); //`)
	if strings.Contains(js, `var want="); alert(1)`) {
		t.Errorf("the filter must be embedded as a quoted literal, never as source, got:\n%s", js)
	}
	if !strings.Contains(js, `\"); alert(1); //`) {
		t.Errorf("the filter should survive as an escaped string literal, got:\n%s", js)
	}
}

func TestReadAndWaitAgreeOnWhatThePageSays(t *testing.T) {
	read := textScript("tok", "")
	wait := waitScript("tok", "ส่งแล้ว", 5000)

	// Two definitions of "the text of this page" is the failure here, not a
	// tidiness complaint: `wait` polling document.body while `read` also reads
	// same-origin frames means a word `read` can see is a word `wait` reports
	// absent — and an absent word reads as a slow page, so the model waits again.
	for name, js := range map[string]string{"textScript": read, "waitScript": wait} {
		if !strings.Contains(js, "aetoxText()") {
			t.Errorf("%s must take the page's text from the one shared reader, got:\n%s", name, js)
		}
	}
	if !strings.Contains(wait, "contentDocument") {
		t.Errorf("waitScript must be able to see into same-origin frames, got:\n%s", wait)
	}
}

func TestPageTextReadsShadowRootsThroughTheirChildren(t *testing.T) {
	js := textScript("tok", "")

	// The mechanism, pinned, because the first version of this got it wrong on
	// a plausible-sounding assumption: shadow content is rendered, so innerText
	// "must" report it. Measured on a real page 2026-08-22 — a host whose
	// shadow root renders a paragraph has host.innerText == "" and contributes
	// nothing to document.body.innerText, while the shadow root's own children
	// report the text normally. A shadow root has no .body, which is how the
	// two kinds of extra root are told apart here.
	if !strings.Contains(js, "if(r.body)") {
		t.Errorf("page text must tell a frame document from a shadow root by .body, got:\n%s", js)
	}
	if !strings.Contains(js, "r.children") {
		t.Errorf("shadow-root text must be read through its children, not its host, got:\n%s", js)
	}
}
