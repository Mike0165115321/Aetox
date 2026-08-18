package ooxml

import (
	"strings"
	"testing"
)

// The reader's contract with the writer: what goes in comes back out with its
// shape intact. They are tested together rather than against fixture XML on
// purpose — a fixture records what the writer emitted the day it was copied,
// and the pair drifting apart is exactly the failure a fixture cannot see.
func TestADocumentReadsBackWithItsShape(t *testing.T) {
	parts, err := BuildDOCX([]Block{
		{Kind: BlockHeading, Text: "ภาคผนวก ก", Level: 1},
		{Kind: BlockParagraph, Text: "คำนำสั้น ๆ"},
		{Kind: BlockBullets, Items: []string{"ข้อหนึ่ง", "ข้อสอง"}},
		{Kind: BlockImage, Text: "ภาพที่ ก-1 หน้าหลัก", Image: &Picture{
			Ext: "png", Data: pngFixture(t, 40, 30), WidthPx: 40, HeightPx: 30, AltText: "หน้าหลัก.png",
		}},
		{Kind: BlockTable, Columns: []string{"หมวด", "ยอด"}, Rows: [][]string{{"อาหาร", "95"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := WritePackage(parts)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ReadDOCX(data)
	if err != nil {
		t.Fatalf("ReadDOCX: %v", err)
	}

	if doc.Pictures != 1 {
		t.Errorf("Pictures = %d, want 1", doc.Pictures)
	}
	want := []struct {
		kind, style, text string
	}{
		{ReadParagraph, "Heading1", "ภาคผนวก ก"},
		{ReadParagraph, "", "คำนำสั้น ๆ"},
		{ReadParagraph, styleListParagraph, "ข้อหนึ่ง"},
		{ReadParagraph, styleListParagraph, "ข้อสอง"},
		{ReadImage, "", ""},
		{ReadParagraph, styleCaption, "ภาพที่ ก-1 หน้าหลัก"},
		{ReadTable, "", ""},
	}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("read %d blocks, want %d: %+v", len(doc.Blocks), len(want), doc.Blocks)
	}
	for i, w := range want {
		got := doc.Blocks[i]
		if got.Kind != w.kind {
			t.Errorf("block %d kind = %q, want %q", i+1, got.Kind, w.kind)
		}
		if w.text != "" && got.Text != w.text {
			t.Errorf("block %d text = %q, want %q", i+1, got.Text, w.text)
		}
		if w.style != "" && got.Style != w.style {
			t.Errorf("block %d style = %q, want %q", i+1, got.Style, w.style)
		}
	}

	// A list item has to be distinguishable from a paragraph that merely uses
	// the same style: `ListParagraph` is an indent, `numPr` is the marker.
	if !doc.Blocks[2].Listed {
		t.Error("a bullet did not report as a list item")
	}
	if doc.Blocks[1].Listed {
		t.Error("a plain paragraph reported as a list item")
	}
	// The picture keeps the name it came in under, which is what lets an agent
	// talk about a specific figure.
	if alts := doc.Blocks[4].Alts; len(alts) != 1 || alts[0] != "หน้าหลัก.png" {
		t.Errorf("picture alts = %v, want the file it came from", alts)
	}
	table := doc.Blocks[6]
	if table.Rows != 2 || table.Columns != 2 {
		t.Errorf("table is %dx%d, want 2x2", table.Rows, table.Columns)
	}
	if len(table.Cells) != 2 || table.Cells[1][0] != "อาหาร" {
		t.Errorf("table cells did not survive: %v", table.Cells)
	}
}

// A table's cells are paragraphs, and a reader that does not know that reports
// every cell twice: once inside the table and once as body text after it.
func TestCellsDoNotEscapeTheirTable(t *testing.T) {
	parts, _ := BuildDOCX([]Block{
		{Kind: BlockTable, Columns: []string{"ก", "ข"}, Rows: [][]string{{"หนึ่ง", "สอง"}}},
		{Kind: BlockParagraph, Text: "ย่อหน้าหลังตาราง"},
	})
	data, _ := WritePackage(parts)
	doc, err := ReadDOCX(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range doc.Blocks {
		if block.Kind == ReadParagraph && block.Text != "ย่อหน้าหลังตาราง" {
			t.Errorf("a cell escaped its table as a paragraph: %q", block.Text)
		}
	}
	// Word puts an empty paragraph after a table, and reporting it would put a
	// blank block between every table and whatever follows it.
	if got := len(doc.Blocks); got != 2 {
		t.Errorf("read %d blocks, want the table and the paragraph: %+v", got, doc.Blocks)
	}
}

// One paragraph can hold several pictures — somebody pasting screenshots in a
// row without pressing Enter produces exactly that, and a real document on this
// machine turned out to be one paragraph carrying five figures. Reported as one
// picture, an agent asked to caption them writes one caption and stops.
func TestSeveralPicturesInOneParagraphAreAllCounted(t *testing.T) {
	body := `<?xml version="1.0"?><w:document xmlns:w="w" xmlns:wp="wp"><w:body><w:p>` +
		`<w:r><w:drawing><wp:inline><wp:docPr id="1" name="Picture 1" descr="ก.png"/></wp:inline></w:drawing></w:r>` +
		`<w:r><w:drawing><wp:inline><wp:docPr id="2" name="Picture 2" descr="ข.png"/></wp:inline></w:drawing></w:r>` +
		`<w:r><w:drawing><wp:inline><wp:docPr id="3" name="Picture 3"/></wp:inline></w:drawing></w:r>` +
		`</w:p></w:body></w:document>`
	doc, err := parseDocumentXML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("read %d blocks, want 1: %+v", len(doc.Blocks), doc.Blocks)
	}
	if got := doc.Blocks[0].Pictures; got != 3 {
		t.Errorf("the paragraph reports %d pictures, want 3", got)
	}
	if doc.Pictures != 3 {
		t.Errorf("the document reports %d pictures, want 3", doc.Pictures)
	}
	// Two of the three name themselves; the third has no descr and is not
	// invented one.
	if got := doc.Blocks[0].Alts; len(got) != 2 || got[0] != "ก.png" || got[1] != "ข.png" {
		t.Errorf("alts = %v, want only the two that carry a name", got)
	}
}

// Documents Aetox did not write are the only ones worth reading, and they carry
// elements this package never emits. The walk must step over what it does not
// know rather than lose the text around it.
func TestUnknownMarkupIsSteppedOverNotChokedOn(t *testing.T) {
	body := `<?xml version="1.0"?><w:document xmlns:w="w" xmlns:mc="mc"><w:body>` +
		`<w:p><w:pPr><w:pStyle w:val="a3"/><w:rPr><w:lang w:val="th-TH"/></w:rPr></w:pPr>` +
		`<w:proofErr w:type="spellStart"/><w:r><w:t xml:space="preserve">ก่อน</w:t></w:r>` +
		`<w:bookmarkStart w:id="0" w:name="_Toc1"/><w:r><w:t>หลัง</w:t></w:r>` +
		`<mc:AlternateContent><mc:Fallback><w:r><w:t>สำรอง</w:t></w:r></mc:Fallback></mc:AlternateContent>` +
		`</w:p>` +
		`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/></w:sectPr></w:body></w:document>`
	doc, err := parseDocumentXML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("read %d blocks, want 1: %+v", len(doc.Blocks), doc.Blocks)
	}
	if got := doc.Blocks[0].Text; !strings.HasPrefix(got, "ก่อนหลัง") {
		t.Errorf("text = %q, want the runs joined in order", got)
	}
	// A style id this package never writes is still the file's own answer, and
	// it is reported rather than normalised into something friendlier.
	if got := doc.Blocks[0].Style; got != "a3" {
		t.Errorf("style = %q, want the id the file actually carries", got)
	}
}

func TestReadDOCXRefusesSomethingThatIsNotADocument(t *testing.T) {
	if _, err := ReadDOCX([]byte("this is not a zip")); err == nil {
		t.Error("a text file was accepted as a Word document")
	}
	parts, _ := BuildXLSX([]Sheet{{Name: "S", Columns: []string{"A"}}})
	data, _ := WritePackage(parts)
	if _, err := ReadDOCX(data); err == nil {
		t.Error("a workbook was accepted as a Word document")
	}
}

// slide2 before slide10: the zip's own order is not the deck's order, and a
// deck read starting at slide 10 is not the deck.
func TestSlidesAreReadInTheDecksOrder(t *testing.T) {
	names := []string{"ppt/slides/slide10.xml", "ppt/slides/slide2.xml", "ppt/slides/slide1.xml"}
	sortSlideNames(names)
	want := []string{"ppt/slides/slide1.xml", "ppt/slides/slide2.xml", "ppt/slides/slide10.xml"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("slide order = %v, want %v", names, want)
		}
	}
}

func TestADeckReadsBackItsWords(t *testing.T) {
	parts, err := BuildPPTX([]Slide{
		{Title: "หัวข้อแรก", Bullets: []string{"ข้อหนึ่ง"}},
		{Title: "หัวข้อสอง"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := WritePackage(parts)
	slides, err := ReadPPTXText(data)
	if err != nil {
		t.Fatalf("ReadPPTXText: %v", err)
	}
	if len(slides) != 2 {
		t.Fatalf("read %d slides, want 2", len(slides))
	}
	if !strings.Contains(slides[0], "หัวข้อแรก") || !strings.Contains(slides[0], "ข้อหนึ่ง") {
		t.Errorf("slide 1 = %q", slides[0])
	}
	if !strings.Contains(slides[1], "หัวข้อสอง") {
		t.Errorf("slide 2 = %q", slides[1])
	}
}
