package ooxml

// A picture, and the two facts every format in this package needs about one.
//
// It lives here rather than in pptx.go because a picture is not a slide's idea.
// A deck puts one on a slide, a document puts one between two paragraphs, and
// both need the same thing off disk: the bytes, the extension, and the pixel
// dimensions that fix the aspect ratio. Two copies of that struct is two places
// for "jpg vs jpeg" to be decided differently, which is the exact bug the
// comment on contentTypeExt below was written about.

// Picture is an image already read off disk. WidthPx and HeightPx are the
// image's own pixels, used only to keep its aspect ratio — how big it is drawn
// is each format's decision, never the file's.
type Picture struct {
	// Ext is the file extension without the dot ("png", "jpg", "gif"), which
	// becomes both the part name and the content-type Default entry.
	Ext      string
	Data     []byte
	WidthPx  int
	HeightPx int
	AltText  string
}

// emuPerPixel is 914400/96: the EMU width of one pixel at the 96 DPI both Word
// and PowerPoint assume for an image that carries no resolution of its own.
// A screenshot's pixels are therefore its natural size on the page, which is
// what makes "the picture is the size it looks" true without anybody sending a
// dimension.
const emuPerPixel = 9525

// contentTypeExt maps a file extension to the image/* subtype. Only "jpg"
// differs from its own name, and a Default entry naming image/jpg rather than
// image/jpeg is the kind of thing PowerPoint accepts and Google Slides does not.
func contentTypeExt(ext string) string {
	if ext == "jpg" {
		return "jpeg"
	}
	return ext
}
