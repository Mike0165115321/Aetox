package model

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// pngOf is a picture of the given size, filled with something that is not one
// flat colour: a downscale of a flat field is right by accident.
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x40, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestFitForWireLeavesASendablePictureAlone(t *testing.T) {
	data := pngOf(t, 640, 480)
	img := Image{MediaType: "image/png", Data: data}
	fitted, note := FitForWire(img)
	if note != "" {
		t.Fatalf("a 640x480 picture needs no note, got %q", note)
	}
	// Byte-identical rather than merely equivalent: a re-encode would miss the
	// provider's prompt cache on a picture that was already fine.
	if !bytes.Equal(fitted.Data, data) {
		t.Fatal("a picture that already fits must come back byte-identical")
	}
}

// The failure this file exists for. 1280 x 10800 is what a full-page capture of
// the owner's fifteen-slide deck measured on 30 ส.ค., and DeepSeek answered
// "You have uploaded an unsupported image" — the height, not the format.
// Shrunk here to keep the test quick; the shape is the same.
func TestFitForWireBoundsATallCapture(t *testing.T) {
	img := Image{MediaType: "image/png", Data: pngOf(t, 128, 10800)}
	fitted, note := FitForWire(img)
	if note == "" {
		t.Fatal("a picture that had to be resized must say so")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(fitted.Data))
	if err != nil {
		t.Fatalf("the fitted picture must still decode: %v", err)
	}
	if cfg.Height > wireMaxSide || cfg.Width > wireMaxSide {
		t.Fatalf("fitted to %dx%d, still over the %d ceiling", cfg.Width, cfg.Height, wireMaxSide)
	}
	// The aspect ratio is the point of the resize: a squashed deck is a
	// different picture, not a smaller one.
	want := float64(128) / float64(10800)
	got := float64(cfg.Width) / float64(cfg.Height)
	if got < want*0.95 || got > want*1.05 {
		t.Fatalf("aspect ratio %v drifted from %v", got, want)
	}
	if fitted.MediaType != "image/png" {
		t.Fatalf("media type %q", fitted.MediaType)
	}
}

// A format this binary has no decoder for is not a broken picture. webp is the
// live case: providers take it, Go's standard library does not read it, and
// re-encoding it here would be the only way to damage it.
func TestFitForWireLeavesWhatItCannotDecode(t *testing.T) {
	data := []byte("RIFF????WEBPVP8 not really a webp")
	img := Image{MediaType: "image/webp", Data: data}
	fitted, note := FitForWire(img)
	if note != "" || !bytes.Equal(fitted.Data, data) || fitted.MediaType != "image/webp" {
		t.Fatalf("undecodable bytes must pass through untouched, got note %q type %q", note, fitted.MediaType)
	}
}

func TestFitForWireEmpty(t *testing.T) {
	fitted, note := FitForWire(Image{MediaType: "image/png"})
	if note != "" || len(fitted.Data) != 0 {
		t.Fatal("no picture, nothing to fit and nothing to say")
	}
}
