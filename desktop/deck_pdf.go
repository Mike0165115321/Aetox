package main

// Printing a deck to PDF. The webview it prints through is deck_render.go's.

import (
	"context"
	"encoding/base64"
	"fmt"
)

// The slide box in inches: 1280x720 CSS pixels at 96dpi. See deck_render.go.
const (
	deckPageWidthInches  = 13.333
	deckPageHeightInches = 7.5
	// CSS pixels per inch. Not a screen's real DPI — the number both Chromium's
	// print pipeline and OOXML assume, which is why 1280x720 is exactly the
	// 16:9 that ooxml.slideWidth has always meant.
	cssPixelsPerInch = 96.0
)

// deckPrintParams is Page.printToPDF's parameter object.
//
// Two of these decide whether the export is usable at all:
//
//   - printBackground. Off by default, and off means every slide with a colour
//     behind it prints white. A deck's section dividers are the first thing to
//     vanish, and the file still opens fine, so nothing anywhere says why it
//     came out wrong.
//   - displayHeaderFooter. On means Chromium stamps the file:// URL and today's
//     date across every slide.
//
// The margins are zero because the slide is the page: the deck's own CSS
// already carries `@page { size: 1280px 720px; margin: 0 }`, and a margin here
// would shrink the artwork inside its own paper.
func deckPrintParams(slideW, slideH float64) string {
	// preferCSSPageSize is off on purpose. It hands the decision to whatever
	// `@page` rule the deck happens to carry, and a deck written for the screen
	// usually carries none or carries A4 — either way the paper stops matching
	// the artwork. The measured slide is the one size that always does.
	w, h := deckPageWidthInches, deckPageHeightInches
	if slideW > 0 && slideH > 0 {
		w, h = slideW/cssPixelsPerInch, slideH/cssPixelsPerInch
	}
	return fmt.Sprintf(
		`{"printBackground":true,"displayHeaderFooter":false,"preferCSSPageSize":false,`+
			`"paperWidth":%g,"paperHeight":%g,`+
			`"marginTop":0,"marginBottom":0,"marginLeft":0,"marginRight":0,"scale":1}`,
		w, h)
}

// exportDeckPDF returns the deck printed to PDF bytes.
func (a *App) exportDeckPDF(ctx context.Context, fileURL string) ([]byte, error) {
	var pdf []byte
	err := a.withExportTab(ctx, fileURL, func(call engineCaller) error {
		// Reveal before flatten, and both before printing. Flattening fixes
		// where the slides are; revealing fixes whether they have anything on
		// them. A deck that draws its content on scroll prints slides two
		// onward as their static chrome and nothing else — see deck_reveal.go.
		if _, err := revealEverything(call); err != nil {
			return err
		}
		rects, err := flattenForExport(call)
		if err != nil {
			return err
		}
		// The page is the deck's own slide, measured, not a number from a
		// constant. A deck whose slides are a few pixels taller than the paper
		// spills a sliver onto a page of its own — which is how eight slides
		// came out as eleven pages.
		raw, err := call("Page.printToPDF", deckPrintParams(rects[0].W, rects[0].H))
		if err != nil {
			return err
		}
		data, err := engineData(raw)
		if err != nil {
			return err
		}
		pdf, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			return fmt.Errorf("the printed bytes did not decode: %w", err)
		}
		if len(pdf) == 0 {
			return fmt.Errorf("เครื่องพิมพ์ออกมาเป็นไฟล์เปล่า")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pdf, nil
}
