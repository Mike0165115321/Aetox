package ooxml

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// CellKind is what the cell *is*, not how it looks. Excel stores a number, a
// date and a numeric-looking string as three different things, and the
// difference is the whole value of exporting a spreadsheet rather than a CSV:
// a column of amounts that arrived as text cannot be summed, which makes the
// file worthless for the accounting work it was produced for.
type CellKind int

const (
	KindBlank CellKind = iota
	KindText
	KindNumber
	KindDate
	KindDateTime
	KindBool
	// KindFormula is a cell whose value Excel works out, not one we computed and
	// wrote down. The difference is the whole point of a total row: a number we
	// wrote is a photograph of an answer, and stops being true the moment the
	// user edits a cell above it.
	KindFormula
)

type Cell struct {
	Kind CellKind
	Text string
	Num  float64
	Bool bool
}

func TextCell(s string) Cell    { return Cell{Kind: KindText, Text: s} }
func NumberCell(f float64) Cell { return Cell{Kind: KindNumber, Num: f} }
func BoolCell(b bool) Cell      { return Cell{Kind: KindBool, Bool: b} }
func BlankCell() Cell           { return Cell{Kind: KindBlank} }

// FormulaCell holds the expression without its leading `=`, which is how the
// file stores it. No cached result travels with it: the workbook asks Excel to
// calculate on load, so a made-up cached value can never be what the user sees.
func FormulaCell(expr string) Cell {
	return Cell{Kind: KindFormula, Text: strings.TrimPrefix(strings.TrimSpace(expr), "=")}
}

// DateCell stores a date the way Excel does — a day count, not a string — so
// sorting, filtering and date arithmetic work on it. A zero clock time is kept
// as a plain date so the cell reads "2026-08-03" rather than "2026-08-03 00:00".
func DateCell(t time.Time) Cell {
	kind := KindDateTime
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		kind = KindDate
	}
	return Cell{Kind: kind, Num: excelSerial(t)}
}

// excelSerial converts a time to Excel's day-serial.
//
// The epoch is 1899-12-30 rather than 1900-01-01 because Excel deliberately
// keeps Lotus 1-2-3's belief that 1900 was a leap year; shifting the epoch back
// two days is the standard way to land on the right serial for every date from
// 1900-03-01 onward, which is every date anyone exports. Dates before that are
// off by one — not worth carrying a special case for.
func excelSerial(t time.Time) float64 {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	naive := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
	return naive.Sub(base).Hours() / 24
}

// dateLayouts are ISO-8601 shapes only, on purpose. "03/08/2026" is 3 August in
// Thailand and 8 March in the United States, and a spreadsheet that guesses
// wrong is worse than one that leaves the value as text — the error is silent
// and the file looks right.
var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02",
}

// InferCell types a value that arrived as JSON.
//
// The model sends rows as plain JSON, so the wire types carry the intent: a
// bare number is a number, a quoted "1,234.50" stays text. Nothing is coerced
// out of a string except an ISO date, and never the other way — an invoice
// number like "0012" or a Thai national ID must not lose its leading zero to a
// helpful conversion.
func InferCell(v any) Cell {
	switch value := v.(type) {
	case nil:
		return BlankCell()
	case bool:
		return BoolCell(value)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return TextCell(strconv.FormatFloat(value, 'g', -1, 64))
		}
		return NumberCell(value)
	case int:
		return NumberCell(float64(value))
	case int64:
		return NumberCell(float64(value))
	case json.Number:
		if f, err := value.Float64(); err == nil {
			return NumberCell(f)
		}
		return TextCell(value.String())
	case string:
		trimmed := strings.TrimSpace(value)
		// A leading `=` is how every spreadsheet on earth spells "this is a
		// formula", so it is what a model reaches for without being told, and
		// what a user typing into the tool by hand would write. A lone "=" is
		// not one — it is a typo, and treating it as an empty formula produces a
		// file Excel offers to repair.
		if len(trimmed) > 1 && strings.HasPrefix(trimmed, "=") {
			return FormulaCell(trimmed)
		}
		if trimmed != "" {
			for _, layout := range dateLayouts {
				if t, err := time.Parse(layout, trimmed); err == nil {
					return DateCell(t)
				}
			}
		}
		return TextCell(value)
	default:
		return TextCell(fmt.Sprint(value))
	}
}

// Sheet is one tab. Columns is the header row; a row shorter than the header is
// padded with blanks and a longer one keeps its extra cells, because losing
// data silently is the worse failure.
type Sheet struct {
	Name    string
	Columns []string
	Rows    [][]Cell
	// Formats names how each column is *displayed*, one entry per column, and
	// says nothing about what its cells are. A column of amounts is a column of
	// numbers whether or not anybody asked for thousands separators — the format
	// is what stops the user reading 1234.5 and reaching for a calculator.
	//
	// Per column rather than per cell because that is how a table works: the
	// alternative asks the model to repeat the same word down four hundred rows,
	// and one row where it forgot is a cell that silently reads differently from
	// its neighbours.
	Formats []string
}

const (
	styleDefault  = 0
	styleHeader   = 1
	styleDate     = 2
	styleDateTime = 3
	styleMoney    = 4
	styleInteger  = 5
	stylePercent  = 6
)

// styleForFormat maps the names the tool accepts onto the style table below.
// A name nobody recognises falls back to the default rather than failing the
// whole workbook: a column that came out unformatted is a cosmetic complaint,
// and a refused export is a lost job.
func styleForFormat(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "money", "currency", "amount":
		return styleMoney
	case "integer", "int", "count":
		return styleInteger
	case "percent", "percentage":
		return stylePercent
	case "date":
		return styleDate
	case "datetime":
		return styleDateTime
	default:
		return styleDefault
	}
}

// BuildXLSX turns sheets into the parts of a .xlsx package.
//
// Strings go in as inline strings rather than through a shared-string table.
// The table exists to deduplicate repeated text across a large workbook; for
// the report-sized output this produces it buys nothing and costs a part, an
// index to keep consistent, and a second place for the file to be malformed.
func BuildXLSX(sheets []Sheet) ([]Part, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("ooxml: workbook needs at least one sheet")
	}

	names := sheetNames(sheets)

	var contentTypes strings.Builder
	contentTypes.WriteString(xmlHeader)
	contentTypes.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	contentTypes.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	contentTypes.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	contentTypes.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	contentTypes.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)

	var workbook strings.Builder
	workbook.WriteString(xmlHeader)
	workbook.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)

	var rels strings.Builder
	rels.WriteString(xmlHeader)
	rels.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)

	parts := make([]Part, 0, len(sheets)+5)
	for i, sheet := range sheets {
		target := fmt.Sprintf("worksheets/sheet%d.xml", i+1)
		contentTypes.WriteString(`<Override PartName="/xl/` + target + `" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`)
		fmt.Fprintf(&workbook, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, escapeXML(names[i]), i+1, i+1)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="%s"/>`, i+1, target)
		parts = append(parts, Part{Name: "xl/" + target, Data: []byte(worksheetXML(sheet))})
	}
	contentTypes.WriteString(`</Types>`)
	// fullCalcOnLoad is what makes a formula cell honest: no cell carries a
	// cached result, so the first thing every reader does is work them out.
	// Without it a formula written with no <v> can render blank until the user
	// touches something.
	workbook.WriteString(`</sheets><calcPr calcId="0" fullCalcOnLoad="1"/></workbook>`)
	fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, len(sheets)+1)
	rels.WriteString(`</Relationships>`)

	// [Content_Types].xml must come first — WritePackage enforces it, and the
	// worksheets were appended before it existed, so the fixed parts are
	// prepended here rather than the slice being built in reader order.
	head := []Part{
		{Name: "[Content_Types].xml", Data: []byte(contentTypes.String())},
		{Name: "_rels/.rels", Data: []byte(rootRels)},
		{Name: "xl/workbook.xml", Data: []byte(workbook.String())},
		{Name: "xl/_rels/workbook.xml.rels", Data: []byte(rels.String())},
		{Name: "xl/styles.xml", Data: []byte(stylesXML)},
	}
	return append(head, parts...), nil
}

const rootRels = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
	`</Relationships>`

// stylesXML is the smallest style table Excel accepts, plus the three formats
// this package actually uses. The fonts, fills, borders and cellStyleXfs lists
// are not optional decoration: Excel reports the workbook as corrupt if any of
// them is missing, even when every cell uses style 0.
//
// No <color theme="..."/> anywhere — a theme reference without a theme part is
// exactly the kind of dangling link that triggers the repair prompt.
const stylesXML = xmlHeader +
	`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
	`<numFmts count="5">` +
	`<numFmt numFmtId="164" formatCode="yyyy\-mm\-dd"/>` +
	`<numFmt numFmtId="165" formatCode="yyyy\-mm\-dd\ hh:mm"/>` +
	// No currency symbol in the format: which currency it is belongs in the
	// header, where a person reads it, rather than repeated down four hundred
	// cells a formula reads. Thousands separators and two decimals are what the
	// cell itself needs to stop being misread.
	`<numFmt numFmtId="166" formatCode="#,##0.00"/>` +
	`<numFmt numFmtId="167" formatCode="#,##0"/>` +
	`<numFmt numFmtId="168" formatCode="0.00%"/>` +
	`</numFmts>` +
	`<fonts count="2">` +
	`<font><sz val="11"/><color rgb="FF000000"/><name val="Calibri"/><family val="2"/></font>` +
	`<font><b/><sz val="11"/><color rgb="FF000000"/><name val="Calibri"/><family val="2"/></font>` +
	`</fonts>` +
	`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
	`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
	`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
	`<cellXfs count="7">` +
	`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
	`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>` +
	`<xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="165" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="166" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="167" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="168" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`</cellXfs>` +
	`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
	`</styleSheet>`

func worksheetXML(sheet Sheet) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)

	// A header row that scrolls away turns a long export into a guessing game
	// about which column is which, so it is frozen. The child order below is
	// fixed by the schema (sheetViews, then cols, then sheetData) and Excel
	// rejects the file if they are swapped.
	if len(sheet.Columns) > 0 {
		b.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	}

	if widths := columnWidths(sheet); len(widths) > 0 {
		b.WriteString(`<cols>`)
		for i, w := range widths {
			fmt.Fprintf(&b, `<col min="%d" max="%d" width="%.2f" customWidth="1"/>`, i+1, i+1, w)
		}
		b.WriteString(`</cols>`)
	}

	b.WriteString(`<sheetData>`)
	rowNum := 1
	if len(sheet.Columns) > 0 {
		b.WriteString(`<row r="1">`)
		for i, name := range sheet.Columns {
			writeCell(&b, i, rowNum, TextCell(name), styleHeader)
		}
		b.WriteString(`</row>`)
		rowNum++
	}
	for _, row := range sheet.Rows {
		fmt.Fprintf(&b, `<row r="%d">`, rowNum)
		for i, cell := range row {
			writeCell(&b, i, rowNum, cell, styleFor(cell, sheet.columnFormat(i)))
		}
		b.WriteString(`</row>`)
		rowNum++
	}
	b.WriteString(`</sheetData>`)

	// The filter dropdowns, over the header and everything under it. After
	// </sheetData> because the schema says so — Excel refuses the file if it
	// sits with the other view settings up top.
	//
	// Always on rather than asked for: a table the user cannot sort or filter is
	// a picture of a table, and remembering to switch it on is not a decision
	// worth handing to whoever wrote the brief.
	if ref := filterRef(sheet); ref != "" {
		fmt.Fprintf(&b, `<autoFilter ref="%s"/>`, ref)
	}
	b.WriteString(`</worksheet>`)
	return b.String()
}

// columnFormat is the format named for column i, or "" — tolerant of a Formats
// list shorter than the header, which is the normal shape when only the money
// columns were named.
func (s Sheet) columnFormat(i int) string {
	if i < 0 || i >= len(s.Formats) {
		return ""
	}
	return s.Formats[i]
}

// styleFor decides how one cell is displayed. The column's own format wins,
// including over a formula: `=SUM(...)` in a money column is money, and a
// column that says nothing falls back to what the value *is*.
func styleFor(cell Cell, format string) int {
	if s := styleForFormat(format); s != styleDefault {
		return s
	}
	switch cell.Kind {
	case KindDate:
		return styleDate
	case KindDateTime:
		return styleDateTime
	default:
		return styleDefault
	}
}

// filterRef is A1 through the last column of the last row. Empty when there is
// no header, because a filter needs one to name the columns it filters.
func filterRef(sheet Sheet) string {
	if len(sheet.Columns) == 0 {
		return ""
	}
	cols := len(sheet.Columns)
	for _, row := range sheet.Rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	return "A1:" + columnLetters(cols-1) + strconv.Itoa(len(sheet.Rows)+1)
}

// writeCell emits one <c>. A blank cell with the default style is skipped
// entirely — sparse rows are legal and smaller, and Excel renders the gap.
func writeCell(b *strings.Builder, col, row int, cell Cell, style int) {
	if cell.Kind == KindBlank && style == styleDefault {
		return
	}
	ref := columnLetters(col) + strconv.Itoa(row)
	attrs := `r="` + ref + `"`
	if style != styleDefault {
		attrs += ` s="` + strconv.Itoa(style) + `"`
	}
	switch cell.Kind {
	case KindNumber, KindDate, KindDateTime:
		// 'f' rather than 'g': an exponent form is legal XML but Excel shows
		// some of them as text, and a serial date in exponent notation is not
		// a date at all.
		fmt.Fprintf(b, `<c %s><v>%s</v></c>`, attrs, strconv.FormatFloat(cell.Num, 'f', -1, 64))
	case KindFormula:
		// No <v>. A cached value we invented would be what the user sees until
		// something forces a recalculation, which is the same failure as writing
		// the total down by hand — so the workbook asks for a full calculation
		// on load (see calcPr) and the answer is always Excel's own.
		fmt.Fprintf(b, `<c %s><f>%s</f></c>`, attrs, escapeXML(cell.Text))
	case KindBool:
		v := "0"
		if cell.Bool {
			v = "1"
		}
		fmt.Fprintf(b, `<c %s t="b"><v>%s</v></c>`, attrs, v)
	case KindBlank:
		fmt.Fprintf(b, `<c %s/>`, attrs)
	default:
		// xml:space="preserve" keeps leading and trailing spaces, which matter
		// in exports of fixed-width source data.
		fmt.Fprintf(b, `<c %s t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, attrs, escapeXML(cell.Text))
	}
}

// columnLetters converts a zero-based index to a spreadsheet column name:
// 0→A, 25→Z, 26→AA. Bijective base-26, so the usual off-by-one of ordinary
// base conversion (which would give A@ instead of AA) is handled by the
// decrement before each digit.
func columnLetters(index int) string {
	var out [8]byte
	pos := len(out)
	for index >= 0 {
		pos--
		out[pos] = byte('A' + index%26)
		index = index/26 - 1
	}
	return string(out[pos:])
}

// columnWidths sizes each column to its widest value, so the export opens
// readable instead of as a row of ###. Clamped at both ends: too narrow hides
// headers, and one long free-text note would otherwise push a column off-screen.
func columnWidths(sheet Sheet) []float64 {
	count := len(sheet.Columns)
	for _, row := range sheet.Rows {
		if len(row) > count {
			count = len(row)
		}
	}
	if count == 0 {
		return nil
	}
	widths := make([]float64, count)
	for i, name := range sheet.Columns {
		widths[i] = float64(displayWidth(name))
	}
	for _, row := range sheet.Rows {
		for i, cell := range row {
			if w := float64(displayWidth(cellDisplay(cell))); w > widths[i] {
				widths[i] = w
			}
		}
	}
	for i := range widths {
		// +2 leaves room for the header's filter arrow and a little padding.
		widths[i] = math.Max(8, math.Min(60, widths[i]+2))
	}
	return widths
}

func cellDisplay(cell Cell) string {
	switch cell.Kind {
	case KindText:
		return cell.Text
	case KindNumber:
		return strconv.FormatFloat(cell.Num, 'f', -1, 64)
	case KindDate:
		return "0000-00-00"
	case KindDateTime:
		return "0000-00-00 00:00"
	case KindBool:
		return "FALSE"
	case KindFormula:
		// Its result is not known here — only Excel works it out — and sizing
		// the column to the *expression* would make a `=SUMIFS(...)` column
		// sixty characters wide to hold a number. A total-sized placeholder is
		// the honest guess, and the alternative was zero: a total row rendering
		// as ### in the one cell the reader looks at first.
		return "0,000,000.00"
	default:
		return ""
	}
}

// displayWidth counts columns of text, not runes.
//
// Thai vowels and tone marks stack above and below the consonant rather than
// occupying their own space, so counting them makes every Thai column roughly
// half again too wide — visible as a wall of whitespace on the first export
// anyone here would try. Wide CJK forms have the opposite problem.
func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case r >= 0x0E31 && r <= 0x0E3A, r >= 0x0E47 && r <= 0x0E4E:
			// Thai above/below vowels and tone marks: zero advance.
		case r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0xA4CF,
			r >= 0xAC00 && r <= 0xD7A3, r >= 0xF900 && r <= 0xFAFF,
			r >= 0xFF00 && r <= 0xFF60, r >= 0xFFE0 && r <= 0xFFE6:
			width += 2
		default:
			width++
		}
	}
	return width
}

// sheetNames makes every tab name legal and unique.
//
// Excel's rules are non-negotiable and it does not repair a violation — it
// refuses the file. The characters below are reserved by the formula syntax for
// sheet references, the limit is 31 characters, and two tabs cannot share a
// name. A model naming tabs after Thai months or file paths hits all three.
func sheetNames(sheets []Sheet) []string {
	names := make([]string, len(sheets))
	used := make(map[string]bool, len(sheets))
	for i, sheet := range sheets {
		name := strings.Map(func(r rune) rune {
			switch r {
			case ':', '\\', '/', '?', '*', '[', ']':
				return '-'
			}
			if r < 0x20 {
				return -1
			}
			return r
		}, strings.TrimSpace(sheet.Name))
		name = strings.Trim(name, "'")
		if name == "" {
			name = fmt.Sprintf("Sheet%d", i+1)
		}
		name = truncateRunes(name, 31)

		candidate := name
		for n := 2; used[strings.ToLower(candidate)]; n++ {
			suffix := fmt.Sprintf(" (%d)", n)
			candidate = truncateRunes(name, 31-len([]rune(suffix))) + suffix
		}
		used[strings.ToLower(candidate)] = true
		names[i] = candidate
	}
	return names
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
