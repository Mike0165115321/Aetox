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
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/ooxml"
)

// How much of a file is worth looking at for a thumbnail. These are display
// bounds, not safety bounds — a card shows a few lines, so reading a 40MB log
// to print six of its lines is pure waste.
const (
	previewTextBytes  = 24 << 10 // enough for far more lines than a card shows
	previewTextRunes  = 1400
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
		// document renders as a broken document.
		if info.Size() <= previewTextBytes {
			return ArtifactPreview{Kind: "html", Text: string(head)}, nil
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

// ooxmlText pulls the words out of a .docx or .pptx.
//
// There is no reader for these in internal/ooxml — it builds them — and a full
// one is not what a thumbnail needs. Both formats are a zip of XML where the
// text lives in one leaf element (w:t for Word, a:t for PowerPoint), so the
// words come out by walking the token stream and keeping the character data
// inside those elements. Structure is deliberately not reconstructed: the card
// shows a few lines, and a paragraph break is as much shape as that needs.
func ooxmlText(path, ext string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	want, leaf, breakOn := "word/document.xml", "t", "p"
	if ext == ".pptx" {
		want, leaf, breakOn = "ppt/slides/slide", "t", "p"
	}

	names := []string{}
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, want) && strings.HasSuffix(f.Name, ".xml") {
			names = append(names, f.Name)
		}
	}
	// slide2 before slide10: the zip's own order is not the deck's order.
	sort.Slice(names, func(i, j int) bool { return slideLess(names[i], names[j]) })

	var out strings.Builder
	for _, name := range names {
		f := fileByName(&zr.Reader, name)
		if f == nil {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		err = appendXMLText(&out, rc, leaf, breakOn)
		rc.Close()
		if err != nil && err != io.EOF {
			continue
		}
		if out.Len() > previewTextBytes {
			break
		}
	}
	return strings.TrimSpace(out.String()), nil
}

func fileByName(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// slideLess orders slide2.xml before slide10.xml, which a plain string sort
// gets backwards — a deck preview that opens on slide 10 is not a preview.
func slideLess(a, b string) bool {
	na, oka := trailingNumber(a)
	nb, okb := trailingNumber(b)
	if oka && okb && na != nb {
		return na < nb
	}
	return a < b
}

func trailingNumber(name string) (int, bool) {
	base := strings.TrimSuffix(filepath.Base(name), ".xml")
	i := len(base)
	for i > 0 && base[i-1] >= '0' && base[i-1] <= '9' {
		i--
	}
	if i == len(base) {
		return 0, false
	}
	n := 0
	for _, c := range base[i:] {
		n = n*10 + int(c-'0')
	}
	return n, true
}

func appendXMLText(out *strings.Builder, r io.Reader, leaf, breakOn string) error {
	dec := xml.NewDecoder(r)
	// Word and PowerPoint files declare entities we do not need and will not
	// fetch; leaving this nil keeps the decoder from reaching for anything.
	dec.Entity = xml.HTMLEntity
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Local == leaf {
				depth++
			}
		case xml.EndElement:
			if el.Name.Local == leaf && depth > 0 {
				depth--
			}
			if el.Name.Local == breakOn && out.Len() > 0 &&
				!strings.HasSuffix(out.String(), "\n") {
				out.WriteByte('\n')
			}
		case xml.CharData:
			if depth > 0 {
				out.Write(el)
			}
		}
		if out.Len() > previewTextBytes {
			return nil
		}
	}
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
