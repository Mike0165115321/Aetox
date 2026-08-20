package main

// Photographing what somebody drew on a slide.
//
// The browser side of this is one call: a tab is a native window, and the
// engine will hand back a picture of it (BrowserCapturePNG). A deck is an
// <iframe> inside the app's own webview, and there is nothing to point that
// call at — the app cannot photograph its own surface, and the marks live only
// in the live DOM, so the deck on disk does not carry them either.
//
// So the picture is made from two halves that both already exist. The slide is
// rendered the way every export renders it (deck_render.go's off-screen
// webview), and the ink comes over from the frontend as a transparent PNG the
// size of the slide. Laying one on the other is stdlib.
//
// Rendering rather than screen-scraping is also the more honest picture: the
// deck is photographed at its own 1280x720, not at whatever fraction of it the
// panel happened to be showing, so the model reads the slide at the size the
// file describes.
//
// What this deliberately does NOT capture is live state — a deck that had been
// clicked into some position by hand comes back in the state the file loads to,
// with the marks on top. revealEverything is what makes that the right slide
// rather than the first one, and it is the same pass the .pptx and .pdf exports
// make.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"strings"
)

// maxInkBytes bounds the decoded data URL. The ink is one slide's worth of
// strokes on a transparent background, which compresses to tens of kilobytes;
// anything past this is not a drawing and must not be turned into an image.Image
// before anybody notices.
const maxInkBytes = 8 << 20

// DeckCaptureDrawing renders one slide of a deck with the user's marks over it
// and hands back a PNG data URL, the same shape BrowserCapturePNG returns — so
// the frontend attaches a drawing on a slide exactly the way it attaches one on
// a page.
//
// slide is 1-based, the number the panel shows.
func (a *App) DeckCaptureDrawing(relPath string, slide int, ink string) (string, error) {
	root := strings.TrimSpace(a.cur().cfg.SandboxRoot)
	if root == "" {
		return "", fmt.Errorf("no project open")
	}
	full, err := safeSandboxPath(root, relPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(full); err != nil {
		return "", errFileGone
	}
	marks, err := decodeInk(ink)
	if err != nil {
		return "", err
	}
	shot, err := a.captureDeckSlide(context.Background(), fileURLForPath(full), slide)
	if err != nil {
		return "", err
	}
	out, err := overlayInk(shot, marks)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(out), nil
}

// captureDeckSlide is exportDeckImages for exactly one slide.
//
// The two passes are not optional and are not this file's to invent: revealing
// so a slide has its content at all, flattening so it has a place of its own to
// be clipped to. deck_image.go carries the reasoning for both.
func (a *App) captureDeckSlide(ctx context.Context, fileURL string, slide int) ([]byte, error) {
	spec, ok := deckImageFormats["png"]
	if !ok {
		return nil, fmt.Errorf("ไม่มีตัวเขียนภาพ png")
	}
	var shot []byte
	err := a.withExportTab(ctx, fileURL, func(call engineCaller) error {
		if _, err := revealEverything(call); err != nil {
			return err
		}
		rects, err := flattenForExport(call)
		if err != nil {
			return err
		}
		if slide < 1 || slide > len(rects) {
			return fmt.Errorf("เด็คนี้มี %d สไลด์ ไม่มีใบที่ %d", len(rects), slide)
		}
		shot, err = captureSlide(call, spec, rects[slide-1])
		return err
	})
	if err != nil {
		return nil, err
	}
	return shot, nil
}

// decodeInk turns the frontend's data URL into a picture.
//
// Only PNG is accepted, and that is the format the canvas is asked for: it is
// the one the frontend can produce with an alpha channel, and an opaque overlay
// would hide the slide it is meant to annotate rather than mark it.
func decodeInk(dataURL string) (image.Image, error) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, fmt.Errorf("รอยวาดต้องเป็น PNG")
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[len(prefix):])
	if err != nil {
		return nil, fmt.Errorf("อ่านรอยวาดไม่ได้: %w", err)
	}
	if len(raw) > maxInkBytes {
		return nil, fmt.Errorf("รอยวาดใหญ่เกินไป")
	}
	return png.Decode(bytes.NewReader(raw))
}

// overlayInk draws the marks over the slide and encodes the result.
//
// The two arrive the same size in every normal path: the panel pins the deck's
// iframe to the slide box (SlidesPane.fit), so the canvas the marks are drawn on
// is that box, and the shot is clipped to the same rect at scale 1. The scaler
// below is the seatbelt for the case where they are not — a deck that declares
// its own size, or a panel that had not finished measuring — and it is
// nearest-neighbour on purpose: this is a hand-drawn overlay of a few strokes,
// not a photograph, and a resampler good enough to matter here would be a
// dependency to carry for a case that should not arise.
func overlayInk(slidePNG []byte, marks image.Image) ([]byte, error) {
	base, err := png.Decode(bytes.NewReader(slidePNG))
	if err != nil {
		return nil, fmt.Errorf("อ่านภาพสไลด์ไม่ได้: %w", err)
	}
	b := base.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), base, b.Min, draw.Src)
	draw.Draw(out, out.Bounds(), fitToBounds(marks, out.Bounds()), image.Point{}, draw.Over)

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fitToBounds returns src at the size of dst, unchanged when it already is.
func fitToBounds(src image.Image, dst image.Rectangle) image.Image {
	sb := src.Bounds()
	if sb.Dx() == dst.Dx() && sb.Dy() == dst.Dy() {
		return src
	}
	if sb.Dx() == 0 || sb.Dy() == 0 {
		return image.NewRGBA(dst)
	}
	scaled := image.NewRGBA(image.Rect(0, 0, dst.Dx(), dst.Dy()))
	for y := 0; y < dst.Dy(); y++ {
		sy := sb.Min.Y + y*sb.Dy()/dst.Dy()
		for x := 0; x < dst.Dx(); x++ {
			sx := sb.Min.X + x*sb.Dx()/dst.Dx()
			scaled.Set(x, y, src.At(sx, sy))
		}
	}
	return scaled
}
