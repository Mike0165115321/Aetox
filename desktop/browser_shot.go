package main

// A picture of the page.
//
// This exists for one reason: an annotation is a mark on something. "ตรงบริเวณ
// นี้" is not a selector and not a node — it is a place on a rendering, and the
// only honest way to carry a place on a rendering is the rendering. Text was
// enough while the user was pointing at elements; the moment they draw, the
// answer has to be seen.
//
// The engine renders it, not the OS: a screen grab of the tab's window would
// come back with whatever floats above it, and would be a lie about what the
// page looks like the moment anything overlaps. See the CapturePreview patch in
// third_party/go-webview2 (AETOX-PATCH.md).

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// shotResult is one finished (or failed) capture, in the portable vocabulary —
// browser.go's tabView must not name any one engine's types.
type shotResult struct {
	PNG []byte
	Err error
}

// BrowserCapturePNG returns what the tab is showing as a PNG data URL.
//
// The frontend hands that straight to SaveChatImageData, which is the path a
// pasted screenshot already takes — a captured page is an attached image like
// any other once it exists, and inventing a second way to attach one would be a
// second answer to a question already settled.
func (a *App) BrowserCapturePNG(id string) (string, error) {
	host, err := a.browserHostLazy()
	if err != nil {
		return "", err
	}
	if !host.live(id) {
		return "", fmt.Errorf("no browser tab %q", id)
	}

	// capture() is called ON the webview's thread and answers off it. Both
	// halves matter, and getting either backwards fails in a way that reads
	// like the engine refusing rather than like a bug:
	//
	//   - The COM call must happen inside do(). WebView2 is apartment-threaded,
	//     so the same call made from a goroutine is not slow or racy, it is
	//     rejected outright.
	//   - The waiting must happen outside it. The answer is delivered by the
	//     message pump this thread is running, so waiting here would be waiting
	//     for the thing that would wake us.
	//
	// Hence the channel of a channel: do() hands back where the answer will
	// arrive, and the arrival is awaited on this goroutine.
	ready := make(chan (<-chan shotResult), 1)
	host.onTab(id, func(v tabView, _ *browserTab) { ready <- v.capture() })

	var answer <-chan shotResult
	select {
	case answer = <-ready:
	case <-time.After(3 * time.Second):
		return "", fmt.Errorf("the browser did not start the capture")
	}

	select {
	case r := <-answer:
		if r.Err != nil {
			return "", r.Err
		}
		if len(r.PNG) == 0 {
			return "", fmt.Errorf("the page came back as an empty picture")
		}
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(r.PNG), nil
	case <-time.After(8 * time.Second):
		return "", fmt.Errorf("the page did not answer with a picture")
	}
}

// maxFullCaptureHeight bounds one full-page picture, in CSS pixels.
//
// Not a policy number: Chromium refuses to allocate a texture past roughly this
// and answers with an empty image, which arrives looking exactly like the blank
// frame a hidden window produces. Cutting deliberately at a height we know it
// will draw, and saying that the picture was cut, beats handing back a blank
// one and a working exit code.
const maxFullCaptureHeight = 16384

// BrowserCaptureFullPNG photographs the whole document rather than the part of
// it that happens to be on screen, and reports whether it had to stop short.
//
// The engine draws it, through the same two CDP methods the deck exporter has
// used since it shipped: Page.getLayoutMetrics for how big the document really
// is, then Page.captureScreenshot with captureBeyondViewport and a clip that
// addresses the document rather than the viewport (deck_image.go says the same
// thing about slide 8). Scrolling and stitching would be the alternative, and
// it is worse in every way that matters — a page with a fixed header repeats it
// once per stitch, and a page that lazy-loads on scroll changes underneath the
// photographs being joined.
//
// The cost of the engine drawing it: this is CDP, so it is what
// browser_windows.go implements and what a future WebKit host will have to
// answer for itself. A caller that gets an error here still has the viewport
// capture, which is portable.
func (a *App) BrowserCaptureFullPNG(ctx context.Context, id string) (dataURL string, cutAt int, err error) {
	host, hostErr := a.browserHostLazy()
	if hostErr != nil {
		return "", 0, hostErr
	}
	if !host.live(id) {
		return "", 0, fmt.Errorf("no browser tab %q", id)
	}

	raw, err := callEngineOn(ctx, host, id, "Page.getLayoutMetrics", "{}")
	if err != nil {
		return "", 0, err
	}
	// cssContentSize is the document in CSS pixels, which is the unit `clip`
	// wants. contentSize is the same rectangle in device pixels on a scaled
	// display, and reading that one produces a picture of the top-left corner
	// on exactly the machines whose users would not think to mention their
	// display scaling.
	var metrics struct {
		CSSContentSize struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"cssContentSize"`
		ContentSize struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"contentSize"`
	}
	if jsonErr := json.Unmarshal([]byte(raw), &metrics); jsonErr != nil {
		return "", 0, fmt.Errorf("the engine's page size did not parse: %w", jsonErr)
	}
	w, h := metrics.CSSContentSize.Width, metrics.CSSContentSize.Height
	if w <= 0 || h <= 0 {
		w, h = metrics.ContentSize.Width, metrics.ContentSize.Height
	}
	if w <= 0 || h <= 0 {
		return "", 0, fmt.Errorf("the engine did not report a page size")
	}
	if h > maxFullCaptureHeight {
		h = maxFullCaptureHeight
		cutAt = maxFullCaptureHeight
	}

	raw, err = callEngineOn(ctx, host, id, "Page.captureScreenshot", fullCaptureParams(w, h))
	if err != nil {
		return "", 0, err
	}
	data, err := engineData(raw)
	if err != nil {
		return "", 0, err
	}
	return "data:image/png;base64," + data, cutAt, nil
}

// fullCaptureParams is Page.captureScreenshot's parameter object for a whole
// document. Named rather than inlined so the one flag that makes it work can be
// asserted without a live webview.
//
// captureBeyondViewport is that flag: without it the clip is still read against
// the viewport, so everything below the fold comes back blank — the same
// failure deck_image.go's captureParams was written to avoid, arrived at from
// the other direction.
func fullCaptureParams(w, h float64) string {
	return fmt.Sprintf(
		`{"format":"png","captureBeyondViewport":true,`+
			`"clip":{"x":0,"y":0,"width":%g,"height":%g,"scale":1}}`, w, h)
}
