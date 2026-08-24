package model

// Naming a call that touched more than one file.
//
// apply_patch reached the timeline as the bare word "apply_patch": its paths
// live inside edits[] and neither reader looked in there — the completed parse
// found nothing, and the streaming scan found the first path only by accident
// of scanning the whole string. Two spellings for one call, which is also how
// one call draws two rows.

import (
	"encoding/json"
	"testing"
)

func jsonUnmarshal(raw string, out any) error { return json.Unmarshal([]byte(raw), out) }

func TestSubjectNamesEveryFileInABatch(t *testing.T) {
	args := map[string]any{"edits": []any{
		map[string]any{"path": "one.go", "old_string": "a", "new_string": "b"},
		map[string]any{"path": "two.go", "old_string": "c", "new_string": "d"},
		map[string]any{"path": "three.go", "old_string": "e", "new_string": "f"},
	}}
	if got, want := SubjectFromArgs(args), "one.go +2"; got != want {
		t.Errorf("SubjectFromArgs = %q, want %q", got, want)
	}
}

// Two edits to the same file are one file. A count that says +1 for a call that
// touched one file is worse than no count.
func TestSubjectCountsFilesNotEdits(t *testing.T) {
	args := map[string]any{"edits": []any{
		map[string]any{"path": "one.go", "old_string": "a", "new_string": "b"},
		map[string]any{"path": "one.go", "old_string": "c", "new_string": "d"},
	}}
	if got, want := SubjectFromArgs(args), "one.go"; got != want {
		t.Errorf("SubjectFromArgs = %q, want %q", got, want)
	}
}

// The label a row is born with, and the label it ends up with, have to be the
// same string — a row is matched to its streamed guess by label when the engine
// sends no call id.
func TestStreamedAndFinishedSubjectsAgree(t *testing.T) {
	raw := `{"edits":[{"path":"one.go","old_string":"a","new_string":"b"},` +
		`{"path":"two.go","old_string":"c","new_string":"d"}]}`
	streamed, ok := SubjectFromPartialArgs(raw)
	if !ok {
		t.Fatal("nothing readable in a complete apply_patch call")
	}
	var parsed map[string]any
	if err := jsonUnmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if finished := SubjectFromArgs(parsed); streamed != finished {
		t.Errorf("streamed %q, finished %q — one call, two rows", streamed, finished)
	}
}

// The ordinary single-path call is untouched: it was never the broken one, and
// every row in every stored transcript is spelled this way.
func TestSubjectStillNamesAPlainPath(t *testing.T) {
	if got := SubjectFromArgs(map[string]any{"path": "desktop/app.go"}); got != "desktop/app.go" {
		t.Errorf("got %q", got)
	}
	if got, _ := SubjectFromPartialArgs(`{"path":"desktop/app.go","content":"x"}`); got != "desktop/app.go" {
		t.Errorf("streamed got %q", got)
	}
}
