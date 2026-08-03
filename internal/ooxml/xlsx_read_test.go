package ooxml

import (
	"testing"
	"time"
)

func previewOf(t *testing.T, sheets []Sheet) *WorkbookPreview {
	t.Helper()
	parts, err := BuildXLSX(sheets)
	if err != nil {
		t.Fatalf("BuildXLSX: %v", err)
	}
	data, err := WritePackage(parts)
	if err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	preview, err := ReadXLSX(data)
	if err != nil {
		t.Fatalf("ReadXLSX: %v", err)
	}
	return preview
}

// The round trip that matters: what the user sees in the preview must be what
// they would see in Excel. Anything else and the preview is a second, quieter
// source of truth.
func TestPreviewShowsWhatTheWriterWrote(t *testing.T) {
	day := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	preview := previewOf(t, []Sheet{{
		Name:    "สรุปสลิป",
		Columns: []string{"เลขที่", "ร้าน", "ยอด", "วันที่", "ชำระแล้ว"},
		Rows: [][]Cell{
			{TextCell("0012"), TextCell("ร้านกาแฟ"), NumberCell(185.5), DateCell(day), BoolCell(true)},
			{TextCell("0013"), TextCell("ค่าไฟ"), NumberCell(1240), DateCell(day), BoolCell(false)},
		},
	}})

	if len(preview.Sheets) != 1 {
		t.Fatalf("sheet count = %d, want 1", len(preview.Sheets))
	}
	sheet := preview.Sheets[0]
	if sheet.Name != "สรุปสลิป" {
		t.Errorf("sheet name = %q", sheet.Name)
	}
	want := [][]string{
		{"เลขที่", "ร้าน", "ยอด", "วันที่", "ชำระแล้ว"},
		{"0012", "ร้านกาแฟ", "185.5", "2026-08-03", "TRUE"},
		{"0013", "ค่าไฟ", "1240", "2026-08-03", "FALSE"},
	}
	if len(sheet.Rows) != len(want) {
		t.Fatalf("row count = %d, want %d: %v", len(sheet.Rows), len(want), sheet.Rows)
	}
	for r, row := range want {
		for c, cell := range row {
			if got := sheet.Rows[r][c]; got != cell {
				t.Errorf("cell [%d][%d] = %q, want %q", r, c, got, cell)
			}
		}
	}
}

// A date in xlsx is a day count plus a number format. A preview that skipped
// the format lookup would show 46237 where Excel shows a date — the same
// failure the writer exists to prevent, in the other direction.
func TestPreviewResolvesDatesThroughTheNumberFormat(t *testing.T) {
	preview := previewOf(t, []Sheet{{
		Name:    "Dates",
		Columns: []string{"วัน", "เวลา", "ตัวเลขธรรมดา"},
		Rows: [][]Cell{{
			DateCell(time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)),
			DateCell(time.Date(2026, 8, 3, 14, 30, 0, 0, time.UTC)),
			NumberCell(46237),
		}},
	}})
	row := preview.Sheets[0].Rows[1]
	if row[0] != "2026-08-03" {
		t.Errorf("date = %q, want 2026-08-03", row[0])
	}
	if row[1] != "2026-08-03 14:30" {
		t.Errorf("datetime = %q, want 2026-08-03 14:30", row[1])
	}
	// The same serial with no date format is just a number, and showing it as a
	// date would invent meaning the file does not carry.
	if row[2] != "46237" {
		t.Errorf("plain number = %q, want 46237", row[2])
	}
}

func TestPreviewMultipleSheetsInTabOrder(t *testing.T) {
	preview := previewOf(t, []Sheet{
		{Name: "หนึ่ง", Columns: []string{"a"}},
		{Name: "สอง", Columns: []string{"b"}},
		{Name: "สาม", Columns: []string{"c"}},
	})
	want := []string{"หนึ่ง", "สอง", "สาม"}
	if len(preview.Sheets) != 3 {
		t.Fatalf("sheet count = %d, want 3", len(preview.Sheets))
	}
	for i, name := range want {
		if preview.Sheets[i].Name != name {
			t.Errorf("sheet %d = %q, want %q", i, preview.Sheets[i].Name, name)
		}
	}
}

// Every workbook not produced here uses a shared-string table, so this is the
// path that decides whether the preview works on the user's own files or only
// on ours. Hand-built because our writer never emits one.
func TestPreviewReadsAForeignWorkbookWithSharedStrings(t *testing.T) {
	parts := []Part{
		{Name: "[Content_Types].xml", Data: []byte(xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`)},
		{Name: "_rels/.rels", Data: []byte(rootRels)},
		{Name: "xl/workbook.xml", Data: []byte(xmlHeader +
			`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="รายงาน" sheetId="1" r:id="rId1"/></sheets></workbook>`)},
		{Name: "xl/_rels/workbook.xml.rels", Data: []byte(xmlHeader +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
			`</Relationships>`)},
		{Name: "xl/sharedStrings.xml", Data: []byte(xmlHeader +
			`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="3" uniqueCount="3">` +
			`<si><t>หัวข้อ</t></si>` +
			// Rich text: several runs that concatenate into one value. Taking
			// only the first is how half a sentence goes missing.
			`<si><r><t>ครึ่ง</t></r><r><t>หลัง</t></r></si>` +
			`<si><t>ท้ายสุด</t></si>` +
			`</sst>`)},
		// Builtin numFmtId 14 is a date and never appears in numFmts, so a
		// reader that only consults numFmts shows a serial here.
		{Name: "xl/styles.xml", Data: []byte(xmlHeader +
			`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
			`<cellXfs count="2"><xf numFmtId="0"/><xf numFmtId="14"/></cellXfs>` +
			`</styleSheet>`)},
		{Name: "xl/worksheets/sheet1.xml", Data: []byte(xmlHeader +
			`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>` +
			// A sparse row: B is missing, so C must not slide into its place.
			`<row r="2"><c r="A2"><v>42</v></c><c r="C2" t="s"><v>2</v></c></row>` +
			// A formula cell: the cached value is what Excel shows, not the expression.
			`<row r="3"><c r="A3"><f>SUM(A2:A2)</f><v>42</v></c><c r="B3" s="1"><v>46237</v></c></row>` +
			`</sheetData></worksheet>`)},
	}
	data, err := WritePackage(parts)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := ReadXLSX(data)
	if err != nil {
		t.Fatalf("ReadXLSX on a foreign workbook: %v", err)
	}
	sheet := preview.Sheets[0]
	if sheet.Name != "รายงาน" {
		t.Errorf("sheet name = %q", sheet.Name)
	}
	if sheet.Rows[0][0] != "หัวข้อ" {
		t.Errorf("shared string not resolved: %q", sheet.Rows[0][0])
	}
	if sheet.Rows[0][1] != "ครึ่งหลัง" {
		t.Errorf("rich-text runs were not joined: %q", sheet.Rows[0][1])
	}
	if got := sheet.Rows[1]; len(got) != 3 || got[1] != "" || got[2] != "ท้ายสุด" {
		t.Errorf("a sparse row slid its values left: %v", got)
	}
	if sheet.Rows[2][0] != "42" {
		t.Errorf("formula cell = %q, want its cached value 42", sheet.Rows[2][0])
	}
	if sheet.Rows[2][1] != "2026-08-03" {
		t.Errorf("builtin date format was not recognised: %q", sheet.Rows[2][1])
	}
}

// A format code's quoted literals are text, not tokens. A Thai currency format
// carrying a month abbreviation in quotes would otherwise turn every amount in
// the column into a date.
func TestQuotedLiteralsInAFormatCodeAreNotDateTokens(t *testing.T) {
	cases := map[string]bool{
		`yyyy-mm-dd`:      true,
		`d mmm yyyy`:      true,
		`hh:mm`:           true,
		`0.00`:            false,
		`#,##0.00`:        false,
		`"มี.ค."#,##0.00`: false,
		`0.00" บาท"`:      false,
		`[$-409]#,##0`:    false,
	}
	for code, want := range cases {
		if got := looksLikeDateFormat(code); got != want {
			t.Errorf("looksLikeDateFormat(%q) = %v, want %v", code, got, want)
		}
	}
}

// A preview is a glance. Showing a prefix of a huge sheet as if it were the
// whole thing is the one failure mode that misleads rather than annoys.
func TestPreviewTruncatesAndSaysSo(t *testing.T) {
	rows := make([][]Cell, maxPreviewRows+50)
	for i := range rows {
		rows[i] = []Cell{NumberCell(float64(i))}
	}
	preview := previewOf(t, []Sheet{{Name: "Big", Columns: []string{"n"}, Rows: rows}})
	sheet := preview.Sheets[0]

	if len(sheet.Rows) != maxPreviewRows {
		t.Errorf("rows shown = %d, want the cap of %d", len(sheet.Rows), maxPreviewRows)
	}
	if !sheet.Truncated {
		t.Error("a truncated sheet does not report itself as truncated")
	}
	if sheet.TotalRows != len(rows)+1 {
		t.Errorf("TotalRows = %d, want %d (data plus the header)", sheet.TotalRows, len(rows)+1)
	}
}

func TestColumnIndexInvertsColumnLetters(t *testing.T) {
	for _, index := range []int{0, 1, 25, 26, 51, 701, 702, 16383} {
		if got := columnIndex(columnLetters(index) + "7"); got != index {
			t.Errorf("columnIndex(%q) = %d, want %d", columnLetters(index)+"7", got, index)
		}
	}
}

func TestReadXLSXRejectsWhatIsNotAWorkbook(t *testing.T) {
	if _, err := ReadXLSX([]byte("this is not a zip")); err == nil {
		t.Error("plain bytes were accepted as a workbook")
	}
	// A valid package that is not a workbook — a .pptx, say — must be refused
	// rather than previewed as an empty grid.
	deck, err := BuildPPTX([]Slide{{Title: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := WritePackage(deck)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadXLSX(data); err == nil {
		t.Error("a .pptx was accepted as a workbook")
	}
}
