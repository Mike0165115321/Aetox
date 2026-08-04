package ooxml

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func buildDeck(t *testing.T, slides []Slide) map[string]string {
	t.Helper()
	parts, err := BuildPPTX(slides)
	if err != nil {
		t.Fatalf("BuildPPTX: %v", err)
	}
	data, err := WritePackage(parts)
	if err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	// openPackage (xlsx_test.go) is where the real structural checks live: it
	// unzips and parses every part, so a malformed slide fails here regardless
	// of what this test then asserts.
	return openPackage(t, data)
}

func pngFixture(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// PowerPoint does not name the missing piece — it offers to repair the file and
// then shows an empty deck — so the required parts are asserted where the
// failure can say which one is absent.
func TestDeckHasEveryRequiredPart(t *testing.T) {
	parts := buildDeck(t, []Slide{{Title: "หนึ่ง"}})
	for _, required := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"ppt/presentation.xml",
		"ppt/_rels/presentation.xml.rels",
		"ppt/slideMasters/slideMaster1.xml",
		"ppt/slideMasters/_rels/slideMaster1.xml.rels",
		"ppt/slideLayouts/slideLayout1.xml",
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels",
		"ppt/theme/theme1.xml",
		"ppt/slides/slide1.xml",
		"ppt/slides/_rels/slide1.xml.rels",
	} {
		if _, ok := parts[required]; !ok {
			t.Errorf("missing required part %s", required)
		}
	}
	// A theme that is never related from the master is a theme PowerPoint
	// ignores, and the Thai font setting below rides on it.
	if !strings.Contains(parts["ppt/slideMasters/_rels/slideMaster1.xml.rels"], `Target="../theme/theme1.xml"`) {
		t.Error("the slide master does not reference the theme")
	}
}

// Every part named in the manifest must exist, every relationship must resolve,
// and no two relationships in one file may share an id. A dangling link in
// either direction is the usual cause of "repair this presentation".
func TestDeckContentTypesAndRelationshipsResolve(t *testing.T) {
	parts := buildDeck(t, []Slide{
		{Title: "หนึ่ง", Bullets: []string{"a"}},
		{Title: "สอง", Notes: "พูดถึงตัวเลขไตรมาสสาม"},
		{Title: "สาม"},
	})

	types := parts["[Content_Types].xml"]
	for name := range parts {
		if name == "[Content_Types].xml" || strings.HasSuffix(name, ".rels") || strings.HasPrefix(name, "ppt/media/") {
			continue
		}
		if !strings.Contains(types, `PartName="/`+name+`"`) {
			t.Errorf("%s has no Override in [Content_Types].xml", name)
		}
	}

	rels := parts["ppt/_rels/presentation.xml.rels"]
	for i := 1; i <= 3; i++ {
		target := fmt.Sprintf("slides/slide%d.xml", i)
		if !strings.Contains(rels, `Target="`+target+`"`) {
			t.Errorf("no relationship targets %s", target)
		}
	}
	for id := 1; id <= 6; id++ {
		if n := strings.Count(rels, fmt.Sprintf(`Id="rId%d"`, id)); n > 1 {
			t.Errorf("relationship id rId%d appears %d times in presentation rels", id, n)
		}
	}
	// Slide ids below 256 are rejected outright.
	for i := 1; i <= 3; i++ {
		if !strings.Contains(parts["ppt/presentation.xml"], fmt.Sprintf(`id="%d"`, 255+i)) {
			t.Errorf("slide %d has no sldId at or above 256", i)
		}
	}
}

// The risk OFFICE-EXPORT-PLAN.md §7 names first. Thai is a complex script: with
// no `cs` typeface PowerPoint chooses a complex-script font by itself, and the
// one it chooses routinely has different metrics from the Latin face — which is
// what leaves vowels and tone marks floating beside their consonants.
func TestThaiTextNamesAComplexScriptFont(t *testing.T) {
	parts := buildDeck(t, []Slide{{
		Title:   "สรุปยอดขายไตรมาส ๓",
		Bullets: []string{"ยอดรวมโตขึ้น ๑๒%", "ลูกค้าใหม่ ๔๘ ราย"},
		Notes:   "เน้นว่ากำไรมาจากลูกค้าเก่า",
	}})
	slide := parts["ppt/slides/slide1.xml"]

	if !strings.Contains(slide, `<a:cs typeface="+mn-cs"/>`) {
		t.Errorf("no complex-script typeface on the runs — Thai marks will drift:\n%s", slide)
	}
	if !strings.Contains(slide, "สรุปยอดขายไตรมาส ๓") {
		t.Errorf("Thai title did not survive:\n%s", slide)
	}
	if !strings.Contains(slide, "ยอดรวมโตขึ้น ๑๒%") {
		t.Errorf("Thai bullet did not survive:\n%s", slide)
	}
	if !strings.Contains(parts["ppt/notesSlides/notesSlide1.xml"], "เน้นว่ากำไรมาจากลูกค้าเก่า") {
		t.Error("Thai speaker notes did not survive")
	}
	// The theme is what `+mn-cs` resolves through, so the script mapping has to
	// be there or the reference points at nothing.
	theme := parts["ppt/theme/theme1.xml"]
	if !strings.Contains(theme, `<a:font script="Thai"`) {
		t.Error("the theme has no Thai script font, so +mn-cs resolves to whatever PowerPoint picks")
	}
}

// Notes are a separate part with its own relationships in both directions, and
// a notesSlide with no notesMaster is the case PowerPoint is least forgiving of.
func TestSpeakerNotesAreWiredBothWays(t *testing.T) {
	parts := buildDeck(t, []Slide{{Title: "one"}, {Title: "two", Notes: "say this"}})

	if _, ok := parts["ppt/notesMasters/notesMaster1.xml"]; !ok {
		t.Fatal("a deck with notes has no notes master")
	}
	if !strings.Contains(parts["ppt/presentation.xml"], "<p:notesMasterIdLst>") {
		t.Error("the presentation does not list its notes master")
	}
	notesRels := parts["ppt/notesSlides/_rels/notesSlide2.xml.rels"]
	if !strings.Contains(notesRels, `Target="../slides/slide2.xml"`) {
		t.Error("the notes slide does not point back at its slide")
	}
	if !strings.Contains(notesRels, `Target="../notesMasters/notesMaster1.xml"`) {
		t.Error("the notes slide does not reference the notes master")
	}
	if !strings.Contains(parts["ppt/slides/_rels/slide2.xml.rels"], `Target="../notesSlides/notesSlide2.xml"`) {
		t.Error("the slide does not point at its notes")
	}
	// The presenter finds the text through the `body` placeholder; a plain text
	// box would store the note and show nothing.
	if !strings.Contains(parts["ppt/notesSlides/notesSlide2.xml"], `<p:ph type="body" idx="1"/>`) {
		t.Error("the note is not in a body placeholder, so the notes pane stays empty")
	}
}

// Each master owns its own theme part. The notes master sharing theme1 with
// the slide master looks legal on paper — OPC allows two relationships to one
// part — and PowerPoint answers it with "พบปัญหากับเนื้อหา" and a repair
// prompt, on every deck that has speaker notes. Found by bisecting variants
// through a live PowerPoint via COM (2026-08-04): giving the notes master a
// byte-identical theme2.xml was the single change from repair-prompt to
// opens-clean.
func TestNotesMasterOwnsItsOwnThemePart(t *testing.T) {
	parts := buildDeck(t, []Slide{{Title: "one", Notes: "say this"}})

	if _, ok := parts["ppt/theme/theme2.xml"]; !ok {
		t.Fatal("a deck with notes has no theme2.xml — the notes master is sharing the slide master's theme, which PowerPoint treats as a corrupt file")
	}
	if parts["ppt/theme/theme2.xml"] != parts["ppt/theme/theme1.xml"] {
		t.Error("theme2 drifted from theme1 — the two masters should present one design")
	}
	if !strings.Contains(parts["ppt/notesMasters/_rels/notesMaster1.xml.rels"], `Target="../theme/theme2.xml"`) {
		t.Error("the notes master does not point at its own theme part")
	}
	if !strings.Contains(parts["[Content_Types].xml"], `PartName="/ppt/theme/theme2.xml"`) {
		t.Error("theme2.xml has no content-type override, so it is not a theme to any reader")
	}

	// No notes → no second theme; the part rides with the notes master only.
	bare := buildDeck(t, []Slide{{Title: "no notes"}})
	if _, ok := bare["ppt/theme/theme2.xml"]; ok {
		t.Error("a deck without notes shipped a second theme it has no master for")
	}
}

// A deck with no notes must not carry a notes master: an empty notesMasterIdLst
// referencing a part that is not there is exactly the dangling link that makes
// PowerPoint offer to repair the file.
func TestADeckWithoutNotesCarriesNoNotesMaster(t *testing.T) {
	parts := buildDeck(t, []Slide{{Title: "just a title"}})
	if _, ok := parts["ppt/notesMasters/notesMaster1.xml"]; ok {
		t.Error("a deck with no notes still shipped a notes master")
	}
	if strings.Contains(parts["ppt/presentation.xml"], "notesMasterIdLst") {
		t.Error("the presentation lists a notes master it does not have")
	}
}

func TestImageIsEmbeddedWithItsContentTypeAndRelationship(t *testing.T) {
	parts := buildDeck(t, []Slide{{
		Title: "chart",
		Image: &SlideImage{Ext: "png", Data: pngFixture(t, 800, 600), WidthPx: 800, HeightPx: 600},
	}})

	if _, ok := parts["ppt/media/image1.png"]; !ok {
		t.Fatal("the picture was not embedded — the deck would open with a broken frame")
	}
	if !strings.Contains(parts["[Content_Types].xml"], `<Default Extension="png" ContentType="image/png"/>`) {
		t.Error("png has no Default content type, which PowerPoint rejects")
	}
	if !strings.Contains(parts["ppt/slides/_rels/slide1.xml.rels"], `Target="../media/image1.png"`) {
		t.Error("nothing relates the slide to its picture")
	}
	if !strings.Contains(parts["ppt/slides/slide1.xml"], `<a:blip r:embed="rId2"/>`) {
		t.Error("the slide has no picture fill pointing at the image relationship")
	}
}

// A jpeg Default naming image/jpg rather than image/jpeg is the kind of thing
// PowerPoint tolerates and Google Slides does not.
func TestJpegDeclaresTheRightContentType(t *testing.T) {
	parts := buildDeck(t, []Slide{{
		Image: &SlideImage{Ext: "jpg", Data: []byte{0xFF, 0xD8, 0xFF}, WidthPx: 100, HeightPx: 100},
	}})
	if !strings.Contains(parts["[Content_Types].xml"], `<Default Extension="jpg" ContentType="image/jpeg"/>`) {
		t.Errorf("jpg is not declared as image/jpeg:\n%s", parts["[Content_Types].xml"])
	}
}

// An image stretched to fill its box is worse than a smaller one: a chart
// squashed 20% is misleading rather than merely ugly.
func TestImageKeepsItsAspectRatioAndStaysOnTheSlide(t *testing.T) {
	cases := []struct{ w, h int }{{4000, 500}, {500, 4000}, {1000, 1000}}
	for _, c := range cases {
		img := &SlideImage{Ext: "png", WidthPx: c.w, HeightPx: c.h, Data: []byte{1}}
		x, y, w, h := imageFrame(img, false, contentW)

		want := float64(c.w) / float64(c.h)
		got := float64(w) / float64(h)
		if diff := want - got; diff > 0.01 || diff < -0.01 {
			t.Errorf("%dx%d: aspect ratio %.3f, want %.3f", c.w, c.h, got, want)
		}
		if x < 0 || y < 0 || x+w > slideWidth || y+h > slideHeight {
			t.Errorf("%dx%d: frame (%d,%d %dx%d) falls off a %dx%d slide", c.w, c.h, x, y, w, h, slideWidth, slideHeight)
		}
	}
}

// Bullets and a picture share the slide, so the picture must not land on top of
// the text.
func TestPictureBesideBulletsClearsTheText(t *testing.T) {
	bodyW := (contentW - 457200) / 2
	img := &SlideImage{Ext: "png", WidthPx: 1000, HeightPx: 1000, Data: []byte{1}}
	x, _, _, _ := imageFrame(img, true, bodyW)
	if x < marginX+bodyW {
		t.Errorf("picture starts at %d, inside the text column that ends at %d", x, marginX+bodyW)
	}
}

func TestBuildPPTXRejectsAnEmptyDeck(t *testing.T) {
	if _, err := BuildPPTX(nil); err == nil {
		t.Error("a deck with no slides must be an error")
	}
}
