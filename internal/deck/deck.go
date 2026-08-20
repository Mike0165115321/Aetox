// Package deck reads a slide deck written as HTML.
//
// The contract it implements is docs/architecture/html-deck-2026-08-19.md, and
// this package is where that contract is *authoritative*. A deck is authored as
// HTML because that is the language a model writes best and the one a browser
// already renders; every export then comes off the same file. So there has to be
// exactly one place that decides what counts as a slide, and it is here.
//
// The frontend carries a cheap copy of the marker test (isDeck in
// stores/workbench.svelte.ts) purely to route a freshly-read file into the slide
// pane without a round trip. That copy is a routing hint and nothing else — when
// the two disagree the answer is this package's, and the visible result is a
// pane that opens and reports no slides rather than a wrong export.
//
// Nothing here writes. Turning slides into a .pptx is ooxml's job and it already
// does it: Slides returns []ooxml.Slide, which BuildPPTX has accepted since the
// day it shipped. That is the whole reason the HTML anatomy was specified to map
// one-to-one onto that struct — the move to HTML costs zero new OOXML code, and
// pptx.go does not grow again.
package deck

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	// Registered for the side effect, the same three internal/skill/picture.go
	// pays for: without them image.DecodeConfig cannot measure anything, and an
	// embedded picture with no dimensions is a stretched picture.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/Mike0165115321/Aetox/internal/ooxml"
)

// A picture is embedded whole, so it is also the whole file size. The same
// 20 MB internal/skill/picture.go uses, and for the same reason: past any
// screenshot or chart, short of unmailable.
const maxPictureBytes = 20 << 20

// slideClass is the marker, and it is the only one.
//
// A `<meta name="aetox-deck">` alongside it was drafted and dropped: it would be
// a second place answering one question, and on the day the two disagree nobody
// can say which is right. This class has to exist anyway — the pane pages on it
// and the exporters cut on it — so it identifies the file too.
const slideClass = "slide"

// notesClass marks speaker notes. They are not part of the slide, which is why
// they are an <aside> the deck's own stylesheet hides rather than a hidden div:
// the element says what it is, and a reader that ignores CSS still gets it right.
const notesClass = "notes"

// Is reports whether HTML is a deck rather than an ordinary page, and Count
// says how many slides are in it.
//
// Both parse. A byte-level prefilter was written here first and removed: it had
// to be case-insensitive to be correct, which meant either lowercasing the whole
// file — several megabytes once pictures are embedded — or hand-rolling a folded
// scan. Parsing is what the answer actually depends on, and a deck is small
// enough that the caller can bound it by file size instead (desktop/decks.go).
func Is(source []byte) bool { return Count(source) > 0 }

// Count is Is with the number, for a listing that wants to say "8 สไลด์"
// without paying for the pictures Slides would decode.
func Count(source []byte) int {
	doc, err := html.Parse(bytes.NewReader(source))
	if err != nil {
		return 0
	}
	n := 0
	tag := slideTag(doc)
	walk(doc, func(node *html.Node) bool {
		if isSlide(node, tag) {
			n++
			// A slide inside a slide is not a thing; stop descending so a
			// nested marker cannot inflate the count.
			return false
		}
		return true
	})
	return n
}

// Slides reads every slide out of a deck, in document order.
//
// The anatomy is the contract's table: the first heading is the title, list
// items are bullets, the first picture is the picture, and <aside class="notes">
// is what the presenter sees. Everything else on the slide is design, which is
// the deck's business and not this function's — a slide whose point is a chart
// drawn in CSS reduces to its title here, and says so through Report rather than
// arriving silently empty.
// baseDir is the folder the deck sits in, and it is what lets a picture stored
// beside the deck be loaded. Pass "" when there is no folder to resolve
// against; then only embedded pictures work.
func Slides(source []byte, baseDir, limit string) ([]ooxml.Slide, error) {
	doc, err := html.Parse(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("deck: this file does not parse as HTML: %w", err)
	}

	var out []ooxml.Slide
	tag := slideTag(doc)
	walk(doc, func(node *html.Node) bool {
		if !isSlide(node, tag) {
			return true
		}
		out = append(out, readSlide(node, baseDir, limit))
		return false
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("deck: no slides here. A slide is <section class=%q> (a <div> of that class is read the same way in a file that has no sections), and this file has neither", slideClass)
	}
	return out, nil
}

func readSlide(section *html.Node, baseDir, limit string) ooxml.Slide {
	var slide ooxml.Slide

	walk(section, func(node *html.Node) bool {
		if node == section {
			return true
		}
		// Notes are lifted whole and then not descended into. Descending would
		// let a <ul> the presenter wrote for themselves arrive on the slide as
		// bullets, which is the exact inversion the contract is built to avoid:
		// the sentences belong to the speaker, the landing points to the screen.
		if hasClass(node, notesClass) {
			slide.Notes = text(node)
			return false
		}
		switch node.DataAtom {
		case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
			if slide.Title == "" {
				slide.Title = text(node)
			}
		case atom.Li:
			if line := text(node); line != "" {
				slide.Bullets = append(slide.Bullets, line)
			}
		case atom.Img:
			if slide.Image != nil {
				return true // one picture per slide; ooxml.Slide holds one
			}
			// A picture that cannot be read does not stop the export any more.
			//
			// It used to, on the argument that an export which silently dropped
			// a figure was the failure this codebase keeps writing down. That
			// argument weighed the wrong two things against each other: the
			// alternative to a deck missing one picture was a deck that would
			// not export AT ALL, over one decorative image the owner could see
			// perfectly well on screen. And it is only ever the .pptx and .md
			// that lose it — .pdf and the picture exports go through the
			// renderer, which fetches whatever the page fetches.
			//
			// Owner, after hitting it on a real deck: "เอาเพดานออกดิแม่ง".
			slide.Image, _ = readPicture(attr(node, "src"), attr(node, "alt"), baseDir, limit)
		}
		return true
	})
	return slide
}

// readPicture turns an <img src> into bytes: a data URI decoded, or a file
// stored beside the deck read off disk.
//
// **Reading from disk was added after the rule bit a real deck**, and the
// correction is worth writing down because the original reasoning was not wrong,
// it was misapplied. The contract asks for embedded pictures so that one .html
// file is the whole deck the way one .pptx is. That is about the HTML
// travelling — and an export is not the HTML travelling. A .pptx embeds whatever
// it is handed, so a deck whose chart sits in the folder next to it produces
// exactly as portable a .pptx as one that inlined the same bytes. Refusing there
// enforced the rule in the one place it bought nothing, and stopped an export
// that had every byte it needed sitting on the same disk.
//
// What is still refused is a picture from the network. Reaching out during an
// export is a different act from reading a file the user already has, and not
// one an export button should do quietly.
func readPicture(src, alt, baseDir, limit string) (*ooxml.Picture, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, nil
	}
	if !strings.HasPrefix(strings.ToLower(src), "data:") {
		return readPictureFile(src, alt, baseDir, limit)
	}
	_, payload, found := strings.Cut(src, ",")
	if !found {
		return nil, fmt.Errorf("this picture's data: URI has no comma, so there is nothing after the header to decode")
	}
	// Whitespace is legal inside a base64 data URI and common when a generator
	// wraps long lines; the decoder rejects it, so it goes first.
	payload = strings.Join(strings.Fields(payload), "")
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("this picture's base64 does not decode: %w", err)
	}
	if len(data) > maxPictureBytes {
		return nil, fmt.Errorf("this picture is %d bytes, over the %d limit for an embedded picture", len(data), maxPictureBytes)
	}
	return measurePicture(data, alt)
}

// measurePicture validates the bytes and reads the dimensions off them.
//
// Decoding the header is also the format check: a file announcing PNG and
// carrying something else would otherwise produce a document PowerPoint calls
// damaged, naming no cause anyone could act on. The dimensions come from the
// bytes rather than the markup, because a guessed aspect ratio is a stretched
// picture — the kind of wrong that looks deliberate.
func measurePicture(data []byte, alt string) (*ooxml.Picture, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("this picture is not a readable png, jpeg or gif")
	}
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	return &ooxml.Picture{
		Ext:      ext,
		Data:     data,
		WidthPx:  config.Width,
		HeightPx: config.Height,
		AltText:  strings.TrimSpace(alt),
	}, nil
}

// isSlide reports whether a node is one slide: an element of the document's own
// slide tag, carrying the marker.
//
// The element is checked as well as the class. `class="slide"` on a <div> inside
// a slide is a styling choice somebody is entitled to make, and treating it as a
// slide boundary would cut the deck in the middle of a slide.
func isSlide(node *html.Node, tag atom.Atom) bool {
	return node.Type == html.ElementNode && node.DataAtom == tag && hasClass(node, slideClass)
}

// slideTag decides which element this document draws its slide boundaries with.
//
// `<section class="slide">` is the contract and wins whenever the document has
// one. The fallback to `<div>` exists because that is what presentation
// templates in the wild are written with — the `slides` skill people install
// among them — and a file that renders as a deck in any browser and then opens
// here as source code teaches the reader nothing except that this feature is
// broken. Reading it is free; refusing it buys nothing.
//
// A fallback rather than a second accepted spelling, and that distinction is
// what leaves the rule above intact: in a document that HAS sections, a
// `<div class="slide">` is still somebody's styling choice and still not a
// boundary. Only a document with no sections at all is read the other way, so
// no existing deck changes its answer.
func slideTag(doc *html.Node) atom.Atom {
	sections := false
	walk(doc, func(node *html.Node) bool {
		if isSlide(node, atom.Section) {
			sections = true
			return false
		}
		return true
	})
	if sections {
		return atom.Section
	}
	return atom.Div
}

// hasClass matches on whole class names.
//
// `class="slideshow-wrapper"` contains the letters of the marker and is not a
// slide. A substring test there would drag unrelated pages into the slide pane
// as empty decks, which reads as the feature being broken rather than as the
// page not being a deck.
func hasClass(node *html.Node, want string) bool {
	if node.Type != html.ElementNode {
		return false
	}
	for _, name := range strings.Fields(attr(node, "class")) {
		if strings.EqualFold(name, want) {
			return true
		}
	}
	return false
}

func attr(node *html.Node, name string) string {
	for _, a := range node.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

// text is the readable content of a node, with runs of whitespace collapsed.
//
// Collapsing matters more than it looks: HTML indentation is newlines and tabs,
// and carrying them into a .pptx run puts the author's source formatting on the
// slide. <script> and <style> are skipped because their contents are code that
// happens to be text, and a stylesheet arriving as a bullet is a memorable bug.
func text(node *html.Node) string {
	var b strings.Builder
	walk(node, func(n *html.Node) bool {
		switch n.DataAtom {
		case atom.Script, atom.Style:
			return false
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			// A separator, because <b>ก</b><i>ข</i> and ก ข are different
			// strings and neither should become "กข" by accident of markup.
			b.WriteString(" ")
		}
		return true
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

// walk visits node and its descendants. Returning false from visit stops the
// descent at that node without stopping the walk.
func walk(node *html.Node, visit func(*html.Node) bool) {
	if node == nil {
		return
	}
	if !visit(node) {
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walk(child, visit)
	}
}

// readPictureFile loads a picture stored beside the deck.
//
// The file must live in the deck's own folder or below it. A src of
// ../../../.ssh/id_rsa is not a picture and an export is not a way to read one,
// so the check is on the resolved path rather than on the text — ".." spelled
// any other way is caught the same.
func readPictureFile(src, alt, baseDir, limit string) (*ooxml.Picture, error) {
	lower := strings.ToLower(src)
	for _, scheme := range []string{"http:", "https:", "//"} {
		if strings.HasPrefix(lower, scheme) {
			return nil, fmt.Errorf("this picture comes from the web. Save it next to the deck, " +
				"or embed it as a data: URI, so exporting does not have to go online")
		}
	}
	if baseDir == "" {
		return nil, fmt.Errorf("this picture points at a file, but there is no folder to look in. " +
			"Embed it as a data: URI")
	}

	// The src is a URL path even when it names a local file, so a space arrives
	// as %20 and a Thai filename arrives percent-encoded. Un-escaping before
	// touching the disk is what makes "รูป 1.png" the file it looks like.
	clean := src
	if unescaped, err := url.PathUnescape(src); err == nil {
		clean = unescaped
	}
	if i := strings.IndexAny(clean, "?#"); i >= 0 {
		clean = clean[:i] // a cache-busting ?v=2 is not part of the filename
	}

	full := filepath.Join(baseDir, filepath.FromSlash(clean))
	// The boundary is the project, not the deck's own folder.
	//
	// It was the folder first, and that was too tight to be useful: a deck at
	// output/<session>/deck.html pointing at ../assets/logo.png is an ordinary
	// layout, not an escape attempt. What still has to hold is that an export
	// cannot be turned into a way to read ../../../.ssh/id_rsa — so the check
	// stays, one level out, on the resolved path rather than on the text.
	if limit != "" {
		rel, err := filepath.Rel(limit, full)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("this picture is outside the project")
		}
	}

	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("%s is not next to the deck", clean)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a folder", clean)
	}
	if info.Size() > maxPictureBytes {
		return nil, fmt.Errorf("%s is %d bytes, over the %d limit for an embedded picture", clean, info.Size(), maxPictureBytes)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return measurePicture(data, alt)
}
