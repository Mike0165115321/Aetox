package skill

// doc_write is phase 3 of OFFICE-EXPORT-PLAN.md, and the cheapest of the three
// now that the container and the pattern are settled: a document is a flat run
// of blocks, with none of xlsx's cell typing and none of pptx's geometry.
//
// What it is *for* is the thing neither of the others covers — a report, a memo,
// a letter — which is where most office work that is not a spreadsheet or a deck
// actually lives.

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

type docWriteSkill struct {
	root         string
	outputSubdir func() string
}

func (*docWriteSkill) Name() string { return "doc_write" }

func (*docWriteSkill) Description() string {
	return "สร้างไฟล์ Word (.docx) จากโครงเอกสาร — หัวข้อ ย่อหน้า บุลเล็ต ลำดับเลข ตาราง ภาษาไทยไม่เพี้ยน"
}

func (*docWriteSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Destination path for the document. The .docx extension is added if missing.",
			},
			"blocks": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "The document, top to bottom. One entry per heading, paragraph, list or table.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"type": map[string]any{
							"type": "string",
							"enum": []string{"heading", "paragraph", "bullets", "numbered", "table", "lineitems"},
						},
						"text": map[string]any{
							"type":        "string",
							"description": "The wording, for a heading or a paragraph.",
						},
						"level": map[string]any{
							"type":        "integer",
							"description": "Heading depth, 1 to 3. Defaults to 1.",
						},
						"items": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "The lines of a bullets or numbered list.",
						},
						"columns": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "A table's header row.",
						},
						"rows": map[string]any{
							"type":        "array",
							"description": "A table's data rows, each an array of cell text in column order.",
							"items": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
						},
						"align": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string", "enum": []string{"", "left", "right", "center"}},
							"description": "Per column.",
						},
						"widths": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "integer"},
							"description": "Relative column weights.",
						},
						"plain": map[string]any{
							"type":        "boolean",
							"description": "No borders — layout, not data.",
						},
						"lines": map[string]any{
							"type":        "array",
							"description": "Priced rows; amounts are computed, never sent.",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"text":  map[string]any{"type": "string"},
									"note":  map[string]any{"type": "string"},
									"qty":   map[string]any{"type": "number", "description": "Defaults to 1."},
									"price": map[string]any{"type": "number"},
								},
								"required":             []string{"text"},
								"additionalProperties": false,
							},
						},
						"totals": map[string]any{
							"type":        "array",
							"description": "Summary rows, in order.",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"label": map[string]any{"type": "string"},
									"kind":  map[string]any{"type": "string", "enum": []string{"subtotal", "rate", "total"}},
									"rate":  map[string]any{"type": "number", "description": "Fraction of subtotal; 0.07 = VAT 7%, negative deducts."},
								},
								"required":             []string{"label", "kind"},
								"additionalProperties": false,
							},
						},
					},
					"required":             []string{"type"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"path", "blocks"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "doc_write",
			// Capability only, no when-to-pick-me language and no routing to
			// sibling tools (owner, 2026-08-04) — the registry already lists
			// every option, and which one answers the request is the model's call.
			//
			// One line, and deliberately (owner, 2026-08-06): "เขียนแค่มันคือ
			// เครื่องมือในการสร้างเอกสารก็พอแล้ว อนาคตจะเจาะลึกอีกเยอะ เอเจนอันนี้".
			// How to build the blocks is the schema's job and the schema already
			// says it; the document agent is where the craft will live as it gets
			// built out, and prose here would only be a second place to keep in
			// sync with it — paid for on every request that carries this tool.
			Description: "Create a Word document (.docx).",
			Parameters:  payload,
		},
	}
}

// Execute is the command-line shape: doc_write <path> <json>, matching the
// other two writers'. Nobody hand-types it; it keeps the skill reachable from
// the CLI dispatcher like every other one.
func (s *docWriteSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("doc_write skill unavailable")
		return newToolOutput("doc_write", "doc_write", "", start, false, err), err
	}
	args, _ := input["args"].([]string)
	if len(args) < 2 {
		err := errors.New("usage: doc_write <path> <blocks-json>")
		return newToolOutput("doc_write", "doc_write", "", start, false, err), err
	}
	var blocks any
	if err := json.Unmarshal([]byte(strings.Join(args[1:], " ")), &blocks); err != nil {
		err = fmt.Errorf("blocks must be JSON: %w", err)
		return newToolOutput("doc_write", "doc_write", "", start, false, err), err
	}
	return s.run(start, args[0], blocks)
}

func (s *docWriteSkill) ExecuteTool(_ context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("doc_write skill unavailable")
		return newToolOutput("doc_write", "doc_write", "", start, false, err), err
	}
	path, _ := args["path"].(string)
	return s.run(start, path, args["blocks"])
}

func (s *docWriteSkill) run(start time.Time, requestPath string, rawBlocks any) (Output, error) {
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		// Carries its remedy, because a refusal that does not is one nothing can
		// learn from — including the learning floor, which quotes these messages
		// verbatim as lessons (desktop/summarize.go). "path is required" produced
		// exactly the card that deserved: a worker told to avoid a condition the
		// sentence never named. What was actually happening was a call with no
		// arguments at all, three times over, so the message now says what a
		// call has to have and what the smallest real one looks like.
		err := errors.New(`path is required, and so is blocks — a call with neither is an empty call. ` +
			`Smallest real one: {"path":"รายงาน.docx","blocks":[{"type":"heading","text":"หัวข้อ"}]}. ` +
			`path is just a filename; it lands in this session's output folder on its own`)
		return newToolOutput("doc_write", "doc_write", "", start, false, err), err
	}
	if !strings.EqualFold(filepath.Ext(requestPath), ".docx") {
		requestPath = strings.TrimSuffix(requestPath, filepath.Ext(requestPath)) + ".docx"
	}

	blocks, err := parseBlocks(rawBlocks)
	if err != nil {
		return newToolOutput("doc_write", "doc_write "+requestPath, "", start, false, err), err
	}

	original := requestPath
	requestPath = placedWrite(s.outputSubdir, requestPath)
	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("doc_write", "doc_write "+requestPath, "", start, false, err), err
	}
	if err := ensureWriteDir(targetPath); err != nil {
		return newToolOutput("doc_write", "doc_write "+requestPath, "", start, false, err), err
	}

	parts, err := ooxml.BuildDOCX(blocks)
	if err != nil {
		return newToolOutput("doc_write", "doc_write "+requestPath, "", start, false, err), err
	}
	if err := ooxml.WriteFile(targetPath, parts); err != nil {
		return newToolOutput("doc_write", "doc_write "+requestPath, "", start, false, err), err
	}

	output := fmt.Sprintf("doc_write done: %s (%d block(s))", requestPath, len(blocks))
	if requestPath != original {
		output += onDiskNote(s.root, targetPath)
	}
	out := newToolOutput("doc_write", "doc_write "+requestPath, output, start, false, nil)
	out.Artifacts = []string{requestPath}
	return out, nil
}

// parseBlocks reads the loose JSON the model sent into typed blocks.
//
// Same posture as the other two writers: strict about what cannot be guessed,
// forgiving about shapes with exactly one sensible reading. A failed tool call
// costs a whole turn and the model usually cannot tell from the error what it
// got wrong.
func parseBlocks(raw any) ([]ooxml.Block, error) {
	list, ok := raw.([]any)
	if !ok {
		if raw == nil {
			return nil, errors.New("blocks is required: an array of {type, ...}")
		}
		return nil, errors.New("blocks must be an array of {type, ...}")
	}
	if len(list) == 0 {
		return nil, errors.New("blocks is empty: a document needs at least one block")
	}

	blocks := make([]ooxml.Block, 0, len(list))
	for i, entry := range list {
		object, ok := entry.(map[string]any)
		if !ok {
			// A bare string is a paragraph. There is no other reading of it.
			if text, isText := entry.(string); isText {
				blocks = append(blocks, ooxml.Block{Kind: ooxml.BlockParagraph, Text: text})
				continue
			}
			return nil, fmt.Errorf("block %d must be an object with a type", i+1)
		}

		kind, _ := object["type"].(string)
		block := ooxml.Block{Kind: ooxml.BlockKind(strings.ToLower(strings.TrimSpace(kind)))}
		block.Text, _ = object["text"].(string)
		if level, ok := object["level"].(float64); ok {
			block.Level = int(level)
		}
		for _, item := range asList(object["items"]) {
			block.Items = append(block.Items, textOf(item))
		}
		for _, column := range asList(object["columns"]) {
			block.Columns = append(block.Columns, textOf(column))
		}
		for _, row := range asList(object["rows"]) {
			values, ok := row.([]any)
			if !ok {
				values = []any{row}
			}
			cells := make([]string, 0, len(values))
			for _, value := range values {
				cells = append(cells, textOf(value))
			}
			block.Rows = append(block.Rows, cells)
		}
		for _, value := range asList(object["align"]) {
			block.Align = append(block.Align, textOf(value))
		}
		for _, value := range asList(object["widths"]) {
			if n, ok := value.(float64); ok {
				block.Widths = append(block.Widths, int(n))
				continue
			}
			block.Widths = append(block.Widths, 0)
		}
		block.Plain, _ = object["plain"].(bool)
		for _, value := range asList(object["lines"]) {
			entry, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("block %d has a line that is not an object with text/qty/price", i+1)
			}
			line := ooxml.LineItem{}
			line.Text, _ = entry["text"].(string)
			line.Note, _ = entry["note"].(string)
			// A missing quantity is one of the thing, which is how a flat fee is
			// written and is worth defaulting.
			line.Qty, _ = entry["qty"].(float64)
			// A missing price is not free. Left to default it prints 0.00 on a
			// priced document — a figure nobody sent, in the one place this tool
			// exists to keep figures honest — so it is refused where it can
			// still be corrected. An explicit 0 is allowed: a line given away is
			// a real thing to put on an invoice.
			raw, has := entry["price"]
			if !has {
				return nil, fmt.Errorf("block %d: line %q has no price — send one (0 if it is free), because a missing price prints as 0.00", i+1, line.Text)
			}
			price, ok := raw.(float64)
			if !ok {
				return nil, fmt.Errorf("block %d: line %q has a price that is not a number (%v) — send 15000, not \"฿15,000\"", i+1, line.Text, raw)
			}
			line.Price = price
			block.Lines = append(block.Lines, line)
		}
		for _, value := range asList(object["totals"]) {
			entry, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("block %d has a total that is not an object with label/kind", i+1)
			}
			row := ooxml.TotalRow{}
			row.Label, _ = entry["label"].(string)
			kind, _ := entry["kind"].(string)
			row.Kind = ooxml.TotalKind(strings.ToLower(strings.TrimSpace(kind)))
			row.Rate, _ = entry["rate"].(float64)
			// A rate row with no rate is a row of zero that reads as a real
			// charge of nothing — refused where it can still be corrected,
			// rather than printed onto an invoice.
			if row.Kind == ooxml.TotalRate && row.Rate == 0 {
				return nil, fmt.Errorf("block %d has a rate row %q with no rate: give it one (0.07 for 7%% VAT) or use kind subtotal/total", i+1, row.Label)
			}
			block.Totals = append(block.Totals, row)
		}

		switch block.Kind {
		case ooxml.BlockHeading, ooxml.BlockParagraph:
			if strings.TrimSpace(block.Text) == "" {
				return nil, fmt.Errorf("block %d is a %s with no text", i+1, block.Kind)
			}
		case ooxml.BlockBullets, ooxml.BlockNumbered:
			if len(block.Items) == 0 {
				return nil, fmt.Errorf("block %d is a %s list with no items", i+1, block.Kind)
			}
		case ooxml.BlockTable:
			if len(block.Columns) == 0 && len(block.Rows) == 0 {
				return nil, fmt.Errorf("block %d is a table with no columns and no rows", i+1)
			}
		case ooxml.BlockLineItems:
			if len(block.Lines) == 0 {
				return nil, fmt.Errorf("block %d is a lineitems block with no lines", i+1)
			}
			// Four labels is the shape the renderer fills: description,
			// quantity, unit price, amount. Two is the flat-fee shape:
			// description and amount. Anything else would leave a column the
			// renderer has no figure for, printed empty on a priced document.
			if n := len(block.Columns); n != 4 && n != 2 {
				return nil, fmt.Errorf("block %d is a lineitems block with %d column labels: give 4 (description, quantity, unit price, amount) or 2 (description, amount)", i+1, n)
			}
		case "":
			return nil, fmt.Errorf("block %d has no type: one of heading, paragraph, bullets, numbered, table, lineitems", i+1)
		default:
			return nil, fmt.Errorf("block %d has unknown type %q: one of heading, paragraph, bullets, numbered, table, lineitems", i+1, kind)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// textOf renders a JSON value as document text. Unlike a spreadsheet cell there
// is nothing to preserve but the characters — a document has no type a number
// could be stored as.
func textOf(v any) string {
	if text, ok := v.(string); ok {
		return text
	}
	if v == nil {
		return ""
	}
	if f, ok := v.(float64); ok {
		// %v on a float64 gives "1.2345e+06" past a certain size, which is not
		// what anyone typed.
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.10f", f), "0"), ".")
	}
	return fmt.Sprint(v)
}
