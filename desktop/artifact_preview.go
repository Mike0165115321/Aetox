package main

// What a produced file looks like inside, for the ผลงาน gallery.
//
// A grid of filenames answers "what is it called" and nothing else — and a
// name is the one thing a person already forgot by the time they come looking.
// นิสัยการทำงานที่ดีของทีมเล็ก.docx and สรุปผลการทดสอบระบบ (Demo Round 2).docx are
// the same card twice until you can see a line of either one.
//
// Read on demand, one file at a time, never during the sweep: ListArtifacts is
// the page's first paint and it is capped at 500 rows. Cracking open 500 zips
// to draw a grid nobody has scrolled to yet is the kind of eager work that
// makes a gallery feel broken. The frontend asks for the cards it is actually
// showing.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mikedev115/Aetox/internal/ooxml"
)

// How much of a file is worth looking at for a thumbnail. These are display
// bounds, not safety bounds — a card shows a few lines, so reading a 40MB log
// to print six of its lines is pure waste.
const (
	previewTextBytes = 24 << 10 // enough for far more lines than a card shows
	previewTextRunes = 1400
	// The ceiling on a page the card *renders* rather than quotes, and a
	// separate number from previewTextBytes on purpose.
	//
	// It was the same number, and that is a constant doing two jobs whose
	// natural sizes are nothing like each other. previewTextBytes is "how much
	// do we need to read to fill an excerpt" — a few screens, generously. This
	// is "how big a document can we hand to an iframe", which is a different
	// question with a much larger honest answer.
	//
	// Reported 2026-08-14: two landing pages side by side in the gallery, one
	// drawn as the page it is and the other as a wall of <!DOCTYPE html>. The
	// difference between them was 22 KB against 49 KB, and nothing on screen
	// could have told you that was the reason.
	//
	// 512 KB renders anything a session actually produces and still bounds what
	// one card can hold in memory. Past it the excerpt is the honest fallback,
	// exactly as before.
	previewHTMLBytes  = 512 << 10
	previewImageBytes = 4 << 20 // past this a thumbnail costs more than it gives
	previewZipBytes   = 8 << 20
	previewSheetRows  = 6
	previewSheetCols  = 6
)

// ArtifactPreview is a look inside one produced file.
//
// Kind is what the card should draw, decided here rather than in the window:
// the frontend cannot tell a readable .docx from a corrupt one without opening
// it, and opening it is this side's job. "none" is an honest answer — a PDF or
// a zip has no cheap preview, and a card that says nothing beats a card that
// prints mojibake.
type ArtifactPreview struct {
	Kind    string     `json:"kind"` // text | markdown | html | image | sheet | none
	Text    string     `json:"text,omitempty"`
	DataURL string     `json:"dataUrl,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
	Sheet   string     `json:"sheet,omitempty"` // which sheet Rows came from
}

// ArtifactPreview reads one file the gallery is showing.
//
// The path is checked against the same roots ListArtifacts swept, not trusted
// from the window: this method takes an absolute path, and an absolute path
// arriving from the frontend is a request to read any file on the machine
// unless something says otherwise. The gallery's own roots are that something.
func (a *App) ArtifactPreview(path string) (ArtifactPreview, error) {
	full, err := a.artifactPath(path)
	if err != nil {
		return ArtifactPreview{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return ArtifactPreview{}, err
	}
	if info.IsDir() {
		return ArtifactPreview{Kind: "none"}, nil
	}
	ext := strings.ToLower(filepath.Ext(full))

	switch {
	case previewImageExt[ext]:
		if info.Size() > previewImageBytes {
			return ArtifactPreview{Kind: "none"}, nil
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return ArtifactPreview{}, err
		}
		kind := mime.TypeByExtension(ext)
		if kind == "" {
			kind = "image/png"
		}
		return ArtifactPreview{
			Kind:    "image",
			DataURL: "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data),
		}, nil

	case ext == ".xlsx":
		if info.Size() > previewZipBytes {
			return ArtifactPreview{Kind: "none"}, nil
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return ArtifactPreview{}, err
		}
		book, err := ooxml.ReadXLSX(data)
		if err != nil || book == nil || len(book.Sheets) == 0 {
			return ArtifactPreview{Kind: "none"}, nil
		}
		return sheetPreview(book), nil

	case ext == ".docx" || ext == ".pptx":
		if info.Size() > previewZipBytes {
			return ArtifactPreview{Kind: "none"}, nil
		}
		text, err := ooxmlText(full, ext)
		if err != nil || strings.TrimSpace(text) == "" {
			return ArtifactPreview{Kind: "none"}, nil
		}
		return ArtifactPreview{Kind: "text", Text: clipRunes(text, previewTextRunes)}, nil
	}

	// Everything else is judged by its bytes, not its name. An extension we do
	// not know is usually still text (.ps1, .toml, .rs, a file with no
	// extension at all), and refusing on the strength of a lookup table would
	// blank exactly the cards a coding session produces.
	head, err := readHead(full, previewTextBytes)
	if err != nil {
		return ArtifactPreview{}, err
	}
	if !looksLikeText(head) {
		return ArtifactPreview{Kind: "none"}, nil
	}
	text := clipRunes(strings.ToValidUTF8(string(head), ""), previewTextRunes)
	switch ext {
	case ".md", ".markdown":
		return ArtifactPreview{Kind: "markdown", Text: text}, nil
	case ".html", ".htm", ".svg":
		// Whole, not clipped: this one is rendered rather than read, and half a
		// document renders as a broken document. Which is why the file is read
		// again in full here rather than reusing `head` — head stops at
		// previewTextBytes, and a page that fits under previewHTMLBytes but not
		// under that would otherwise render as its own first 24 KB.
		if info.Size() <= previewHTMLBytes {
			whole, err := os.ReadFile(full)
			if err == nil {
				return ArtifactPreview{Kind: "html", Text: string(whole)}, nil
			}
			// Unreadable on the second pass but readable on the first: the file
			// is going away underneath us. The excerpt already in hand beats an
			// error on a card.
		}
		return ArtifactPreview{Kind: "text", Text: text}, nil
	}
	return ArtifactPreview{Kind: "text", Text: text}, nil
}

var previewImageExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true,
}

// artifactPath resolves a gallery path and refuses anything outside the output
// folders the gallery itself sweeps. Symlinks are resolved before the check —
// a link inside output/ pointing at C:\Users\...\.ssh is otherwise a read of
// whatever it points at.
func (a *App) artifactPath(path string) (string, error) {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return "", fmt.Errorf("no file given")
	}
	full, err := filepath.Abs(clean)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(full); err == nil {
		full = resolved
	}
	for _, root := range a.artifactRoots() {
		if root == "" {
			continue
		}
		dir, err := filepath.Abs(filepath.Join(root, outputDir))
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			dir = resolved
		}
		rel, err := filepath.Rel(dir, full)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return full, nil
	}
	return "", fmt.Errorf("ไฟล์นี้ไม่ได้อยู่ในโฟลเดอร์ผลงาน")
}

func sheetPreview(book *ooxml.WorkbookPreview) ArtifactPreview {
	s := book.Sheets[0]
	out := ArtifactPreview{Kind: "sheet", Sheet: s.Name}
	for i, row := range s.Rows {
		if i >= previewSheetRows {
			break
		}
		cells := row
		if len(cells) > previewSheetCols {
			cells = cells[:previewSheetCols]
		}
		trimmed := make([]string, len(cells))
		for j, c := range cells {
			trimmed[j] = clipRunes(strings.TrimSpace(c), 40)
		}
		out.Rows = append(out.Rows, trimmed)
	}
	if len(out.Rows) == 0 {
		return ArtifactPreview{Kind: "none"}
	}
	return out
}

// ooxmlText pulls the words out of a .docx or .pptx, for a card that shows a
// few lines of it.
//
// It used to walk the zip and the XML itself, which was the right call when
// nothing else in the codebase could read one of these — and stopped being the
// right call the moment `read` could (internal/ooxml/docx_read.go). Two walks
// over the same format is two answers to "what does this file say", and the one
// that drifts is always the one nobody is looking at.
//
// The card wants prose, so the structure the reader returns is flattened back
// down to lines here. That is a rendering decision belonging to the card, and it
// is the only part of this that is still the preview's own business.
func ooxmlText(path, ext string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if ext == ".pptx" {
		slides, err := ooxml.ReadPPTXText(data)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(strings.Join(slides, "\n\n")), nil
	}

	doc, err := ooxml.ReadDOCX(data)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, block := range doc.Blocks {
		switch block.Kind {
		case ooxml.ReadTable:
			for _, row := range block.Cells {
				out.WriteString(strings.Join(row, "\t") + "\n")
			}
		default:
			if block.Text != "" {
				out.WriteString(block.Text + "\n")
			}
		}
		if out.Len() > previewTextBytes {
			break
		}
	}
	return strings.TrimSpace(out.String()), nil
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	read, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:read], nil
}

// looksLikeText is the same question `file` asks: a NUL byte means binary, and
// so does a head that is mostly bytes no text has. Checked on the bytes rather
// than the extension so an unknown-but-readable file still gets a preview and
// a .docx renamed to .txt does not print zip headers into a card.
func looksLikeText(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	odd := 0
	for _, c := range b {
		if c < 0x09 || (c > 0x0d && c < 0x20) {
			odd++
		}
	}
	return odd*100/len(b) < 5
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
