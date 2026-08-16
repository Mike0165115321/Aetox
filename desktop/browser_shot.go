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
	"encoding/base64"
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
	t := host.tab(id)
	if t == nil {
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
	host.backend.do(func() { ready <- t.view.capture() })

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
