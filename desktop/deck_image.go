package main

// Photographing a deck, one picture per slide.
//
// Unlike printing, this needs the page actually rendered, which is what kept it
// off the export menu: a hidden WebView2 paints nothing (browser_capture.go;
// WebView2Feedback #1077, #2983). The export webview is parked outside every
// monitor rather than hidden, and a window in that state composites normally —
// see deck_render.go's header for the full argument.
//
// That argument is reasoning, not documentation, so it is not trusted. Every
// capture is checked for being a single flat colour before it is written
// (notBlank). If the reasoning is wrong on some machine, the user gets an error
// naming the slide instead of a folder of blank rectangles — which is the whole
// difference between a feature that failed and a feature that lied.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strings"
)

// Slide positions are measured, never computed. The obvious arithmetic — slide
// N is at y = N*720 — holds only for a deck whose CSS stacks slides with no gap,
// and the contract deliberately says how big a slide is without saying how a
// deck arranges them. Measuring is flattenForExport's job (deck_flatten.go),
// because the same pass that puts the slides in flow is the one that can say
// where they landed.

type slideRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// deckImageFormat is one picture format the export menu can offer.
type deckImageFormat struct {
	cdp     string // "png" or "jpeg" — the protocol's own spelling
	quality int    // jpeg only; 0 leaves it out
}

var deckImageFormats = map[string]deckImageFormat{
	"png": {cdp: "png"},
	// 92 is the point where the next percent costs more bytes than it returns
	// visible quality on flat, text-heavy artwork, which is what a slide is.
	"jpg": {cdp: "jpeg", quality: 92},
	// Smaller than png at the same visible quality on flat, text-heavy artwork,
	// which is what a slide is. Lossless here (no quality field): webp'"'"'s lossy
	// mode saves little on this kind of image and costs the crispness of type.
	"webp": {cdp: "webp"},
}

// exportDeckImages returns one picture per slide, in slide order.
func (a *App) exportDeckImages(ctx context.Context, fileURL, format string) ([][]byte, error) {
	spec, ok := deckImageFormats[strings.ToLower(strings.TrimSpace(format))]
	if !ok {
		return nil, fmt.Errorf("ไม่รู้จักรูปแบบภาพ %q", format)
	}

	var shots [][]byte
	err := a.withExportTab(ctx, fileURL, func(call engineCaller) error {
		// Same two passes the PDF export makes, and needed here for the same
		// two reasons: revealing so a slide has its content, flattening so it
		// has its own place. Without the first, seven of eight pictures come
		// back as the deck's static chrome on an empty background — a failure
		// notBlank cannot catch, because a header on a background is not blank.
		if _, err := revealEverything(call); err != nil {
			return err
		}
		rects, err := flattenForExport(call)
		if err != nil {
			return err
		}
		for i, r := range rects {
			shot, err := captureSlide(call, spec, r)
			if err != nil {
				return fmt.Errorf("สไลด์ %d: %w", i+1, err)
			}
			shots = append(shots, shot)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return shots, nil
}

// captureParams is Page.captureScreenshot's parameter object for one slide.
//
// captureBeyondViewport is what makes slide 8 reachable without scrolling to
// it: the clip addresses the whole document rather than whatever happens to be
// on screen, so every slide is photographed from one page state. Without it a
// deck taller than the viewport comes back with everything below the fold blank
// — and blank is exactly the failure notBlank exists to catch, which would make
// a missing flag look like a broken machine.
func captureParams(spec deckImageFormat, r slideRect) string {
	params := fmt.Sprintf(
		`{"format":%q,"captureBeyondViewport":true,`+
			`"clip":{"x":%g,"y":%g,"width":%g,"height":%g,"scale":1}`,
		spec.cdp, r.X, r.Y, r.W, r.H)
	if spec.quality > 0 {
		params += fmt.Sprintf(`,"quality":%d`, spec.quality)
	}
	return params + "}"
}

func captureSlide(call engineCaller, spec deckImageFormat, r slideRect) ([]byte, error) {
	raw, err := call("Page.captureScreenshot", captureParams(spec, r))
	if err != nil {
		return nil, err
	}
	data, err := engineData(raw)
	if err != nil {
		return nil, err
	}
	shot, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("ภาพที่ได้ถอดรหัสไม่ออก: %w", err)
	}
	if err := notBlank(shot); err != nil {
		return nil, err
	}
	return shot, nil
}

// notBlank rejects a capture that is one flat colour.
//
// This is the guard on deck_render.go's central claim — that a window parked
// off-screen still composites. If that is ever wrong, WebView2 does not fail:
// it hands back a perfectly valid picture of nothing, and the export writes a
// folder of blank rectangles and reports success. That is the failure this
// codebase keeps writing down, where the honest answer was "I could not".
//
// A real slide always has something on it: the contract's own anatomy starts
// with a heading, and a section divider is at minimum a title on a background.
// A rendered slide with exactly one colour in it does not occur.
func notBlank(shot []byte) error {
	img, _, err := image.Decode(bytes.NewReader(shot))
	if err != nil {
		return fmt.Errorf("ภาพที่ได้ไม่ใช่ไฟล์ภาพที่อ่านได้: %w", err)
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return errors.New("ภาพที่ได้กว้างหรือสูงเป็นศูนย์")
	}
	firstR, firstG, firstB, _ := img.At(b.Min.X, b.Min.Y).RGBA()
	// A grid rather than every pixel: a 1280x720 slide is nearly a million
	// pixels per slide and the answer is the same after a few hundred. Stepping
	// by a prime-ish stride avoids landing only on a repeating background.
	const step = 37
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r != firstR || g != firstG || bl != firstB {
				return nil
			}
		}
	}
	return errors.New("ตัวเรนเดอร์ส่งภาพเปล่ากลับมา — เครื่องนี้อาจไม่วาดหน้าต่างที่อยู่นอกจอ")
}

// mustJSONString quotes a string for embedding in a JSON literal. The script
// above is a constant, so the only way this fails is a programming error.
func mustJSONString(s string) string {
	quoted, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(quoted)
}
