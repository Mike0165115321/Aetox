// Package testpdf builds a small but structurally complete PDF, for tests that
// need a real document rather than a file that merely ends in .pdf.
//
// It lives in its own package because two packages need the same fixture —
// internal/skill's pdf_read tests and the desktop tool-coverage test — and the
// offsets in a cross-reference table are exactly the kind of thing that goes
// quietly wrong when the same twenty lines are maintained in two places.
package testpdf

import (
	"bytes"
	"fmt"
	"strings"
)

// escapeText escapes the three characters that would otherwise end or unbalance
// a PDF literal string.
var escapeText = strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)

// Minimal returns a one-page PDF whose only content is text drawn in Helvetica,
// so an extractor run over it should hand back exactly that text.
//
// The cross-reference table, startxref and %%EOF are all real. That matters
// more than it looks: extractors rebuild a missing xref and carry on, so a
// truncated fixture still passes — but it passes down the damaged-file recovery
// path, printing "Couldn't read xref table" on stderr every time. Once that
// warning is normal, it is indistinguishable from a real failure, and the next
// person to read a broken build is told the document is corrupt when it is not.
func Minimal(text string) []byte {
	stream := "BT /F1 18 Tf 20 100 Td (" + escapeText.Replace(text) + ") Tj ET\n"
	bodies := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(bodies))
	for i, body := range bodies {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}

	// Each entry is exactly 20 bytes, trailing two-byte EOL included: a reader
	// seeks into this table by multiplying, so one short line misaligns
	// everything after it.
	startxref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(bodies)+1)
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(bodies)+1, startxref)
	return buf.Bytes()
}
