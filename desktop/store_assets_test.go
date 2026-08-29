package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The Microsoft Store package's icon set, checked against the .ico it is made
// from (DECISIONS §207).
//
// Owner, 29 ส.ค., with the Store build and the installer build side by side on
// one taskbar: "ไอที่ขอบแดงๆพังๆอ่ะ คือโปรแกรมเราที่โหลด จาก ไมโครซอฟสโตร ครับ
// แต่พอโหลดของเราตรงๆกลับไม่เป็น". Same app, same mark, and only the Store copy
// had a frayed edge — because the package shipped one 44x44 PNG and Windows
// rescaled it to every taskbar size, while icon.ico hands the installed build
// a hand-drawn frame at each of those sizes.
//
// These live in Go rather than in the release workflow on purpose. The
// workflow runs on a tag, which is the last moment anybody wants to discover
// that the icons are wrong; `go test ./...` runs on every push.

const storeAssetsDir = "build/windows/msix/Assets"

// taskbarSizes are the sizes Windows asks for by pixel. 24 and 32 are the
// taskbar at 100%; 30, 36, 40 and 48 are the same taskbar at 125%, 150%, 175%
// and 200%, which is where most people actually run.
var taskbarSizes = []int{16, 20, 24, 30, 32, 36, 40, 48, 56, 60, 64, 72, 80, 96, 256}

func TestTheStorePackageCarriesEveryIconSizeTheTaskbarAsksFor(t *testing.T) {
	for _, size := range taskbarSizes {
		for _, form := range []string{"", "_altform-unplated"} {
			name := storeAssetName(size, form)
			img := decodeStoreAsset(t, name)
			if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
				t.Errorf("%s is %dx%d, want %dx%d", name, b.Dx(), b.Dy(), size, size)
			}
		}
	}
}

// The unplated half is not a nicety. The taskbar and Alt-Tab draw the icon with
// no plate behind it and ask for that variant by name; without it Windows falls
// back to whatever else it can find and rescales.
func TestEveryStoreIconSizeHasItsUnplatedTwin(t *testing.T) {
	for _, size := range taskbarSizes {
		plated := decodeStoreAsset(t, storeAssetName(size, ""))
		unplated := decodeStoreAsset(t, storeAssetName(size, "_altform-unplated"))
		if plated.Bounds() != unplated.Bounds() {
			t.Errorf("targetsize-%d and its unplated twin are different sizes", size)
		}
	}
}

// The point of the whole change: where icon.ico already carries a frame at
// exactly this size, the Store asset must BE that frame rather than a rescale
// of a bigger one. This is the difference the owner could see.
func TestStoreIconsReuseTheHandDrawnFrameWhereThereIsOne(t *testing.T) {
	frames := icoFrames(t, filepath.Join("build", "windows", "icon.ico"))
	checked := 0
	for _, size := range taskbarSizes {
		frame, ok := frames[size]
		if !ok {
			continue // no frame at this size; the asset is a resize and that is fine
		}
		asset := decodeStoreAsset(t, storeAssetName(size, ""))
		if diff := pixelsDiffering(frame, asset); diff != 0 {
			t.Errorf("targetsize-%d differs from icon.ico's own %dx%d frame in %d pixels — it was resampled when it did not have to be",
				size, size, size, diff)
		}
		checked++
	}
	// A guard on the guard: if the .ico is ever rebuilt with only one frame,
	// the loop above would pass by checking nothing at all.
	if checked < 5 {
		t.Errorf("only %d sizes were checked against icon.ico — the icon has lost its small frames", checked)
	}
}

// A package with no resources.pri ignores every qualifier in these filenames
// and reads the one unqualified file the manifest names, which is exactly the
// state this all started in. The config that builds the index has to exist for
// the release workflow to have anything to point at.
func TestTheStorePackageHasAPriConfigToIndexWith(t *testing.T) {
	path := filepath.Join("build", "windows", "msix", "priconfig.xml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}

	// makepri parses this file with something that is not an XML parser: an
	// element name inside a comment is read as an element, and the whole file
	// then fails with "resources node not found" — a message about the one
	// node that is unmistakably there. Cost an afternoon once.
	for _, comment := range xmlComments(body) {
		if bytes.Contains(comment, []byte("<")) {
			t.Errorf("priconfig.xml has an angle bracket inside a comment — makepri will reject the whole file: %.60s", comment)
		}
	}

	// An autoResourcePackage split sends everything above scale-100 into
	// resource packages that this repo never uploads. Asked of the directives
	// only: the comment above them explains the trap by name, and a test that
	// cannot tell an explanation from an instruction is not reading the file,
	// it is grepping it.
	if directives := stripXMLComments(body); bytes.Contains(directives, []byte("autoResourcePackage")) {
		t.Error("priconfig.xml asks for resource packages — the single .msix would ship scale-100 only")
	}
}

// xmlComments returns the inside of each <!-- --> block.
func xmlComments(body []byte) [][]byte {
	var out [][]byte
	for rest := body; ; {
		i := bytes.Index(rest, []byte("<!--"))
		if i < 0 {
			return out
		}
		rest = rest[i+4:]
		j := bytes.Index(rest, []byte("-->"))
		if j < 0 {
			return append(out, rest)
		}
		out = append(out, rest[:j])
		rest = rest[j+3:]
	}
}

func stripXMLComments(body []byte) []byte {
	var out []byte
	for rest := body; ; {
		i := bytes.Index(rest, []byte("<!--"))
		if i < 0 {
			return append(out, rest...)
		}
		out = append(out, rest[:i]...)
		rest = rest[i+4:]
		j := bytes.Index(rest, []byte("-->"))
		if j < 0 {
			return out
		}
		rest = rest[j+3:]
	}
}

func storeAssetName(size int, form string) string {
	return fmt.Sprintf("Square44x44Logo.targetsize-%d%s.png", size, form)
}

func decodeStoreAsset(t *testing.T, name string) image.Image {
	t.Helper()
	path := filepath.Join(storeAssetsDir, name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%s is missing — re-run desktop/build/windows/msix/make-logos.ps1: %v", path, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("%s will not decode: %v", path, err)
	}
	return img
}

// icoFrames reads the PNG-compressed frames out of an .ico, keyed by width.
// Every frame this repo ships is PNG-compressed; a BMP one is skipped rather
// than decoded, because the only thing that would buy is a test that fails on
// an icon nobody has made yet.
func icoFrames(t *testing.T, path string) map[int]image.Image {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(raw) < 6 || binary.LittleEndian.Uint16(raw[2:]) != 1 {
		t.Fatalf("%s is not an .ico", path)
	}
	out := map[int]image.Image{}
	count := int(binary.LittleEndian.Uint16(raw[4:]))
	for i := range count {
		o := 6 + i*16
		if o+16 > len(raw) {
			break
		}
		// 0 in the width byte means 256 — the field is one byte wide.
		w := int(raw[o])
		if w == 0 {
			w = 256
		}
		size := int(binary.LittleEndian.Uint32(raw[o+8:]))
		off := int(binary.LittleEndian.Uint32(raw[o+12:]))
		if off+size > len(raw) {
			continue
		}
		blob := raw[off : off+size]
		if !bytes.HasPrefix(blob, []byte("\x89PNG")) {
			continue
		}
		img, err := png.Decode(bytes.NewReader(blob))
		if err != nil {
			continue
		}
		out[w] = img
	}
	if len(out) == 0 {
		t.Fatalf("%s carried no readable frame", path)
	}
	return out
}

func pixelsDiffering(a, b image.Image) int {
	if a.Bounds() != b.Bounds() {
		return a.Bounds().Dx() * a.Bounds().Dy()
	}
	n := 0
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb || aa != ba {
				n++
			}
		}
	}
	return n
}
