package skill

// What `read` does when the file is a Word document, a deck or a workbook.
//
// A new tool would have been the obvious shape and it is the wrong one twice
// over. It would cost every request in the block forever (§132) for a capacity
// most sessions never use, and it would leave `read` still refusing the file —
// so the model's first attempt fails, and the recovery depends on it noticing a
// sibling tool it was not thinking about. `read` is already the answer to "what
// is in this file"; an Office file is a file.
//
// The refusal it replaces was honest and useless: *"read target is a binary
// file — there is no text to read"*, about a document that is nothing but text.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/ooxml"
)

// officeExt is what read opens rather than refuses. The template variants are
// deliberately absent: .dotx and .potx are the same XML in a different wrapper,
// but nothing produces one here yet and a format nobody has tested is a claim
// rather than a capability.
var officeExt = map[string]bool{
	".docx": true,
	".pptx": true,
	".xlsx": true,
}

// A document read whole crosses into the model's context, so it is bounded by
// what it costs there rather than by what a disk can hold. 20 MB is past any
// report and short of the scanned-everything files that are pictures wearing a
// document's extension.
const maxOfficeReadBytes = 20 << 20

// readOffice renders one Office file for the model.
func (s *readSkill) readOffice(targetPath, shown, ext string, size int64) (string, error) {
	if size > maxOfficeReadBytes {
		return "", fmt.Errorf("%s is %d bytes, too large to read into a reply — open it in the program that owns it", shown, size)
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return "", err
	}
	switch ext {
	case ".docx":
		doc, err := ooxml.ReadDOCX(data)
		if err != nil {
			return "", err
		}
		return renderDocument(doc, shown), nil
	case ".pptx":
		slides, err := ooxml.ReadPPTXText(data)
		if err != nil {
			return "", err
		}
		return renderDeck(slides, shown), nil
	default:
		book, err := ooxml.ReadXLSX(data)
		if err != nil {
			return "", err
		}
		return renderWorkbook(book, shown), nil
	}
}

// renderDocument writes a document the way an editor would list it.
//
// Numbered, because the number is the whole point: it is how the next
// instruction names the thing to change. Style before text on every line, so
// the shape of the document is legible by scanning the left edge — which is
// what somebody asking "is this structured properly" is actually looking at.
func renderDocument(doc *ooxml.ReadDocument, shown string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %d block(s)", shown, len(doc.Blocks))
	if doc.Pictures > 0 {
		fmt.Fprintf(&b, ", %d picture(s)", doc.Pictures)
	}
	b.WriteString("\n")
	if len(doc.Blocks) == 0 {
		b.WriteString("(the document has no content)")
		return b.String()
	}

	for i, block := range doc.Blocks {
		fmt.Fprintf(&b, "\n[%d] %s", i+1, blockLabel(block))
		switch block.Kind {
		case ooxml.ReadTable:
			for _, row := range block.Cells {
				b.WriteString("\n  " + strings.Join(row, " | "))
			}
		default:
			if block.Text != "" {
				for _, line := range strings.Split(block.Text, "\n") {
					b.WriteString("\n  " + line)
				}
			}
		}
	}
	if doc.Truncated() {
		b.WriteString("\n\n... (truncated — the document has more blocks than one read returns)")
	}
	return b.String()
}

// blockLabel names one block the way the tool description names a block to
// write, so reading a document and writing one use one vocabulary.
func blockLabel(block ooxml.ReadBlock) string {
	switch block.Kind {
	case ooxml.ReadTable:
		return fmt.Sprintf("table %dx%d", block.Rows, block.Columns)
	case ooxml.ReadImage:
		label := "image"
		if block.Pictures > 1 {
			// Said out loud, because a caption written for "the image" in a
			// paragraph that holds five of them is four captions short.
			label = fmt.Sprintf("%d images in one paragraph", block.Pictures)
		}
		if len(block.Alts) > 0 {
			label += " " + strings.Join(block.Alts, ", ")
		}
		return label
	default:
		label := "paragraph"
		if block.Style != "" {
			label = block.Style
		}
		if block.Listed {
			label += " (list item)"
		}
		return label
	}
}

func renderDeck(slides []string, shown string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %d slide(s)\n", shown, len(slides))
	for i, text := range slides {
		fmt.Fprintf(&b, "\n[%d]\n", i+1)
		if strings.TrimSpace(text) == "" {
			b.WriteString("  (no text on this slide)\n")
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderWorkbook(book *ooxml.WorkbookPreview, shown string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %d sheet(s)\n", shown, len(book.Sheets))
	for _, sheet := range book.Sheets {
		fmt.Fprintf(&b, "\n[%s] %d row(s)\n", sheet.Name, sheet.TotalRows)
		for _, row := range sheet.Rows {
			b.WriteString("  " + strings.Join(row, " | ") + "\n")
		}
		if sheet.Truncated {
			fmt.Fprintf(&b, "  ... (%d rows in total, the rest not shown)\n", sheet.TotalRows)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// officeExtOf is the lowercase extension when read should open the file as an
// Office document, and "" otherwise.
func officeExtOf(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if officeExt[ext] {
		return ext
	}
	return ""
}
