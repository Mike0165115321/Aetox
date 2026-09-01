package skill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A tiny real PNG (1x1, transparent), because the verifier judges bytes and
// the test should hand it real ones.
var testPNG = []byte{
	0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
	0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
	0, 0, 0, 13, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x62, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
	0x0d, 0x0a, 0x2d, 0xb4,
	0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

// The download step judges what it got by the bytes, saves what is real, and
// refuses the ordinary lie — an HTML page in place of a file.
func TestMediaFetchSavesRealMediaAndRefusesPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/logo.png":
			// Content type lies on purpose: the sniffer must not care.
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(testPNG)
		case "/hero.jpg":
			// The hotlink-guard shape: 200 OK, and the body is a login page.
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("<!DOCTYPE html><html><head><title>Sign in</title></head></html>"))
		case "/track.mp3":
			_, _ = w.Write(append([]byte("ID3"), make([]byte, 64)...))
		case "/empty":
			// 200 with nothing in it.
		}
	}))
	defer server.Close()

	root := t.TempDir()
	s := &mediaFetchSkill{root: root, httpClient: server.Client()}

	out, err := s.fetch(context.Background(), server.URL+"/logo.png", "assets/logo.png")
	if err != nil || !out.Success {
		t.Fatalf("fetching a real png failed: %v (%s)", err, out.Content)
	}
	for _, want := range []string{"png", "1x1"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("the receipt does not say %q:\n%s", want, out.Content)
		}
	}
	saved, readErr := os.ReadFile(filepath.Join(root, "assets", "logo.png"))
	if readErr != nil || string(saved) != string(testPNG) {
		t.Errorf("the saved file is not the body that was downloaded: %v", readErr)
	}

	// A sound file is a sound file even when ffprobe is not around to time it.
	out, err = s.fetch(context.Background(), server.URL+"/track.mp3", "assets/track.mp3")
	if err != nil || !strings.Contains(out.Content, "mp3") {
		t.Errorf("fetching an mp3 = %v:\n%s", err, out.Content)
	}

	// The page wearing a .jpg name is refused, with the page named as the
	// problem — and nothing lands on disk.
	_, err = s.fetch(context.Background(), server.URL+"/hero.jpg", "assets/hero.jpg")
	if err == nil || !strings.Contains(err.Error(), "หน้าเว็บ") {
		t.Errorf("an HTML body was not refused as a page: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "assets", "hero.jpg")); statErr == nil {
		t.Error("the refused page was saved anyway")
	}

	_, err = s.fetch(context.Background(), server.URL+"/empty", "assets/x.png")
	if err == nil {
		t.Error("an empty body was accepted")
	}
}

// A name that disagrees with its own bytes is saved (the caller chose the
// name) but the receipt says so, because the mismatch ruins an export later.
func TestMediaFetchNamesAnExtensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testPNG)
	}))
	defer server.Close()

	s := &mediaFetchSkill{root: t.TempDir(), httpClient: server.Client()}
	out, err := s.fetch(context.Background(), server.URL+"/x", "assets/photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "ไม่ตรงกับเนื้อไฟล์") {
		t.Errorf("a png saved as .jpg went unremarked:\n%s", out.Content)
	}
}

// The sniffer, straight: every accepted kind by its magic, and the boundary.
func TestSniffMediaKind(t *testing.T) {
	cases := map[string]string{
		"jpg":  "\xFF\xD8\xFF\xE0rest",
		"gif":  "GIF89a....",
		"webp": "RIFF\x00\x00\x00\x00WEBPVP8 ",
		"wav":  "RIFF\x00\x00\x00\x00WAVEfmt ",
		"ogg":  "OggS\x00\x02....",
		"flac": "fLaC\x00\x00\x00\x22",
		"mp3":  "\xFF\xFB\x90\x00....",
		"svg":  `<svg xmlns="http://www.w3.org/2000/svg"></svg>`,
		"":     "PK\x03\x04 a zip is not media",
	}
	for want, body := range cases {
		if got := sniffMediaKind([]byte(body)); got != want {
			t.Errorf("sniffMediaKind(%q...) = %q, want %q", body[:4], got, want)
		}
	}
	if got := sniffMediaKind(testPNG); got != "png" {
		t.Errorf("sniffMediaKind(png) = %q", got)
	}
}
