package main

// The webview a deck is exported through, and the two things asked of it.
//
// A deck is HTML (docs/architecture/html-deck-2026-08-19.md), so the thing that
// knows how to render it is a browser, and this app carries one already —
// WebView2, embedded per browser tab. Exporting is therefore not a rendering
// problem: it is loading the file into a webview nobody watches and asking the
// engine for the answer in the shape somebody else's program can open.
//
// **Off-screen, not hidden, and its own window — three tries to get there.** A
// window at `ShowWindow(SW_HIDE)` produces no frames (browser_capture.go's
// comment; WebView2Feedback #1077 and #2983), so a screenshot of one comes back
// blank. A WS_VISIBLE window merely parked outside every monitor is a different
// state: Windows composites it as if someone were looking. But the second try
// made that window a WS_CHILD of the app's, and a child is CLIPPED to its
// parent's client area — outside it, it draws nothing, which is the first
// problem again. Moving it inside worked and put the deck on screen over the
// whole application for the length of every export, because a child window
// paints over its parent's content whatever the Z order says.
//
// So: its own top-level WS_POPUP, off-screen. Composited, and nobody sees it.
// browser_windows.go builds it, keyed on this file's exportTabID.
//
// That claim is the load-bearing one and it is not from documentation. It is
// why deck_image.go refuses a capture that comes back a single flat colour
// rather than writing it out: if the reasoning above is ever wrong on some
// machine, the failure has to arrive as an error and not as eight blank slides.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	// The deck's own slide box. 1280x720 CSS pixels is the size the contract
	// fixes, and 13.333x7.5 inches is that at 96dpi — which is also exactly
	// ooxml's 12192000 EMU. One number, three places that must agree.
	deckSlideWidthPx  = 1280
	deckSlideHeightPx = 720

	// Loading is a local file with its pictures already inline, so this bounds
	// a hang rather than a slow network. The engine calls get their own, longer
	// budget: printing lays out every slide before it answers.
	deckLoadTimeout   = 20 * time.Second
	deckEngineTimeout = 60 * time.Second
)

// exportTabID is outside the agent's namespace (agentTabPrefix) on purpose, so
// nothing downstream mistakes this for a page the agent opened and offers to
// read it. §81's rule that the user's browsing never becomes agent-readable
// runs on that prefix, and an export tab is neither the agent's nor the user's.
const exportTabID = "deck-export"

// withExportTab loads a deck in an unseen webview and hands `work` a function
// that runs engine methods on it. The tab is gone by the time it returns.
func (a *App) withExportTab(ctx context.Context, fileURL string, work func(call engineCaller) error) error {
	host, err := a.browserHostLazy()
	if err != nil {
		return err
	}

	// A previous export that died mid-flight would otherwise leave its tab
	// behind, and open() returns early when the id is taken — so the next
	// export would quietly work on the previous deck.
	a.BrowserClose(exportTabID)

	// The export webview gets a top-level window of its own, parked outside
	// every monitor — browser_windows.go arranges that on this id, because the
	// reason is a Win32 fact rather than a caller's preference: a WS_CHILD
	// window paints over its parent's client area no matter what the Z order
	// says, so no child window can be both composited and unseen. Being
	// composited is not optional; a window that produces no frames photographs
	// as a blank rectangle (browser_capture.go, WebView2Feedback #1077, #2983).
	host.open(exportTabID, fileURL, 0, 0, deckSlideWidthPx, deckSlideHeightPx)
	defer a.BrowserClose(exportTabID)

	if err := a.waitForDeckLoad(ctx, host, exportTabID); err != nil {
		return err
	}
	return work(func(method, params string) (string, error) {
		return callEngineOn(ctx, host, exportTabID, method, params)
	})
}

// engineCaller runs one engine method and returns its JSON answer.
type engineCaller func(method, paramsJSON string) (string, error)

func callEngineOn(ctx context.Context, host *browserHost, id, method, params string) (string, error) {
	out := make(chan engineReply, 1)
	host.onTab(id, func(v tabView, _ *browserTab) {
		// Issued HERE, on the webview's own thread, and read outside it.
		//
		// Both halves matter and getting the first one wrong is what
		// 0x802A000C means: "This method can only be called from the thread
		// that created the object". WebView2 is apartment-threaded, and the
		// first version of this line wrapped the whole call in `go func()`,
		// which left the thread onTab had just handed over — every export
		// failed, and requireHostThread had been saying so into the log the
		// entire time (§127.8's lesson, hit again from the other side).
		//
		// The read stays outside because the answer arrives through the message
		// pump this thread is running: blocking here would be waiting for the
		// thing that would wake us. Same shape as browser_capture.go's.
		src := v.callEngine(method, params)
		go func() { out <- <-src }()
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(deckEngineTimeout):
		return "", fmt.Errorf("%s ใช้เวลานานเกินไป", method)
	case r := <-out:
		return r.JSON, r.Err
	}
}

// engineData pulls the base64 "data" field every CDP method that returns bytes
// answers with — Page.printToPDF and Page.captureScreenshot both.
func engineData(raw string) (string, error) {
	var answer struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return "", fmt.Errorf("the engine's answer did not parse: %w", err)
	}
	if answer.Data == "" {
		// An answer without the field is an engine that declined, and saying so
		// beats writing a zero-byte file that opens as damage.
		return "", errors.New("ตัวเรนเดอร์ไม่ได้ส่งข้อมูลกลับมา")
	}
	return answer.Data, nil
}

// waitForDeckLoad blocks until the export tab reports a finished navigation.
//
// It waits on the tab's own latch rather than sleeping, and reads navLoaded
// afterwards: navigation-completed fires for the engine's error page too, so a
// missing file would otherwise export a tidy copy of "ERR_FILE_NOT_FOUND" and
// report success. That exact failure is why GetIsSuccess was bound in the first
// place (AETOX-PATCH.md, the second patch).
func (a *App) waitForDeckLoad(ctx context.Context, host *browserHost, id string) error {
	// onTab runs behind the same FIFO queue as open, so this cannot observe a
	// tab that does not exist yet — it simply runs after it.
	latch := make(chan chan struct{}, 1)
	host.onTab(id, func(_ tabView, t *browserTab) {
		_, done := t.latch()
		latch <- done
	})

	var done chan struct{}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(deckLoadTimeout):
		return errors.New("เปิดเด็คในตัวเรนเดอร์ไม่สำเร็จ")
	case done = <-latch:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(deckLoadTimeout):
		return errors.New("เด็คโหลดไม่เสร็จภายในเวลาที่รอ")
	case <-done:
	}

	ok := make(chan bool, 1)
	host.onTab(id, func(_ tabView, t *browserTab) { ok <- t.navLoaded() })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(deckLoadTimeout):
		return errors.New("ตัวเรนเดอร์ไม่ตอบว่าโหลดสำเร็จหรือไม่")
	case loaded := <-ok:
		if !loaded {
			return errors.New("ตัวเรนเดอร์เปิดไฟล์เด็คไม่ได้")
		}
	}
	return nil
}
