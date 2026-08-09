package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The failure this whole file exists for, pinned at the tool boundary.
//
// On the reference platform every checked-out file is CRLF (`core.autocrlf` is
// on by default and this repository ships no `.gitattributes`), `read` hands
// back those bytes as they are, and a `\r` is invisible in the rendering. So a
// model joins the lines it saw with `\n` and an exact-only match found nothing
// — on every multi-line edit, every time, while single-line edits kept working
// and made it look like flakiness.
func TestEditMatchesTextTheModelCannotSeeTheLineEndingsOf(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "note.md", "alpha\r\nbeta\r\ngamma\r\n")
	s := &editSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path": "note.md", "old_string": "alpha\nbeta", "new_string": "alpha\nBETA",
	}); err != nil {
		t.Fatalf("a multi-line edit joined with \\n against a CRLF file must apply: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "BETA") {
		t.Fatalf("the edit did not land: %q", got)
	}
	// And the file is still the file it was. Writing the replacement verbatim
	// would leave one LF line in a CRLF file, which is a worse bug than the one
	// being fixed: it lands in everybody's diff and nobody typed it.
	if strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Errorf("the edit left mixed line endings behind: %q", got)
	}
}

// The same thing the other way round: text carrying CRLF into an LF file.
func TestEditKeepsAnLFFileOnLF(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "note.md", "alpha\nbeta\ngamma\n")
	s := &editSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path": "note.md", "old_string": "alpha\r\nbeta", "new_string": "alpha\r\nBETA",
	}); err != nil {
		t.Fatalf("a CRLF-joined old_string against an LF file must apply: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "\r") {
		t.Errorf("an LF file came back with carriage returns in it: %q", string(data))
	}
}

// Line endings are set aside for *matching*; they never widen what counts as a
// unique match. Two places are still two places.
func TestLineEndingToleranceDoesNotWeakenTheUniquenessRule(t *testing.T) {
	root := t.TempDir()
	writeEditFixture(t, root, "note.md", "x\r\ny\r\nx\r\ny\r\n")
	s := &editSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"path": "note.md", "old_string": "x\ny", "new_string": "x\nz",
	})
	if err == nil {
		t.Fatal("an old_string matching twice must still be refused")
	}
	if !strings.Contains(err.Error(), "matches 2 times") {
		t.Errorf("err = %v, want the ordinary ambiguity refusal", err)
	}
}

// When nothing matches, the tool holds the file and answers from it. "Re-read
// the file" was the old answer and it is the one recovery that cannot work for
// the failure that actually arrives — the model reads the same characters and
// composes the same string.
func TestNoMatchIsDiagnosedRatherThanHandedBack(t *testing.T) {
	root := t.TempDir()
	writeEditFixture(t, root, "a.go", "func handler() {\n\treturn 1\n}\n")
	s := &editSkill{root: root}

	for _, c := range []struct {
		name string
		old  string
		want string
	}{
		{
			// read prefixes every line with `%6d\t`; an old_string that carries
			// one silently never matches, and the tool can see that it does.
			name: "read's line-number prefix carried over",
			old:  "     2\t\treturn 1",
			want: "line-number prefix",
		},
		{
			// Spaces where the file has a tab — the classic invisible one.
			name: "indentation differs",
			old:  "    return 1",
			want: "different leading whitespace",
		},
		{
			name: "a later line is what differs",
			old:  "func handler() {\n\treturn 99",
			want: "its first line matches at line 1",
		},
		{
			name: "not in this file at all",
			old:  "func absent() {",
			want: "no line of old_string appears in this file",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := s.ExecuteTool(context.Background(), map[string]any{
				"path": "a.go", "old_string": c.old, "new_string": "x",
			})
			if err == nil {
				t.Fatal("expected the edit to be refused")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to name %q", err, c.want)
			}
			if strings.Contains(err.Error(), "re-read") {
				t.Errorf("err = %v, want a diagnosis rather than an order to read the file again", err)
			}
		})
	}
}

// apply_patch carried the identical bug and a heavier cost: a patch that cannot
// apply in full writes nothing, so one invisible `\r` threw away every edit in
// the batch.
func TestApplyPatchMatchesAcrossLineEndings(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "note.md", "alpha\r\nbeta\r\ngamma\r\ndelta\r\n")
	s := &applyPatchSkill{root: root}

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"edits": []any{
			map[string]any{"path": "note.md", "old_string": "alpha\nbeta", "new_string": "alpha\nBETA"},
			map[string]any{"path": "note.md", "old_string": "gamma\ndelta", "new_string": "gamma\nDELTA"},
		},
	})
	if err != nil {
		t.Fatalf("a two-edit patch joined with \\n against a CRLF file must apply: %v", err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	if !strings.Contains(got, "BETA") || !strings.Contains(got, "DELTA") {
		t.Fatalf("both edits should have landed: %q", got)
	}
	if strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Errorf("the patch left mixed line endings behind: %q", got)
	}
}

func TestDominantEOL(t *testing.T) {
	for _, c := range []struct {
		name    string
		content string
		want    string
	}{
		{"all CRLF", "a\r\nb\r\n", "\r\n"},
		{"all LF", "a\nb\n", "\n"},
		{"empty", "", "\n"},
		{"no newline at all", "a", "\n"},
		// A mostly-CRLF file with a stray LF stays CRLF, so successive edits do
		// not convert it a line at a time.
		{"mostly CRLF", "a\r\nb\r\nc\r\nd\n", "\r\n"},
		{"mostly LF", "a\nb\nc\nd\r\n", "\n"},
	} {
		if got := dominantEOL(c.content); got != c.want {
			t.Errorf("%s: dominantEOL = %q, want %q", c.name, got, c.want)
		}
	}
}

// The same harm one tool over. The prompt sends `write` here for the "replacing
// nearly all of an existing file" case, so on a CRLF checkout taking the
// caller's newlines literally rewrites every line of the file in git's eyes:
// the model meant to replace a function and the diff says it touched the whole
// file.
func TestWriteKeepsAnExistingFilesLineEndings(t *testing.T) {
	root := t.TempDir()
	path := writeEditFixture(t, root, "note.md", "alpha\r\nbeta\r\n")
	s := &writeSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path": "note.md", "content": "alpha\nBETA\ngamma\n",
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if strings.Count(string(got), "\n") != strings.Count(string(got), "\r\n") {
		t.Errorf("overwriting a CRLF file left LF lines in it: %q", string(got))
	}
	if !strings.Contains(string(got), "gamma") {
		t.Errorf("the new content did not land: %q", string(got))
	}
}

// And the limit of that rule: a file that does not exist yet has no convention
// to honour, so the bytes are the caller's to choose. Inventing one here would
// be the tool overreaching — which is why the check is on the outgoing file
// rather than on the platform.
func TestWriteLeavesANewFileExactlyAsTyped(t *testing.T) {
	root := t.TempDir()
	s := &writeSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"path": "fresh.txt", "content": "one\ntwo\n",
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "fresh.txt"))
	if string(got) != "one\ntwo\n" {
		t.Errorf("a new file was rewritten: %q", string(got))
	}
}
