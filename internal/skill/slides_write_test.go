package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readPart(t *testing.T, path, part string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("result is not a readable package: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != part {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	t.Fatalf("package has no part %s", part)
	return ""
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSlidesWriteProducesADeckPowerPointCanRead(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "chart.png"), 800, 600)
	s := &slidesWriteSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "นำเสนอ.pptx",
		"slides": [
			{"title": "สรุปยอดขายไตรมาส ๓", "bullets": ["โตขึ้น ๑๒%", "ลูกค้าใหม่ ๔๘ ราย"], "notes": "เน้นว่ากำไรมาจากลูกค้าเก่า"},
			{"title": "แนวโน้ม", "image": "chart.png"}
		]
	}`))
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false: %s", out.Stderr)
	}

	deck := filepath.Join(root, "นำเสนอ.pptx")
	slide1 := readPart(t, deck, "ppt/slides/slide1.xml")
	if !strings.Contains(slide1, "สรุปยอดขายไตรมาส ๓") {
		t.Errorf("Thai title did not survive:\n%s", slide1)
	}
	if !strings.Contains(slide1, `<a:cs typeface="+mn-cs"/>`) {
		t.Error("no complex-script typeface — Thai vowels will drift off their consonants")
	}
	if notes := readPart(t, deck, "ppt/notesSlides/notesSlide1.xml"); !strings.Contains(notes, "เน้นว่ากำไรมาจากลูกค้าเก่า") {
		t.Error("speaker notes did not survive")
	}
	if img := readPart(t, deck, "ppt/media/image1.png"); len(img) == 0 {
		t.Error("the picture was not embedded, so the deck is not self-contained")
	}
	if !strings.Contains(out.Content, "2 slide(s)") {
		t.Errorf("receipt does not report the slide count: %q", out.Content)
	}
	if len(out.Artifacts) != 1 || out.Artifacts[0] != "นำเสนอ.pptx" {
		t.Errorf("Artifacts = %v, want the deck's path", out.Artifacts)
	}
}

func TestSlidesWriteForcesThePptxExtension(t *testing.T) {
	root := t.TempDir()
	s := &slidesWriteSkill{root: root}

	for _, given := range []string{"deck", "deck2.md", "deck3.ppt"} {
		args := callArgs(t, `{"slides":[{"title":"x"}]}`)
		args["path"] = given
		if _, err := s.ExecuteTool(context.Background(), args); err != nil {
			t.Fatalf("%s: %v", given, err)
		}
		want := strings.TrimSuffix(given, filepath.Ext(given)) + ".pptx"
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Errorf("%s did not land at %s: %v", given, want, err)
		}
	}
}

func TestSlidesWriteStaysInsideTheSandbox(t *testing.T) {
	root := t.TempDir()
	s := &slidesWriteSkill{root: root}

	for _, escape := range []string{"../escaped.pptx", "a/../../escaped.pptx"} {
		args := callArgs(t, `{"slides":[{"title":"x"}]}`)
		args["path"] = escape
		if _, err := s.ExecuteTool(context.Background(), args); err == nil {
			t.Errorf("%s was allowed out of the sandbox", escape)
		}
	}
	// An image path is the second way out, and it is the easier one to forget:
	// the destination is checked, so a picture read from anywhere on disk would
	// be embedded into a file the user can then send to somebody.
	args := callArgs(t, `{"path":"d.pptx","slides":[{"title":"x","image":"../../../secret.png"}]}`)
	if _, err := s.ExecuteTool(context.Background(), args); err == nil {
		t.Error("an image outside the sandbox was embedded")
	}
}

// A deck that silently dropped the chart it was asked for is worse than one
// that was never written — the user sends it before noticing.
func TestSlidesWriteFailsLoudlyOnABadImage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fake.png"), []byte("this is not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &slidesWriteSkill{root: root}

	cases := map[string]string{
		"missing image":    `{"path":"a.pptx","slides":[{"title":"x","image":"nope.png"}]}`,
		"not really a png": `{"path":"b.pptx","slides":[{"title":"x","image":"fake.png"}]}`,
	}
	for name, body := range cases {
		if _, err := s.ExecuteTool(context.Background(), callArgs(t, body)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	for _, unwanted := range []string{"a.pptx", "b.pptx"} {
		if _, err := os.Stat(filepath.Join(root, unwanted)); err == nil {
			t.Errorf("a rejected call still wrote %s", unwanted)
		}
	}
}

// The same output-folder rule every file-consuming skill follows: a picture an
// earlier tool wrote is found by the name the model remembers.
func TestSlidesWriteFindsAnImageInTheSessionOutputFolder(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "session-1", "chart.png"), 400, 300)
	s := &slidesWriteSkill{root: root, outputSubdir: func() string { return "session-1" }}

	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "deck.pptx",
		"slides": [{"title": "x", "image": "chart.png"}]
	}`))
	if err != nil {
		t.Fatalf("picture in the session folder was not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "session-1", "deck.pptx")); err != nil {
		t.Fatalf("deck did not land in the session folder: %v", err)
	}
	if !strings.Contains(out.Content, "on disk:") {
		t.Errorf("moved file did not report its real location: %q", out.Content)
	}
}

func TestSlidesWriteRejectsWhatItCannotRead(t *testing.T) {
	root := t.TempDir()
	s := &slidesWriteSkill{root: root}

	cases := map[string]string{
		"no path":      `{"slides":[{"title":"x"}]}`,
		"no slides":    `{"path":"a.pptx"}`,
		"empty slides": `{"path":"a.pptx","slides":[]}`,
		"empty slide":  `{"path":"a.pptx","slides":[{}]}`,
		"slide number": `{"path":"a.pptx","slides":[42]}`,
	}
	for name, body := range cases {
		if _, err := s.ExecuteTool(context.Background(), callArgs(t, body)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	// A slide sent as a bare string is the one loose shape with a single
	// sensible reading, so it is read rather than rejected.
	if _, err := s.ExecuteTool(context.Background(), callArgs(t, `{"path":"ok.pptx","slides":["หัวข้อเดียว"]}`)); err != nil {
		t.Errorf("a bare-string slide was rejected: %v", err)
	}
	if !strings.Contains(readPart(t, filepath.Join(root, "ok.pptx"), "ppt/slides/slide1.xml"), "หัวข้อเดียว") {
		t.Error("a bare-string slide lost its title")
	}
}

func TestSlidesWriteIsRegisteredAsATool(t *testing.T) {
	r := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	s, ok := r.Get("slides_write")
	if !ok {
		t.Fatal("slides_write is not in the default registry, so the model never sees it")
	}
	tool, ok := s.(Tool)
	if !ok {
		t.Fatal("slides_write is not a Tool, so it has no definition to send")
	}
	if name := tool.ToolDefinition().Function.Name; name != "slides_write" {
		t.Errorf("tool name = %q", name)
	}
}
