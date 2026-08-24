package skill

// A file at a URL, read with the reader that already exists for it.
//
// `web_fetch` checked one thing about a response — "does the content type say
// html" — and handed anything else over as-is. For JSON and plain text that is
// right and always was. For a PDF it meant **the raw bytes of the file went into
// the model's context**: forty thousand characters of binary, paid for in
// tokens, carrying nothing. Same for a .docx, a .xlsx, a .png, a zip.
//
// The readers were all there the whole time. `pdf_read` reads a PDF, `read`
// reads Office files — both want a path on disk, which is the only thing a
// downloaded body did not have. So this saves the body to a temp file and hands
// it to them.
//
// Owner, 24 ส.ค., after the video link turned out to be the same shape:
// *"นี่ขนาดดูเผิน ๆ ผมยังขาดเลย"*. It is the same shape: the question was right
// and the answer was garbage.
//
// **The rule for anything with no reader: name it, never dump it.** A model told
// "this is a 4MB zip" knows what to do next. A model handed four thousand
// mojibake characters has been charged for the privilege of learning nothing.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// webFileMax is the biggest body worth saving and reading. Under webFetchMaxBody
// by construction, so anything that got here already survived that cap.
const webFileMax = 2 << 20

// readableFromBody reports how to read a downloaded body, or "" for the ordinary
// text path.
//
// Content type first, extension second. Servers lie about content type more
// often than URLs lie about their extension — application/octet-stream for a
// PDF is routine — so the extension gets a vote rather than being ignored.
func fetchedFileKind(u *url.URL, contentType string) string {
	ext := strings.ToLower(filepath.Ext(u.Path))
	switch {
	case strings.Contains(contentType, "application/pdf"), ext == ".pdf":
		return ".pdf"
	case strings.Contains(contentType, "wordprocessingml"), ext == ".docx":
		return ".docx"
	case strings.Contains(contentType, "spreadsheetml"), ext == ".xlsx":
		return ".xlsx"
	case strings.Contains(contentType, "presentationml"), ext == ".pptx":
		return ".pptx"
	}
	return ""
}

// binaryKinds are the content types whose bytes are never text, and whose text
// is never worth guessing at. Anything here is described rather than returned.
func looksBinaryType(u *url.URL, contentType, body string) bool {
	switch {
	case strings.HasPrefix(contentType, "image/"),
		strings.HasPrefix(contentType, "audio/"),
		strings.HasPrefix(contentType, "video/"),
		strings.HasPrefix(contentType, "font/"),
		strings.Contains(contentType, "zip"),
		strings.Contains(contentType, "octet-stream"),
		strings.Contains(contentType, "msdownload"):
		return true
	}
	// No content type worth trusting: ask the bytes. The same NUL check `read`
	// uses, and for the same reason — it is the one signal a server cannot get
	// wrong on its behalf.
	return strings.Contains(body, "\x00")
}

// readFetchedFile saves a downloaded body and reads it with the tool that owns
// that kind of file.
//
// The temp file is the whole trick, and it is deleted on the way out: the
// readers take a path because they run converters that take a path, and nothing
// about that needs the file to survive the read.
func readFetchedFile(ctx context.Context, kind string, body []byte, shown string) (string, error) {
	if len(body) > webFileMax {
		return "", fmt.Errorf("%s is %d bytes, too large to read over the wire", shown, len(body))
	}
	dir, err := os.MkdirTemp("", "aetox-webfile-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "download"+kind)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return "", err
	}

	switch kind {
	case ".pdf":
		text, pdfErr := runPdfToText(ctx, path, converterEnv())
		if pdfErr != nil {
			return "", pdfErr
		}
		return text, nil
	case ".docx", ".xlsx", ".pptx":
		// A zero-value readSkill: readOffice takes the resolved path outright
		// and never consults the sandbox root, which is what lets a file that
		// was never in the workspace go through the same reader as one that was.
		return (&readSkill{}).readOffice(path, shown, kind, int64(len(body)))
	}
	return "", fmt.Errorf("no reader for %s", kind)
}

// describeBinary is what a body with no reader gets instead of its bytes.
//
// It names the thing and its size and stops. That is a complete answer to "what
// is at this URL" for a file this app cannot read, and it is the answer that
// costs eight tokens instead of ten thousand.
func describeBinary(u *url.URL, contentType string, size int) string {
	name := filepath.Base(u.Path)
	if name == "" || name == "/" || name == "." {
		name = u.Host
	}
	kind := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	if kind == "" {
		kind = "an unknown type"
	}
	return fmt.Sprintf("URL: %s\n\n%s is %s, %s — not text, so there is nothing to read here.\n"+
		"Its bytes were deliberately not returned: they would be unreadable and would cost the whole reply.\n"+
		"If the user needs this file, say where it is and let them fetch it.",
		u.String(), name, kind, humanBytes(size))
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
