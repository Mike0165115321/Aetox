package main

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPickScriptCarriesTokenAndOpts(t *testing.T) {
	opts := `{"accent":"#378add","hint":"คลิกเพื่อชี้"}`
	js := pickScript("abc123", opts)

	if !strings.Contains(js, `"abc123"`) {
		t.Error("pickScript() does not embed the token — the page could not prove which request it answers")
	}
	if !strings.Contains(js, `#378add`) {
		t.Error("pickScript() dropped the theme opts, so the overlay would draw in its fallback colours")
	}
	if !strings.Contains(js, bridgePost) {
		t.Errorf("pickScript() does not post over the engine bridge (%s)", bridgePost)
	}
	if !strings.Contains(js, `__aetox:"pick"`) {
		t.Error("pickScript() posts an envelope onMessage will not route")
	}
}

// The overlay's colours and wording come from the frontend, so opts is
// attacker-shaped input the moment anything upstream is wrong. It must arrive
// as a JS string literal to be JSON.parse'd, never as code.
func TestPickScriptEscapesHostileOpts(t *testing.T) {
	// An opts that closes the JS string literal and appends code. The text
	// itself survives into the script — it has to, it is the argument — so what
	// is checked is that its quotes arrive escaped, leaving it one inert string
	// for JSON.parse instead of a statement the page runs.
	opts := `{"hint":"x"};window.evil=1;//`
	js := pickScript("tok", opts)

	_, after, found := strings.Cut(js, "JSON.parse(")
	if !found {
		t.Fatal("pickScript() should parse opts rather than splice it")
	}
	// Unquote reads exactly as much as one complete literal, so this fails the
	// moment the payload's own quote is the one that ends it.
	got, err := strconv.Unquote(strings.TrimSuffix(after[:strings.Index(after, "\n")], ")}catch(e){}"))
	if err != nil {
		t.Fatalf("the JSON.parse argument is not one string literal: %v", err)
	}
	if got != opts {
		t.Errorf("opts arrived as %q, want %q — anything but a round trip means it was spliced, not quoted", got, opts)
	}
}

// %% in the format string is easy to get wrong, and the failure is invisible in
// review: the pill loses its centring and sits wherever the page's left edge is.
func TestPickScriptKeepsPercentInCSS(t *testing.T) {
	js := pickScript("tok", "{}")
	if !strings.Contains(js, "left:50%;transform:translateX(-50%)") {
		t.Error("pickScript() mangled the pill's percentage CSS")
	}
	if strings.Contains(js, "%!") {
		t.Error("pickScript() has a bad format verb — see the %!(...) marker in the output")
	}
}

func TestClaimPickConsumesTheTokenExactlyOnce(t *testing.T) {
	tab := &browserTab{}
	token := tab.armPick()

	if tab.claimPick("some other token") {
		t.Error("claimPick() accepted a token that was never minted")
	}
	if !tab.claimPick(token) {
		t.Error("claimPick() rejected the token armPick just minted")
	}
	if tab.claimPick(token) {
		t.Error("claimPick() accepted the same token twice — a replayed pick would land in the composer again")
	}
}

func TestClaimPickRejectsAnEmptyTokenOnAnIdleTab(t *testing.T) {
	tab := &browserTab{}
	if tab.claimPick("") {
		t.Error("claimPick(\"\") passed on a tab with no pick running — an unarmed tab must answer nothing")
	}
}

func TestOnMessageEmitsAPickFromTheRealPage(t *testing.T) {
	var gotEvent string
	var gotData []any
	app := &App{emit: func(event string, data ...any) { gotEvent, gotData = event, data }}
	h := &browserHost{app: app}
	tab := &browserTab{}
	token := tab.armPick()

	msg, _ := json.Marshal(aetoxMsg{
		Aetox: "pick", Token: token, URL: "https://example.com/app",
		Picks: []browserPick{{Selector: "button.btn-primary", Tag: "button", W: 188, H: 36}},
	})
	h.onMessage("web-1", tab, string(msg), "https://example.com/")

	if gotEvent != "browser:pick:web-1" {
		t.Fatalf("emitted event = %q, want browser:pick:web-1", gotEvent)
	}
	payload, ok := gotData[0].(map[string]any)
	if !ok {
		t.Fatalf("event payload = %#v, want a map", gotData[0])
	}
	picks, ok := payload["picks"].([]browserPick)
	if !ok || len(picks) != 1 || picks[0].Selector != "button.btn-primary" {
		t.Fatalf("event payload picks = %#v, want the one picked element", payload["picks"])
	}
}

func TestOnMessageRejectsAPickFromASpoofedOrigin(t *testing.T) {
	emitted := false
	app := &App{emit: func(string, ...any) { emitted = true }}
	h := &browserHost{app: app}
	tab := &browserTab{}
	token := tab.armPick()

	// A frame at evil.com claiming to be the page the user is pointing at.
	msg, _ := json.Marshal(aetoxMsg{Aetox: "pick", Token: token, URL: "https://example.com/app"})
	h.onMessage("web-1", tab, string(msg), "https://evil.com/")

	if emitted {
		t.Error("a pick claiming another origin reached the composer")
	}
	// And it must not have burned the token: the real page is still picking.
	if !tab.claimPick(token) {
		t.Error("a rejected cross-origin pick consumed the token, killing the live mode")
	}
}

func TestOnMessageRejectsAnUnsolicitedPick(t *testing.T) {
	emitted := false
	app := &App{emit: func(string, ...any) { emitted = true }}
	h := &browserHost{app: app}
	tab := &browserTab{} // no mode running

	h.onMessage("web-1", tab, `{"__aetox":"pick","url":"https://example.com/","picks":[{"selector":"body"}]}`, "https://example.com/")

	if emitted {
		t.Error("a page pushed a pick nobody asked for into the composer")
	}
}

// One script, two modes, chosen inside the page from opts — so what is checked
// here is the runtime guard, not two different scripts.
func TestPickScriptDrawsInkOnlyInDrawMode(t *testing.T) {
	js := pickScript("tok", `{"mode":"draw","doneLabel":"เสร็จ"}`)

	if !strings.Contains(js, `createElement("canvas")`) {
		t.Fatal("there is no ink layer in the script at all")
	}
	guard := strings.Index(js, `if(MODE==="draw"){`)
	canvas := strings.Index(js, `ink=document.createElement("canvas")`)
	if guard < 0 || canvas < guard {
		t.Error("the ink layer is built before the mode is checked, so pointing would draw a canvas over every page")
	}
	if !strings.Contains(js, "เสร็จ") {
		t.Error("draw mode dropped the button wording, so the only way out would be the Enter key")
	}
}

// Drawing ends with the marks still on the page: the picture is taken from
// outside, against this document, a moment later. A finish that tore the ink
// down with the rest of the overlay would photograph a page with nothing on it.
func TestPickScriptKeepsTheInkStandingAfterAFinish(t *testing.T) {
	js := pickScript("tok", `{"mode":"draw"}`)
	if !strings.Contains(js, "teardown(true)") {
		t.Error("finish() takes the ink down with the controls")
	}
	if !strings.Contains(js, "drawn:!!drawn") {
		t.Error("the answer does not say a drawing is waiting, so nothing would go and photograph it")
	}
}

// A drag across the middle of a page encloses nothing at all — the case that
// made the region look like a feature that did not work.
func TestPickScriptFallsBackToOverlappingElements(t *testing.T) {
	js := pickScript("tok", "{}")
	if !strings.Contains(js, "if(cand.length===0)cand=touch") {
		t.Error("a region that fully encloses nothing still comes back empty")
	}
}

func TestBrowserCapturePNGHandsBackADataURL(t *testing.T) {
	b := &fakeBackend{}
	app := &App{}
	view := &fakeView{shot: shotResult{PNG: []byte("\x89PNG fake")}}
	app.browsers = &browserHost{app: app, backend: b, tabs: map[string]*browserTab{"web-1": {}}, views: map[string]tabView{"web-1": view}}

	go func() {
		// Stands in for the host thread's pump.
		for i := 0; i < 50; i++ {
			b.drain()
			time.Sleep(time.Millisecond)
		}
	}()

	got, err := app.BrowserCapturePNG("web-1")
	if err != nil {
		t.Fatalf("BrowserCapturePNG() = %v", err)
	}
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Errorf("BrowserCapturePNG() = %q, want a PNG data URL", got)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, "data:image/png;base64,"))
	if err != nil || string(decoded) != "\x89PNG fake" {
		t.Errorf("the data URL does not carry the bytes the engine gave (%v)", err)
	}
}

func TestBrowserCapturePNGReportsAnEmptyPicture(t *testing.T) {
	b := &fakeBackend{}
	app := &App{}
	app.browsers = &browserHost{app: app, backend: b, tabs: map[string]*browserTab{"web-1": {}}, views: map[string]tabView{"web-1": &fakeView{}}}

	go func() {
		for i := 0; i < 50; i++ {
			b.drain()
			time.Sleep(time.Millisecond)
		}
	}()

	if _, err := app.BrowserCapturePNG("web-1"); err == nil {
		t.Error("BrowserCapturePNG() reported success over an empty capture")
	}
}

// The bar is drawn on somebody else's page, whose background nobody knows.
// Borrowing the app's panel colour makes it a pale box on a white page —
// invisible exactly where it is needed.
func TestPickScriptBarCarriesItsOwnPalette(t *testing.T) {
	js := pickScript("tok", `{"accent":"#ff0000"}`)

	for _, borrowed := range []string{"O.panel", "O.text", "O.border"} {
		if strings.Contains(js, borrowed) {
			t.Errorf("the bar takes %s from the app theme, so it disappears on a page the theme did not expect", borrowed)
		}
	}
	if !strings.Contains(js, "O.accent") {
		t.Error("the accent should still travel — the ink and the primary button are the app's colour")
	}
}

// "There is something to send now" is said once, in two places: the label stops
// repeating the instruction and starts counting, and the button that sends it
// fills with the colour of the ink.
func TestPickScriptLightsDoneOnlyWhenThereIsInk(t *testing.T) {
	js := pickScript("tok", `{"mode":"draw"}`)

	if !strings.Contains(js, "paintDone(strokes.length>0)") {
		t.Error("the done button's state is not derived from whether anything was drawn")
	}
	if !strings.Contains(js, `doneBtn.style.background=armed?ACC:"transparent"`) {
		t.Error("the done button does not fill with the ink colour once there is ink")
	}
	if !strings.Contains(js, "0 0 18px 2px") {
		t.Error("the done button has no halo, so nothing marks the moment there is something to send")
	}
}

// A backend that records whether the capture had already been asked for by the
// time the queued command returned — which is the whole of the contract, and
// the only part of "runs on the host thread" a fake can actually observe.
type hostThreadBackend struct {
	fakeBackend
	view    *threadNotingView
	inThred atomic.Bool
}

func (b *hostThreadBackend) do(fn func()) {
	b.fakeBackend.do(func() {
		fn()
		b.inThred.Store(b.view.asked.Load())
	})
}

type threadNotingView struct {
	fakeView
	asked atomic.Bool
}

func (v *threadNotingView) capture() <-chan shotResult {
	v.asked.Store(true)
	return v.fakeView.capture()
}

// WebView2 is apartment-threaded: the capture has to be ASKED FOR on the thread
// that owns the webview, and awaited off it. A call made from a goroutine is
// not slow or racy — it is refused outright, and the refusal reads like the
// engine saying no rather than like a bug. The first version of this queued a
// goroutine inside the queued command, which put the COM call back off-thread.
func TestBrowserCapturePNGAsksOnTheHostThread(t *testing.T) {
	view := &threadNotingView{}
	view.shot = shotResult{PNG: []byte("\x89PNG fake")}
	b := &hostThreadBackend{view: view}
	app := &App{}
	app.browsers = &browserHost{app: app, backend: b, tabs: map[string]*browserTab{"web-1": {}}, views: map[string]tabView{"web-1": view}}

	go func() {
		for i := 0; i < 50; i++ {
			b.drain()
			time.Sleep(time.Millisecond)
		}
	}()

	if _, err := app.BrowserCapturePNG("web-1"); err != nil {
		t.Fatalf("BrowserCapturePNG() = %v", err)
	}
	if !b.inThred.Load() {
		t.Error("the queued command returned without asking for the capture — the COM call was handed to a goroutine, where WebView2 refuses it")
	}
}
