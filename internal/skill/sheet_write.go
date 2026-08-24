package skill

// sheet_write is the first tool in Aetox that produces a binary file.
//
// Everything the agent could hand back until now was text, and for the people
// this is built for a .md file is not a finished job — a file that opens in
// Excel is. The website's own flagship example ends in a table, and the best it
// could do was a CSV: no header, no column widths, and Thai text that Excel
// mis-decodes often enough that the first thing the user sees is garbage.
//
// The division of labour is the same one image_ocr uses against a model with no
// vision: the model sends structure as ordinary JSON and knows nothing about
// OOXML, and internal/ooxml assembles the ZIP of XML that Excel actually wants.
// A small local model that can only type still returns a working spreadsheet,
// because the part it cannot do is not asked of it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/ooxml"
)

type sheetWriteSkill struct {
	root         string
	outputSubdir func() string
	// files is the shared record a write checks and every toucher updates.
	// Nil is supported and means no guard (filestate.go).
	files *FileState
}

// Excel's own limits. Hitting either produces a file it refuses to open, so
// they are checked here where the error can say which sheet was too big.
const (
	maxSheetRows    = 1048576
	maxSheetColumns = 16384
)

func (*sheetWriteSkill) Name() string { return "sheet_write" }

func (*sheetWriteSkill) Description() string {
	return "สร้างไฟล์ Excel (.xlsx) จากตารางข้อมูล — หลายชีตได้ ตัวเลขบวกกันได้จริง ภาษาไทยไม่เพี้ยน"
}

func (*sheetWriteSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Destination path for the workbook. The .xlsx extension is added if missing.",
			},
			"sheets": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "One entry per tab in the workbook.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Tab name, up to 31 characters",
						},
						"columns": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "Header row, one label per column",
						},
						"formats": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
								"enum": []string{"", "money", "integer", "percent", "date", "datetime"},
							},
							"description": "Display per column, in order; percent holds the fraction (0.07 = 7%).",
						},
						"rows": map[string]any{
							"type":        "array",
							"description": "Data rows. Each row is an array of values in column order.",
							"items": map[string]any{
								"type":  "array",
								"items": map[string]any{},
							},
						},
					},
					"required":             []string{"name", "columns", "rows"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"path", "sheets"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "sheet_write",
			// The typing rule is in the tool description rather than the system
			// prompt for the same reason write's placement rule is: it must
			// travel with every request. It is also the single thing that
			// decides whether the file is useful — a column of amounts sent as
			// "฿1,234.50" arrives as text, SUM returns 0, and the export is
			// worthless for the accounting job it was made for.
			// Capability only, no when-to-pick-me language (owner, 2026-08-04).
			// The typing rules stay: they are how to hold the tool, and getting
			// them wrong silently produces a workbook whose SUM returns 0.
			Description: "Create an Excel file (.xlsx) — opens in Excel, LibreOffice and Google Sheets. " +
				"Send each value with its natural JSON type: a number as a bare number (1234.5, not \"฿1,234.50\") so it can be summed, " +
				"and a date as an ISO string (\"2026-08-03\" or \"2026-08-03 14:30\") so it becomes a real date. " +
				"Anything else stays text, which is what identifiers like \"0012\" need. " +
				"A string starting with = is a live formula (\"=SUM(B2:B13)\"). " +
				"The result reports where the file actually landed.",
			Parameters: payload,
		},
	}
}

// Execute is the command-line shape: sheet_write <path> <json>, where the JSON
// is the same `sheets` array the tool takes. Nobody hand-types this; it exists
// so the skill is reachable from the CLI dispatcher like every other one.
func (s *sheetWriteSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("sheet_write skill unavailable")
		return newToolOutput("sheet_write", "sheet_write", "", start, false, err), err
	}
	args, _ := input["args"].([]string)
	if len(args) < 2 {
		err := errors.New("usage: sheet_write <path> <sheets-json>")
		return newToolOutput("sheet_write", "sheet_write", "", start, false, err), err
	}
	var sheets any
	if err := json.Unmarshal([]byte(strings.Join(args[1:], " ")), &sheets); err != nil {
		err = fmt.Errorf("sheets must be JSON: %w", err)
		return newToolOutput("sheet_write", "sheet_write", "", start, false, err), err
	}
	return s.run(start, args[0], sheets)
}

func (s *sheetWriteSkill) ExecuteTool(_ context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("sheet_write skill unavailable")
		return newToolOutput("sheet_write", "sheet_write", "", start, false, err), err
	}
	path, _ := args["path"].(string)
	return s.run(start, path, args["sheets"])
}

func (s *sheetWriteSkill) run(start time.Time, requestPath string, rawSheets any) (Output, error) {
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		// Same shape as doc_write's, and for the reason written there: a refusal
		// that names only the field it is missing teaches nothing to a caller
		// that sent nothing.
		err := errors.New(`path is required, and so is sheets — a call with neither is an empty call. ` +
			`Smallest real one: {"path":"ยอดขาย.xlsx","sheets":[{"name":"Sheet1","columns":["เดือน","ยอด"],"rows":[["ม.ค.",1000]]}]}. ` +
			`path is just a filename; it lands in this session's output folder on its own`)
		return newToolOutput("sheet_write", "sheet_write", "", start, false, err), err
	}
	// A model that types "report" or "report.xls" still gets a workbook, and
	// the receipt shows the real name so it tells the user the right thing.
	if !strings.EqualFold(filepath.Ext(requestPath), ".xlsx") {
		requestPath = strings.TrimSuffix(requestPath, filepath.Ext(requestPath)) + ".xlsx"
	}

	sheets, err := parseSheets(rawSheets)
	if err != nil {
		return newToolOutput("sheet_write", "sheet_write "+requestPath, "", start, false, err), err
	}

	original := requestPath
	requestPath = placedWrite(s.outputSubdir, requestPath)
	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("sheet_write", "sheet_write "+requestPath, "", start, false, err), err
	}
	if err := ensureWriteDir(targetPath); err != nil {
		return newToolOutput("sheet_write", "sheet_write "+requestPath, "", start, false, err), err
	}

	parts, err := ooxml.BuildXLSX(sheets)
	if err != nil {
		return newToolOutput("sheet_write", "sheet_write "+requestPath, "", start, false, err), err
	}
	// Same act, same refusal as `write` and `doc_write` (filestate.go).
	if err := s.files.guardStale(targetPath, requestPath); err != nil {
		return newToolOutput("sheet_write", "sheet_write "+requestPath, "", start, false, err), err
	}

	if err := ooxml.WriteFile(targetPath, parts); err != nil {
		return newToolOutput("sheet_write", "sheet_write "+requestPath, "", start, false, err), err
	}
	s.files.Note(targetPath)

	output := "sheet_write done: " + requestPath + " (" + describeSheets(sheets) + ")"
	if requestPath != original {
		output += onDiskNote(s.root, targetPath)
	}
	out := newToolOutput("sheet_write", "sheet_write "+requestPath, output, start, false, nil)
	// The workbook is the answer, not a side effect of it — so it travels back
	// as a thing the UI can put a button on rather than only as a filename in a
	// sentence. The placed path, because that is what resolves under the
	// sandbox root from the outside.
	out.Artifacts = []string{requestPath}
	return out, nil
}

func describeSheets(sheets []ooxml.Sheet) string {
	rows := 0
	for _, sheet := range sheets {
		rows += len(sheet.Rows)
	}
	return fmt.Sprintf("%d sheet(s), %d data row(s)", len(sheets), rows)
}

// parseSheets reads the loose JSON the model sent into typed sheets.
//
// It is deliberately forgiving about shape and strict about nothing else: a
// tool call that fails on a missing field costs a whole turn, and the model
// usually cannot tell from the error what it got wrong. Missing rows means an
// empty sheet, a short row is padded on output, and a value of any type becomes
// a cell. What it will not do is guess — see ooxml.InferCell for where the line
// is drawn.
func parseSheets(raw any) ([]ooxml.Sheet, error) {
	list, ok := raw.([]any)
	if !ok {
		if raw == nil {
			return nil, errors.New("sheets is required: an array of {name, columns, rows}")
		}
		return nil, errors.New("sheets must be an array of {name, columns, rows}")
	}
	if len(list) == 0 {
		return nil, errors.New("sheets is empty: a workbook needs at least one sheet")
	}

	sheets := make([]ooxml.Sheet, 0, len(list))
	for i, entry := range list {
		object, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("sheet %d must be an object with name, columns and rows", i+1)
		}
		sheet := ooxml.Sheet{}
		sheet.Name, _ = object["name"].(string)

		for _, column := range asList(object["columns"]) {
			if text, ok := column.(string); ok {
				sheet.Columns = append(sheet.Columns, text)
				continue
			}
			sheet.Columns = append(sheet.Columns, fmt.Sprint(column))
		}
		if len(sheet.Columns) > maxSheetColumns {
			return nil, fmt.Errorf("sheet %q has %d columns, Excel allows %d", sheet.Name, len(sheet.Columns), maxSheetColumns)
		}

		// How the columns are displayed. An unknown name is left plain rather
		// than refused: a column that came out unformatted is a cosmetic
		// complaint, and a refused export is a lost job.
		for _, format := range asList(object["formats"]) {
			text, _ := format.(string)
			sheet.Formats = append(sheet.Formats, text)
		}

		rows := asList(object["rows"])
		// The header occupies one row, so the data budget is one short of the
		// hard limit.
		if len(rows) > maxSheetRows-1 {
			return nil, fmt.Errorf("sheet %q has %d rows, Excel allows %d", sheet.Name, len(rows), maxSheetRows-1)
		}
		sheet.Rows = make([][]ooxml.Cell, 0, len(rows))
		for j, row := range rows {
			values, ok := row.([]any)
			if !ok {
				// A row sent as a bare value rather than an array is a common
				// single-column mistake and there is exactly one sensible
				// reading of it, so it is read rather than rejected.
				values = []any{row}
			}
			if len(values) > maxSheetColumns {
				return nil, fmt.Errorf("sheet %q row %d has %d values, Excel allows %d columns", sheet.Name, j+1, len(values), maxSheetColumns)
			}
			cells := make([]ooxml.Cell, 0, len(values))
			for _, value := range values {
				cells = append(cells, ooxml.InferCell(value))
			}
			sheet.Rows = append(sheet.Rows, cells)
		}
		sheets = append(sheets, sheet)
	}
	return sheets, nil
}

func asList(v any) []any {
	list, _ := v.([]any)
	return list
}
