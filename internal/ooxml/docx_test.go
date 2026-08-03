package ooxml

import (
	"strings"
	"testing"
)

func buildDoc(t *testing.T, blocks []Block) map[string]string {
	t.Helper()
	parts, err := BuildDOCX(blocks)
	if err != nil {
		t.Fatalf("BuildDOCX: %v", err)
	}
	data, err := WritePackage(parts)
	if err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	// openPackage unzips and XML-parses every part, so a malformed document
	// fails there before any assertion below runs.
	return openPackage(t, data)
}

func TestDocumentHasEveryRequiredPart(t *testing.T) {
	parts := buildDoc(t, []Block{{Kind: BlockParagraph, Text: "หนึ่ง"}})
	for _, required := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"word/document.xml",
		"word/_rels/document.xml.rels",
		"word/styles.xml",
		"word/numbering.xml",
	} {
		if _, ok := parts[required]; !ok {
			t.Errorf("missing required part %s", required)
		}
	}

	types := parts["[Content_Types].xml"]
	for name := range parts {
		if name == "[Content_Types].xml" || strings.HasSuffix(name, ".rels") {
			continue
		}
		if !strings.Contains(types, `PartName="/`+name+`"`) {
			t.Errorf("%s has no Override in [Content_Types].xml", name)
		}
	}
	rels := parts["word/_rels/document.xml.rels"]
	for _, target := range []string{"styles.xml", "numbering.xml"} {
		if !strings.Contains(rels, `Target="`+target+`"`) {
			t.Errorf("nothing relates the document to %s, so Word ignores it", target)
		}
	}
}

// sectPr is not merely conventionally last in w:body — the schema puts it at the
// end of the sequence, and a document with a paragraph after it is one Word
// offers to repair.
func TestSectionPropertiesAreTheLastThingInTheBody(t *testing.T) {
	parts := buildDoc(t, []Block{
		{Kind: BlockHeading, Text: "หัวข้อ", Level: 1},
		{Kind: BlockParagraph, Text: "เนื้อความ"},
	})
	body := parts["word/document.xml"]

	sectPr := strings.Index(body, "<w:sectPr>")
	if sectPr < 0 {
		t.Fatal("no sectPr — the document has no page size or margins")
	}
	if after := body[sectPr:]; strings.Contains(after, "<w:p>") || strings.Contains(after, "<w:tbl>") {
		t.Errorf("content appears after sectPr:\n%s", after)
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "</w:body></w:document>") {
		t.Error("the body does not close immediately after sectPr")
	}
}

// A heading is a heading because of w:outlineLvl. Without it Word renders large
// bold text that never reaches the navigation pane or a table of contents —
// which looks right on screen and is wrong in every way that matters.
func TestHeadingsAreRealHeadings(t *testing.T) {
	parts := buildDoc(t, []Block{
		{Kind: BlockHeading, Text: "หนึ่ง", Level: 1},
		{Kind: BlockHeading, Text: "สอง", Level: 2},
		{Kind: BlockHeading, Text: "สาม", Level: 3},
		{Kind: BlockHeading, Text: "ลึกเกินไป", Level: 9},
		{Kind: BlockHeading, Text: "ตื้นเกินไป", Level: 0},
	})
	body := parts["word/document.xml"]
	styles := parts["word/styles.xml"]

	for level := 1; level <= 3; level++ {
		id := "Heading" + string(rune('0'+level))
		if !strings.Contains(body, `<w:pStyle w:val="`+id+`"/>`) {
			t.Errorf("no paragraph uses %s", id)
		}
		if !strings.Contains(styles, `w:styleId="`+id+`"`) {
			t.Errorf("%s is referenced but never defined — a dangling style renders as Normal", id)
		}
		if !strings.Contains(styles, `<w:outlineLvl w:val="`+string(rune('0'+level-1))+`"/>`) {
			t.Errorf("%s has no outlineLvl, so it is not a real heading", id)
		}
	}
	// Out-of-range levels are clamped rather than producing a dangling
	// Heading9 / Heading0 reference.
	if strings.Contains(body, "Heading9") || strings.Contains(body, "Heading0") {
		t.Errorf("a heading level was not clamped into 1-3:\n%s", body)
	}
}

// The Thai fix, and the reason it lives in docDefaults rather than on every run.
//
// Word keeps a separate font, size and language for complex scripts, and Thai
// reads that set. A document that sets only w:rFonts w:ascii and w:sz renders
// Thai in whatever Word substitutes, at whatever size it picks.
func TestThaiGetsAComplexScriptFontSizeAndLanguage(t *testing.T) {
	parts := buildDoc(t, []Block{{Kind: BlockParagraph, Text: "สรุปยอดขายไตรมาส ๓"}})
	styles := parts["word/styles.xml"]

	if !strings.Contains(styles, "<w:docDefaults>") {
		t.Fatal("no docDefaults — every run would have to repeat the Thai settings")
	}
	if !strings.Contains(styles, `w:cs="Leelawadee UI"`) {
		t.Error("no complex-script font: Thai falls back to whatever Word substitutes")
	}
	if !strings.Contains(styles, `<w:szCs w:val="22"/>`) {
		t.Error("no complex-script size: Thai renders at a different size from the Latin text beside it")
	}
	if !strings.Contains(styles, `w:bidi="th-TH"`) {
		t.Error("no complex-script language: Word does not know the text is Thai")
	}
	// Tone marks stack above the vowel above the consonant. An exact line height
	// clips the top of that stack — fine on screen, wrong on paper.
	if !strings.Contains(styles, `w:lineRule="auto"`) {
		t.Error("line spacing is not auto, which clips stacked Thai marks")
	}
	if !strings.Contains(parts["word/document.xml"], "สรุปยอดขายไตรมาส ๓") {
		t.Error("Thai text did not survive")
	}
}

// A heading in bold needs `b` AND `bCs` — the second is bold-for-complex-scripts,
// and a run with only `b` renders a Thai heading in regular weight beside bold
// Latin.
func TestBoldAppliesToComplexScriptsToo(t *testing.T) {
	parts := buildDoc(t, []Block{{Kind: BlockHeading, Text: "หัวข้อ", Level: 1}})
	styles := parts["word/styles.xml"]
	if !strings.Contains(styles, "<w:b/><w:bCs/>") {
		t.Errorf("heading bold does not cover complex scripts:\n%s", styles)
	}
}

func TestListsReferenceNumberingThatExists(t *testing.T) {
	parts := buildDoc(t, []Block{
		{Kind: BlockBullets, Items: []string{"หนึ่ง", "สอง"}},
		{Kind: BlockNumbered, Items: []string{"ขั้นแรก", "ขั้นที่สอง"}},
	})
	body := parts["word/document.xml"]
	numbering := parts["word/numbering.xml"]

	for _, numID := range []string{"1", "2"} {
		if !strings.Contains(body, `<w:numId w:val="`+numID+`"/>`) {
			t.Errorf("no paragraph uses numId %s", numID)
		}
		if !strings.Contains(numbering, `<w:num w:numId="`+numID+`">`) {
			t.Errorf("numId %s is referenced but not defined — the list silently loses its bullets", numID)
		}
	}
	if !strings.Contains(numbering, `<w:numFmt w:val="bullet"/>`) {
		t.Error("no bullet format defined")
	}
	if !strings.Contains(numbering, `<w:numFmt w:val="decimal"/>`) {
		t.Error("no decimal format defined")
	}
	// A hanging indent is what keeps the marker clear of the text.
	if !strings.Contains(numbering, `w:hanging="360"`) {
		t.Error("no hanging indent, so the bullet overlaps the first word")
	}
	// Symbol is a legacy symbol-encoded face: its bullet is only a bullet while
	// that exact font is applied, and Google Docs and LibreOffice routinely draw
	// it as a box.
	if strings.Contains(numbering, "Symbol") {
		t.Error("the bullet depends on the Symbol font, which does not travel")
	}
	if !strings.Contains(numbering, `<w:lvlText w:val="•"/>`) {
		t.Error("the bullet is not a plain U+2022")
	}
}

func TestTablesCarryAGridAndACellParagraph(t *testing.T) {
	parts := buildDoc(t, []Block{{
		Kind:    BlockTable,
		Columns: []string{"หมวด", "จำนวน", "ยอด"},
		Rows: [][]string{
			{"อาหาร", "3", "1240"},
			{"ขนส่ง", "1"}, // short row
		},
	}})
	body := parts["word/document.xml"]

	// The grid declares the column count; a grid that disagrees with the cells
	// in a row is a layout Word resolves by guessing.
	if got := strings.Count(body, "<w:gridCol "); got != 3 {
		t.Errorf("gridCol count = %d, want 3", got)
	}
	if got := strings.Count(body, "<w:tc>"); got != 9 {
		t.Errorf("cell count = %d, want 9 — a short row was not padded", got)
	}
	// Every w:tc must end with a w:p. A cell holding only a w:tcPr is the single
	// most common way a hand-built table makes Word declare the file unreadable.
	for _, cell := range strings.Split(body, "<w:tc>")[1:] {
		if end := strings.Index(cell, "</w:tc>"); end >= 0 {
			if !strings.Contains(cell[:end], "<w:p>") {
				t.Fatalf("a table cell contains no paragraph:\n%s", cell[:end])
			}
		}
	}
	if !strings.Contains(body, "<w:tblBorders>") {
		t.Error("no borders — Word's default table draws none, which is not what anyone means by a table")
	}
	if !strings.Contains(body, "<w:tblHeader/>") {
		t.Error("the header row does not repeat across pages")
	}
}

// Two tables in a row merge into one in Word unless a paragraph separates them,
// which silently fuses two unrelated tables into one.
func TestAdjacentTablesAreSeparated(t *testing.T) {
	parts := buildDoc(t, []Block{
		{Kind: BlockTable, Columns: []string{"a"}, Rows: [][]string{{"1"}}},
		{Kind: BlockTable, Columns: []string{"b"}, Rows: [][]string{{"2"}}},
	})
	body := parts["word/document.xml"]
	between := body[strings.Index(body, "</w:tbl>"):strings.LastIndex(body, "<w:tbl>")]
	if !strings.Contains(between, "<w:p/>") {
		t.Errorf("nothing separates two adjacent tables, so Word merges them:\n%s", between)
	}
	// A body ending in a table with only sectPr after it is the one arrangement
	// Word treats as malformed rather than merely unusual.
	tail := body[strings.LastIndex(body, "</w:tbl>"):]
	if !strings.Contains(tail[:strings.Index(tail, "<w:sectPr>")], "<w:p/>") {
		t.Errorf("the document ends with a table and no closing paragraph:\n%s", tail)
	}
}

// Without xml:space="preserve" the XML reader strips a leading or trailing
// space, which turns "ยอด: " into "ยอด:" wherever text is assembled from parts.
func TestRunsPreserveSurroundingSpace(t *testing.T) {
	parts := buildDoc(t, []Block{{Kind: BlockParagraph, Text: "  เว้นวรรคหน้าหลัง  "}})
	body := parts["word/document.xml"]
	if !strings.Contains(body, `<w:t xml:space="preserve">  เว้นวรรคหน้าหลัง  </w:t>`) {
		t.Errorf("surrounding space was not preserved:\n%s", body)
	}
}

func TestControlCharactersAreStrippedFromADocument(t *testing.T) {
	parts := buildDoc(t, []Block{{Kind: BlockParagraph, Text: "before\x01\x0bafter"}})
	body := parts["word/document.xml"]
	if !strings.Contains(body, "beforeafter") {
		t.Errorf("control characters were not stripped:\n%s", body)
	}
	if strings.Contains(body, "\x01") || strings.Contains(body, "&#x1;") {
		t.Error("a control character reached the XML, which Word rejects")
	}
}

func TestBuildDOCXRejectsAnEmptyDocument(t *testing.T) {
	if _, err := BuildDOCX(nil); err == nil {
		t.Error("a document with no blocks must be an error")
	}
}

// Word resolves Thai against w:rFonts/@w:cs. Google Docs has no complex-script
// model at all and reads @w:ascii/@w:hAnsi. Two of the three target readers
// disagree about which attribute to read, so the only setting that satisfies
// both is all four naming the same family — and setting only w:cs, which is
// "correct" for Word, renders Thai in the Latin font in Google Docs.
func TestEveryFontSlotNamesTheSameFamily(t *testing.T) {
	parts := buildDoc(t, []Block{{Kind: BlockParagraph, Text: "สวัสดี"}})
	styles := parts["word/styles.xml"]

	for _, slot := range []string{"w:ascii", "w:eastAsia", "w:hAnsi", "w:cs"} {
		if !strings.Contains(styles, slot+`="`+docxFont+`"`) {
			t.Errorf("%s does not name %s — Thai and Latin will not agree", slot, docxFont)
		}
	}
	// w:cstheme does not fall back to w:cs, it replaces it — and this package
	// ships no theme part for it to resolve against.
	if strings.Contains(styles, "Theme=") {
		t.Errorf("a theme font reference reached styles.xml:\n%s", styles)
	}
	// Thai is a complex script but left-to-right; w:rtl reverses run order.
	if strings.Contains(parts["word/document.xml"], "<w:rtl/>") {
		t.Error("w:rtl was emitted for Thai, which reverses the text")
	}
}

// Numbering state lives on the abstractNum, not on the num. Two list blocks
// sharing one abstractNum do not restart — the second counts 3, 4, 5 — and
// "define the bullet once and reference it twice" is exactly the instinct that
// produces it.
func TestEachListBlockGetsItsOwnNumbering(t *testing.T) {
	parts := buildDoc(t, []Block{
		{Kind: BlockNumbered, Items: []string{"หนึ่ง", "สอง"}},
		{Kind: BlockParagraph, Text: "คั่นกลาง"},
		{Kind: BlockNumbered, Items: []string{"เริ่มใหม่", "ต่อ"}},
		{Kind: BlockBullets, Items: []string{"จุด"}},
	})
	numbering := parts["word/numbering.xml"]
	body := parts["word/document.xml"]

	if got := strings.Count(numbering, "<w:abstractNum "); got != 3 {
		t.Errorf("abstractNum count = %d, want one per list block (3)", got)
	}
	if got := strings.Count(numbering, "<w:num "); got != 3 {
		t.Errorf("num count = %d, want one per list block (3)", got)
	}
	// Two numbered blocks must reference different numIds, or the second
	// continues the first's count instead of restarting at 1.
	for _, numID := range []string{"1", "2", "3"} {
		if !strings.Contains(body, `<w:numId w:val="`+numID+`"/>`) {
			t.Errorf("no paragraph uses numId %s", numID)
		}
		if !strings.Contains(numbering, `<w:num w:numId="`+numID+`">`) {
			t.Errorf("numId %s is referenced but not defined", numID)
		}
	}
	// numId 0 is the format's "no numbering" sentinel.
	if strings.Contains(body, `<w:numId w:val="0"/>`) {
		t.Error("a list paragraph uses numId 0, which means no numbering")
	}
	// Every abstractNum before every num: interleaved, Word opens the file and
	// then silently drops all numbering.
	if lastAbstract, firstNum := strings.LastIndex(numbering, "<w:abstractNum "), strings.Index(numbering, "<w:num "); lastAbstract > firstNum {
		t.Error("a w:num appears before a w:abstractNum — Word drops all numbering, silently")
	}
	// Omitting w:start on a decimal level makes it count from zero.
	if got := strings.Count(numbering, `<w:start w:val="1"/>`); got != 3 {
		t.Errorf("start count = %d, want one per level (3) — a level without it counts from zero", got)
	}
}

// Cell padding and table width both normally come from the built-in TableNormal
// style, which this package does not ship.
func TestTablesCarryTheirOwnWidthAndPadding(t *testing.T) {
	parts := buildDoc(t, []Block{{
		Kind:    BlockTable,
		Columns: []string{"a", "b", "c"},
		Rows:    [][]string{{"1", "2", "3"}},
	}})
	body := parts["word/document.xml"]

	if !strings.Contains(body, `<w:tblW w:w="`+itoa(textWidth)+`" w:type="dxa"/>`) {
		t.Errorf("the table has no explicit dxa width, so the grid is advisory:\n%s", body)
	}
	if !strings.Contains(body, "<w:tblCellMar>") {
		t.Error("no cell margins — text sits against the borders")
	}
	// sum(gridCol) must equal tblW or the reader recomputes the grid.
	sum := 0
	for _, chunk := range strings.Split(body, `<w:gridCol w:w="`)[1:] {
		n := 0
		for _, r := range chunk {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		sum += n
	}
	if sum != textWidth {
		t.Errorf("gridCol widths sum to %d, want %d", sum, textWidth)
	}
	if !strings.Contains(body, "<w:cantSplit/><w:tblHeader/>") {
		t.Error("trPr children are not in Word's canonical order")
	}
}

// Row height is computed from the paragraph mark, so a header cell whose mark
// is still default weight gets a visibly short row — the table-side version of
// the same complex-script split that governs w:b and w:bCs.
func TestHeaderCellsBoldTheParagraphMarkToo(t *testing.T) {
	parts := buildDoc(t, []Block{{
		Kind:    BlockTable,
		Columns: []string{"หัวข้อ"},
		Rows:    [][]string{{"ค่า"}},
	}})
	body := parts["word/document.xml"]
	if !strings.Contains(body, `<w:pPr><w:spacing w:before="40" w:after="40" w:line="240" w:lineRule="auto"/><w:rPr><w:b/><w:bCs/></w:rPr></w:pPr>`) {
		t.Errorf("the header cell's paragraph mark is not bold:\n%s", body)
	}
}

// A newline inside a w:t is not a line break — XML collapses it to whitespace
// and the reader draws one long line.
func TestNewlinesBecomeLineBreaks(t *testing.T) {
	parts := buildDoc(t, []Block{{Kind: BlockParagraph, Text: "บรรทัดแรก\nบรรทัดที่สอง\r\nบรรทัดที่สาม"}})
	body := parts["word/document.xml"]
	if got := strings.Count(body, "<w:br/>"); got != 2 {
		t.Errorf("br count = %d, want 2:\n%s", got, body)
	}
	if strings.Contains(body, "บรรทัดแรก\nบรรทัดที่สอง") {
		t.Error("a raw newline survived into a w:t, where it renders as a space")
	}
}

// Every part except the manifest and the root rels must be reachable from some
// relationship: a part with a content type and no relationship is silently
// ignored, and the document opens clean with the feature simply absent.
func TestEveryPartIsReachableFromARelationship(t *testing.T) {
	cases := map[string][]Part{}
	docx, err := BuildDOCX([]Block{{Kind: BlockBullets, Items: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	cases["docx"] = docx
	xlsx, err := BuildXLSX([]Sheet{{Name: "S", Columns: []string{"A"}}})
	if err != nil {
		t.Fatal(err)
	}
	cases["xlsx"] = xlsx
	pptx, err := BuildPPTX([]Slide{{Title: "t", Notes: "n", Image: &SlideImage{Ext: "png", Data: []byte{1}, WidthPx: 1, HeightPx: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	cases["pptx"] = pptx

	for name, parts := range cases {
		t.Run(name, func(t *testing.T) {
			rels := ""
			for _, p := range parts {
				if strings.HasSuffix(p.Name, ".rels") {
					rels += string(p.Data)
				}
			}
			for _, p := range parts {
				if p.Name == "[Content_Types].xml" || strings.HasSuffix(p.Name, ".rels") {
					continue
				}
				// Targets are relative to the owning part's folder, so match on
				// the file name rather than the full part name.
				base := p.Name[strings.LastIndex(p.Name, "/")+1:]
				if !strings.Contains(rels, base) {
					t.Errorf("%s is in the package but nothing relates to it — it is silently ignored", p.Name)
				}
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
