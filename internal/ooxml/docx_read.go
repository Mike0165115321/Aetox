package ooxml

// Reading a Word file back, and the first thing in this package that reads one
// for the *agent* rather than for a preview card.
//
// OFFICE-EXPORT-PLAN.md §8 ruled out reading Office files, and that ruling was
// about the thing it says: a reader that reconstructs a document faithfully
// enough to render it, which is a second Word. This is not that and cannot
// become it. It answers one question — what is in this document, in order — and
// it answers it in the same vocabulary the writer takes, so that what comes
// back can be reasoned about by the agent that has to work on the file.
//
// The gap it closes is embarrassing when written down. The preview card could
// already pull the words out of a .docx (desktop/artifact_preview.go), so the
// window could read a Word file and the agent whose whole job is documents
// could not — while its own profile promised to review drafts somebody sends
// it. The extractor was never wrong; it was in the wrong layer, and one layer
// too far from the only caller that needed it badly.
//
// What it deliberately does not do: fonts, sizes, colours, page geometry,
// numbering values, or anything else you would need to draw the document. An
// agent revising a report needs to know that paragraph 12 is a Heading 2 that
// says "ผลการทดสอบ". It does not need to know the heading is 13pt.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Block kinds a read document reports. They are the writer's kinds where the
// two overlap, so a document read here and rebuilt there does not need a
// translation table that could disagree with itself.
const (
	ReadParagraph = "paragraph"
	ReadTable     = "table"
	ReadImage     = "image"
)

// ReadBlock is one thing in a document, in reading order.
type ReadBlock struct {
	Kind string `json:"kind"`
	// Style is the paragraph style id as the file spells it — "Heading1",
	// "Caption", "ListParagraph" — and empty for body text. The id rather than
	// a friendly name because it is what the file actually contains, and an
	// agent about to change something has to name it the way the file does.
	Style string `json:"style,omitempty"`
	Text  string `json:"text,omitempty"`
	// Listed reports a paragraph carrying numbering — a bullet or a numbered
	// item. Which marker it draws is in the numbering part and is not read: the
	// distinction that matters to a reader is list versus prose.
	Listed bool `json:"listed,omitempty"`
	// Rows and Columns describe a table. Cells carries the text, row-major.
	Rows    int        `json:"rows,omitempty"`
	Columns int        `json:"columns,omitempty"`
	Cells   [][]string `json:"cells,omitempty"`
	// Pictures is how many drawings this block holds, and it is a count rather
	// than a flag because one paragraph can hold several.
	//
	// That is not a corner case. Somebody pasting screenshots in a row without
	// pressing Enter produces exactly it, and a real document on this machine
	// (2026-08-18) turned out to be a single paragraph carrying five figures.
	// Reported as one picture, an agent asked to caption them would write one
	// caption and believe it had finished.
	Pictures int `json:"pictures,omitempty"`
	// Alts are those pictures' alternative texts, in order, with the ones that
	// have none left out — so the list is shorter than Pictures whenever the
	// document came from somewhere that does not fill it in, which is most
	// places.
	Alts []string `json:"alts,omitempty"`
}

// ReadDocument is a Word file, in order.
type ReadDocument struct {
	Blocks []ReadBlock `json:"blocks"`
	// Pictures is how many drawings the document holds, counted separately
	// because it is the number somebody asks about ("did the figures survive?")
	// and because a picture can sit inside a paragraph that also has text.
	Pictures int `json:"pictures"`
}

// Bounds. A document is read to be worked on, not to be pasted into a prompt
// whole, and a 400-page contract read in full is a context window spent before
// the work starts. Truncation is reported by the caller rather than hidden.
const (
	maxReadBlocks    = 2000
	maxReadTableRows = 200
	maxReadCellRunes = 500
	maxReadParaRunes = 4000
)

// ReadDOCX pulls the structure out of a .docx.
func ReadDOCX(data []byte) (*ReadDocument, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a readable Word file: %w", err)
	}
	body, err := partBytes(zr, "word/document.xml")
	if err != nil {
		return nil, err
	}
	return parseDocumentXML(body)
}

// partBytes reads one named entry out of an OOXML package.
func partBytes(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("%s is missing from the package", name)
}

// parseDocumentXML walks the body once.
//
// A token walk rather than a struct unmarshal, and that is the load-bearing
// choice: WordprocessingML nests the same element names at several depths (a
// w:p inside a w:tc inside a w:tbl inside a w:tc), and every writer in the
// world emits a different subset of it. A walk that tracks depth ignores what
// it does not recognise, which is the only posture that survives a document
// Aetox did not write — and every document worth reading is one Aetox did not
// write.
func parseDocumentXML(body []byte) (*ReadDocument, error) {
	doc := &ReadDocument{}
	decoder := xml.NewDecoder(bytes.NewReader(body))

	// Paragraph state, reused rather than reallocated per w:p.
	var text strings.Builder
	var style string
	var listed bool
	var drawings int
	var alts []string
	inParagraph := false

	// Table state. tableDepth counts nesting so a paragraph inside a cell is
	// collected into the cell rather than emitted as a body paragraph, and a
	// nested table does not close its parent.
	tableDepth := 0
	var table ReadBlock
	var row []string

	push := func(b ReadBlock) {
		if len(doc.Blocks) < maxReadBlocks {
			doc.Blocks = append(doc.Blocks, b)
		}
	}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("document.xml is not readable: %w", err)
		}

		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "tbl":
				tableDepth++
				if tableDepth == 1 {
					table = ReadBlock{Kind: ReadTable}
				}
			case "tr":
				row = nil
			case "tc":
				// A cell's paragraphs are collected into one string; a cell
				// holding two paragraphs is one value with a line between them,
				// which is what it looks like on the page.
				text.Reset()
			case "p":
				if tableDepth == 0 {
					inParagraph = true
					text.Reset()
					style, listed, drawings, alts = "", false, 0, nil
				}
			case "pStyle":
				if inParagraph {
					style = attr(element, "val")
				}
			case "numPr":
				if inParagraph {
					listed = true
				}
			case "drawing":
				doc.Pictures++
				if inParagraph {
					drawings++
				}
			case "docPr":
				// One docPr per drawing, and it is the drawing's own name. A
				// paragraph with three pictures has three of these.
				if inParagraph {
					if descr := strings.TrimSpace(attr(element, "descr")); descr != "" {
						alts = append(alts, descr)
					}
				}
			case "br", "tab":
				text.WriteString(" ")
			case "t":
				var value string
				if err := decoder.DecodeElement(&value, &element); err != nil {
					return nil, fmt.Errorf("document.xml is not readable: %w", err)
				}
				text.WriteString(value)
			}

		case xml.EndElement:
			switch element.Name.Local {
			case "tc":
				if tableDepth > 0 {
					row = append(row, clip(strings.TrimSpace(text.String()), maxReadCellRunes))
					text.Reset()
				}
			case "tr":
				if tableDepth == 1 {
					if len(table.Cells) < maxReadTableRows {
						table.Cells = append(table.Cells, row)
					}
					table.Rows++
					if len(row) > table.Columns {
						table.Columns = len(row)
					}
				}
				row = nil
			case "tbl":
				if tableDepth == 1 {
					push(table)
				}
				if tableDepth > 0 {
					tableDepth--
				}
			case "p":
				if tableDepth != 0 || !inParagraph {
					break
				}
				inParagraph = false
				body := clip(strings.TrimSpace(text.String()), maxReadParaRunes)
				switch {
				case drawings > 0:
					// A paragraph holding a drawing is the picture, and any
					// text beside it belongs to the picture rather than to the
					// prose around it.
					push(ReadBlock{Kind: ReadImage, Pictures: drawings, Alts: alts, Text: body, Style: style})
				case body == "" && style == "":
					// An empty unstyled paragraph is spacing. Word files are
					// full of them and reporting each one as a block would bury
					// the document in its own whitespace.
				default:
					push(ReadBlock{Kind: ReadParagraph, Style: style, Text: body, Listed: listed})
				}
			}
		}
	}
	return doc, nil
}

// Truncated reports whether the document held more than was read.
func (d *ReadDocument) Truncated() bool { return len(d.Blocks) >= maxReadBlocks }

func attr(element xml.StartElement, name string) string {
	for _, a := range element.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func clip(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

// ReadPPTXText pulls the words off every slide, in slide order.
//
// A deck gets text rather than structure because a slide has no equivalent of a
// paragraph style to report: what a reader needs from somebody else's deck is
// what it says, and what an editor would need is geometry this package does not
// pretend to read.
func ReadPPTXText(data []byte) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a readable presentation: %w", err)
	}
	var names []string
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			names = append(names, f.Name)
		}
	}
	// slide2 before slide10: the zip's own order is not the deck's order.
	sortSlideNames(names)

	slides := make([]string, 0, len(names))
	for _, name := range names {
		part, err := partBytes(zr, name)
		if err != nil {
			continue
		}
		slides = append(slides, slideText(part))
	}
	return slides, nil
}

func slideText(part []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(part))
	var lines []string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "t" {
			continue
		}
		var value string
		if decoder.DecodeElement(&value, &start) != nil {
			continue
		}
		if value = strings.TrimSpace(value); value != "" {
			lines = append(lines, value)
		}
	}
	return strings.Join(lines, "\n")
}

// sortSlideNames orders slide2.xml before slide10.xml, which a plain string
// sort gets backwards — a deck read starting at slide 10 is not the deck.
func sortSlideNames(names []string) {
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && slideNumber(names[j]) < slideNumber(names[j-1]); j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
}

func slideNumber(name string) int {
	digits := 0
	started := false
	for _, r := range name {
		if r >= '0' && r <= '9' {
			digits = digits*10 + int(r-'0')
			started = true
			continue
		}
		if started {
			break
		}
	}
	return digits
}
