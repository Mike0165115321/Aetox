package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func changeTool(t *testing.T, root string) *changeSkill {
	t.Helper()
	return &changeSkill{root: root}
}

func seedFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A change that reached across files comes back saying nobody else has read it.
// The context that wrote a change is the one least able to see what is wrong
// with it, and self-checking — which the assistant does well and always — is a
// different thing from a second reader.
func TestAMultiFileChangeSaysNobodyHasReadIt(t *testing.T) {
	root := t.TempDir()
	seedFiles(t, root, map[string]string{"a.go": "alpha\n", "b.go": "alpha\n"})
	s := changeTool(t, root)

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"action": "batch",
		"edits": []any{
			map[string]any{"path": "a.go", "find": "alpha", "replace": "beta"},
			map[string]any{"path": "b.go", "find": "alpha", "replace": "beta"},
		},
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if !strings.Contains(out.Content, "agent=reviewer") {
		t.Errorf("a two-file change came back with no note:\n%s", out.Content)
	}
	if !strings.Contains(out.Content, "2 files") {
		t.Errorf("the note did not say how far the change reached:\n%s", out.Content)
	}
	// The note is a fact and a door, never an instruction: a tool result that
	// tells the model what to do next is arguing with the system prompt.
	for _, order := range []string{"you should", "you must", "always "} {
		if strings.Contains(strings.ToLower(out.Content), order) {
			t.Errorf("the note gives an order (%q):\n%s", order, out.Content)
		}
	}
}

// One file is one file, however many places inside it moved. A note on every
// edit is a note nobody reads by the third one.
func TestAOneFileChangeIsLeftAlone(t *testing.T) {
	root := t.TempDir()
	seedFiles(t, root, map[string]string{"a.go": "alpha\nalpha two\n"})
	s := changeTool(t, root)

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"action": "batch", "path": "a.go",
		"edits": []any{
			map[string]any{"find": "alpha two", "replace": "beta two"},
			map[string]any{"find": "alpha", "replace": "beta"},
		},
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if strings.Contains(out.Content, "reviewer") {
		t.Errorf("a single-file change was told to find a reviewer:\n%s", out.Content)
	}
}

// The other four acts are untouched. A whole-file write, one edit, an append and
// a delete each say what they always said.
func TestTheOtherActsCarryNoNote(t *testing.T) {
	root := t.TempDir()
	seedFiles(t, root, map[string]string{"a.go": "alpha\n"})
	s := changeTool(t, root)

	for _, call := range []map[string]any{
		{"action": "write", "path": "new.go", "content": "package main\n"},
		{"action": "edit", "path": "a.go", "find": "alpha", "replace": "beta"},
		{"action": "append", "path": "a.go", "replace": "gamma\n"},
	} {
		out, err := s.ExecuteTool(context.Background(), call)
		if err != nil {
			t.Fatalf("%v: %v", call["action"], err)
		}
		if strings.Contains(out.Content, "reviewer") {
			t.Errorf("%v carried the note:\n%s", call["action"], out.Content)
		}
	}
}

// A batch that failed says why it failed and nothing else — a note about who
// should read a change that was never written is noise on top of an error.
func TestAFailedBatchCarriesNoNote(t *testing.T) {
	root := t.TempDir()
	seedFiles(t, root, map[string]string{"a.go": "alpha\n", "b.go": "alpha\n"})
	s := changeTool(t, root)

	out, _ := s.ExecuteTool(context.Background(), map[string]any{
		"action": "batch",
		"edits": []any{
			map[string]any{"path": "a.go", "find": "alpha", "replace": "beta"},
			map[string]any{"path": "b.go", "find": "nothing like this", "replace": "x"},
		},
	})
	if strings.Contains(out.Content, "reviewer") {
		t.Errorf("a batch that applied nothing still asked for a reviewer:\n%s", out.Content)
	}
}
