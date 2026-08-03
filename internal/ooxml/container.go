// Package ooxml assembles Office documents — .xlsx today, .pptx and .docx on
// the same container later.
//
// Every OOXML file is a ZIP whose entries ("parts") are XML documents plus a
// manifest naming their content types, so one container writer is the whole
// shared foundation and each format is a different set of parts on top of it.
// That is why this is written by hand against archive/zip and encoding/xml
// rather than pulled from a library: the two Go libraries that cover all three
// formats (unioffice, gooxml) are AGPLv3, which Aetox cannot take on top of
// Apache-2.0, and the one permissive option (excelize, BSD-3) covers only xlsx
// while adding 5.5 MB to a 15.8 MB binary — measured 2026-08-03, against a
// 3 MB budget — for formulas and charts that OFFICE-EXPORT-PLAN.md §8 has
// already ruled out of scope. Writing the container once buys all three
// formats and no new dependency at all.
package ooxml

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Part is one entry inside the package. Name is the path within the ZIP, always
// forward-slashed and never leading-slashed, exactly as the OPC spec numbers
// part names minus the root slash ("xl/workbook.xml", not "/xl/workbook.xml").
type Part struct {
	Name string
	Data []byte
}

// xmlHeader prefixes every XML part. standalone="yes" is what Office itself
// emits; some strict readers reject the file without the declaration entirely.
const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// WritePackage serialises parts into an OOXML package.
//
// Ordering matters more than it looks: readers are allowed to stream the ZIP,
// and both Excel and PowerPoint expect the content-type manifest first, so the
// caller's slice order is preserved rather than sorted. Deflate is used for
// every part — XML compresses roughly ten to one, and a stored-only package is
// large enough that mail servers notice.
func WritePackage(parts []Part) ([]byte, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("ooxml: package has no parts")
	}
	if parts[0].Name != "[Content_Types].xml" {
		return nil, fmt.Errorf("ooxml: first part must be [Content_Types].xml, got %q", parts[0].Name)
	}

	seen := make(map[string]bool, len(parts))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, part := range parts {
		name := strings.TrimPrefix(filepath.ToSlash(part.Name), "/")
		if name == "" {
			return nil, fmt.Errorf("ooxml: part with empty name")
		}
		// A duplicate part name produces a ZIP that unzips fine and that Office
		// then refuses with its uninformative "unreadable content" prompt, so it
		// is worth catching here where the message can name the part.
		if seen[name] {
			return nil, fmt.Errorf("ooxml: duplicate part %q", name)
		}
		seen[name] = true

		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return nil, fmt.Errorf("ooxml: create %s: %w", name, err)
		}
		if _, err := w.Write(part.Data); err != nil {
			return nil, fmt.Errorf("ooxml: write %s: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("ooxml: close package: %w", err)
	}
	return buf.Bytes(), nil
}

// WriteFile writes a package to disk atomically-ish: a half-written .xlsx is
// not a smaller spreadsheet, it is a file Excel refuses to open, and the user
// finding that out is worse than the write having failed loudly. The whole
// package is built in memory first (these are kilobytes), then renamed into
// place so the destination either has the old file or the complete new one.
func WriteFile(path string, parts []Part) error {
	data, err := WritePackage(parts)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".aetox-ooxml-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	// Windows will not rename onto an existing file, and the whole point of
	// this tool is regenerating the same report over yesterday's copy.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// escapeXML writes text as XML character data.
//
// encoding/xml's EscapeText is not used directly because it turns \n into
// &#xA; and, more importantly, happily passes through control characters that
// XML 1.0 forbids outright. Those arrive in practice: OCR of a scanned slip
// yields stray 0x01s, and a single one is enough for Excel to declare the whole
// workbook unreadable. They are dropped rather than escaped — there is no
// escape that makes them legal.
func escapeXML(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\t', '\n', '\r':
			b.WriteRune(r)
		default:
			if r < 0x20 || r == 0xFFFE || r == 0xFFFF {
				continue
			}
			// The unpaired-surrogate range cannot appear in valid UTF-8, but
			// decoding invalid bytes yields U+FFFD, which is legal — so the only
			// case left to drop is an explicit surrogate code point.
			if r >= 0xD800 && r <= 0xDFFF {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
