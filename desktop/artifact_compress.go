package main

// Making the pictures smaller, on purpose, by the user, from the gallery.
//
// The reason this exists is measurable and was measured before it was written
// (owner's machine, 25 ส.ค.): 46 browser screenshots came to 18.5 MB, and every
// one of them is a PNG of a web page — text on a flat background, the worst
// possible thing to keep in a lossless format. Re-encoding those as PNG returns
// 5–10%. The same files as JPEG return 75–90%: 705 KB becomes 77, 933 becomes
// 190, 510 becomes 48. Nothing else in the toolbox comes close, so nothing else
// is offered.
//
// Downscaling is deliberately NOT here. It was the obvious second lever and the
// measurement killed it: these images are about 1200 px wide already, and
// capping the long side at 1600 produced files byte-for-byte the size of the
// originals. A control that does nothing is worse than a control that is
// missing, because the user has to try it to find out.
//
// The one real hazard is that JPEG has no alpha. An icon or a logo the design
// desk produced is a PNG *because* it has a transparent background, and turning
// it into a JPEG hands the user back a picture with a black or white box behind
// it. So transparency decides the treatment, not the extension and not the
// folder: anything actually transparent is re-encoded as PNG and keeps its
// format, and only a fully opaque image is allowed to become a JPEG.

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// jpegQuality is where the measurement above was taken. High enough that a
// screenshot of text stays readable at 1:1, low enough to be the whole point.
const jpegQuality = 80

// CompressReport is what the gallery shows when the run finishes: how many
// files were rewritten, how many were left exactly as they were, and the two
// totals whose difference is the answer the user asked for.
type CompressReport struct {
	Files   int   `json:"files"`
	Skipped int   `json:"skipped"`
	Before  int64 `json:"before"`
	After   int64 `json:"after"`
	// Error is the first thing that went wrong, if anything did. One file that
	// will not decode must not lose the user the other forty-five.
	Error string `json:"error,omitempty"`
}

// CompressArtifacts rewrites the images among the given paths and reports what
// that gave back.
//
// Every path is checked against the gallery's own roots, exactly as opening and
// deleting are: these arrive over a JS binding and are not trusted. A path that
// is not an image, or that would not come out smaller, is counted as skipped
// and left untouched — "compress" that quietly makes a file bigger is a bug
// wearing a feature's name.
func (a *App) CompressArtifacts(paths []string) (CompressReport, error) {
	var report CompressReport
	for _, raw := range paths {
		full, ok := a.insideOutput(raw)
		if !ok {
			return report, fmt.Errorf("บีบอัดได้เฉพาะไฟล์ในโฟลเดอร์ผลงานเท่านั้น")
		}
		before, after, err := compressImageFile(full)
		switch {
		case err != nil:
			report.Skipped++
			if report.Error == "" {
				report.Error = fmt.Sprintf("%s: %v", filepath.Base(full), err)
			}
		case after == 0:
			report.Skipped++
			report.Before += before
			report.After += before
		default:
			report.Files++
			report.Before += before
			report.After += after
		}
	}
	return report, nil
}

// compressImageFile rewrites one image and answers with its size before and
// after. An `after` of 0 means "left alone" — not an image, or nothing to gain.
func compressImageFile(path string) (before int64, after int64, err error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return 0, 0, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	before = int64(len(raw))

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return before, 0, err
	}

	// The fork: what this picture is allowed to become.
	var (
		out     bytes.Buffer
		newPath = path
	)
	if hasTransparency(img) {
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&out, img); err != nil {
			return before, 0, err
		}
	} else {
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return before, 0, err
		}
		newPath = strings.TrimSuffix(path, filepath.Ext(path)) + ".jpg"
	}

	// Not smaller is not compressed. A JPEG already at this quality, or a PNG
	// that was packed better than Go packs it, keeps every byte it has.
	if int64(out.Len()) >= before {
		return before, 0, nil
	}
	// Renaming onto a name that is taken would eat somebody else's file.
	if newPath != path {
		if _, statErr := os.Stat(newPath); statErr == nil {
			return before, 0, nil
		}
	}
	// Written beside the original and moved into place, so a crash midway
	// leaves the original whole rather than half a file with the right name.
	tmp := newPath + ".compressing"
	if err := os.WriteFile(tmp, out.Bytes(), 0o644); err != nil {
		return before, 0, err
	}
	if err := os.Rename(tmp, newPath); err != nil {
		_ = os.Remove(tmp)
		return before, 0, err
	}
	if newPath != path {
		if err := os.Remove(path); err != nil {
			return before, 0, err
		}
	}
	return before, int64(out.Len()), nil
}

// hasTransparency reports whether any pixel is not fully opaque.
//
// Asked of the decoded image rather than of the file's colour model, because a
// PNG can carry an alpha channel it never uses — every screenshot does — and
// treating those as transparent would rule out the exact files this feature was
// built for. Whole-image, not sampled: a logo can be opaque everywhere except
// its corners, and a sample that misses the corners hands back a JPEG with a
// black square where the transparency was.
func hasTransparency(img image.Image) bool {
	switch pix := img.(type) {
	case *image.RGBA:
		for i := 3; i < len(pix.Pix); i += 4 {
			if pix.Pix[i] != 0xff {
				return true
			}
		}
		return false
	case *image.NRGBA:
		for i := 3; i < len(pix.Pix); i += 4 {
			if pix.Pix[i] != 0xff {
				return true
			}
		}
		return false
	case *image.YCbCr, *image.Gray, *image.CMYK:
		// No alpha channel exists in these at all.
		return false
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, alpha := img.At(x, y).RGBA(); alpha != 0xffff {
				return true
			}
		}
	}
	return false
}
