package model

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// wireMaxSide is the longest side a picture may have when it goes on the wire,
// in pixels.
//
// It is not a guess and it is not Aetox's preference: it is the smallest
// per-side ceiling among the providers in the picker, so a picture that clears
// it clears all of them. DeepSeek documents 8192 px per side (and 4096 once a
// request carries fifteen pictures or more); Anthropic refuses above 8000.
// 8000 is therefore the number that is true everywhere.
//
// It cost a turn to learn. On 30 ส.ค. a full-page capture of a fifteen-slide
// deck came back 1280 x 10800 and DeepSeek answered
// "messages.66.content[2].image[0]: You have uploaded an unsupported image" —
// a 400 that names a format problem for a picture that is a perfectly valid
// PNG. Nothing in Aetox bounded the height: the capture path allowed 16384 px
// and every producer handed its bytes straight to the provider.
//
// The second half of that failure is why this is prevention and not only a
// nicer error. The rejected picture stayed in the history, so the next turn the
// owner typed died on the same 400 before it reached the model at all, and so
// would every turn after it. See IsImageRejection for the way out of a
// conversation that is already holding one.
const wireMaxSide = 8000

// FitForWire bounds a picture to what every provider will accept, and answers
// with a note when it had to change it.
//
// Three ways out, in order:
//
//   - The bytes do not decode here. Left exactly as they are, because "this
//     binary has no decoder" is not "this picture is broken": webp is the case
//     that matters, and re-encoding it would be the one thing that could damage
//     it. Same call CharCost makes on the same bytes for the same reason.
//   - It already fits. Returned untouched — byte-identical, so a provider's
//     prompt cache still recognises a picture it has already been sent.
//   - It is too big. Scaled down, keeping the aspect ratio, and re-encoded as
//     PNG.
//
// The note is for the caller to hand to the model. A picture that arrived
// smaller than it was taken is a fact the model needs: it is about to measure
// something in it.
func FitForWire(img Image) (Image, string) {
	if len(img.Data) == 0 {
		return img, ""
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return img, ""
	}
	if cfg.Width <= wireMaxSide && cfg.Height <= wireMaxSide {
		return img, ""
	}

	src, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		// The header parsed and the pixels did not. Nothing to do but send what
		// we were given: this is already the shape a provider will reject, and
		// a silent drop would be worse than its error message.
		return img, ""
	}
	dst := downscaleToFit(src, wireMaxSide)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return img, ""
	}
	bounds := dst.Bounds()
	note := fmt.Sprintf(
		"ภาพนี้ขนาดจริง %d x %d พิกเซล เกินด้านละ %d ที่ผู้ให้บริการรับได้ จึงย่อเหลือ %d x %d ก่อนส่ง",
		cfg.Width, cfg.Height, wireMaxSide, bounds.Dx(), bounds.Dy())
	return Image{MediaType: "image/png", Data: buf.Bytes()}, note
}

// downscaleToFit shrinks src so that neither side is longer than maxSide,
// keeping the aspect ratio.
//
// A box filter — every destination pixel is the average of the source pixels it
// covers — rather than nearest-neighbour, because the pictures that reach here
// are screenshots of text, and dropping rows out of a line of text turns it
// into a different line of text. Averaging blurs; sampling invents.
//
// Written out rather than taken from golang.org/x/image/draw. One dependency to
// scale one screenshot per capture is a poor trade in a binary that ships with
// as few as this one does, and the loop below is the whole of what that package
// would be used for here.
func downscaleToFit(src image.Image, maxSide int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	long := w
	if h > long {
		long = h
	}
	scale := float64(maxSide) / float64(long)
	dw, dh := int(float64(w)*scale), int(float64(h)*scale)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0 := b.Min.Y + y*h/dh
		y1 := b.Min.Y + (y+1)*h/dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < dw; x++ {
			x0 := b.Min.X + x*w/dw
			x1 := b.Min.X + (x+1)*w/dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					pr, pg, pb, pa := src.At(sx, sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					bl += uint64(pb)
					a += uint64(pa)
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.Set(x, y, color.RGBA64{
				R: uint16(r / n),
				G: uint16(g / n),
				B: uint16(bl / n),
				A: uint16(a / n),
			})
		}
	}
	return dst
}
