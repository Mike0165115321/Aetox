package ooxml

import (
	"fmt"
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
	docx, err := BuildDOCX([]Block{
		{Kind: BlockBullets, Items: []string{"x"}},
		{Kind: BlockImage, Image: &Picture{Ext: "png", Data: []byte{1}, WidthPx: 4, HeightPx: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases["docx"] = docx
	xlsx, err := BuildXLSX([]Sheet{{Name: "S", Columns: []string{"A"}}})
	if err != nil {
		t.Fatal(err)
	}
	cases["xlsx"] = xlsx
	pptx, err := BuildPPTX([]Slide{{Title: "t", Notes: "n", Image: &Picture{Ext: "png", Data: []byte{1}, WidthPx: 1, HeightPx: 1}}})
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

// The reason lineitems exists. Every figure on a priced document is worked out
// here, and the printed lines must add up to the printed total — a document
// whose own numbers disagree is worse than one that is simply wrong, because
// the reader can see it.
func TestPricedLinesAddUpOnThePage(t *testing.T) {
	amounts, totals := computeLines(
		[]LineItem{
			{Text: "ออกแบบ", Qty: 1, Price: 15000},
			{Text: "ถ่ายภาพ", Qty: 24, Price: 750},
			{Text: "ค่าที่ปรึกษา", Price: 4999.995}, // no qty: one of it
		},
		[]TotalRow{
			{Label: "มูลค่าสินค้า", Kind: TotalSubtotal},
			{Label: "ภาษีมูลค่าเพิ่ม 7%", Kind: TotalRate, Rate: 0.07},
			{Label: "รวมทั้งสิ้น", Kind: TotalTotal},
		},
	)

	want := []float64{15000, 18000, 5000} // 4999.995 rounds half away from zero
	for i, w := range want {
		if amounts[i] != w {
			t.Errorf("line %d amount = %v, want %v", i+1, amounts[i], w)
		}
	}
	if totals[0].Amount != 38000 {
		t.Errorf("subtotal = %v, want 38000", totals[0].Amount)
	}
	if totals[1].Amount != 2660 {
		t.Errorf("VAT = %v, want 2660", totals[1].Amount)
	}
	if totals[2].Amount != 40660 {
		t.Errorf("total = %v, want 40660", totals[2].Amount)
	}
	// The printed figures, not the maths behind them, are what has to reconcile.
	if totals[0].Amount+totals[1].Amount != totals[2].Amount {
		t.Error("the printed subtotal and VAT do not add to the printed total")
	}
}

// A deduction is a negative rate, which is how one mechanism covers both VAT
// and withholding tax. Both are assessed on the goods, so the second must not
// be charged against a figure that already contains the first.
func TestADeductionIsANegativeRateAssessedOnTheSubtotal(t *testing.T) {
	_, totals := computeLines(
		[]LineItem{{Text: "บริการ", Qty: 1, Price: 100000}},
		[]TotalRow{
			{Label: "มูลค่าบริการ", Kind: TotalSubtotal},
			{Label: "ภาษีมูลค่าเพิ่ม 7%", Kind: TotalRate, Rate: 0.07},
			{Label: "หัก ณ ที่จ่าย 3%", Kind: TotalRate, Rate: -0.03},
			{Label: "ยอดชำระสุทธิ", Kind: TotalTotal},
		},
	)

	// 3% of 100,000 — not of 107,000, which would be 3,210 and is the classic
	// way to be wrong on a Thai invoice.
	if totals[2].Amount != -3000 {
		t.Errorf("withholding = %v, want -3000 (3%% of the subtotal, not of subtotal+VAT)", totals[2].Amount)
	}
	if totals[3].Amount != 104000 {
		t.Errorf("net payable = %v, want 104000", totals[3].Amount)
	}
}

// Money reads as money on a document: separated, two decimals, always both.
func TestMoneyIsFormattedTheWayADocumentReadsIt(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want string
	}{
		{1234.5, "1,234.50"},
		{0, "0.00"},
		{-3000, "-3,000.00"},
		{1234567.891, "1,234,567.89"},
		{999, "999.00"},
	} {
		if got := formatMoney(c.in); got != c.want {
			t.Errorf("formatMoney(%v) = %q, want %q", c.in, got, c.want)
		}
	}
	// A count is a count: "2 ชิ้น", not "2.00 ชิ้น".
	if got := formatQuantity(24); got != "24" {
		t.Errorf("formatQuantity(24) = %q, want 24", got)
	}
	if got := formatQuantity(1.5); got != "1.5" {
		t.Errorf("formatQuantity(1.5) = %q, want 1.5", got)
	}
}

// A column of amounts that reads down its left edge is the loudest sign a
// document was generated rather than written. w:jc must sit after w:spacing:
// the other order is not a preference, it is a document Word refuses.
func TestAColumnCanBeRightAlignedInSchemaOrder(t *testing.T) {
	parts := buildDoc(t, []Block{{
		Kind:    BlockTable,
		Columns: []string{"รายการ", "จำนวนเงิน"},
		Rows:    [][]string{{"ค่าบริการ", "1,000.00"}},
		Align:   []string{"", "right"},
		Widths:  []int{4, 1},
	}})

	doc := parts["word/document.xml"]
	if !strings.Contains(doc, `<w:jc w:val="right"/>`) {
		t.Errorf("no right alignment in:\n%s", doc)
	}
	if strings.Contains(doc, `<w:jc w:val="right"/><w:spacing`) {
		t.Error("w:jc was written before w:spacing, which Word refuses")
	}
	// 4:1 of the text width — and the columns must still sum to exactly it, or
	// the reader throws the weights away and recomputes an even grid. Asserted
	// as the invariant rather than two magic numbers, so a change to the page
	// margins does not read as a broken feature.
	wide, narrow := gridWidths(2, []int{4, 1})[0], gridWidths(2, []int{4, 1})[1]
	if wide+narrow != textWidth {
		t.Errorf("columns sum to %d, want the table width %d", wide+narrow, textWidth)
	}
	if wide < narrow*3 || wide > narrow*5 {
		t.Errorf("4:1 weights gave %d:%d", wide, narrow)
	}
	if !strings.Contains(doc, fmt.Sprintf(`<w:gridCol w:w="%d"/>`, wide)) {
		t.Errorf("the computed width is not in the document:\n%s", doc)
	}
	// An even split is what a table with no weights still gets.
	if even := gridWidths(2, nil); even[0] != even[1] {
		t.Errorf("unweighted columns came out uneven: %v", even)
	}
}

// A plain table is the same machinery used as a layout — the seller and buyer
// blocks side by side. A grid of boxes there announces a table nobody meant.
func TestAPlainTableDrawsNoBordersAndNoHeaderShading(t *testing.T) {
	parts := buildDoc(t, []Block{{
		Kind:    BlockTable,
		Columns: []string{"ผู้ขาย", "ผู้ซื้อ"},
		Rows:    [][]string{{"บริษัท ก", "บริษัท ข"}},
		Plain:   true,
	}})

	doc := parts["word/document.xml"]
	if strings.Contains(doc, "<w:tblBorders>") {
		t.Error("a plain table was given borders")
	}
	if strings.Contains(doc, `w:fill="F2F4F7"`) {
		t.Error("a plain table shaded its header row")
	}
}

// `**` is what a model writes for bold unprompted. An unbalanced one is a
// literal, because a document that swallows a stray asterisk is worse than one
// that prints it.
func TestInlineBoldIsWrittenAsRunsAndUnbalancedMarkersStayLiteral(t *testing.T) {
	parts := buildDoc(t, []Block{
		{Kind: BlockParagraph, Text: "ยอด **40,660.00** บาท"},
		{Kind: BlockParagraph, Text: "สูตร a ** b"},
	})

	doc := parts["word/document.xml"]
	if !strings.Contains(doc, `<w:rPr><w:b/><w:bCs/></w:rPr><w:t xml:space="preserve">40,660.00</w:t>`) {
		t.Errorf("the emphasised span is not its own bold run:\n%s", doc)
	}
	if strings.Contains(doc, ">ยอด **<") {
		t.Error("the markers were kept in the visible text")
	}
	if !strings.Contains(doc, "a ** b") {
		t.Error("an unbalanced marker was swallowed instead of printed")
	}
}


// A picture is the first thing this writer puts in the package that is not XML,
// and there are three ways to get that wrong that all present as the same
// dialog: no content type for the extension, no relationship pointing at the
// part, or a relationship id the body never names. Word says "unreadable
// content" for all three and names none of them, so all three are asserted here.
func TestAPictureIsEmbeddedWithItsTypeItsRelationshipAndItsCaption(t *testing.T) {
	parts := buildDoc(t, []Block{
		{Kind: BlockHeading, Text: "ภาคผนวก ก"},
		{Kind: BlockImage, Text: "ภาพที่ ก-1 หน้าหลัก", Image: &Picture{
			Ext: "png", Data: pngFixture(t, 800, 600), WidthPx: 800, HeightPx: 600, AltText: "หน้าหลัก.png",
		}},
	})

	if _, ok := parts["word/media/image1.png"]; !ok {
		t.Fatal("the picture is not in the package at all")
	}
	if !strings.Contains(parts["[Content_Types].xml"], `<Default Extension="png" ContentType="image/png"/>`) {
		t.Error("png has no content type, so Word refuses the whole document")
	}
	rels := parts["word/_rels/document.xml.rels"]
	if !strings.Contains(rels, `Id="rId3"`) || !strings.Contains(rels, `Target="media/image1.png"`) {
		t.Errorf("nothing relates the document to the picture: %s", rels)
	}

	body := parts["word/document.xml"]
	if !strings.Contains(body, `r:embed="rId3"`) {
		t.Error("the drawing does not name the relationship, so the picture is a blank frame")
	}
	// r:embed is an attribute and an attribute cannot declare its own namespace.
	if !strings.Contains(body, `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`) {
		t.Error("xmlns:r is not on the root, so document.xml is not well-formed XML")
	}
	if !strings.Contains(body, `<w:pStyle w:val="Caption"/>`) {
		t.Error("the caption is not the Caption style, so Table of Figures cannot find it")
	}
	if !strings.Contains(body, "ภาพที่ ก-1 หน้าหลัก") {
		t.Error("the caption text is missing")
	}
	if !strings.Contains(parts["word/styles.xml"], `<w:name w:val="caption"/>`) {
		t.Error("the Caption style is referenced but never defined, which Google Docs ignores")
	}
}

// The failure this prevents is invisible in Word and total in Google Docs:
// Word tolerates a repeated wp:docPr id, Google Docs drops every picture after
// the first duplicate — so a twenty-figure appendix arrives with one figure and
// no error anywhere.
func TestEveryPictureGetsItsOwnIdPartAndRelationship(t *testing.T) {
	parts := buildDoc(t, []Block{
		{Kind: BlockImage, Image: &Picture{Ext: "png", Data: pngFixture(t, 40, 30), WidthPx: 40, HeightPx: 30}},
		{Kind: BlockImage, Image: &Picture{Ext: "jpg", Data: []byte{0xFF, 0xD8, 0xFF}, WidthPx: 40, HeightPx: 30}},
	})

	for _, name := range []string{"word/media/image1.png", "word/media/image2.jpg"} {
		if _, ok := parts[name]; !ok {
			t.Errorf("%s is missing", name)
		}
	}
	body := parts["word/document.xml"]
	for _, want := range []string{`<wp:docPr id="1"`, `<wp:docPr id="2"`, `r:embed="rId3"`, `r:embed="rId4"`} {
		if !strings.Contains(body, want) {
			t.Errorf("document.xml does not contain %s", want)
		}
	}
	// jpg names image/jpeg or Google Slides and Google Docs reject the file.
	if !strings.Contains(parts["[Content_Types].xml"], `<Default Extension="jpg" ContentType="image/jpeg"/>`) {
		t.Error("jpg is declared as image/jpg, which Google's readers refuse")
	}
}

// A drawing wider than the text column is not wrapped or clipped — Word draws it
// over the margin and off the paper. And a small picture is never enlarged to
// fill the width, because blowing a 200 px logo up to A4 is a defect that looks
// like a decision.
func TestAPictureFitsTheTextColumnAndIsNeverEnlarged(t *testing.T) {
	wide := imageFitOf(t, 3000, 1500)
	if wide.w != maxImageWidth {
		t.Errorf("a 3000px picture is %d EMU wide, want the text column %d", wide.w, maxImageWidth)
	}
	// Shape kept: half as tall as it is wide, as it was sent.
	if wide.h != maxImageWidth/2 {
		t.Errorf("the aspect ratio moved: %d x %d", wide.w, wide.h)
	}

	small := imageFitOf(t, 200, 100)
	if small.w != 200*emuPerPixel {
		t.Errorf("a 200px picture was resized to %d EMU, want its own %d", small.w, 200*emuPerPixel)
	}

	// The bound that was missed the first time, and the one an appendix hits
	// every page: a phone screenshot. Fitted to the text column alone it is 13.6
	// inches tall on a 9.7 inch page, and Word does not wrap or paginate an
	// inline drawing — it clips the bottom off, silently, in a document that
	// opens perfectly.
	phone := imageFitOf(t, 1170, 2532)
	if phone.h > maxImageHeight {
		t.Errorf("a phone screenshot is %d EMU tall against a page of %d — Word clips the bottom off", phone.h, maxImageHeight)
	}
	if phone.w > maxImageWidth {
		t.Errorf("fitting the height pushed the width to %d, over %d", phone.w, maxImageWidth)
	}
	// Fitting twice must not squash it: the shape is the picture.
	if got, want := float64(phone.w)/float64(phone.h), 1170.0/2532.0; got < want*0.995 || got > want*1.005 {
		t.Errorf("aspect ratio is %.4f after both bounds, want %.4f", got, want)
	}
}

type fitted struct{ w, h int64 }

func imageFitOf(t *testing.T, w, h int) fitted {
	t.Helper()
	cx, cy := imageFit(&Picture{WidthPx: w, HeightPx: h}, maxImageHeight)
	return fitted{cx, cy}
}

// Both refusals happen where the caller can still act on them. A picture drawn
// at nothing by nothing is a document that opens clean and is missing the one
// thing it was written for.
func TestAnImageBlockWithNothingToDrawIsRefused(t *testing.T) {
	for name, block := range map[string]Block{
		"no picture":    {Kind: BlockImage, Text: "ภาพที่ 1"},
		"no bytes":      {Kind: BlockImage, Image: &Picture{Ext: "png", WidthPx: 10, HeightPx: 10}},
		"no dimensions": {Kind: BlockImage, Image: &Picture{Ext: "png", Data: []byte{1}}},
	} {
		if _, err := BuildDOCX([]Block{block}); err == nil {
			t.Errorf("%s: accepted, and the document would open without the figure", name)
		}
	}
}
