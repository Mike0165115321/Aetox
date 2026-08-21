package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// Jupyter notebooks, which the file tools could not touch at all.
//
// A .ipynb is JSON with the code buried inside it: `"source": ["def f():\n",
// "    return 1\n"]`, one array element per line, plus every output the cell
// ever produced — stdout, tracebacks, and base64 images that run to tens of
// thousands of characters. That shape breaks both halves of the normal path:
//
//   - `read` handed the model raw JSON, so looking at five lines of code cost
//     an enormous amount of context, most of it a picture the model could not
//     see anyway.
//   - `edit` matches an exact string, and the string in the file is
//     JSON-escaped. One wrong character and the notebook will not open.
//
// So the notebook gets its own pair: read renders it as cells, and notebook_edit
// changes one cell through the JSON rather than through the text. Everything
// else about the file — its metadata, its outputs, its format version — is left
// exactly as found, because a notebook rewritten by a tool that "tidied" it is
// a diff nobody can review.

const notebookExt = ".ipynb"

// notebook is the subset that has to survive a round trip. Everything not named
// here rides along in Extra and is written back untouched.
type notebook struct {
	Cells []notebookCell  `json:"cells"`
	Extra map[string]json.RawMessage `json:"-"`
}

type notebookCell struct {
	CellType string          `json:"cell_type"`
	Source   json.RawMessage `json:"source"`
	Extra    map[string]json.RawMessage
}

// loadNotebook parses just enough to edit a cell, keeping every other key.
func loadNotebook(path string) (*notebook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("not a readable notebook: %w", err)
	}
	rawCells, ok := top["cells"]
	if !ok {
		return nil, errors.New("not a notebook: no cells")
	}
	var cellObjects []map[string]json.RawMessage
	if err := json.Unmarshal(rawCells, &cellObjects); err != nil {
		return nil, fmt.Errorf("notebook cells are not a list: %w", err)
	}
	nb := &notebook{Extra: map[string]json.RawMessage{}}
	for key, value := range top {
		if key != "cells" {
			nb.Extra[key] = value
		}
	}
	for _, obj := range cellObjects {
		cell := notebookCell{Extra: map[string]json.RawMessage{}}
		for key, value := range obj {
			switch key {
			case "cell_type":
				_ = json.Unmarshal(value, &cell.CellType)
			case "source":
				cell.Source = value
			default:
				cell.Extra[key] = value
			}
		}
		nb.Cells = append(nb.Cells, cell)
	}
	return nb, nil
}

// sourceText joins a cell's source, which nbformat allows to be either a list
// of lines or one string. Both are legal and both are in the wild.
func (c notebookCell) sourceText() string {
	var lines []string
	if err := json.Unmarshal(c.Source, &lines); err == nil {
		return strings.Join(lines, "")
	}
	var single string
	if err := json.Unmarshal(c.Source, &single); err == nil {
		return single
	}
	return ""
}

// setSource writes the list-of-lines form, keeping the trailing newline on
// every line but the last — which is what Jupyter itself writes, and what keeps
// the diff against a notebook saved by Jupyter to just the cell that changed.
func (c *notebookCell) setSource(text string) error {
	lines := strings.SplitAfter(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	if len(lines) == 0 {
		lines = []string{}
	}
	encoded, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	c.Source = encoded
	return nil
}

func (n *notebook) save(path string) error {
	cells := make([]map[string]json.RawMessage, 0, len(n.Cells))
	for _, cell := range n.Cells {
		obj := map[string]json.RawMessage{}
		for key, value := range cell.Extra {
			obj[key] = value
		}
		cellType, err := json.Marshal(cell.CellType)
		if err != nil {
			return err
		}
		obj["cell_type"] = cellType
		obj["source"] = cell.Source
		cells = append(cells, obj)
	}
	top := map[string]json.RawMessage{}
	for key, value := range n.Extra {
		top[key] = value
	}
	encodedCells, err := json.Marshal(cells)
	if err != nil {
		return err
	}
	top["cells"] = encodedCells
	// Indented, and with a trailing newline: that is how Jupyter writes the
	// file, and a tool that reformats it turns a one-cell change into a diff
	// covering the whole notebook.
	out, err := json.MarshalIndent(top, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// renderNotebook is what read returns for a .ipynb: the cells, numbered, with
// their type, and outputs summarised rather than pasted. A traceback is the
// answer often enough to be worth keeping; a base64 PNG never is.
func renderNotebook(nb *notebook, shown string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %d cells\n", shown, len(nb.Cells))
	for i, cell := range nb.Cells {
		fmt.Fprintf(&b, "\n[%d] %s\n", i, cell.CellType)
		text := strings.TrimRight(cell.sourceText(), "\n")
		if text == "" {
			b.WriteString("(empty)\n")
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			b.WriteString("  " + line + "\n")
		}
		if summary := summariseOutputs(cell); summary != "" {
			b.WriteString(summary)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// summariseOutputs keeps what a person would read and drops what they would
// scroll past.
func summariseOutputs(cell notebookCell) string {
	raw, ok := cell.Extra["outputs"]
	if !ok {
		return ""
	}
	var outputs []map[string]json.RawMessage
	if json.Unmarshal(raw, &outputs) != nil || len(outputs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, out := range outputs {
		var kind string
		_ = json.Unmarshal(out["output_type"], &kind)
		switch kind {
		case "error":
			var name, message string
			_ = json.Unmarshal(out["ename"], &name)
			_ = json.Unmarshal(out["evalue"], &message)
			fmt.Fprintf(&b, "  → error: %s: %s\n", name, message)
		case "stream":
			var lines []string
			if json.Unmarshal(out["text"], &lines) == nil {
				text := strings.TrimRight(strings.Join(lines, ""), "\n")
				fmt.Fprintf(&b, "  → output: %s\n", firstNotebookLine(text))
			}
		default:
			fmt.Fprintf(&b, "  → %s (not shown)\n", emptyOr(kind, "output"))
		}
	}
	return b.String()
}

func firstNotebookLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " …"
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func emptyOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// --- notebook_edit -----------------------------------------------------------

type notebookEditSkill struct {
	root         string
	outputSubdir func() string
}

func (*notebookEditSkill) Name() string { return "notebook_edit" }

func (*notebookEditSkill) Description() string {
	return "แก้ไข เพิ่ม หรือลบเซลล์ใน Jupyter notebook"
}

func (*notebookEditSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path of the .ipynb file",
			},
			"cell": map[string]any{
				"type":        "integer",
				"description": "Which cell, counting from 0 — the number read shows in brackets. For insert, the new cell lands at this position; omit it to append at the end.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"replace", "insert", "delete"},
				"description": "replace (default) rewrites the cell, insert adds a new one, delete removes it.",
			},
			"source": map[string]any{
				"type":        "string",
				"description": "The cell's new content as plain text, newlines and all. Not JSON-escaped — this tool does that.",
			},
			"cell_type": map[string]any{
				"type":        "string",
				"enum":        []string{"code", "markdown"},
				"description": "For insert; defaults to code. Ignored by delete, and optional for replace (the cell keeps its type unless you say otherwise).",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "notebook_edit",
			Description: "Change one cell of a Jupyter notebook. Use this rather than edit or write for .ipynb: the file is JSON with the code escaped inside it, so an exact-string edit either fails to match or corrupts the notebook. " +
				"read shows a notebook as numbered cells — those numbers are what this takes. Outputs, metadata and format version are left untouched.",
			Parameters: payload,
		},
	}
}

func (s *notebookEditSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args, _ := input["args"].([]string)
	if len(args) < 2 {
		err := errors.New("usage: notebook_edit <path> <cell> [source]")
		return newToolOutput("notebook_edit", "notebook_edit", "", time.Now(), false, err), err
	}
	cell, convErr := strconv.Atoi(strings.TrimSpace(args[1]))
	if convErr != nil {
		err := errors.New("cell must be a number")
		return newToolOutput("notebook_edit", "notebook_edit", "", time.Now(), false, err), err
	}
	call := map[string]any{"path": args[0], "cell": cell}
	if len(args) > 2 {
		call["source"] = strings.Join(args[2:], " ")
	}
	return s.ExecuteTool(ctx, call)
}

func (s *notebookEditSkill) ExecuteTool(_ context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		err := errors.New("path is required")
		return newToolOutput("notebook_edit", "notebook_edit", "", start, false, err), err
	}
	path = PlacedPath(s.root, s.outputSubdir, path)
	command := "notebook_edit " + path
	if !strings.EqualFold(filepath.Ext(path), notebookExt) {
		err := errors.New("notebook_edit only works on .ipynb files — use edit for anything else")
		return newToolOutput("notebook_edit", command, "", start, false, err), err
	}
	target, err := resolveSandboxPath(s.root, path)
	if err != nil {
		return newToolOutput("notebook_edit", command, "", start, false, err), err
	}
	nb, err := loadNotebook(target)
	if err != nil {
		return newToolOutput("notebook_edit", command, "", start, false, err), err
	}

	mode := strings.ToLower(strings.TrimSpace(stringArg(args["mode"])))
	if mode == "" {
		mode = "replace"
	}
	index := intArg(args["cell"])
	source, _ := args["source"].(string)
	cellType := strings.ToLower(strings.TrimSpace(stringArg(args["cell_type"])))

	switch mode {
	case "insert":
		if _, given := args["cell"]; !given {
			index = len(nb.Cells)
		}
		if index < 0 || index > len(nb.Cells) {
			err := fmt.Errorf("cell %d is outside this notebook (%d cells)", index, len(nb.Cells))
			return newToolOutput("notebook_edit", command, "", start, false, err), err
		}
		if cellType == "" {
			cellType = "code"
		}
		fresh := notebookCell{CellType: cellType, Extra: map[string]json.RawMessage{}}
		if cellType == "code" {
			// nbformat requires both on a code cell; a notebook missing them
			// is rejected by Jupyter on open.
			fresh.Extra["outputs"] = json.RawMessage(`[]`)
			fresh.Extra["execution_count"] = json.RawMessage(`null`)
		}
		fresh.Extra["metadata"] = json.RawMessage(`{}`)
		if err := fresh.setSource(source); err != nil {
			return newToolOutput("notebook_edit", command, "", start, false, err), err
		}
		nb.Cells = append(nb.Cells, notebookCell{})
		copy(nb.Cells[index+1:], nb.Cells[index:])
		nb.Cells[index] = fresh

	case "delete":
		if index < 0 || index >= len(nb.Cells) {
			err := fmt.Errorf("cell %d is outside this notebook (%d cells)", index, len(nb.Cells))
			return newToolOutput("notebook_edit", command, "", start, false, err), err
		}
		nb.Cells = append(nb.Cells[:index], nb.Cells[index+1:]...)

	case "replace":
		if index < 0 || index >= len(nb.Cells) {
			err := fmt.Errorf("cell %d is outside this notebook (%d cells)", index, len(nb.Cells))
			return newToolOutput("notebook_edit", command, "", start, false, err), err
		}
		if _, given := args["source"]; !given {
			err := errors.New("source is required to replace a cell")
			return newToolOutput("notebook_edit", command, "", start, false, err), err
		}
		before := nb.Cells[index].sourceText()
		if err := nb.Cells[index].setSource(source); err != nil {
			return newToolOutput("notebook_edit", command, "", start, false, err), err
		}
		if cellType != "" {
			nb.Cells[index].CellType = cellType
		}
		// The cell's recorded output describes code that is no longer there.
		// Leaving it is how a model reads a stale traceback as current.
		if _, isCode := nb.Cells[index].Extra["outputs"]; isCode {
			nb.Cells[index].Extra["outputs"] = json.RawMessage(`[]`)
			nb.Cells[index].Extra["execution_count"] = json.RawMessage(`null`)
		}
		out := newToolOutput("notebook_edit", command,
			fmt.Sprintf("notebook_edit done: %s cell %d", path, index), start, false, nil)
		out.LinesAdded, out.LinesRemoved = LineDelta(before, source)
		// A notebook's unit is the cell, so the hunks are numbered within it —
		// which is the only numbering that means anything here, and what the
		// row's own "cell N" is already telling the reader.
		out.Diff = UnifiedDiff(before, source)
		if err := nb.save(target); err != nil {
			return newToolOutput("notebook_edit", command, "", start, false, err), err
		}
		return out, nil

	default:
		err := fmt.Errorf("unknown mode %q — use replace, insert or delete", mode)
		return newToolOutput("notebook_edit", command, "", start, false, err), err
	}

	if err := nb.save(target); err != nil {
		return newToolOutput("notebook_edit", command, "", start, false, err), err
	}
	return newToolOutput("notebook_edit", command,
		fmt.Sprintf("notebook_edit done: %s %s at cell %d (%d cells now)", path, mode, index, len(nb.Cells)),
		start, false, nil), nil
}
