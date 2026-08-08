package ooxml

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openPackage is what every test here starts from: CI has no Excel, so the most
// a test can prove is that the bytes are a ZIP, that each part is well-formed
// XML, and that the parts the spec requires are present. Whether Excel actually
// opens it is checked by hand once per phase and recorded in the plan.
func openPackage(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("package is not a readable zip: %v", err)
	}
	parts := make(map[string]string, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") {
			decoder := xml.NewDecoder(bytes.NewReader(body))
			for {
				_, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("%s is not well-formed XML: %v", f.Name, err)
				}
			}
		}
		parts[f.Name] = string(body)
	}
	return parts
}

func buildPackage(t *testing.T, sheets []Sheet) map[string]string {
	t.Helper()
	parts, err := BuildXLSX(sheets)
	if err != nil {
		t.Fatalf("BuildXLSX: %v", err)
	}
	data, err := WritePackage(parts)
	if err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return openPackage(t, data)
}

// The spec's required parts. Excel does not report which one is missing — it
// offers to repair the file and then shows an empty workbook — so the list is
// asserted here where the failure can name the part.
func TestWorkbookHasEveryRequiredPart(t *testing.T) {
	parts := buildPackage(t, []Sheet{{Name: "Data", Columns: []string{"A"}, Rows: [][]Cell{{TextCell("x")}}}})
	for _, required := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"xl/workbook.xml",
		"xl/_rels/workbook.xml.rels",
		"xl/styles.xml",
		"xl/worksheets/sheet1.xml",
	} {
		if _, ok := parts[required]; !ok {
			t.Errorf("missing required part %s", required)
		}
	}
}

// Every part named in [Content_Types].xml has to exist and every worksheet has
// to be reachable through a relationship. A dangling entry in either direction
// is the most common way a hand-built package opens as "unreadable content".
func TestContentTypesAndRelationshipsResolve(t *testing.T) {
	sheets := []Sheet{
		{Name: "One", Columns: []string{"A"}},
		{Name: "Two", Columns: []string{"B"}},
		{Name: "Three", Columns: []string{"C"}},
	}
	parts := buildPackage(t, sheets)

	types := parts["[Content_Types].xml"]
	for name := range parts {
		if name == "[Content_Types].xml" || strings.HasSuffix(name, ".rels") {
			continue
		}
		if !strings.Contains(types, `PartName="/`+name+`"`) {
			t.Errorf("%s has no Override in [Content_Types].xml", name)
		}
	}

	rels := parts["xl/_rels/workbook.xml.rels"]
	workbook := parts["xl/workbook.xml"]
	for i := range sheets {
		target := "worksheets/sheet" + string(rune('1'+i)) + ".xml"
		if !strings.Contains(rels, `Target="`+target+`"`) {
			t.Errorf("no relationship targets %s", target)
		}
		if _, ok := parts["xl/"+target]; !ok {
			t.Errorf("relationship targets %s but the part is absent", target)
		}
	}
	if !strings.Contains(rels, `Target="styles.xml"`) {
		t.Error("styles.xml is never related from the workbook, so Excel ignores it")
	}
	// The styles relationship must not reuse a sheet's id.
	for i := range sheets {
		id := `Id="rId` + string(rune('1'+i)) + `"`
		if strings.Count(rels, id) != 1 {
			t.Errorf("relationship id %s is not unique", id)
		}
	}
	if strings.Count(workbook, "<sheet ") != len(sheets) {
		t.Errorf("workbook.xml lists %d sheets, want %d", strings.Count(workbook, "<sheet "), len(sheets))
	}
}

// The point of the whole tool. A number written as text looks identical on
// screen and silently breaks SUM, which is the one thing the person exporting
// an accounting sheet is going to do with it.
func TestNumbersAreNumericCellsAndTextStaysText(t *testing.T) {
	parts := buildPackage(t, []Sheet{{
		Name:    "Slips",
		Columns: []string{"Ref", "Amount"},
		Rows: [][]Cell{
			{TextCell("0012"), NumberCell(1234.5)},
		},
	}})
	sheet := parts["xl/worksheets/sheet1.xml"]

	if !strings.Contains(sheet, `<c r="B2"><v>1234.5</v></c>`) {
		t.Errorf("amount is not a numeric cell:\n%s", sheet)
	}
	// A leading zero survives only as text — coercing "0012" to 12 destroys an
	// invoice number, so InferCell must never do it.
	if !strings.Contains(sheet, `t="inlineStr"`) || !strings.Contains(sheet, `>0012<`) {
		t.Errorf("reference lost its leading zeros:\n%s", sheet)
	}
	if strings.Contains(sheet, `<c r="A2"><v>12</v>`) {
		t.Error(`"0012" was converted to the number 12`)
	}
}

func TestInferCellTypesJSONValues(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  CellKind
	}{
		{"bare number is a number", float64(42), KindNumber},
		{"formatted currency stays text", "฿1,234.50", KindText},
		{"iso date becomes a date", "2026-08-03", KindDate},
		{"iso timestamp keeps its clock", "2026-08-03 14:30", KindDateTime},
		{"ambiguous slash date stays text", "03/08/2026", KindText},
		{"leading zeros stay text", "0012", KindText},
		{"null is blank", nil, KindBlank},
		{"bool is a bool", true, KindBool},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InferCell(tc.value).Kind; got != tc.want {
				t.Errorf("InferCell(%#v).Kind = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// Excel stores a date as a day count from 1899-12-30 plus a number format. The
// serial is checked against a known value because an epoch that is off by one
// shifts every date in every export by a day — a mistake nobody notices until
// a report is already out.
func TestDatesAreSerialsWithADateFormat(t *testing.T) {
	day := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	if got := excelSerial(day); got != 46237 {
		t.Errorf("excelSerial(2026-08-03) = %v, want 46237", got)
	}
	// Excel's own reference point: 2008-01-01 is serial 39448.
	if got := excelSerial(time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC)); got != 39448 {
		t.Errorf("excelSerial(2008-01-01) = %v, want 39448", got)
	}
	// Noon is half a day.
	if got := excelSerial(time.Date(2008, 1, 1, 12, 0, 0, 0, time.UTC)); got != 39448.5 {
		t.Errorf("excelSerial(2008-01-01 12:00) = %v, want 39448.5", got)
	}

	parts := buildPackage(t, []Sheet{{
		Name:    "Dates",
		Columns: []string{"When"},
		Rows:    [][]Cell{{DateCell(day)}},
	}})
	sheet := parts["xl/worksheets/sheet1.xml"]
	if !strings.Contains(sheet, `<c r="A2" s="2"><v>46237</v></c>`) {
		t.Errorf("date cell is not a styled serial:\n%s", sheet)
	}
	// Without the numFmt the serial renders as 46237 and the user sees a number
	// where a date should be.
	if !strings.Contains(parts["xl/styles.xml"], `numFmtId="164"`) {
		t.Error("styles.xml has no date number format")
	}
}

// The reason this exists rather than a CSV. The bytes must come back out
// unchanged, and the header must be the exact Thai the caller passed.
func TestThaiTextSurvivesRoundTrip(t *testing.T) {
	const header = "ยอดเงินรวม"
	const value = "ค่าอาหารและเครื่องดื่ม ๙๙ บาท"
	parts := buildPackage(t, []Sheet{{
		Name:    "สรุปรายเดือน",
		Columns: []string{header},
		Rows:    [][]Cell{{TextCell(value)}},
	}})
	sheet := parts["xl/worksheets/sheet1.xml"]
	if !strings.Contains(sheet, header) {
		t.Errorf("Thai header did not survive:\n%s", sheet)
	}
	if !strings.Contains(sheet, value) {
		t.Errorf("Thai value did not survive:\n%s", sheet)
	}
	if !strings.Contains(parts["xl/workbook.xml"], "สรุปรายเดือน") {
		t.Error("Thai sheet name did not survive")
	}
}

// Control characters are XML 1.0 violations with no legal escape, and they turn
// up in real input — OCR of a scanned slip is a reliable source of them. One is
// enough to make Excel declare the whole workbook unreadable.
func TestControlCharactersAreStrippedNotEscaped(t *testing.T) {
	parts := buildPackage(t, []Sheet{{
		Name:    "Dirty",
		Columns: []string{"Note"},
		Rows:    [][]Cell{{TextCell("before\x01\x0bafter")}},
	}})
	sheet := parts["xl/worksheets/sheet1.xml"]
	if !strings.Contains(sheet, "beforeafter") {
		t.Errorf("control characters were not stripped:\n%s", sheet)
	}
	if strings.Contains(sheet, "&#x1;") || strings.Contains(sheet, "\x01") {
		t.Error("a control character reached the XML, which Excel rejects")
	}
}

// Excel refuses a file whose tab names break its rules rather than repairing
// them, and a model naming tabs after file paths or Thai months breaks all
// three rules at once.
func TestSheetNamesAreMadeLegalAndUnique(t *testing.T) {
	names := sheetNames([]Sheet{
		{Name: "reports/2026:Q1"},
		{Name: "  "},
		{Name: strings.Repeat("ก", 40)},
		{Name: "Data"},
		{Name: "data"},
		{Name: "Data"},
	})

	if strings.ContainsAny(names[0], `:\/?*[]`) {
		t.Errorf("reserved characters survived: %q", names[0])
	}
	if names[1] == "" {
		t.Error("an empty name must be replaced, Excel rejects it")
	}
	if got := len([]rune(names[2])); got > 31 {
		t.Errorf("name is %d characters, Excel allows 31: %q", got, names[2])
	}
	// Excel compares tab names case-insensitively, so "data" collides with
	// "Data" even though the strings differ.
	seen := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			t.Errorf("duplicate sheet name %q in %v", name, names)
		}
		seen[key] = true
	}
}

func TestColumnLettersFollowBijectiveBase26(t *testing.T) {
	cases := map[int]string{0: "A", 25: "Z", 26: "AA", 51: "AZ", 52: "BA", 701: "ZZ", 702: "AAA", 16383: "XFD"}
	for index, want := range cases {
		if got := columnLetters(index); got != want {
			t.Errorf("columnLetters(%d) = %q, want %q", index, got, want)
		}
	}
}

// Thai vowels and tone marks stack on the consonant instead of taking their own
// space. Counting them makes every Thai column noticeably too wide, which is
// the first thing anyone here would see on the first export.
func TestDisplayWidthIgnoresThaiCombiningMarks(t *testing.T) {
	// "กิน" is three code points but two columns wide — the sara i sits above.
	if got, want := displayWidth("กิน"), 2; got != want {
		t.Errorf("displayWidth(กิน) = %d, want %d", got, want)
	}
	if got, want := displayWidth("abc"), 3; got != want {
		t.Errorf("displayWidth(abc) = %d, want %d", got, want)
	}
	if got, want := displayWidth("日本語"), 6; got != want {
		t.Errorf("displayWidth(日本語) = %d, want %d", got, want)
	}
}

func TestColumnWidthsAreClampedAndCoverEveryColumn(t *testing.T) {
	widths := columnWidths(Sheet{
		Columns: []string{"ID"},
		Rows: [][]Cell{
			{TextCell("x"), TextCell(strings.Repeat("long ", 60))},
		},
	})
	if len(widths) != 2 {
		t.Fatalf("got %d widths, want one per column including the row's extra: %v", len(widths), widths)
	}
	if widths[0] < 8 {
		t.Errorf("narrow column was not padded to the minimum: %v", widths[0])
	}
	if widths[1] > 60 {
		t.Errorf("one long note pushed the column off screen: %v", widths[1])
	}
}

// A short row is padded on screen and a long one keeps its cells: dropping the
// extra values would lose data silently, which is worse than a ragged sheet.
func TestRaggedRowsKeepEveryValue(t *testing.T) {
	parts := buildPackage(t, []Sheet{{
		Name:    "Ragged",
		Columns: []string{"A", "B"},
		Rows: [][]Cell{
			{TextCell("one")},
			{TextCell("one"), TextCell("two"), TextCell("three")},
		},
	}})
	sheet := parts["xl/worksheets/sheet1.xml"]
	if !strings.Contains(sheet, `r="C3"`) {
		t.Errorf("the third value of a long row was dropped:\n%s", sheet)
	}
}

func TestWritePackageRejectsMalformedInput(t *testing.T) {
	if _, err := WritePackage(nil); err == nil {
		t.Error("an empty package must be an error")
	}
	if _, err := WritePackage([]Part{{Name: "xl/workbook.xml"}}); err == nil {
		t.Error("[Content_Types].xml must be the first part")
	}
	dup := []Part{{Name: "[Content_Types].xml"}, {Name: "a.xml"}, {Name: "a.xml"}}
	if _, err := WritePackage(dup); err == nil {
		t.Error("a duplicate part name must be an error, Excel rejects the file")
	}
	if _, err := BuildXLSX(nil); err == nil {
		t.Error("a workbook with no sheets must be an error")
	}
}

// A half-written .xlsx is not a smaller spreadsheet, it is a file Excel refuses
// to open. Rewriting an existing workbook must leave either the old file or the
// complete new one.
func TestWriteFileReplacesAnExistingWorkbook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.xlsx")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	parts, err := BuildXLSX([]Sheet{{Name: "S", Columns: []string{"A"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, parts); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	openPackage(t, data)

	// The temporary file must not be left behind next to the result.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("write left extra files behind: %v", entries)
	}
}

// The reason this feature exists. A total written as a number is a photograph
// of an answer: the user edits one amount above it and the total silently stops
// being true. A formula cell carries no cached value at all, so what the reader
// sees is always what their own spreadsheet worked out.
func TestAFormulaIsCalculatedRatherThanRemembered(t *testing.T) {
	parts := buildPackage(t, []Sheet{{
		Name:    "Bill",
		Columns: []string{"Item", "Amount"},
		Rows: [][]Cell{
			{TextCell("a"), NumberCell(100)},
			{TextCell("b"), NumberCell(250)},
			{TextCell("รวม"), FormulaCell("=SUM(B2:B3)")},
		},
	}})

	sheet := parts["xl/worksheets/sheet1.xml"]
	if !strings.Contains(sheet, `<f>SUM(B2:B3)</f>`) {
		t.Errorf("the total is not a formula:\n%s", sheet)
	}
	// A <v> beside the <f> is the cached answer, and a cached answer we invented
	// is exactly the failure the formula was meant to remove.
	if strings.Contains(sheet, `<f>SUM(B2:B3)</f><v>`) {
		t.Error("a cached value was written beside the formula")
	}
	// Without this, a formula with no cached value can render blank until the
	// user touches the sheet.
	if !strings.Contains(parts["xl/workbook.xml"], `fullCalcOnLoad="1"`) {
		t.Error("the workbook does not ask its reader to calculate on load")
	}
}

// `=` is how every spreadsheet spells "formula", so it is what a model reaches
// for unprompted. A lone "=" is a typo rather than an empty formula: writing it
// as one produces a file Excel offers to repair.
func TestALeadingEqualsMakesAFormulaAndALoneEqualsDoesNot(t *testing.T) {
	if got := InferCell("=B2*C2"); got.Kind != KindFormula || got.Text != "B2*C2" {
		t.Errorf("InferCell(=B2*C2) = %+v, want a formula holding B2*C2", got)
	}
	if got := InferCell("="); got.Kind != KindText {
		t.Errorf("InferCell(=) = %+v, want plain text", got)
	}
	// Text that merely mentions arithmetic is still text.
	if got := InferCell("1+1=2"); got.Kind != KindText {
		t.Errorf("InferCell(1+1=2) = %+v, want plain text", got)
	}
}

// A column's format decides how its cells read, including its formulas — a
// total in a money column is money. Percent holds the fraction, which is the
// one place a caller can be wrong by a factor of a hundred.
func TestAColumnFormatStylesItsCellsIncludingFormulas(t *testing.T) {
	parts := buildPackage(t, []Sheet{{
		Name:    "Rates",
		Columns: []string{"Item", "Amount", "Share", "Count"},
		Formats: []string{"", "money", "percent", "integer"},
		Rows: [][]Cell{
			{TextCell("a"), NumberCell(1234.5), NumberCell(0.07), NumberCell(3)},
			{TextCell("รวม"), FormulaCell("=SUM(B2:B2)"), BlankCell(), BlankCell()},
		},
	}})

	sheet := parts["xl/worksheets/sheet1.xml"]
	for _, want := range []string{
		`<c r="B2" s="4">`, // money
		`<c r="C2" s="6">`, // percent
		`<c r="D2" s="5">`, // integer
		`<c r="B3" s="4">`, // the formula wears its column's format too
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("missing %s in:\n%s", want, sheet)
		}
	}
	styles := parts["xl/styles.xml"]
	for _, want := range []string{`formatCode="#,##0.00"`, `formatCode="#,##0"`, `formatCode="0.00%"`} {
		if !strings.Contains(styles, want) {
			t.Errorf("styles.xml is missing %s", want)
		}
	}
}

// A format list shorter than the header is the normal shape — only the money
// columns get named — and must not throw the rest of the row out of alignment.
func TestAShortFormatListLeavesTheRestPlain(t *testing.T) {
	parts := buildPackage(t, []Sheet{{
		Name:    "Partial",
		Columns: []string{"Item", "Amount", "Note"},
		Formats: []string{"", "money"},
		Rows:    [][]Cell{{TextCell("a"), NumberCell(5), TextCell("n")}},
	}})

	sheet := parts["xl/worksheets/sheet1.xml"]
	if !strings.Contains(sheet, `<c r="B2" s="4">`) {
		t.Errorf("the named column lost its format:\n%s", sheet)
	}
	if !strings.Contains(sheet, `<c r="C2" t="inlineStr">`) {
		t.Errorf("the unnamed column did not stay plain:\n%s", sheet)
	}
}

// A table nobody can sort or filter is a picture of a table. The filter covers
// the header and every row under it, and sits after </sheetData> because the
// schema says so — Excel refuses the file if it sits with the view settings.
func TestTheFilterCoversEveryRowAndSitsWhereTheSchemaWantsIt(t *testing.T) {
	parts := buildPackage(t, []Sheet{{
		Name:    "Data",
		Columns: []string{"A", "B", "C"},
		Rows:    [][]Cell{{TextCell("1")}, {TextCell("2")}},
	}})

	sheet := parts["xl/worksheets/sheet1.xml"]
	if !strings.Contains(sheet, `<autoFilter ref="A1:C3"/>`) {
		t.Errorf("filter is missing or covers the wrong range:\n%s", sheet)
	}
	if strings.Index(sheet, "<autoFilter") < strings.Index(sheet, "</sheetData>") {
		t.Error("autoFilter was written before </sheetData>, which Excel refuses")
	}
}

// A sheet with no header has no columns to filter by, so it gets no filter
// rather than one over a range that names nothing.
func TestASheetWithNoHeaderGetsNoFilter(t *testing.T) {
	parts := buildPackage(t, []Sheet{{Name: "Bare", Rows: [][]Cell{{TextCell("x")}}}})
	if strings.Contains(parts["xl/worksheets/sheet1.xml"], "<autoFilter") {
		t.Error("a headerless sheet was given a filter")
	}
}
