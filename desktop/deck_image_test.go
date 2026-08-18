package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func solidPNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The guard on deck_render.go's central claim — that a webview parked outside
// every monitor still composites.
//
// If that reasoning is ever wrong on some machine, WebView2 does not fail: it
// answers with a perfectly valid picture of nothing. Without this check the
// export would write a folder of blank rectangles and report success, which is
// the failure this codebase keeps writing down — answering wrongly where the
// honest answer was "I could not".
func TestABlankCaptureIsRefusedRatherThanWritten(t *testing.T) {
	white := solidPNG(t, 1280, 720, color.White)
	err := notBlank(white)
	if err == nil {
		t.Fatal("a blank capture was accepted")
	}
	// The message has to point at the machine, not at the deck: the user did
	// nothing wrong and there is nothing in their file to fix.
	if !strings.Contains(err.Error(), "นอกจอ") {
		t.Errorf("the error should say what is suspected: %v", err)
	}

	// Transparent-black is what an unpainted surface hands back, and it is a
	// different uniform colour from white — both must be caught.
	if err := notBlank(solidPNG(t, 400, 300, color.RGBA{})); err == nil {
		t.Error("a transparent capture was accepted")
	}
}

func TestARealSlideIsAccepted(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			img.Set(x, y, color.White)
		}
	}
	// One dark band where a title would be. The sampler steps across the image
	// rather than reading every pixel, so this also pins that the step is fine
	// enough to find a heading-sized mark.
	for y := 80; y < 160; y++ {
		for x := 88; x < 1000; x++ {
			img.Set(x, y, color.RGBA{R: 16, G: 19, B: 26, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := notBlank(buf.Bytes()); err != nil {
		t.Errorf("a slide with a title on it was refused: %v", err)
	}
}

func TestBytesThatAreNotAnImageAreRefused(t *testing.T) {
	if err := notBlank([]byte("this is not a png")); err == nil {
		t.Fatal("junk was accepted as a capture")
	}
}

// jpg is the extension people know; jpeg is what the protocol calls it. Sending
// the wrong one gets a protocol error, not a jpg.
func TestJpgIsSentToTheEngineAsJpeg(t *testing.T) {
	spec, ok := deckImageFormats["jpg"]
	if !ok {
		t.Fatal("jpg is offered in the menu but has no spec")
	}
	if spec.cdp != "jpeg" {
		t.Errorf("cdp format = %q, want jpeg", spec.cdp)
	}
	if spec.quality <= 0 || spec.quality > 100 {
		t.Errorf("quality = %d, want 1..100", spec.quality)
	}
	if png := deckImageFormats["png"]; png.quality != 0 {
		t.Error("png must not carry a quality — the protocol rejects it")
	}
}

// Every slide is photographed from the same page state rather than by scrolling
// to each one, which is what captureBeyondViewport buys. Without it, slide 8 of
// a 720px-tall viewport is outside the capture and comes back empty.
func TestCaptureParamsReachSlidesBelowTheFold(t *testing.T) {
	var got map[string]any
	// The same builder captureSlide uses, given a rect far down the page.
	raw := captureParams(deckImageFormats["png"], slideRect{X: 0, Y: 5236, W: 1280, H: 720})
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("the capture parameters are not valid JSON: %v\n%s", err, raw)
	}
	if got["captureBeyondViewport"] != true {
		t.Error("captureBeyondViewport is off, so any slide below the fold comes back empty")
	}
	clip, _ := got["clip"].(map[string]any)
	if clip == nil {
		t.Fatal("no clip: the whole page would be photographed as one picture")
	}
	if clip["y"] != float64(5236) {
		t.Errorf("clip.y = %v, want the slide's measured position", clip["y"])
	}
	if clip["width"] != float64(1280) || clip["height"] != float64(720) {
		t.Errorf("clip is %vx%v, want the slide's own size", clip["width"], clip["height"])
	}
	if clip["scale"] != float64(1) {
		t.Errorf("scale = %v, want 1 so the picture is the size the deck was designed at", clip["scale"])
	}
}

// The flattening script has to find the slides the contract defines, and must
// report page coordinates rather than viewport ones — a rect read straight off
// getBoundingClientRect is relative to the scroll position, so slide 5 would be
// photographed at slide 1's place.
func TestFlattenScriptMeasuresInPageCoordinates(t *testing.T) {
	if !strings.Contains(flattenScript, "section.slide") {
		t.Error("the script does not look for the contract's own marker")
	}
	if !strings.Contains(flattenScript, "scrollX") || !strings.Contains(flattenScript, "scrollY") {
		t.Error("the rects are not converted to page coordinates")
	}
}

// Every override the flattener makes is conditional, and that is not a detail.
// A blanket stylesheet fixed stacked decks and broke every deck that was already
// fine — the real one that exposed it lays its slides out with
// `display:flex; justify-content:center`, and being forced to `block` dropped
// the vertical centring out of all eight slides.
func TestFlattenOnlyOverridesWhatIsWrong(t *testing.T) {
	// display is the one that did the damage: it may only be touched when the
	// slide is not displayed at all.
	if !strings.Contains(flattenScript, "cs.display==='none'") {
		t.Error("display is not gated on the slide actually being hidden")
	}
	for _, gated := range []string{"cs.overflow!=='visible'", "cs.visibility!=='visible'", "cs.float!=='none'"} {
		if !strings.Contains(flattenScript, gated) {
			t.Errorf("an override is applied unconditionally: expected a check for %s", gated)
		}
	}
	// A scrolling ancestor is the trap that looks like success, so the walk up
	// the tree is not optional.
	if !strings.Contains(flattenScript, "parentElement") {
		t.Error("the flattener never looks at the slides' ancestors")
	}
	if !strings.Contains(flattenScript, "docHeight") {
		t.Error("the report carries no document height, so a trapped document cannot be caught")
	}
}

// The reveal pass has to survive the deck un-revealing what it just showed. The
// deck that exposed this does `classList.toggle('visible', i===current)` — the
// walk fires it eight times and leaves one slide wearing the class.
func TestRevealPinsWhatItFindsRatherThanGuessingClassNames(t *testing.T) {
	for _, name := range []string{"reveal", "visible", "in-view", "active", "fade"} {
		if strings.Contains(revealScript, "'"+name+"'") {
			t.Errorf("the reveal pass guesses at the class name %q instead of reading the result", name)
		}
	}
	// What it pins instead: the computed values, as inline !important, which
	// outrank the class the deck is about to remove.
	for _, prop := range []string{"opacity", "transform"} {
		if !strings.Contains(revealScript, `setProperty('`+prop+`'`) {
			t.Errorf("the reveal pass does not pin %s", prop)
		}
	}
	if !strings.Contains(revealScript, "'important'") {
		t.Error("the pinned values are not !important, so the deck's own class wins")
	}
	if !strings.Contains(revealScript, "scrollIntoView") {
		t.Error("nothing scrolls, so a scroll-triggered reveal never fires")
	}
}

func TestStillStackedSlidesAreRefused(t *testing.T) {
	stacked := flattenReport{DocHeight: 720, Rects: []slideRect{
		{X: 0, Y: 0, W: 1280, H: 720}, {X: 0, Y: 0, W: 1280, H: 720}}}
	err := checkFlattened(stacked)
	if err == nil {
		t.Fatal("a deck whose slides are still stacked was accepted")
	}
	// The message has to name which two, so the author can look at them.
	if !strings.Contains(err.Error(), "1") || !strings.Contains(err.Error(), "2") {
		t.Errorf("the error must name the slides: %v", err)
	}

	laid := flattenReport{DocHeight: 1440, Rects: []slideRect{
		{Y: 0, W: 1280, H: 720}, {Y: 720, W: 1280, H: 720}}}
	if err := checkFlattened(laid); err != nil {
		t.Errorf("a properly laid-out deck was refused: %v", err)
	}
	if err := checkFlattened(flattenReport{DocHeight: 720, Rects: []slideRect{{W: 1280, H: 720}}}); err != nil {
		t.Errorf("a one-slide deck cannot be stacked: %v", err)
	}
}

// The trap that looks like success: slides at different positions inside a
// container the document never grows to hold. Every position checks out, and
// printing clips everything past the first screen.
func TestADocumentTooShortForItsSlidesIsRefused(t *testing.T) {
	trapped := flattenReport{DocHeight: 720, Rects: []slideRect{
		{Y: 0, W: 1280, H: 720}, {Y: 720, W: 1280, H: 720}, {Y: 1440, W: 1280, H: 720}}}
	err := checkFlattened(trapped)
	if err == nil {
		t.Fatal("a document too short to hold its slides was accepted")
	}
	if !strings.Contains(err.Error(), "2160") {
		t.Errorf("the error should say how tall the slides actually are: %v", err)
	}
}
