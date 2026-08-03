package ooxml

// Reading a workbook back, for one purpose: showing the user what they just
// got without making them leave the app.
//
// This is deliberately not the counterpart of the writer. It does not
// round-trip, it does not preserve formatting, and it will never be the input
// to an edit — everything here produces *display strings*, and the button that
// hands the file to Excel stays exactly where it is for anything more than a
// glance. OFFICE-EXPORT-PLAN.md §8 rules out reading Office files as a feature;
// what this is instead is the preview for a file the agent just wrote, which is
// a much smaller problem: the shape is known because we chose it.
//
// It still reads more than we write. Every workbook produced anywhere else uses
// a shared-string table where ours uses inline strings, and a preview that only
// worked on Aetox's own files would be the more confusing half-feature.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Preview limits. A preview is a glance, not the file: a hundred thousand rows
// would arrive as JSON, cross an IPC boundary and be turned into DOM nodes, and
// the answer to "what did I get?" was already on screen after twenty.
const (
	maxPreviewRows    = 500
	maxPreviewColumns = 64
	maxPreviewSheets  = 20
)

// SheetPreview is one tab of a workbook, already rendered to strings.
type SheetPreview struct {
	Name string     `json:"name"`
	Rows [][]string `json:"rows"`
	// Truncated reports that the sheet has more than the preview shows, so the
	// UI can say so rather than quietly displaying a prefix as if it were all.
	Truncated bool `json:"truncated"`
	// TotalRows is the real row count, for that message.
	TotalRows int `json:"totalRows"`
}

// WorkbookPreview is what the file pane draws.
type WorkbookPreview struct {
	Sheets []SheetPreview `json:"sheets"`
}

// ReadXLSX renders a workbook for display.
func ReadXLSX(data []byte) (*WorkbookPreview, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a readable workbook: %w", err)
	}
	parts := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		parts[strings.TrimPrefix(f.Name, "/")] = body
	}
	if _, ok := parts["xl/workbook.xml"]; !ok {
		return nil, fmt.Errorf("not a workbook: xl/workbook.xml is missing")
	}

	strings_ := readSharedStrings(parts["xl/sharedStrings.xml"])
	dateStyles := readDateStyles(parts["xl/styles.xml"])
	names, targets := readWorkbookSheets(parts["xl/workbook.xml"], parts["xl/_rels/workbook.xml.rels"])

	preview := &WorkbookPreview{}
	for i, target := range targets {
		if i >= maxPreviewSheets {
			break
		}
		body, ok := parts[target]
		if !ok {
			continue
		}
		sheet := readWorksheet(body, strings_, dateStyles)
		sheet.Name = names[i]
		preview.Sheets = append(preview.Sheets, sheet)
	}
	if len(preview.Sheets) == 0 {
		return nil, fmt.Errorf("workbook has no readable sheets")
	}
	return preview, nil
}

// readWorkbookSheets pairs each sheet's name with the part that holds it.
//
// The order in workbook.xml is the tab order the user sees; the relationship id
// is the only link to the file, and the two are in different parts. Falls back
// to positional sheetN.xml naming when the rels part is unreadable, because a
// preview that shows the right cells under a wrong tab name is far better than
// no preview.
func readWorkbookSheets(workbook, rels []byte) (names []string, targets []string) {
	type relationship struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	}
	var relDoc struct {
		Items []relationship `xml:"Relationship"`
	}
	_ = xml.Unmarshal(rels, &relDoc)
	byID := make(map[string]string, len(relDoc.Items))
	for _, r := range relDoc.Items {
		target := r.Target
		if !strings.HasPrefix(target, "/") && !strings.HasPrefix(target, "xl/") {
			target = "xl/" + target
		}
		byID[r.ID] = strings.TrimPrefix(target, "/")
	}

	var wbDoc struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	_ = xml.Unmarshal(workbook, &wbDoc)

	for i, s := range wbDoc.Sheets {
		name := s.Name
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("Sheet%d", i+1)
		}
		target, ok := byID[s.ID]
		if !ok {
			target = fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		}
		names = append(names, name)
		targets = append(targets, target)
	}
	return names, targets
}

// readSharedStrings loads the shared-string table.
//
// Aetox's own writer never produces one — it uses inline strings, which cost a
// part and an index less. Every other producer does, so a preview without this
// would show an empty grid for any workbook the user did not get from here.
//
// A rich-text entry is several <r><t> runs that concatenate into one value;
// taking only the first is how half a sentence goes missing.
func readSharedStrings(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var doc struct {
		Items []struct {
			T    string   `xml:"t"`
			Runs []string `xml:"r>t"`
		} `xml:"si"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	out := make([]string, 0, len(doc.Items))
	for _, item := range doc.Items {
		if len(item.Runs) > 0 {
			out = append(out, strings.Join(item.Runs, ""))
			continue
		}
		out = append(out, item.T)
	}
	return out
}

// readDateStyles works out which cell-style indexes mean "this number is a
// date", because in xlsx that is the only thing separating 46237 from
// 2026-08-03 — the cell holds a day count either way, and the number format is
// what a reader has to consult to tell them apart.
func readDateStyles(data []byte) map[int]bool {
	dates := map[int]bool{}
	if len(data) == 0 {
		return dates
	}
	var doc struct {
		NumFmts []struct {
			ID   int    `xml:"numFmtId,attr"`
			Code string `xml:"formatCode,attr"`
		} `xml:"numFmts>numFmt"`
		Xfs []struct {
			NumFmtID int `xml:"numFmtId,attr"`
		} `xml:"cellXfs>xf"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return dates
	}

	custom := map[int]bool{}
	for _, f := range doc.NumFmts {
		custom[f.ID] = looksLikeDateFormat(f.Code)
	}
	for style, xf := range doc.Xfs {
		if isBuiltinDateFormat(xf.NumFmtID) || custom[xf.NumFmtID] {
			dates[style] = true
		}
	}
	return dates
}

// isBuiltinDateFormat lists the reserved format ids that are dates or times.
// They are fixed by the spec and never written into styles.xml, so a reader
// that only consults numFmts misses every date in a file that used them — which
// is most files, since Excel's own date formats live here.
func isBuiltinDateFormat(id int) bool {
	switch {
	case id >= 14 && id <= 22:
		return true
	case id >= 27 && id <= 36:
		return true
	case id >= 45 && id <= 47:
		return true
	case id >= 50 && id <= 58:
		return true
	}
	return false
}

// looksLikeDateFormat decides whether a custom format code is a date.
//
// Quoted literals are skipped: a currency format like "m" for metres, or a
// Thai baht format carrying the word "มี.ค." in quotes, would otherwise be read
// as a month token and turn an amount into a date.
func looksLikeDateFormat(code string) bool {
	inQuote := false
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c == '"':
			inQuote = !inQuote
		case inQuote:
			// skip
		case c == '\\' || c == '[':
			// An escaped character or a locale/colour block: skip the next one.
			if c == '[' {
				for i < len(code) && code[i] != ']' {
					i++
				}
			} else {
				i++
			}
		case c == 'y' || c == 'Y' || c == 'd' || c == 'D':
			return true
		case c == 'm' || c == 'M' || c == 'h' || c == 'H' || c == 's' || c == 'S':
			return true
		}
	}
	return false
}

// readWorksheet turns one sheet part into display rows.
//
// Streamed rather than unmarshalled whole: a sheet is the one part of a
// workbook with no size bound, and the preview only needs its first screenful.
func readWorksheet(data []byte, shared []string, dateStyles map[int]bool) SheetPreview {
	sheet := SheetPreview{}
	decoder := xml.NewDecoder(bytes.NewReader(data))

	var row []string
	inRow := false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			if end, ok := token.(xml.EndElement); ok && end.Name.Local == "row" && inRow {
				inRow = false
				sheet.TotalRows++
				if len(sheet.Rows) < maxPreviewRows {
					sheet.Rows = append(sheet.Rows, row)
				} else {
					sheet.Truncated = true
				}
			}
			continue
		}
		switch start.Name.Local {
		case "row":
			inRow = true
			row = nil
		case "c":
			if !inRow {
				continue
			}
			text, column := readCell(decoder, start, shared, dateStyles)
			// A sparse row skips empty cells entirely, so the column reference
			// is the only thing that keeps the remaining values in their own
			// columns instead of sliding left.
			for len(row) < column && len(row) < maxPreviewColumns {
				row = append(row, "")
			}
			if len(row) < maxPreviewColumns {
				row = append(row, text)
			}
		}
	}
	if inRow {
		sheet.TotalRows++
		if len(sheet.Rows) < maxPreviewRows {
			sheet.Rows = append(sheet.Rows, row)
		}
	}
	return sheet
}

// readCell renders one <c> and reports which column it belongs to.
func readCell(decoder *xml.Decoder, start xml.StartElement, shared []string, dateStyles map[int]bool) (string, int) {
	cellType, style, column := "", -1, 0
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "t":
			cellType = attr.Value
		case "s":
			style, _ = strconv.Atoi(attr.Value)
		case "r":
			column = columnIndex(attr.Value)
		}
	}

	var value, inline strings.Builder
	depth := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			// Only <v> and the text inside <is> matter. <f> is a formula: its
			// text is the expression, and showing "SUM(C2:C6)" where the user
			// expects 4440.25 would be worse than showing nothing.
			if t.Name.Local == "f" {
				_ = decoder.Skip()
				depth--
			}
		case xml.CharData:
			// Which builder receives the text depends on where we are: <v> holds
			// the value, <is><t> holds an inline string.
			if cellType == "inlineStr" {
				inline.Write(t)
			} else {
				value.Write(t)
			}
		case xml.EndElement:
			if t.Name.Local == "c" && depth == 0 {
				return renderCell(cellType, style, strings.TrimSpace(value.String()), inline.String(), shared, dateStyles), column
			}
			depth--
		}
	}
	return "", column
}

func renderCell(cellType string, style int, value, inline string, shared []string, dateStyles map[int]bool) string {
	switch cellType {
	case "inlineStr":
		return inline
	case "s":
		index, err := strconv.Atoi(value)
		if err != nil || index < 0 || index >= len(shared) {
			return ""
		}
		return shared[index]
	case "str":
		return value
	case "b":
		if value == "1" {
			return "TRUE"
		}
		return "FALSE"
	case "e":
		return value
	}
	if value == "" {
		return ""
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	if dateStyles[style] {
		return formatSerial(number)
	}
	return strconv.FormatFloat(number, 'f', -1, 64)
}

// formatSerial turns Excel's day count back into a date, inverting excelSerial.
// A serial with a fractional part carries a clock time and is shown with one.
func formatSerial(serial float64) string {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	// Rounded to the second: floating point makes 14:30 arrive as 14:29:59.9997,
	// and a preview that says 14:29 for a cell Excel shows as 14:30 is a bug
	// report waiting to happen.
	moment := base.Add(time.Duration(serial * 24 * float64(time.Hour)).Round(time.Second))
	if moment.Hour() == 0 && moment.Minute() == 0 && moment.Second() == 0 {
		return moment.Format("2006-01-02")
	}
	return moment.Format("2006-01-02 15:04")
}

// columnIndex reads the column out of a cell reference ("B7" → 1), inverting
// columnLetters. Returns 0 for a reference it cannot read, which puts the value
// in the first free column rather than dropping it.
func columnIndex(ref string) int {
	index := 0
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c < 'A' || c > 'Z' {
			break
		}
		index = index*26 + int(c-'A'+1)
	}
	if index == 0 {
		return 0
	}
	return index - 1
}
