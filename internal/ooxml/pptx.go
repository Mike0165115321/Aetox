package ooxml

import (
	"fmt"
	"strings"
)

// A deck is built out of explicit shapes at explicit coordinates rather than
// out of layout placeholders.
//
// Placeholders are the idiomatic way and the fragile one: a slide inherits
// position, size and text style from a layout, the layout from the master, and
// PowerPoint decides what an incomplete chain means. Getting one link wrong
// produces a file that opens with the text somewhere unintended, or with the
// repair prompt — and neither is diagnosable from a CI machine with no
// PowerPoint on it. A shape carrying its own geometry renders the same
// everywhere, and the master and layout below exist only because the format
// requires them to.
//
// EMU is the unit throughout: 914400 per inch, which is why these numbers look
// arbitrary. Font sizes are hundredths of a point.
const (
	emuPerInch = 914400

	// 16:9 at 13.333 × 7.5 inches — the modern PowerPoint default. A deck built
	// at the old 4:3 size opens letterboxed in every current template.
	slideWidth  = 12192000
	slideHeight = 6858000

	marginX     = 838200
	titleTop    = 457200
	titleHeight = 1143000
	bodyTop     = 1828800
	bodyHeight  = 4114800
	contentW    = slideWidth - 2*marginX
)

// SlideImage is a picture to place on a slide, already read off disk. Width and
// height are the image's own pixels, used only to keep its aspect ratio — the
// slide size is decided here, not by the file.
type SlideImage struct {
	// Ext is the file extension without the dot ("png", "jpeg"), which becomes
	// both the part name and the content-type Default entry.
	Ext      string
	Data     []byte
	WidthPx  int
	HeightPx int
	AltText  string
}

// Slide is one slide's worth of content. Every field is optional: a slide with
// only a title is a section divider, and one with only an image is a full-bleed
// picture.
type Slide struct {
	Title   string
	Bullets []string
	Notes   string
	Image   *SlideImage
}

// BuildPPTX turns slides into the parts of a .pptx package.
//
// The part list is longer than xlsx's by a lot, and none of it is optional:
// PowerPoint refuses a presentation with no master, a master with no layout, or
// a master that references a theme that is not there.
func BuildPPTX(slides []Slide) ([]Part, error) {
	if len(slides) == 0 {
		return nil, fmt.Errorf("ooxml: presentation needs at least one slide")
	}

	hasNotes := false
	for _, s := range slides {
		if strings.TrimSpace(s.Notes) != "" {
			hasNotes = true
			break
		}
	}

	var types, presentation, presRels strings.Builder
	types.WriteString(xmlHeader)
	types.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	types.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	types.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	// One Default per image extension actually used. A Default for an extension
	// with no part is legal; a part with no Default is not.
	for _, ext := range imageExtensions(slides) {
		fmt.Fprintf(&types, `<Default Extension="%s" ContentType="image/%s"/>`, ext, contentTypeExt(ext))
	}
	types.WriteString(`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`)
	types.WriteString(`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>`)
	types.WriteString(`<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>`)
	types.WriteString(`<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>`)

	presRels.WriteString(xmlHeader)
	presRels.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	presRels.WriteString(`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>`)
	presRels.WriteString(`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>`)

	parts := []Part{
		{Name: "_rels/.rels", Data: []byte(pptRootRels)},
		{Name: "ppt/slideMasters/slideMaster1.xml", Data: []byte(slideMasterXML)},
		{Name: "ppt/slideMasters/_rels/slideMaster1.xml.rels", Data: []byte(slideMasterRels)},
		{Name: "ppt/slideLayouts/slideLayout1.xml", Data: []byte(slideLayoutXML)},
		{Name: "ppt/slideLayouts/_rels/slideLayout1.xml.rels", Data: []byte(slideLayoutRels)},
		{Name: "ppt/theme/theme1.xml", Data: []byte(themeXML)},
	}

	// Relationship ids on the presentation: 1 is the master, 2 the theme, 3 the
	// notes master when there is one, and the slides take everything after.
	nextRel := 3
	if hasNotes {
		types.WriteString(`<Override PartName="/ppt/notesMasters/notesMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesMaster+xml"/>`)
		// theme2 is the notes master's own copy of the theme — see the
		// notesMasterRels comment for why sharing theme1 corrupts the file.
		types.WriteString(`<Override PartName="/ppt/theme/theme2.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>`)
		fmt.Fprintf(&presRels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesMaster" Target="notesMasters/notesMaster1.xml"/>`, nextRel)
		parts = append(parts,
			Part{Name: "ppt/notesMasters/notesMaster1.xml", Data: []byte(notesMasterXML)},
			Part{Name: "ppt/notesMasters/_rels/notesMaster1.xml.rels", Data: []byte(notesMasterRels)},
			Part{Name: "ppt/theme/theme2.xml", Data: []byte(themeXML)},
		)
		nextRel++
	}

	var slideIDs strings.Builder
	imageSeq := 0
	for i, slide := range slides {
		n := i + 1
		slideRelID := nextRel + i
		// Slide ids must be ≥ 256; PowerPoint rejects the file otherwise, and
		// the number is otherwise meaningless.
		fmt.Fprintf(&slideIDs, `<p:sldId id="%d" r:id="rId%d"/>`, 255+n, slideRelID)
		fmt.Fprintf(&presRels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>`, slideRelID, n)
		fmt.Fprintf(&types, `<Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, n)

		var rels strings.Builder
		rels.WriteString(xmlHeader)
		rels.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
		rels.WriteString(`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`)

		imageRel := ""
		if slide.Image != nil && len(slide.Image.Data) > 0 {
			imageSeq++
			media := fmt.Sprintf("image%d.%s", imageSeq, slide.Image.Ext)
			imageRel = "rId2"
			fmt.Fprintf(&rels, `<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/%s"/>`, media)
			parts = append(parts, Part{Name: "ppt/media/" + media, Data: slide.Image.Data})
		}

		if strings.TrimSpace(slide.Notes) != "" {
			notesRel := "rId3"
			if imageRel == "" {
				notesRel = "rId2"
			}
			fmt.Fprintf(&rels, `<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide%d.xml"/>`, notesRel, n)
			fmt.Fprintf(&types, `<Override PartName="/ppt/notesSlides/notesSlide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"/>`, n)
			parts = append(parts,
				Part{Name: fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", n), Data: []byte(notesSlideXML(slide.Notes))},
				Part{Name: fmt.Sprintf("ppt/notesSlides/_rels/notesSlide%d.xml.rels", n), Data: []byte(notesSlideRels(n))},
			)
		}
		rels.WriteString(`</Relationships>`)

		parts = append(parts,
			Part{Name: fmt.Sprintf("ppt/slides/slide%d.xml", n), Data: []byte(slideXML(slide, imageRel))},
			Part{Name: fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", n), Data: []byte(rels.String())},
		)
	}

	presentation.WriteString(xmlHeader)
	presentation.WriteString(`<p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">`)
	presentation.WriteString(`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`)
	if hasNotes {
		presentation.WriteString(`<p:notesMasterIdLst><p:notesMasterId r:id="rId3"/></p:notesMasterIdLst>`)
	}
	presentation.WriteString(`<p:sldIdLst>` + slideIDs.String() + `</p:sldIdLst>`)
	fmt.Fprintf(&presentation, `<p:sldSz cx="%d" cy="%d"/><p:notesSz cx="%d" cy="%d"/>`, slideWidth, slideHeight, slideHeight, slideWidth)
	presentation.WriteString(`</p:presentation>`)

	types.WriteString(`</Types>`)
	presRels.WriteString(`</Relationships>`)

	head := []Part{
		{Name: "[Content_Types].xml", Data: []byte(types.String())},
		{Name: "ppt/presentation.xml", Data: []byte(presentation.String())},
		{Name: "ppt/_rels/presentation.xml.rels", Data: []byte(presRels.String())},
	}
	// [Content_Types].xml first (WritePackage enforces it); _rels/.rels was
	// appended before the manifest existed, so the fixed head goes in front.
	return append(head, parts...), nil
}

func imageExtensions(slides []Slide) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range slides {
		if s.Image == nil || len(s.Image.Data) == 0 || s.Image.Ext == "" {
			continue
		}
		if !seen[s.Image.Ext] {
			seen[s.Image.Ext] = true
			out = append(out, s.Image.Ext)
		}
	}
	return out
}

// contentTypeExt maps a file extension to the image/* subtype. Only "jpg"
// differs from its own name, and a Default entry naming image/jpg rather than
// image/jpeg is the kind of thing PowerPoint accepts and Google Slides does not.
func contentTypeExt(ext string) string {
	if ext == "jpg" {
		return "jpeg"
	}
	return ext
}

// slideXML lays out one slide.
//
// Text goes into plain text boxes, positioned here. See the file header for why
// this does not use layout placeholders.
func slideXML(slide Slide, imageRel string) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">`)
	b.WriteString(`<p:cSld><p:spTree>`)
	b.WriteString(`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>`)
	b.WriteString(`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`)

	shapeID := 2
	hasImage := imageRel != ""

	// Bullets give up the right half of the slide to a picture; a picture with
	// no bullets gets the whole content area.
	bodyW := contentW
	if hasImage && len(slide.Bullets) > 0 {
		bodyW = (contentW - 457200) / 2
	}

	if title := strings.TrimSpace(slide.Title); title != "" {
		b.WriteString(textBox(shapeID, "Title", marginX, titleTop, contentW, titleHeight,
			[]string{title}, 4000, true, false))
		shapeID++
	}
	if len(slide.Bullets) > 0 {
		b.WriteString(textBox(shapeID, "Body", marginX, bodyTop, bodyW, bodyHeight,
			slide.Bullets, 2000, false, true))
		shapeID++
	}
	if hasImage {
		x, y, w, h := imageFrame(slide.Image, len(slide.Bullets) > 0, bodyW)
		b.WriteString(picture(shapeID, imageRel, slide.Image.AltText, x, y, w, h))
		shapeID++
	}

	b.WriteString(`</p:spTree></p:cSld><p:clrMapOvr><a:overrideClrMapping bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/></p:clrMapOvr></p:sld>`)
	return b.String()
}

// imageFrame places the picture and keeps its aspect ratio. An image stretched
// to fill a box is worse than a smaller one, and a chart squashed 20% is
// misleading rather than merely ugly.
func imageFrame(img *SlideImage, besideText bool, bodyW int) (x, y, w, h int) {
	boxX, boxY, boxW, boxH := marginX, bodyTop, contentW, bodyHeight
	if besideText {
		boxX = marginX + bodyW + 457200
		boxW = contentW - bodyW - 457200
	}
	w, h = boxW, boxH
	if img.WidthPx > 0 && img.HeightPx > 0 {
		// Scale to the tighter of the two constraints, then centre in the box.
		byWidth := float64(boxW) / float64(img.WidthPx)
		byHeight := float64(boxH) / float64(img.HeightPx)
		scale := byWidth
		if byHeight < scale {
			scale = byHeight
		}
		w = int(float64(img.WidthPx) * scale)
		h = int(float64(img.HeightPx) * scale)
	}
	return boxX + (boxW-w)/2, boxY + (boxH-h)/2, w, h
}

func picture(id int, relID, alt string, x, y, w, h int) string {
	var b strings.Builder
	b.WriteString(`<p:pic><p:nvPicPr>`)
	fmt.Fprintf(&b, `<p:cNvPr id="%d" name="Picture %d" descr="%s"/>`, id, id, escapeXML(alt))
	b.WriteString(`<p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr/></p:nvPicPr>`)
	fmt.Fprintf(&b, `<p:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>`, relID)
	fmt.Fprintf(&b, `<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>`, x, y, w, h)
	b.WriteString(`</p:pic>`)
	return b.String()
}

// textBox writes one shape holding paragraphs.
//
// Each run names three typefaces, and the third is the one that matters here.
// Thai is a complex script: without `cs`, PowerPoint picks a complex-script font
// on its own, and the one it picks routinely mismatches the Latin font's
// metrics — which is what makes Thai vowels and tone marks drift off the
// consonants they belong to. `+mn-cs` points at the theme's minor complex-script
// font, which theme1.xml sets for the Thai script explicitly.
func textBox(id int, name string, x, y, w, h int, lines []string, size int, bold, bullets bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s %d"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>`, id, name, id)
	fmt.Fprintf(&b, `<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr>`, x, y, w, h)
	// normAutofit shrinks text that overruns the box instead of letting it spill
	// off the slide — a model given a long bullet list is the normal case.
	b.WriteString(`<p:txBody><a:bodyPr wrap="square" rtlCol="0"><a:normAutofit/></a:bodyPr><a:lstStyle/>`)

	boldAttr := ""
	if bold {
		boldAttr = ` b="1"`
	}
	for _, line := range lines {
		if bullets {
			fmt.Fprintf(&b, `<a:p><a:pPr marL="285750" indent="-285750"><a:buFont typeface="Arial"/><a:buChar char="•"/></a:pPr>`)
		} else {
			b.WriteString(`<a:p>`)
		}
		fmt.Fprintf(&b,
			`<a:r><a:rPr lang="th-TH" sz="%d"%s dirty="0"><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:rPr><a:t>%s</a:t></a:r></a:p>`,
			size, boldAttr, escapeXML(line))
	}
	b.WriteString(`</p:txBody></p:sp>`)
	return b.String()
}

func notesSlideXML(notes string) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<p:notes xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">`)
	b.WriteString(`<p:cSld><p:spTree>`)
	b.WriteString(`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>`)
	b.WriteString(`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`)
	// This one *is* a placeholder: the notes pane finds the text by its `body`
	// placeholder type, so a plain text box here would store the note and show
	// the presenter nothing.
	b.WriteString(`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>`)
	b.WriteString(`<p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>`)
	for _, line := range strings.Split(notes, "\n") {
		fmt.Fprintf(&b, `<a:p><a:r><a:rPr lang="th-TH" dirty="0"><a:cs typeface="+mn-cs"/></a:rPr><a:t>%s</a:t></a:r></a:p>`, escapeXML(line))
	}
	b.WriteString(`</p:txBody></p:sp></p:spTree></p:cSld></p:notes>`)
	return b.String()
}

func notesSlideRels(n int) string {
	return xmlHeader +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesMaster" Target="../notesMasters/notesMaster1.xml"/>` +
		fmt.Sprintf(`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="../slides/slide%d.xml"/>`, n) +
		`</Relationships>`
}

const pptRootRels = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>` +
	`</Relationships>`

const slideMasterRels = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="../theme/theme1.xml"/>` +
	`</Relationships>`

const slideLayoutRels = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/>` +
	`</Relationships>`

// The notes master points at ITS OWN theme part — theme2.xml, byte-identical
// to theme1. Sharing theme1 with the slide master looks legal on paper (OPC
// allows two relationships to one part) and is exactly what shipped: every
// deck with speaker notes opened to "PowerPoint พบปัญหากับเนื้อหา" and a
// repair prompt. PowerPoint requires each master to own its theme; bisected
// with a live PowerPoint via COM, 2026-08-04 — giving the notes master its
// own copy was the single change that turned repair-prompt into opens-clean.
const notesMasterRels = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="../theme/theme2.xml"/>` +
	`</Relationships>`

// The master and layout are as close to empty as the format allows: they carry
// no shapes, because every slide brings its own. `clrMap` is not optional —
// without it PowerPoint cannot resolve a single colour reference and reports
// the file as damaged.
const slideMasterXML = xmlHeader +
	`<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
	`<p:cSld><p:bg><p:bgPr><a:solidFill><a:schemeClr val="bg1"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>` +
	`<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
	`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst>` +
	`<p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles>` +
	`</p:sldMaster>`

const slideLayoutXML = xmlHeader +
	`<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" type="blank" preserve="1">` +
	`<p:cSld name="Blank"><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
	`</p:sldLayout>`

const notesMasterXML = xmlHeader +
	`<p:notesMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
	`<p:cSld><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes Placeholder"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr><a:xfrm><a:off x="685800" y="4400550"/><a:ext cx="5486400" cy="4114800"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>` +
	`<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="th-TH"/></a:p></p:txBody></p:sp>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
	`<p:notesStyle/>` +
	`</p:notesMaster>`

// themeXML is the smallest theme PowerPoint accepts. The colour, font and
// format schemes are all required — a theme missing its formatScheme is
// rejected outright, even though nothing here references a format.
//
// The two `<a:font script="Thai" .../>` entries are the deliberate part. That
// element is the format's own mechanism for "use this face for this script",
// and it is what stops PowerPoint choosing a complex-script font by itself and
// leaving Thai vowels floating beside their consonants instead of over them.
// Leelawadee UI ships with Windows; a machine without it falls back the normal
// way rather than failing.
const themeXML = xmlHeader +
	`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Aetox">` +
	`<a:themeElements>` +
	`<a:clrScheme name="Aetox">` +
	`<a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1>` +
	`<a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>` +
	`<a:dk2><a:srgbClr val="1F2430"/></a:dk2>` +
	`<a:lt2><a:srgbClr val="F2F4F7"/></a:lt2>` +
	`<a:accent1><a:srgbClr val="2F6FED"/></a:accent1>` +
	`<a:accent2><a:srgbClr val="16A34A"/></a:accent2>` +
	`<a:accent3><a:srgbClr val="F59E0B"/></a:accent3>` +
	`<a:accent4><a:srgbClr val="DC2626"/></a:accent4>` +
	`<a:accent5><a:srgbClr val="7C3AED"/></a:accent5>` +
	`<a:accent6><a:srgbClr val="0891B2"/></a:accent6>` +
	`<a:hlink><a:srgbClr val="2F6FED"/></a:hlink>` +
	`<a:folHlink><a:srgbClr val="7C3AED"/></a:folHlink>` +
	`</a:clrScheme>` +
	`<a:fontScheme name="Aetox">` +
	`<a:majorFont><a:latin typeface="Calibri Light"/><a:ea typeface=""/><a:cs typeface="Leelawadee UI"/>` +
	`<a:font script="Thai" typeface="Leelawadee UI"/></a:majorFont>` +
	`<a:minorFont><a:latin typeface="Calibri"/><a:ea typeface=""/><a:cs typeface="Leelawadee UI"/>` +
	`<a:font script="Thai" typeface="Leelawadee UI"/></a:minorFont>` +
	`</a:fontScheme>` +
	`<a:fmtScheme name="Aetox">` +
	`<a:fillStyleLst>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`</a:fillStyleLst>` +
	`<a:lnStyleLst>` +
	`<a:ln w="6350"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln>` +
	`<a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln>` +
	`<a:ln w="19050"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln>` +
	`</a:lnStyleLst>` +
	`<a:effectStyleLst>` +
	`<a:effectStyle><a:effectLst/></a:effectStyle>` +
	`<a:effectStyle><a:effectLst/></a:effectStyle>` +
	`<a:effectStyle><a:effectLst/></a:effectStyle>` +
	`</a:effectStyleLst>` +
	`<a:bgFillStyleLst>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
	`</a:bgFillStyleLst>` +
	`</a:fmtScheme>` +
	`</a:themeElements>` +
	`</a:theme>`
