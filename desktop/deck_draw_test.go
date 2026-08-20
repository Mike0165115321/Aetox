package main

// The half of the drawing that is arithmetic.
//
// Rendering the slide needs a webview and is covered by the export tests; what
// is here is the part that decides whether the marks land on it correctly, and
// that part is pure.

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func pngOf(w, h int, fill color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func inkURL(b []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
}

// The slide keeps its size and the marks sit on top of it. Both halves matter:
// a picture that came back the size of the ink layer would be a picture of the
// panel rather than of the slide.
func TestTheMarksGoOverTheSlideAndTheSlideKeepsItsSize(t *testing.T) {
	slide := pngOf(1280, 720, color.RGBA{20, 22, 26, 255})

	marks := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	marks.Set(640, 360, color.RGBA{255, 59, 48, 255}) // one red pixel, mid-slide
	var inkBuf bytes.Buffer
	if err := png.Encode(&inkBuf, marks); err != nil {
		t.Fatal(err)
	}
	ink, err := decodeInk(inkURL(inkBuf.Bytes()))
	if err != nil {
		t.Fatalf("decodeInk: %v", err)
	}

	out, err := overlayInk(slide, ink)
	if err != nil {
		t.Fatalf("overlayInk: %v", err)
	}
	got, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("the result is not a PNG: %v", err)
	}
	if b := got.Bounds(); b.Dx() != 1280 || b.Dy() != 720 {
		t.Errorf("the picture is %dx%d; the slide is 1280x720", b.Dx(), b.Dy())
	}
	if r, _, _, _ := got.At(640, 360).RGBA(); r>>8 < 200 {
		t.Errorf("the mark is not on the picture: got %v at the point it was drawn", got.At(640, 360))
	}
	// Everywhere else is still the slide. An overlay that painted its own
	// transparent background over the top would come back as a blank rectangle,
	// which is the failure the export path already refuses to write out.
	if r, g, b, _ := got.At(10, 10).RGBA(); r>>8 != 20 || g>>8 != 22 || b>>8 != 26 {
		t.Errorf("the slide under the marks was painted over: %v", got.At(10, 10))
	}
}

// The seatbelt: a deck that declares its own size renders bigger than the box
// the marks were drawn on, and the marks still have to cover it.
func TestMarksDrawnAtOneSizeStillCoverASlideOfAnother(t *testing.T) {
	slide := pngOf(1920, 1080, color.RGBA{0, 0, 0, 255})
	marks := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			marks.Set(x, y, color.RGBA{255, 59, 48, 255})
		}
	}

	out, err := overlayInk(slide, marks)
	if err != nil {
		t.Fatalf("overlayInk: %v", err)
	}
	got, _ := png.Decode(bytes.NewReader(out))
	if b := got.Bounds(); b.Dx() != 1920 || b.Dy() != 1080 {
		t.Fatalf("the picture is %dx%d; the slide is 1920x1080", b.Dx(), b.Dy())
	}
	// The far corner is the half that proves it: an unscaled overlay would have
	// stopped at 1280x720 and left the rest of the slide bare.
	if r, _, _, _ := got.At(1900, 1060).RGBA(); r>>8 < 200 {
		t.Errorf("the marks did not reach the corner of the slide: %v", got.At(1900, 1060))
	}
}

func TestDecodeInkRefusesWhatIsNotADrawing(t *testing.T) {
	if _, err := decodeInk("data:image/jpeg;base64,AAAA"); err == nil {
		t.Error("a jpeg was accepted; only PNG carries the transparency this needs")
	}
	if _, err := decodeInk("not a data url"); err == nil {
		t.Error("a bare string was accepted")
	}
	if _, err := decodeInk("data:image/png;base64,!!!!"); err == nil {
		t.Error("unreadable base64 was accepted")
	}
	// Sized before it is decoded, so a hostile payload never becomes an image.
	big := "data:image/png;base64," + strings.Repeat("A", maxInkBytes+8)
	if _, err := decodeInk(big); err == nil {
		t.Error("an oversized payload was accepted")
	}
}
