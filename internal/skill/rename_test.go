package skill

import (
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/lsp"
)

// applyTextEdits is where a rename either lands exactly or corrupts a file —
// exercised on the two things LSP positions make treacherous: multiple edits
// in one file (offsets must survive each other) and non-ASCII text before an
// edit (positions are UTF-16 units, not bytes).
func TestApplyTextEditsBackToFrontAndUTF16(t *testing.T) {
	src := "func helper() {}\nvar a = helper\nvar b = helper\n"
	edits := []lsp.TextEdit{
		{StartLine: 0, StartChar: 5, EndLine: 0, EndChar: 11, NewText: "assist"},
		{StartLine: 1, StartChar: 8, EndLine: 1, EndChar: 14, NewText: "assist"},
		{StartLine: 2, StartChar: 8, EndLine: 2, EndChar: 14, NewText: "assist"},
	}
	got := applyTextEdits(src, edits)
	want := "func assist() {}\nvar a = assist\nvar b = assist\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	// "แผนที่" is 6 UTF-16 units and 18 bytes: a byte-counting applier would
	// land 12 bytes short and split a Thai character in half.
	thai := "x = 1 # แผนที่ helper\n"
	at := len([]rune("x = 1 # แผนที่ "))
	e := []lsp.TextEdit{{StartLine: 0, StartChar: at, EndLine: 0, EndChar: at + 6, NewText: "assist"}}
	if got := applyTextEdits(thai, e); !strings.Contains(got, "แผนที่ assist") {
		t.Fatalf("UTF-16 positions misapplied: %q", got)
	}
}
