package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEditFixture(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return path
}

func TestEditSkillReplacesUniqueMatch(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "a.txt", "hello old world")
	s := &editSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "a.txt",
		"old_string": "old",
		"new_string": "new",
	})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "hello new world" {
		t.Errorf("content = %q, want %q", string(data), "hello new world")
	}
}

func TestEditSkillEmptyNewStringDeletes(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "a.txt", "keep remove keep")
	s := &editSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "a.txt",
		"old_string": " remove",
		"new_string": "",
	}); err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep keep" {
		t.Errorf("content = %q, want %q", string(data), "keep keep")
	}
}

func TestEditSkillRejectsMissingMatch(t *testing.T) {
	root := t.TempDir()
	writeEditFixture(t, root, "a.txt", "hello")
	s := &editSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "a.txt",
		"old_string": "absent",
		"new_string": "x",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestEditSkillRejectsAmbiguousMatch(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "a.txt", "dup dup")
	s := &editSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "a.txt",
		"old_string": "dup",
		"new_string": "x",
	})
	if err == nil || !strings.Contains(err.Error(), "2 times") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "dup dup" {
		t.Errorf("file modified on rejected edit: %q", string(data))
	}
}

// replace_all is the answer to the error the test above asserts: the ambiguity
// guard stays the default, and this is how the model says it meant all of them.
func TestEditSkillReplaceAll(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "a.txt", "old\nkeep\nold\nold\n")
	s := &editSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":        "a.txt",
		"old_string":  "old",
		"new_string":  "new",
		"replace_all": true,
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "new\nkeep\nnew\nnew\n" {
		t.Errorf("file = %q, want every occurrence replaced and nothing else touched", string(data))
	}
	if !strings.Contains(out.Content, "3 occurrences") {
		t.Errorf("Content = %q, want the count of what changed", out.Content)
	}
	// Three replacements of one line by one line, not one.
	if out.LinesAdded != 3 || out.LinesRemoved != 3 {
		t.Errorf("LinesAdded/Removed = %d/%d, want 3/3", out.LinesAdded, out.LinesRemoved)
	}
}

// replace_all off (or absent) must still refuse an ambiguous match — the guard
// is the default, not a mode the caller opts into.
func TestEditSkillReplaceAllFalseStillRejectsAmbiguity(t *testing.T) {
	root := t.TempDir()
	writeEditFixture(t, root, "a.txt", "dup dup")
	s := &editSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":        "a.txt",
		"old_string":  "dup",
		"new_string":  "x",
		"replace_all": false,
	})
	if err == nil || !strings.Contains(err.Error(), "replace_all") {
		t.Fatalf("expected the ambiguity error to name replace_all as the way out, got %v", err)
	}
}

// The staleness question, answered by the uniqueness rule rather than by a
// read-before-edit ledger: if the file moved under the model, the text it
// remembered either no longer matches (refused) or now matches twice
// (refused). The dangerous case — an edit landing somewhere the model never
// looked — is what these two assert cannot happen.
func TestEditSkillRefusesWhenTheFileMovedUnderIt(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "a.go", "func handler() {\n\treturn 1\n}\n")
	s := &editSkill{root: root}

	// Someone else rewrote the file after the model read it.
	if err := os.WriteFile(path, []byte("func handler() {\n\treturn 2\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"path": "a.go", "old_string": "\treturn 1\n", "new_string": "\treturn 3\n",
	})
	if err == nil {
		t.Fatal("an edit against text that is no longer in the file must fail")
	}
	// It used to say "re-read the file and match the text exactly", which is
	// the most expensive recovery available and, for the failure that actually
	// arrives, the least likely to work — see lineendings.go. The contract now
	// is that the error says what is wrong with this old_string.
	if !strings.Contains(err.Error(), "already been changed") {
		t.Errorf("err = %v, want it to name why nothing matched", err)
	}
	if strings.Contains(err.Error(), "re-read") {
		t.Errorf("err = %v, want a diagnosis rather than an order to read the file again", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "return 2") {
		t.Error("the other change was clobbered")
	}
}

func TestEditSkillRejectsBinaryFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &editSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "bin.dat",
		"old_string": "a",
		"new_string": "x",
	}); err == nil {
		t.Fatal("expected binary-file error, got nil")
	}
}

func TestEditSkillRejectsMissingFile(t *testing.T) {
	s := &editSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "nope.txt",
		"old_string": "a",
		"new_string": "b",
	}); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestEditSkillRejectsEscape(t *testing.T) {
	s := &editSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "../escape.txt",
		"old_string": "a",
		"new_string": "b",
	}); err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestEditSkillRejectsIdenticalStrings(t *testing.T) {
	root := t.TempDir()
	writeEditFixture(t, root, "a.txt", "same")
	s := &editSkill{root: root}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "a.txt",
		"old_string": "same",
		"new_string": "same",
	}); err == nil {
		t.Fatal("expected error for identical strings, got nil")
	}
}

func TestEditSkillExecuteCLIPath(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "a.txt", "hello old world")
	s := &editSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"a.txt", "old", "new"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "hello new world" {
		t.Errorf("content = %q, want %q", string(data), "hello new world")
	}
}

func TestEditSkillExecuteWrongArgCount(t *testing.T) {
	s := &editSkill{root: t.TempDir()}
	for _, args := range [][]string{nil, {"a.txt"}, {"a.txt", "old"}, {"a.txt", "old", "new", "extra"}} {
		if _, err := s.Execute(context.Background(), Input{"args": args}); err == nil {
			t.Errorf("Execute with %d args: expected usage error, got nil", len(args))
		}
	}
}

func TestEditSkillPreservesWhitespaceSignificantStrings(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "a.go", "func a() {\n\treturn 1\n}\n")
	s := &editSkill{root: root}

	// old_string with leading tab and trailing newline must match byte-exact,
	// proving ExecuteTool doesn't trim (the stringSlice hazard).
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "a.go",
		"old_string": "\treturn 1\n",
		"new_string": "\treturn 2\n",
	}); err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "func a() {\n\treturn 2\n}\n" {
		t.Errorf("content = %q, whitespace not preserved", string(data))
	}
}

func TestEditSkillRejectsDirectoryTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "somedir"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &editSkill{root: root}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "somedir",
		"old_string": "a",
		"new_string": "b",
	}); err == nil {
		t.Fatal("expected error for directory target, got nil")
	}
}

func TestEditSkillExecuteToolMissingArgs(t *testing.T) {
	s := &editSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"old_string": "a", "new_string": "b"}); err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"path": "a.txt", "new_string": "b"}); err == nil {
		t.Fatal("expected error for missing old_string, got nil")
	}
}

// Append exists for the file a write left half-finished: the model carries on
// from the byte it stopped at, and nothing already on disk is re-sent.
func TestEditSkillAppendContinuesAFile(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "report.html", "<!DOCTYPE html>\n<html lang=\"th\">\n<body>\n")
	s := &editSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "report.html",
		"new_string": "<h1>สรุป</h1>\n</body>\n</html>\n",
		"mode":       "append",
	})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
	data, _ := os.ReadFile(path)
	want := "<!DOCTYPE html>\n<html lang=\"th\">\n<body>\n<h1>สรุป</h1>\n</body>\n</html>\n"
	if string(data) != want {
		t.Errorf("content = %q, want %q", string(data), want)
	}
}

// No separator of our own. The caller is resuming mid-line as often as not,
// and a newline inserted on its behalf lands inside the content rather than
// between two parts of it.
func TestEditSkillAppendJoinsWithoutInventingASeparator(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "cut.txt", "the sentence was cut in ha")
	s := &editSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "cut.txt",
		"new_string": "lf",
		"mode":       "append",
	}); err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "the sentence was cut in half" {
		t.Errorf("content = %q, want %q", string(data), "the sentence was cut in half")
	}
}

// A file checked out with CRLF keeps CRLF, the same rule replace already
// follows. The reference platform is Windows and a model cannot see a \r.
func TestEditSkillAppendKeepsTheFilesLineEndings(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "crlf.txt", "first\r\nsecond\r\n")
	s := &editSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "crlf.txt",
		"new_string": "third\nfourth\n",
		"mode":       "append",
	}); err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "first\r\nsecond\r\nthird\r\nfourth\r\n" {
		t.Errorf("content = %q, want CRLF throughout", string(data))
	}
}

// Passing both is a model hedging between two different acts. Guessing which
// one it meant is how an append silently becomes a replace.
func TestEditSkillAppendRefusesAnOldString(t *testing.T) {
	root := t.TempDir()
	writeEditFixture(t, root, "a.txt", "hello old world")
	s := &editSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "a.txt",
		"old_string": "old",
		"new_string": "new",
		"mode":       "append",
	})
	if err == nil {
		t.Fatal("expected a refusal when append is given an old_string")
	}
	if !strings.Contains(err.Error(), "no old_string") {
		t.Errorf("the refusal must say which argument to drop, got %q", err.Error())
	}
}

// The old wording sent the model to write, which re-sends the whole file. That
// is the exact cost the truncation path cannot pay, so the way out is named.
func TestEditSkillMissingOldStringPointsAtAppend(t *testing.T) {
	root := t.TempDir()
	writeEditFixture(t, root, "a.txt", "hello")
	s := &editSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"path":       "a.txt",
		"new_string": "more",
	})
	if err == nil {
		t.Fatal("expected a refusal for replace without old_string")
	}
	if !strings.Contains(err.Error(), "append") {
		t.Errorf("refusal must offer append, got %q", err.Error())
	}
}
