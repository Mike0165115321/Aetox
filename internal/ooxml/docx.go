package ooxml

import (
	"fmt"
	"strings"
)

// WordprocessingML, phase 3.
//
// The format looks like the easiest of the three — a document is a flat run of
// paragraphs, with none of xlsx's cell typing and none of pptx's coordinate
// geometry — and it is the strictest. Every property container here
// (`w:rPr`, `w:pPr`, `w:tblPr`, `w:tcPr`, `w:style`, `w:lvl`, `w:sectPr`) is an
// xsd:sequence, so `w:szCs` before `w:sz` is not untidy, it is "Word found
// unreadable content". The xlsx and pptx writers tolerate far more, so the
// habit those two build is the wrong habit here: everything below is emitted by
// a fixed-order writer, never by appending properties as they are discovered.
//
// Units: the twip, 1/20 point, 1440 to the inch — except font sizes, which are
// half-points, and border widths, which are eighths of a point. Three units in
// one file is the format's, not ours.
const (
	twipsPerInch = 1440

	// A4, because the audience is Thai.
	pageWidth  = 11906
	pageHeight = 16838
	pageMargin = twipsPerInch

	// What is actually left to draw in. Table widths must add up to this or the
	// reader recomputes the grid and quietly ignores what we asked for.
	textWidth = pageWidth - 2*pageMargin

	// Every list level is indented half an inch with a quarter-inch hanging
	// marker — the formula Word itself emits, and the one that never lets the
	// bullet collide with the first word.
	listIndent  = 720
	listHanging = 360

	// EMU per twip, exactly. Drawings are the one part of a Word file measured
	// in the DrawingML unit rather than the WordprocessingML one — a fourth unit
	// in a format that already has three — so every picture crosses this
	// conversion on its way onto the page.
	emuPerTwip = 635
	// What is left to draw down the page, as textWidth is what is left across it.
	textHeight = pageHeight - 2*pageMargin

	// The box a picture is fitted into: the printable area, to the twip.
	//
	// Both bounds matter and they fail differently. Too wide and Word draws the
	// picture straight over the right margin and off the paper. Too TALL and it
	// clips the bottom — no warning, no second page, just a screenshot with its
	// lower half missing, which is the failure that ships because the document
	// opens fine and only the person holding the paper can see it.
	//
	// The height bound is the one that actually bites, because the picture an
	// appendix is mostly made of is a phone screenshot: 1170x2532 fitted to the
	// text column is 13.6 inches tall on a 9.7 inch page. Portrait is the normal
	// case, not the edge one.
	maxImageWidth  = textWidth * emuPerTwip
	maxImageHeight = textHeight * emuPerTwip

	// Vertical room for one line of caption, so a full-height figure does not
	// push its own caption onto the next page and leave the reader a picture
	// with no number. One line at 10pt with this file's line spacing, plus the
	// space after it: about a third of an inch.
	//
	// One line, not two. A caption long enough to wrap still spills, and that is
	// the right trade — reserving for the worst caption would shrink every
	// picture in the document to protect a case most of them are not.
	captionRoom = 480 * emuPerTwip
)

// docxFont is the one font family this package uses, and it is used for *all
// four* `w:rFonts` slots — ascii, hAnsi, eastAsia and cs.
//
// Setting only `w:cs` would be correct for Word, which resolves U+0E00–U+0E7F
// against the complex-script slot, and wrong for Google Docs, which has no
// complex-script model at all: it reads `w:ascii`/`w:hAnsi` and would render
// Thai in whatever the Latin font is. Two of our three target readers disagree
// about which attribute to read, so the only answer that satisfies both is to
// make them agree.
//
// Leelawadee UI ships with a base Windows 10/11 install and is what
// internal/ooxml/pptx.go already names. Tahoma is the wider-reach alternative
// (every Windows since XP, and macOS Word) if a document ever has to survive an
// older machine. Not any of the UPC faces — Angsana, Cordia, Browallia and the
// rest moved into the optional "Thai Supplemental Fonts" feature, which is off
// by default on Windows 11, so naming one gets it substituted on a clean
// install.
const docxFont = "Leelawadee UI"

// Style ids, as constants, because `w:pStyle/@w:val` matching is
// case-SENSITIVE: `heading1` against `w:styleId="Heading1"` silently resolves
// to Normal, with no warning anywhere. The styles part and the document body
// must therefore never spell one of these twice.
//
// `w:name` is the opposite — matched case-insensitively — and the canonical
// lowercase spelling ("heading 1") is what binds a style to Word's built-in.
const (
	styleNormal        = "Normal"
	styleListParagraph = "ListParagraph"
	styleCaption       = "Caption"
	styleHeadingPrefix = "Heading"
)

// BlockKind is what a block *is*. The model sends a discriminator rather than a
// one-key object ({"heading": "..."} vs {"paragraph": "..."}) because a tagged
// shape is the one small models emit reliably — a union of key names is the
// shape they get wrong.
type BlockKind string

const (
	BlockHeading   BlockKind = "heading"
	BlockParagraph BlockKind = "paragraph"
	BlockBullets   BlockKind = "bullets"
	BlockNumbered  BlockKind = "numbered"
	BlockTable     BlockKind = "table"
	// BlockLineItems is a priced table whose money is worked out here rather
	// than sent in. Every commercial document is one — quotation, invoice, tax
	// invoice, receipt, purchase order — and every one of them fails the same
	// way: a total that was typed rather than calculated, wrong by an amount
	// nobody downstream can see.
	BlockLineItems BlockKind = "lineitems"
	// BlockImage is a picture between two paragraphs, with the caption that
	// names it. Both halves are one block on purpose: a figure whose caption is
	// a separate paragraph is a figure that survives one edit and then floats
	// away from its own number.
	BlockImage BlockKind = "image"
)

// LineItem is one priced row. Amount is never sent — it is Qty × Price, worked
// out where arithmetic cannot drift.
type LineItem struct {
	Text  string
	Qty   float64
	Price float64
	// Note rides under the description, for the line that needs a second
	// sentence without becoming its own row.
	Note string
}

// TotalKind is what one summary row does to the running amount.
type TotalKind string

const (
	// TotalSubtotal is the sum of every line.
	TotalSubtotal TotalKind = "subtotal"
	// TotalRate is a percentage *of the subtotal* — VAT at 0.07, withholding
	// tax at -0.03. Negative rates are how a deduction is written, which is
	// what makes one mechanism enough for both.
	TotalRate TotalKind = "rate"
	// TotalTotal is the subtotal plus every rate row above it, added from the
	// rounded figures actually printed — so the document adds up on the page,
	// not only in the arithmetic behind it.
	TotalTotal TotalKind = "total"
)

// TotalRow is one line of the summary under a priced table.
type TotalRow struct {
	Label string
	Kind  TotalKind
	Rate  float64
}

// Block is one element of a document, in order.
type Block struct {
	Kind BlockKind
	// Text carries a heading or a paragraph.
	Text string
	// Level is a heading's depth, 1-3. Anything else is clamped.
	Level int
	// Items are the lines of a bullet or numbered list.
	Items []string
	// Columns and Rows are a table's header and body.
	Columns []string
	Rows    [][]string
	// Align is per column: "right", "center", or anything else for left. A
	// column of amounts that reads down its left edge is the single loudest
	// sign a document was generated rather than written.
	Align []string
	// Widths are relative weights per column — {4,1,1,1} gives the description
	// four times the room of each figure. Empty divides the page equally.
	Widths []int
	// Plain drops the borders and the header shading: the same table machinery
	// used as a layout, which is what a seller/buyer block or a run of
	// label-and-value lines actually is.
	Plain bool
	// Lines and Totals belong to BlockLineItems.
	Lines  []LineItem
	Totals []TotalRow
	// Image belongs to BlockImage; Text is then the caption under it, which is
	// why a picture needs no second field for one.
	Image *Picture
}

// headingLevels is capped at 3 on purpose. Google Docs maps heading_1 through
// heading_6 and nothing deeper, and three levels is more structure than a
// generated report has ever needed.
const headingLevels = 3

// BuildDOCX turns blocks into the parts of a .docx package.
func BuildDOCX(blocks []Block) ([]Part, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("ooxml: document needs at least one block")
	}

	// Numbering state lives on the abstractNum, not on the num — so two list
	// blocks pointing at one abstractNum do NOT restart, and the second list
	// counts 3, 4, 5. The instinct carried over from xlsx and pptx ("define the
	// bullet once, reference it twice") produces exactly that. Each list block
	// therefore gets its own abstractNum and its own num.
	var abstracts, nums strings.Builder
	nextList := 0

	// Pictures are the first thing in this writer that puts a part in the
	// package rather than only XML in the body, so the manifest and the
	// relationship file stop being constants: a part with no content-type
	// Default and no relationship pointing at it is the "unreadable content"
	// prompt, and both have to be built from what the body actually used.
	var media []Part
	var imageRels strings.Builder
	var imageExts []string
	seenExt := map[string]bool{}
	imageSeq := 0

	var body strings.Builder
	lastWasTable := false
	for _, block := range blocks {
		switch block.Kind {
		case BlockHeading:
			level := block.Level
			if level < 1 {
				level = 1
			}
			if level > headingLevels {
				level = headingLevels
			}
			body.WriteString(paragraph(fmt.Sprintf("%s%d", styleHeadingPrefix, level), 0, block.Text))
			lastWasTable = false
		case BlockBullets, BlockNumbered:
			// A literal U+2022 in a font that has it, never Word's own idiom of
			// U+F0B7 in the Symbol face: that codepoint is private-use and means
			// nothing without the paired rFonts override, so Google Docs and
			// LibreOffice draw it as an empty box. Copying Word here would be
			// the same two-layer trap the pptx writer had to solve.
			format, marker := "bullet", "•"
			if block.Kind == BlockNumbered {
				format, marker = "decimal", "%1."
			}
			// abstractNumId counts from 0; numId counts from 1 because 0 is the
			// format's reserved "no numbering" sentinel. They are independent id
			// spaces, so the numeric overlap is harmless.
			abstractID, numID := nextList, nextList+1
			nextList++
			fmt.Fprintf(&abstracts,
				`<w:abstractNum w:abstractNumId="%d"><w:multiLevelType w:val="singleLevel"/>`+
					`<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="%s"/><w:lvlText w:val="%s"/><w:lvlJc w:val="left"/>`+
					`<w:pPr><w:ind w:left="%d" w:hanging="%d"/></w:pPr></w:lvl></w:abstractNum>`,
				abstractID, format, marker, listIndent, listHanging)
			fmt.Fprintf(&nums, `<w:num w:numId="%d"><w:abstractNumId w:val="%d"/></w:num>`, numID, abstractID)

			for _, item := range block.Items {
				body.WriteString(paragraph(styleListParagraph, numID, item))
			}
			lastWasTable = false
		case BlockImage:
			if block.Image == nil || len(block.Image.Data) == 0 {
				return nil, fmt.Errorf("ooxml: image block has no picture")
			}
			if block.Image.WidthPx <= 0 || block.Image.HeightPx <= 0 {
				return nil, fmt.Errorf("ooxml: image block has no dimensions, so it would be drawn at nothing by nothing")
			}
			imageSeq++
			ext := block.Image.Ext
			if ext == "" {
				ext = "png"
			}
			if !seenExt[ext] {
				seenExt[ext] = true
				imageExts = append(imageExts, ext)
			}
			// rId1 and rId2 are styles and numbering, so pictures start at 3.
			relID := fmt.Sprintf("rId%d", imageSeq+2)
			name := fmt.Sprintf("image%d.%s", imageSeq, ext)
			fmt.Fprintf(&imageRels,
				`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/%s"/>`,
				relID, name)
			media = append(media, Part{Name: "word/media/" + name, Data: block.Image.Data})
			// A captioned figure gives up a line of its own height so the
			// caption stays on the page with it. An uncaptioned one has the
			// whole printable area.
			room := int64(maxImageHeight)
			if strings.TrimSpace(block.Text) != "" {
				room -= captionRoom
			}
			body.WriteString(imageXML(block.Image, relID, name, imageSeq, room))
			if strings.TrimSpace(block.Text) != "" {
				body.WriteString(paragraph(styleCaption, 0, block.Text))
			}
			lastWasTable = false
		case BlockTable, BlockLineItems:
			// Two tables that are direct siblings merge into one, which silently
			// fuses two unrelated tables.
			if lastWasTable {
				body.WriteString(`<w:p/>`)
			}
			if block.Kind == BlockLineItems {
				body.WriteString(lineItemsXML(block))
			} else {
				body.WriteString(tableXML(block))
			}
			lastWasTable = true
		default:
			body.WriteString(paragraph("", 0, block.Text))
			lastWasTable = false
		}
	}
	// Word's own output puts a paragraph after every table, and a body that ends
	// with one leaves the user no cursor position past it.
	if lastWasTable {
		body.WriteString(`<w:p/>`)
	}

	var document strings.Builder
	document.WriteString(xmlHeader)
	// xmlns:r and xmlns:wp are declared here rather than on the drawing itself
	// because r:embed is an *attribute*, and an attribute cannot carry a
	// namespace declaration of its own. A document with a picture and no xmlns:r
	// on the root is not well-formed XML at all, which Word reports as
	// unreadable content with no mention of a namespace. Declaring them on every
	// document, picture or not, is a few unused bytes against a whole class of
	// failure.
	document.WriteString(`<w:document ` +
		`xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
		`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"><w:body>`)
	document.WriteString(body.String())
	// sectPr is the last child of w:body — not conventionally last, structurally
	// last. pgSz strictly before pgMar, and pgMar's header/footer/gutter look
	// optional and are not.
	fmt.Fprintf(&document,
		`<w:sectPr><w:pgSz w:w="%d" w:h="%d"/>`+
			`<w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="708" w:footer="708" w:gutter="0"/>`+
			`</w:sectPr>`,
		pageWidth, pageHeight, pageMargin, pageMargin, pageMargin, pageMargin)
	document.WriteString(`</w:body></w:document>`)

	// Every abstractNum before every num. Word opens a file with them
	// interleaved and then silently drops all numbering — the list keeps its
	// indent and loses its markers, with nothing anywhere saying why.
	numbering := xmlHeader +
		`<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		abstracts.String() + nums.String() +
		`</w:numbering>`

	return append([]Part{
		{Name: "[Content_Types].xml", Data: []byte(docxContentTypes(imageExts))},
		{Name: "_rels/.rels", Data: []byte(docxRootRels)},
		{Name: "word/document.xml", Data: []byte(document.String())},
		{Name: "word/_rels/document.xml.rels", Data: []byte(docxDocumentRels(imageRels.String()))},
		{Name: "word/styles.xml", Data: []byte(docxStyles)},
		{Name: "word/numbering.xml", Data: []byte(numbering)},
	}, media...), nil
}

// imageFit is how big a picture is drawn, in EMU.
//
// Its own pixels at 96 DPI, which is what makes a screenshot come out the size
// it looks on screen, fitted inside the printable area. Never enlarged: a 200 px
// logo stretched to the full width is blurry in a way that reads as a mistake,
// and the caller who wanted it bigger did not say so.
//
// Both bounds are applied, and the second one is why this is not two lines. A
// picture reduced to the text column can still be taller than the page — that
// is every phone screenshot — so the width fit has to be re-checked against the
// height fit rather than either being trusted on its own.
//
// int64 throughout, because the intermediate product is one dimension in EMU
// times a bound in EMU, and that passes 2^31 an order of magnitude before any
// real picture does.
func imageFit(img *Picture, room int64) (int64, int64) {
	w := int64(img.WidthPx) * emuPerPixel
	h := int64(img.HeightPx) * emuPerPixel
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	if w > maxImageWidth {
		h = h * maxImageWidth / w
		w = maxImageWidth
	}
	if room > 0 && h > room {
		w = w * room / h
		h = room
	}
	return w, h
}

const (
	nsDrawing = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsPicture = "http://schemas.openxmlformats.org/drawingml/2006/picture"
)

// imageXML writes one picture as an inline drawing in its own centred paragraph.
//
// Two things here are load-bearing and neither is visible in the result.
//
// The children of wp:inline are a sequence — extent, effectExtent, docPr,
// cNvGraphicFramePr, graphic — with the same consequence every other sequence in
// this file has: a reordered child is "unreadable content", not a
// differently-placed picture.
//
// wp:docPr/@id must be unique across the document and must not be 0. Word
// tolerates a repeat; Google Docs drops every picture after the first duplicate,
// which is the failure that looks like the tool silently ignored most of the
// pictures it was given.
func imageXML(img *Picture, relID, name string, seq int, room int64) string {
	w, h := imageFit(img, room)
	alt := img.AltText
	if alt == "" {
		alt = name
	}
	var b strings.Builder
	b.WriteString(`<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="80"/></w:pPr><w:r><w:drawing>`)
	fmt.Fprintf(&b, `<wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/>`, w, h)
	b.WriteString(`<wp:effectExtent l="0" t="0" r="0" b="0"/>`)
	fmt.Fprintf(&b, `<wp:docPr id="%d" name="Picture %d" descr="%s"/>`, seq, seq, escapeXML(alt))
	b.WriteString(`<wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="` + nsDrawing + `" noChangeAspect="1"/></wp:cNvGraphicFramePr>`)
	b.WriteString(`<a:graphic xmlns:a="` + nsDrawing + `"><a:graphicData uri="` + nsPicture + `">`)
	b.WriteString(`<pic:pic xmlns:pic="` + nsPicture + `"><pic:nvPicPr>`)
	fmt.Fprintf(&b, `<pic:cNvPr id="%d" name="%s" descr="%s"/>`, seq, escapeXML(name), escapeXML(alt))
	b.WriteString(`<pic:cNvPicPr/></pic:nvPicPr>`)
	fmt.Fprintf(&b, `<pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`, relID)
	fmt.Fprintf(&b, `<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm>`, w, h)
	b.WriteString(`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`)
	b.WriteString(`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`)
	return b.String()
}

// paragraph writes one w:p. style may be empty (the default style); numID 0
// means no list numbering.
//
// w:pPr must be the first child of w:p, and inside it the order is fixed:
// pStyle before numPr, and both a long way before spacing and ind. Swapping any
// two is the repair prompt, not a differently-styled paragraph.
func paragraph(style string, numID int, text string) string {
	var b strings.Builder
	b.WriteString(`<w:p>`)
	if style != "" || numID != 0 {
		b.WriteString(`<w:pPr>`)
		if style != "" {
			fmt.Fprintf(&b, `<w:pStyle w:val="%s"/>`, style)
		}
		if numID != 0 {
			fmt.Fprintf(&b, `<w:numPr><w:ilvl w:val="0"/><w:numId w:val="%d"/></w:numPr>`, numID)
		}
		b.WriteString(`</w:pPr>`)
	}
	b.WriteString(runs(text, false))
	b.WriteString(`</w:p>`)
	return b.String()
}

// runs writes the w:r elements for one paragraph's worth of text.
//
// A newline inside a w:t is not a line break — XML collapses it to whitespace
// and the reader draws one long line — so text is split and joined with w:br,
// which is a sibling of w:t inside the run.
//
// The split is by line only. A Thai grapheme cluster must never straddle two
// runs: Word shapes each run independently, so a consonant in one run and its
// tone mark in the next renders the mark detached — the same drifting-marks
// symptom the pptx writer had to solve. Splitting on newlines cannot land
// inside a cluster, and nothing else here splits at all.
func runs(text string, bold bool) string {
	var b strings.Builder
	for i, segment := range boldSegments(text) {
		emphasis := bold || segment.bold
		lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(segment.text, "\r\n", "\n"), "\r", "\n"), "\n")
		b.WriteString(`<w:r>`)
		if emphasis {
			b.WriteString(runProps(true))
		}
		for j, line := range lines {
			if j > 0 {
				b.WriteString(`<w:br/>`)
			}
			// xml:space="preserve" unconditionally. Without it the reader strips
			// leading and trailing spaces, and Thai uses the space as a phrase
			// separator — so a stripped space changes what the sentence says.
			fmt.Fprintf(&b, `<w:t xml:space="preserve">%s</w:t>`, escapeXML(line))
		}
		b.WriteString(`</w:r>`)
		_ = i
	}
	return b.String()
}

type textSegment struct {
	text string
	bold bool
}

// boldSegments splits **emphasis** out of a line.
//
// `**` is what a model writes for bold without being asked, and what a person
// editing the JSON by hand would try. Supporting it here is cheaper than a
// markup field that has to be explained in every tool description that carries
// this writer.
//
// Only balanced pairs count: an odd `**` is a literal, because a document that
// silently swallows a stray asterisk is worse than one that prints it. The
// split is at author-chosen boundaries, so it cannot land inside a Thai
// grapheme cluster the way an automatic wrap could — the hazard runs() exists
// to avoid stays avoided.
func boldSegments(text string) []textSegment {
	if !strings.Contains(text, "**") {
		return []textSegment{{text: text}}
	}
	var out []textSegment
	rest := text
	for {
		open := strings.Index(rest, "**")
		if open < 0 {
			break
		}
		close := strings.Index(rest[open+2:], "**")
		if close < 0 {
			break // unbalanced: everything left is literal
		}
		inner := rest[open+2 : open+2+close]
		if inner == "" {
			// "****" is not emphasis of nothing; treat the first pair as text
			// so the characters survive.
			out = append(out, textSegment{text: rest[:open+2]})
			rest = rest[open+2:]
			continue
		}
		if open > 0 {
			out = append(out, textSegment{text: rest[:open]})
		}
		out = append(out, textSegment{text: inner, bold: true})
		rest = rest[open+2+close+2:]
	}
	if rest != "" {
		out = append(out, textSegment{text: rest})
	}
	if len(out) == 0 {
		return []textSegment{{text: text}}
	}
	return out
}

// runProps writes a w:rPr in schema order.
//
// Every character property Word tracks has a complex-script twin sitting
// immediately after it in that order — b/bCs, i/iCs, sz/szCs — and Thai reads
// only the twin. A run with `w:b` and no `w:bCs` renders Latin bold and Thai
// regular on the same line; one with `w:sz` and no `w:szCs` renders them at
// different sizes. So the Thai fix and the ordering hazard arrive together,
// which is why this is a writer and not a string built at the call site.
func runProps(bold bool) string {
	if !bold {
		return ""
	}
	return `<w:rPr><w:b/><w:bCs/></w:rPr>`
}

func tableXML(block Block) string {
	columns := len(block.Columns)
	for _, row := range block.Rows {
		if len(row) > columns {
			columns = len(row)
		}
	}
	if columns == 0 {
		return ""
	}
	widths := gridWidths(columns, block.Widths)

	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr>`)
	// tblPr order: tblW, tblBorders, tblLayout, tblCellMar.
	fmt.Fprintf(&b, `<w:tblW w:w="%d" w:type="dxa"/>`, textWidth)
	// The default table style draws no borders at all, in all three readers —
	// a table with a bare w:tblPr is invisible, which is not what anyone means
	// by "a table". Explicit borders also avoid depending on a `TableGrid`
	// style being present, which fails silently when it is not.
	//
	// A `Plain` table is the deliberate exception: the same machinery used as a
	// layout — the seller and buyer blocks side by side, a run of label-and-
	// value lines — where a grid of boxes would announce a table nobody meant.
	if !block.Plain {
		b.WriteString(`<w:tblBorders>` +
			border("top") + border("left") + border("bottom") + border("right") +
			border("insideH") + border("insideV") +
			`</w:tblBorders>`)
	}
	// Without fixed layout the table is autofit and the widths above are
	// advisory. The Thai consequence is specific: a long unbroken Thai string —
	// Thai has no inter-word spaces — can blow one column out to full width.
	b.WriteString(`<w:tblLayout w:type="fixed"/>`)
	// Cell padding normally comes from the built-in TableNormal style, which
	// this package does not ship, so text would otherwise touch the borders.
	b.WriteString(`<w:tblCellMar>` +
		`<w:top w:w="60" w:type="dxa"/><w:left w:w="108" w:type="dxa"/>` +
		`<w:bottom w:w="60" w:type="dxa"/><w:right w:w="108" w:type="dxa"/>` +
		`</w:tblCellMar>`)
	b.WriteString(`</w:tblPr>`)

	b.WriteString(`<w:tblGrid>`)
	for _, w := range widths {
		fmt.Fprintf(&b, `<w:gridCol w:w="%d"/>`, w)
	}
	b.WriteString(`</w:tblGrid>`)

	if len(block.Columns) > 0 {
		// tblHeader repeats the row on every page the table spans, and only
		// works on rows contiguous from the first. cantSplit keeps a tall Thai
		// header cell from being broken across the page break it is marking.
		b.WriteString(`<w:tr><w:trPr><w:cantSplit/><w:tblHeader/></w:trPr>`)
		for i := 0; i < columns; i++ {
			text := ""
			if i < len(block.Columns) {
				text = block.Columns[i]
			}
			b.WriteString(tableCell(text, widths[i], true, alignAt(block.Align, i), block.Plain))
		}
		b.WriteString(`</w:tr>`)
	}
	for _, row := range block.Rows {
		b.WriteString(`<w:tr>`)
		for i := 0; i < columns; i++ {
			text := ""
			if i < len(row) {
				text = row[i]
			}
			b.WriteString(tableCell(text, widths[i], false, alignAt(block.Align, i), block.Plain))
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
	return b.String()
}

// gridWidths turns relative weights into the twentieths-of-a-point the file
// wants. sum(gridCol) must equal tblW or the reader recomputes the grid and
// throws the weights away, so the rounding remainder goes to the last column
// rather than being lost.
func gridWidths(columns int, weights []int) []int {
	total := 0
	for i := 0; i < columns && i < len(weights); i++ {
		if weights[i] > 0 {
			total += weights[i]
		}
	}
	widths := make([]int, columns)
	if total == 0 {
		base := textWidth / columns
		for i := range widths {
			widths[i] = base
		}
		widths[columns-1] += textWidth - base*columns
		return widths
	}
	used := 0
	for i := 0; i < columns; i++ {
		weight := 1
		if i < len(weights) && weights[i] > 0 {
			weight = weights[i]
		} else if i < len(weights) {
			// A zero or negative weight in a list that has real ones is a gap,
			// not a request for an invisible column — it gets a share of one.
			total++
		}
		widths[i] = textWidth * weight / total
		used += widths[i]
	}
	widths[columns-1] += textWidth - used
	return widths
}

func alignAt(align []string, i int) string {
	if i < 0 || i >= len(align) {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(align[i])) {
	case "right", "end":
		return "right"
	case "center", "centre", "middle":
		return "center"
	default:
		return ""
	}
}

// lineItemsXML renders a priced table: the lines, then the summary rows whose
// figures were worked out in money.go rather than sent in.
//
// Rendered as one table so the columns of the summary line up under the columns
// of the lines — a totals block in a table of its own drifts by a few points
// and looks, on the page, exactly like a mistake.
func lineItemsXML(block Block) string {
	amounts, totals := computeLines(block.Lines, block.Totals)

	table := Block{
		Kind:    BlockTable,
		Columns: block.Columns,
		Align:   block.Align,
		Widths:  block.Widths,
	}
	columns := len(block.Columns)
	if columns == 0 {
		return ""
	}
	for i, line := range block.Lines {
		text := line.Text
		if strings.TrimSpace(line.Note) != "" {
			text += "\n" + line.Note
		}
		row := make([]string, columns)
		row[0] = text
		if columns >= 4 {
			qty := line.Qty
			if qty == 0 {
				qty = 1
			}
			row[columns-3] = formatQuantity(qty)
			row[columns-2] = formatMoney(line.Price)
		}
		row[columns-1] = formatMoney(amounts[i])
		table.Rows = append(table.Rows, row)
	}
	// The label sits in the column before the figure and is bold, which is what
	// separates the summary from the lines without a second table or a rule.
	for _, total := range totals {
		row := make([]string, columns)
		labelAt := columns - 2
		if labelAt < 0 {
			labelAt = 0
		}
		row[labelAt] = "**" + total.Label + "**"
		row[columns-1] = "**" + formatMoney(total.Amount) + "**"
		table.Rows = append(table.Rows, row)
	}
	return tableXML(table)
}

// border writes one w:tblBorders child. w:sz is eighths of a point, so 4 is a
// half-point hairline. All four attributes are written even though three are
// schema-optional: some readers draw a zero-width line without w:sz.
func border(edge string) string {
	return fmt.Sprintf(`<w:%s w:val="single" w:sz="4" w:space="0" w:color="BFBFBF"/>`, edge)
}

// tableCell writes one w:tc.
//
// Two rules, both of which are how hand-built tables usually fail:
//
// A w:tc must end with a w:p. A cell holding only its w:tcPr trips the repair
// prompt, so an empty cell still gets an empty paragraph.
//
// The *paragraph mark's own* run properties (w:pPr/w:rPr) carry the header's
// bold as well as the run does, because row height is computed from the mark.
// A Thai header cell whose mark is still default weight gets a visibly short
// row, and an empty cell collapses — the table-side version of the same
// complex-script split that governs w:b and w:bCs.
func tableCell(text string, width int, header bool, align string, plain bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>`, width)
	if header && !plain {
		b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="F2F4F7"/>`)
	}
	b.WriteString(`</w:tcPr>`)

	// pPr order: spacing, then jc, then the paragraph mark's rPr last. w:jc
	// after w:spacing is not a preference — a reader that finds them the other
	// way round rejects the document.
	b.WriteString(`<w:p><w:pPr><w:spacing w:before="40" w:after="40" w:line="240" w:lineRule="auto"/>`)
	if align != "" {
		fmt.Fprintf(&b, `<w:jc w:val="%s"/>`, align)
	}
	if header {
		b.WriteString(runProps(true))
	}
	b.WriteString(`</w:pPr>`)
	b.WriteString(runs(text, header))
	b.WriteString(`</w:p></w:tc>`)
	return b.String()
}

// docxContentTypes is a function rather than a constant because a picture puts
// a part in the package whose type is not xml, and a part with no content type
// is a file Word refuses to open at all.
func docxContentTypes(imageExts []string) string {
	var b strings.Builder
	b.WriteString(docxContentTypesHead)
	// One Default per image extension actually used. A Default for an extension
	// with no part is legal; a part with no Default is not.
	for _, ext := range imageExts {
		fmt.Fprintf(&b, `<Default Extension="%s" ContentType="image/%s"/>`, ext, contentTypeExt(ext))
	}
	b.WriteString(docxContentTypesTail)
	return b.String()
}

const docxContentTypesHead = xmlHeader +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>`

const docxContentTypesTail = "" +
	// "document" twice is correct. wordprocessingml.template.main+xml opens
	// without complaint and makes Word treat the file as a template.
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	// Not optional despite the Default above covering .xml: a part whose
	// relationship declares one type and whose content type says another is
	// what Word refuses outright.
	`<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>` +
	`</Types>`

const docxRootRels = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

// Targets are relative to the owning part's folder, so these are bare file
// names rather than word/-prefixed. A part with no relationship pointing at it
// is *silently* ignored — the document opens clean with every heading as body
// text and no bullets, and nothing anywhere says why.
// images is the pre-rendered run of picture relationships, appended after the
// two fixed ones — which is why rId1 and rId2 are never handed to a picture.
func docxDocumentRels(images string) string {
	return xmlHeader +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>` +
		images +
		`</Relationships>`
}

// docxStyles carries the Thai fix, and carries it once.
//
// w:docDefaults inverts the pPr-before-rPr convention used everywhere else in
// the format: rPrDefault comes first. With the four font slots and both size
// slots set here, an ordinary body run needs no Thai markup at all — only the
// places that override weight or size (headings, table headers) have to repeat
// the complex-script twin.
//
// Line spacing is auto, never exact. Thai stacks a tone mark above a vowel
// above the consonant, and a fixed line height clips the top of that stack:
// fine on screen, wrong on paper.
//
// No w:rFonts attribute here ends in "Theme". This package ships no theme part,
// and w:cstheme does not fall back to w:cs — it replaces it.
// A var rather than a const only because heading() builds the three heading
// styles from one definition — three near-identical copies is how the child
// order drifts between them.
var docxStyles = xmlHeader +
	`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
	`<w:docDefaults>` +
	`<w:rPrDefault><w:rPr>` +
	`<w:rFonts w:ascii="` + docxFont + `" w:eastAsia="` + docxFont + `" w:hAnsi="` + docxFont + `" w:cs="` + docxFont + `"/>` +
	`<w:sz w:val="22"/><w:szCs w:val="22"/>` +
	// w:bidi is the complex-script language — w:val is the Latin/proofing one.
	// Tagging the run as Thai is also what gives the reader dictionary-based
	// line breaking; Thai has no inter-word spaces, so without it a wrap can
	// land mid-word.
	`<w:lang w:val="en-US" w:eastAsia="en-US" w:bidi="th-TH"/>` +
	`</w:rPr></w:rPrDefault>` +
	`<w:pPrDefault><w:pPr>` +
	`<w:spacing w:after="160" w:line="276" w:lineRule="auto"/>` +
	`</w:pPr></w:pPrDefault>` +
	`</w:docDefaults>` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="` + styleNormal + `">` +
	`<w:name w:val="Normal"/><w:qFormat/>` +
	`</w:style>` +
	heading(1, "0", "32") +
	heading(2, "1", "26") +
	heading(3, "2", "24") +
	// Defined rather than merely referenced: Word falls back to its own built-in
	// for a well-known styleId, and Google Docs does not — an undefined
	// ListParagraph reference is the clearest "Word tolerates, Google Docs does
	// not" case in this whole format. contextualSpacing is what makes six items
	// read as a list instead of six paragraphs.
	`<w:style w:type="paragraph" w:styleId="` + styleListParagraph + `">` +
	`<w:name w:val="List Paragraph"/><w:basedOn w:val="` + styleNormal + `"/><w:uiPriority w:val="34"/><w:qFormat/>` +
	`<w:pPr><w:spacing w:after="0"/><w:ind w:left="720"/><w:contextualSpacing/></w:pPr>` +
	`</w:style>` +
	// A figure's caption is a real style, not a small italic paragraph that
	// happens to sit under a picture. The w:name binds it to Word's own built-in
	// "caption", which is what makes Insert > Table of Figures find it, and what
	// makes every caption in a document change together when somebody decides
	// they should look different. 10pt rather than Word's own 9pt: Thai stacks a
	// tone mark above a vowel above the consonant, and 9pt loses the top of that
	// stack on paper.
	`<w:style w:type="paragraph" w:styleId="` + styleCaption + `">` +
	`<w:name w:val="caption"/><w:basedOn w:val="` + styleNormal + `"/><w:next w:val="` + styleNormal + `"/>` +
	`<w:uiPriority w:val="35"/><w:qFormat/>` +
	`<w:pPr><w:jc w:val="center"/><w:spacing w:before="0" w:after="240"/></w:pPr>` +
	`<w:rPr><w:i/><w:iCs/><w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr>` +
	`</w:style>` +
	`</w:styles>`

// heading writes one built-in heading style.
//
// w:outlineLvl is what makes a heading a heading — Navigation Pane, outline,
// table of contents. Without it the result is large bold text that reports as
// body text, which looks right on screen and is wrong everywhere else. The
// w:name is the canonical lowercase built-in spelling, which is what binds the
// style to Word's own definition; unlike w:styleId, it is matched
// case-insensitively.
//
// Child order is fixed and puts pPr and rPr near the END, after qFormat.
func heading(level int, outline, size string) string {
	id := fmt.Sprintf("%s%d", styleHeadingPrefix, level)
	before := map[int]string{1: "360", 2: "280", 3: "240"}[level]
	return `<w:style w:type="paragraph" w:styleId="` + id + `">` +
		fmt.Sprintf(`<w:name w:val="heading %d"/>`, level) +
		`<w:basedOn w:val="` + styleNormal + `"/><w:next w:val="` + styleNormal + `"/>` +
		`<w:uiPriority w:val="9"/><w:qFormat/>` +
		`<w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="` + before + `" w:after="120"/>` +
		`<w:outlineLvl w:val="` + outline + `"/></w:pPr>` +
		// b before bCs, sz before szCs — interleaved, not appended.
		`<w:rPr><w:b/><w:bCs/><w:sz w:val="` + size + `"/><w:szCs w:val="` + size + `"/></w:rPr>` +
		`</w:style>`
}
