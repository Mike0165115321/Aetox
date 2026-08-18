package deck

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A deck the way one is actually written: real markup, Thai text, a nested
// list, notes that contain a list of their own.
const sample = `<!doctype html>
<html lang="th"><head><meta charset="utf-8"><style>li::before{content:"x"}</style></head>
<body>
  <section class="slide cover">
    <div class="rule"></div>
    <h1>ต้นทุนลดลง ๔๐% หลังย้ายระบบ</h1>
    <p>ตัวเลขจากไตรมาสล่าสุด</p>
    <aside class="notes">เปิดด้วยตัวเลขนี้เลย อย่าเล่าประวัติบริษัทก่อน</aside>
  </section>
  <section class="slide">
    <h2>สามอย่างที่เปลี่ยน</h2>
    <ul>
      <li>ค่าเครื่องต่อเดือน</li>
      <li>เวลาที่ทีมเสียไปกับการดูแล</li>
      <li>จำนวนครั้งที่ระบบล่ม</li>
    </ul>
    <aside class="notes">
      ถ้าโดนถามเรื่องค่าย้าย ให้ตอบว่า:
      <ul><li>จ่ายครั้งเดียว</li><li>คืนทุนใน ๗ เดือน</li></ul>
    </aside>
  </section>
</body></html>`

func TestSlidesReadsTheContractTable(t *testing.T) {
	slides, err := Slides([]byte(sample), "", "")
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	if len(slides) != 2 {
		t.Fatalf("got %d slides, want 2", len(slides))
	}

	if want := "ต้นทุนลดลง ๔๐% หลังย้ายระบบ"; slides[0].Title != want {
		t.Errorf("title 1 = %q, want %q", slides[0].Title, want)
	}
	if len(slides[0].Bullets) != 0 {
		t.Errorf("slide 1 has no list, so it must have no bullets, got %q", slides[0].Bullets)
	}
	if !strings.Contains(slides[0].Notes, "อย่าเล่าประวัติบริษัท") {
		t.Errorf("notes 1 = %q", slides[0].Notes)
	}

	if want := "สามอย่างที่เปลี่ยน"; slides[1].Title != want {
		t.Errorf("title 2 = %q, want %q", slides[1].Title, want)
	}
	if len(slides[1].Bullets) != 3 {
		t.Fatalf("bullets 2 = %q, want 3", slides[1].Bullets)
	}
	if want := "เวลาที่ทีมเสียไปกับการดูแล"; slides[1].Bullets[1] != want {
		t.Errorf("bullet = %q, want %q", slides[1].Bullets[1], want)
	}
}

// The inversion the contract exists to prevent. Notes carry the sentences and
// the slide carries the landing points; a <ul> the presenter wrote for
// themselves must not climb onto the screen.
func TestListInsideNotesNeverBecomesABullet(t *testing.T) {
	slides, err := Slides([]byte(sample), "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, bullet := range slides[1].Bullets {
		if strings.Contains(bullet, "คืนทุน") {
			t.Fatalf("a note's own list reached the slide as a bullet: %q", bullet)
		}
	}
	if !strings.Contains(slides[1].Notes, "คืนทุนใน ๗ เดือน") {
		t.Errorf("the note lost its list: %q", slides[1].Notes)
	}
}

func TestStylesheetNeverBecomesText(t *testing.T) {
	slides, err := Slides([]byte(sample), "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range slides {
		if strings.Contains(s.Title, "content") || strings.Contains(s.Notes, "content") {
			t.Errorf("css leaked into the slide: %+v", s)
		}
	}
}

// HTML indentation is newlines and tabs. Carrying them through would put the
// author's source formatting on the slide.
func TestWhitespaceCollapses(t *testing.T) {
	src := "<section class=\"slide\"><h1>\n\t  ยอด   ขาย\n  </h1><ul><li>ก<b>ข</b></li></ul></section>"
	slides, err := Slides([]byte(src), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if slides[0].Title != "ยอด ขาย" {
		t.Errorf("title = %q, want %q", slides[0].Title, "ยอด ขาย")
	}
	// Markup is not a word boundary the reader should erase.
	if slides[0].Bullets[0] != "ก ข" {
		t.Errorf("bullet = %q, want %q", slides[0].Bullets[0], "ก ข")
	}
}

func TestOnlyASectionIsASlide(t *testing.T) {
	// A div styled `slide` inside a slide is a styling choice somebody is
	// entitled to make. Cutting on it would split a slide down the middle.
	src := `<section class="slide"><h1>หนึ่ง</h1><div class="slide-inner"><p>x</p></div></section>`
	if n := Count([]byte(src)); n != 1 {
		t.Errorf("Count = %d, want 1", n)
	}
}

func TestClassMatchIsWholeWord(t *testing.T) {
	if Is([]byte(`<section class="slideshow-wrapper"><h1>x</h1></section>`)) {
		t.Error("slideshow-wrapper is not a slide")
	}
	if !Is([]byte(`<section class="dark slide wide"><h1>x</h1></section>`)) {
		t.Error("a marker among other classes is still a marker")
	}
}

func TestAPageIsNotADeck(t *testing.T) {
	if Is([]byte(`<html><body><section><h1>สวัสดี</h1></section></body></html>`)) {
		t.Error("an ordinary page must keep opening as source")
	}
}

// A picture stored beside the deck loads.
//
// This started as a refusal, on the reasoning that the contract requires
// embedded pictures. A real deck proved the reasoning misapplied: the embedding
// rule is about the .html travelling whole, and an export is not the .html
// travelling — the .pptx embeds whatever it is handed either way. Refusing there
// stopped an export that had every byte it needed on the same disk.
func TestAPictureBesideTheDeckIsLoaded(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "chart.png"), 64, 48)

	src := `<section class="slide"><h1>กราฟ</h1><img src="chart.png" alt="ยอดขาย"></section>`
	slides, err := Slides([]byte(src), dir, dir)
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	img := slides[0].Image
	if img == nil {
		t.Fatal("the picture did not arrive")
	}
	if img.WidthPx != 64 || img.HeightPx != 48 {
		t.Errorf("measured %dx%d, want 64x48", img.WidthPx, img.HeightPx)
	}
}

// A URL-encoded name is what an <img src> carries even when it points at a
// local file, so a Thai filename with a space arrives percent-encoded.
func TestAPictureWithAnEncodedNameIsFound(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "รูป 1.png"), 20, 10)

	src := `<section class="slide"><img src="` + url.PathEscape("รูป 1.png") + `"></section>`
	slides, err := Slides([]byte(src), dir, dir)
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	if slides[0].Image == nil {
		t.Fatal("the picture did not arrive")
	}
}

// A picture problem no longer fails the export. The slide keeps its words and
// loses the figure, because the alternative the owner actually hit was a deck
// that would not export at all over one decorative image.
func TestAnUnreadablePictureDoesNotStopTheExport(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{
		"https://example.com/chart.png", // the web: an export does not go online
		"missing.png",                   // simply not there
		"../../../secret.png",           // outside the project
	} {
		src := `<section class="slide"><h1>กราฟ</h1><ul><li>ประเด็น</li></ul>` +
			`<img src="` + bad + `"></section>`
		slides, err := Slides([]byte(src), dir, dir)
		if err != nil {
			t.Errorf("%s stopped the export: %v", bad, err)
			continue
		}
		if slides[0].Image != nil {
			t.Errorf("%s was loaded and should not have been", bad)
		}
		// The words are the point: losing the picture must not lose the slide.
		if slides[0].Title != "กราฟ" || len(slides[0].Bullets) != 1 {
			t.Errorf("%s took the rest of the slide with it: %+v", bad, slides[0])
		}
	}
}

// The boundary is the project rather than the deck's own folder: a deck at
// output/<session>/deck.html pointing at ../assets/logo.png is an ordinary
// layout, not an escape attempt.
func TestAPictureOneFolderUpInsideTheProjectLoads(t *testing.T) {
	root := t.TempDir()
	deckDir := filepath.Join(root, "output", "s1")
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deckDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(root, "assets", "logo.png"), 32, 16)

	src := `<section class="slide"><img src="../../assets/logo.png"></section>`
	slides, err := Slides([]byte(src), deckDir, root)
	if err != nil {
		t.Fatal(err)
	}
	if slides[0].Image == nil {
		t.Fatal("a picture inside the project one folder up was not loaded")
	}
	if slides[0].Image.WidthPx != 32 {
		t.Errorf("measured %d wide, want 32", slides[0].Image.WidthPx)
	}
}

func TestEmbeddedPictureIsDecodedAndMeasured(t *testing.T) {
	src := `<section class="slide"><h1>กราฟ</h1><img alt="ยอดขายรายเดือน" src="data:image/png;base64,` +
		pngDataURI(t, 40, 25) + `"></section>`
	slides, err := Slides([]byte(src), "", "")
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	img := slides[0].Image
	if img == nil {
		t.Fatal("the picture did not arrive")
	}
	// Measured from the bytes, never from the markup: a guessed aspect ratio is
	// a stretched screenshot, the kind of wrong that looks deliberate.
	if img.WidthPx != 40 || img.HeightPx != 25 {
		t.Errorf("measured %dx%d, want 40x25", img.WidthPx, img.HeightPx)
	}
	if img.Ext != "png" {
		t.Errorf("ext = %q, want png", img.Ext)
	}
	if img.AltText != "ยอดขายรายเดือน" {
		t.Errorf("alt = %q", img.AltText)
	}
}

// jpg, never jpeg. ooxml/picture.go's comment is the reason: a content-type
// Default naming image/jpg is the kind of thing PowerPoint accepts and Google
// Slides does not.
func TestJpegNormalisesToJpg(t *testing.T) {
	src := `<section class="slide"><img src="data:image/jpeg;base64,` + jpegDataURI(t) + `"></section>`
	slides, err := Slides([]byte(src), "", "")
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	if slides[0].Image.Ext != "jpg" {
		t.Errorf("ext = %q, want jpg", slides[0].Image.Ext)
	}
}

// A data: URI wrapped across lines by a generator is legal and common. The
// base64 decoder rejects the newlines, so they have to come out first.
func TestWrappedBase64Decodes(t *testing.T) {
	raw := pngDataURI(t, 8, 8)
	var wrapped strings.Builder
	for i, r := range raw {
		if i > 0 && i%20 == 0 {
			wrapped.WriteString("\n      ")
		}
		wrapped.WriteRune(r)
	}
	src := `<section class="slide"><img src="data:image/png;base64,` + wrapped.String() + `"></section>`
	if _, err := Slides([]byte(src), "", ""); err != nil {
		t.Fatalf("a wrapped data URI must still decode: %v", err)
	}
}

// Bytes that are not a picture are skipped like any other unreadable one. The
// slide keeps its words; only the figure is missing, and only from the .pptx —
// .pdf and the image exports go through the renderer, which shows whatever the
// page shows.
func TestBytesThatAreNotAPictureAreSkipped(t *testing.T) {
	notAnImage := base64.StdEncoding.EncodeToString([]byte("%PDF-1.7 this is not a png"))
	src := `<section class="slide"><h1>กราฟ</h1><img src="data:image/png;base64,` + notAnImage + `"></section>`
	slides, err := Slides([]byte(src), "", "")
	if err != nil {
		t.Fatalf("a file lying about its format stopped the export: %v", err)
	}
	if slides[0].Image != nil {
		t.Error("a file lying about its format was embedded anyway")
	}
	if slides[0].Title != "กราฟ" {
		t.Errorf("the slide lost its title too: %+v", slides[0])
	}
}

func TestNoSlidesSaysWhatASlideIs(t *testing.T) {
	_, err := Slides([]byte(`<html><body><h1>ไม่ใช่เด็ค</h1></body></html>`), "", "")
	if err == nil {
		t.Fatal("a page with no slides was read as a deck")
	}
	// The remedy has to be in the message: the caller is often a model, and
	// "no slides" alone gives it nothing to change.
	if !strings.Contains(err.Error(), `<section class="slide">`) {
		t.Errorf("the error must say what a slide is: %v", err)
	}
}

// One picture per slide, because ooxml.Slide holds one. The second must be
// ignored rather than replace the first, or "the picture on the slide" means
// whichever one the author happened to put last.
func TestSecondPictureDoesNotReplaceTheFirst(t *testing.T) {
	src := `<section class="slide">` +
		`<img alt="แรก" src="data:image/png;base64,` + pngDataURI(t, 30, 10) + `">` +
		`<img alt="สอง" src="data:image/png;base64,` + pngDataURI(t, 60, 20) + `">` +
		`</section>`
	slides, err := Slides([]byte(src), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if slides[0].Image.AltText != "แรก" {
		t.Errorf("kept %q, want the first picture", slides[0].Image.AltText)
	}
}

func pngDataURI(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func jpegDataURI(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	img.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 9, G: 9, B: 9, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
