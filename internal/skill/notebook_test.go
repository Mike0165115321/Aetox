package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A real nbformat 4 notebook: a code cell with an output, a markdown cell, and
// top-level metadata. Every claim below is about not breaking one of these.
const sampleNotebook = `{
 "cells": [
  {
   "cell_type": "code",
   "execution_count": 3,
   "metadata": {},
   "outputs": [
    {"output_type": "stream", "name": "stdout", "text": ["4\n"]}
   ],
   "source": ["x = 2\n", "print(x + x)\n"]
  },
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": ["# A heading\n"]
  }
 ],
 "metadata": {"kernelspec": {"display_name": "Python 3", "name": "python3"}},
 "nbformat": 4,
 "nbformat_minor": 5
}
`

func notebookFixture(t *testing.T) (*notebookEditSkill, *readSkill, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "nb.ipynb"), []byte(sampleNotebook), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return &notebookEditSkill{root: root}, &readSkill{root: root}, root
}

// The reading half, and the reason it matters: raw JSON costs a fortune in
// context to show two lines of Python.
func TestReadRendersANotebookAsCells(t *testing.T) {
	_, reader, _ := notebookFixture(t)

	out, err := reader.ExecuteTool(context.Background(), map[string]any{"path": "nb.ipynb"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"2 cells", "[0] code", "print(x + x)", "[1] markdown", "# A heading"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("rendered notebook missing %q:\n%s", want, out.Content)
		}
	}
	// The escaped-JSON shape must not leak through — that is the thing being
	// fixed.
	if strings.Contains(out.Content, `\n`) || strings.Contains(out.Content, `"source"`) {
		t.Errorf("raw JSON leaked into the rendering:\n%s", out.Content)
	}
	// An output worth reading is summarised, not pasted.
	if !strings.Contains(out.Content, "→ output: 4") {
		t.Errorf("cell output was dropped entirely:\n%s", out.Content)
	}
}

func TestNotebookEditReplacesACell(t *testing.T) {
	editor, reader, root := notebookFixture(t)

	out, err := editor.ExecuteTool(context.Background(), map[string]any{
		"path":   "nb.ipynb",
		"cell":   0,
		"source": "x = 5\nprint(x * x)\n",
	})
	if err != nil {
		t.Fatalf("notebook_edit: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false: %s", out.Stderr)
	}

	rendered, _ := reader.ExecuteTool(context.Background(), map[string]any{"path": "nb.ipynb"})
	if !strings.Contains(rendered.Content, "print(x * x)") {
		t.Errorf("the new source is not there:\n%s", rendered.Content)
	}
	// The stale output described code that no longer exists — a model reads
	// that as the current result.
	if strings.Contains(rendered.Content, "→ output: 4") {
		t.Errorf("the previous run's output survived the edit:\n%s", rendered.Content)
	}

	// Still a notebook Jupyter will open: valid JSON, metadata and version kept.
	data, _ := os.ReadFile(filepath.Join(root, "nb.ipynb"))
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("the notebook is no longer valid JSON: %v", err)
	}
	for _, key := range []string{"metadata", "nbformat", "nbformat_minor"} {
		if _, ok := top[key]; !ok {
			t.Errorf("%q was dropped — Jupyter will refuse the file", key)
		}
	}
	if !strings.Contains(string(data), "python3") {
		t.Error("the kernelspec was lost")
	}
	// Source is written back as a list of lines, the way Jupyter writes it, so
	// the diff stays small against a notebook saved by Jupyter itself.
	if !strings.Contains(string(data), `"x = 5\n"`) {
		t.Errorf("source was not written as a line list:\n%s", string(data))
	}
}

func TestNotebookEditInsertsAndDeletes(t *testing.T) {
	editor, reader, _ := notebookFixture(t)

	if _, err := editor.ExecuteTool(context.Background(), map[string]any{
		"path": "nb.ipynb", "mode": "insert", "cell": 1,
		"source": "# inserted\n", "cell_type": "markdown",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	rendered, _ := reader.ExecuteTool(context.Background(), map[string]any{"path": "nb.ipynb"})
	if !strings.Contains(rendered.Content, "3 cells") || !strings.Contains(rendered.Content, "# inserted") {
		t.Fatalf("insert did not land in the middle:\n%s", rendered.Content)
	}
	if !strings.Contains(rendered.Content, "[1] markdown") {
		t.Errorf("the inserted cell is not at index 1:\n%s", rendered.Content)
	}

	if _, err := editor.ExecuteTool(context.Background(), map[string]any{
		"path": "nb.ipynb", "mode": "delete", "cell": 1,
	}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rendered, _ = reader.ExecuteTool(context.Background(), map[string]any{"path": "nb.ipynb"})
	if strings.Contains(rendered.Content, "# inserted") || !strings.Contains(rendered.Content, "2 cells") {
		t.Errorf("delete did not remove the right cell:\n%s", rendered.Content)
	}
}

// An index the model guessed wrong must be refused by name, not applied to
// whatever cell happens to be there.
func TestNotebookEditRejectsAnIndexOutsideTheNotebook(t *testing.T) {
	editor, _, _ := notebookFixture(t)

	_, err := editor.ExecuteTool(context.Background(), map[string]any{
		"path": "nb.ipynb", "cell": 9, "source": "nope",
	})
	if err == nil || !strings.Contains(err.Error(), "outside this notebook") {
		t.Errorf("err = %v, want it to say the index is out of range", err)
	}
}

func TestNotebookEditRefusesNonNotebooks(t *testing.T) {
	editor, _, root := notebookFixture(t)
	if err := os.WriteFile(filepath.Join(root, "plain.py"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := editor.ExecuteTool(context.Background(), map[string]any{
		"path": "plain.py", "cell": 0, "source": "x = 2",
	})
	if err == nil || !strings.Contains(err.Error(), "edit") {
		t.Errorf("err = %v, want it to point at the right tool", err)
	}
}
