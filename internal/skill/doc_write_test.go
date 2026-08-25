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

	"github.com/Mikedev115/Aetox/internal/ooxml"
)

// readPart pulls one part out of an OOXML package, which is a ZIP. It moved
// here from slides_write_test.go when that tool was retired (the deck is HTML
// now, and its .pptx comes from the exporter); this is the only file left that
// reads inside a produced document.
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

func TestDocWriteProducesADocumentWordCanRead(t *testing.T) {
	root := t.TempDir()
	s := &docWriteSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "บันทึก.docx",
		"blocks": [
			{"type": "heading", "text": "สรุปยอดขายไตรมาส ๓", "level": 1},
			{"type": "paragraph", "text": "เรียน ผู้จัดการฝ่ายขาย"},
			{"type": "bullets", "items": ["ยอดโตขึ้น ๑๒%", "ลูกค้าใหม่ ๔๘ ราย"]},
			{"type": "numbered", "items": ["ต่อสัญญาขนส่ง", "ทำแคมเปญลูกค้าเก่า"]},
			{"type": "table", "columns": ["หมวด", "ยอด"], "rows": [["อาหาร", "95"], ["ขนส่ง", "320.75"]]}
		]
	}`))
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false: %s", out.Stderr)
	}

	body := readPart(t, filepath.Join(root, "บันทึก.docx"), "word/document.xml")
	for _, want := range []string{"สรุปยอดขายไตรมาส ๓", "เรียน ผู้จัดการฝ่ายขาย", "ยอดโตขึ้น ๑๒%", "ต่อสัญญาขนส่ง", "อาหาร"} {
		if !strings.Contains(body, want) {
			t.Errorf("document is missing %q", want)
		}
	}
	if !strings.Contains(body, `<w:pStyle w:val="Heading1"/>`) {
		t.Error("the heading did not become a real heading")
	}
	if !strings.Contains(body, "<w:tbl>") {
		t.Error("the table did not become a table")
	}
	if !strings.Contains(out.Content, "5 block(s)") {
		t.Errorf("receipt does not report the block count: %q", out.Content)
	}
	if len(out.Artifacts) != 1 || out.Artifacts[0] != "บันทึก.docx" {
		t.Errorf("Artifacts = %v, want the document's path", out.Artifacts)
	}
}

func TestDocWriteForcesTheDocxExtension(t *testing.T) {
	root := t.TempDir()
	s := &docWriteSkill{root: root}

	for _, given := range []string{"memo", "memo2.md", "memo3.doc"} {
		args := callArgs(t, `{"blocks":[{"type":"paragraph","text":"x"}]}`)
		args["path"] = given
		if _, err := s.ExecuteTool(context.Background(), args); err != nil {
			t.Fatalf("%s: %v", given, err)
		}
		want := strings.TrimSuffix(given, filepath.Ext(given)) + ".docx"
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Errorf("%s did not land at %s: %v", given, want, err)
		}
	}
}

func TestDocWriteStaysInsideTheSandbox(t *testing.T) {
	root := t.TempDir()
	s := &docWriteSkill{root: root}

	for _, escape := range []string{"../escaped.docx", "a/../../escaped.docx"} {
		args := callArgs(t, `{"blocks":[{"type":"paragraph","text":"x"}]}`)
		args["path"] = escape
		if _, err := s.ExecuteTool(context.Background(), args); err == nil {
			t.Errorf("%s was allowed out of the sandbox", escape)
		}
	}
}

func TestDocWriteHonoursTheSessionOutputFolder(t *testing.T) {
	root := t.TempDir()
	s := &docWriteSkill{root: root, outputSubdir: func() string { return "session-1" }}

	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "report.docx",
		"blocks": [{"type": "paragraph", "text": "x"}]
	}`))
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "session-1", "report.docx")); err != nil {
		t.Fatalf("file did not land in the session folder: %v", err)
	}
	if !strings.Contains(out.Content, "on disk:") {
		t.Errorf("moved file did not report its real location: %q", out.Content)
	}
}

// An unknown block type must fail rather than be dropped: a report missing the
// table it was asked for is worse than one that was never written, because it
// looks complete.
func TestDocWriteRejectsWhatItCannotRead(t *testing.T) {
	root := t.TempDir()
	s := &docWriteSkill{root: root}

	cases := map[string]string{
		"no path":          `{"blocks":[{"type":"paragraph","text":"x"}]}`,
		"no blocks":        `{"path":"a.docx"}`,
		"empty blocks":     `{"path":"a.docx","blocks":[]}`,
		"no type":          `{"path":"a.docx","blocks":[{"text":"x"}]}`,
		"unknown type":     `{"path":"a.docx","blocks":[{"type":"quote","text":"x"}]}`,
		"heading no text":  `{"path":"a.docx","blocks":[{"type":"heading"}]}`,
		"list no items":    `{"path":"a.docx","blocks":[{"type":"bullets"}]}`,
		"table no content": `{"path":"a.docx","blocks":[{"type":"table"}]}`,
	}
	for name, body := range cases {
		if _, err := s.ExecuteTool(context.Background(), callArgs(t, body)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a failed call left files behind: %v", entries)
	}
}

// The loose shapes with exactly one sensible reading are read rather than
// rejected — a failed call costs a whole turn and teaches the model little.
func TestDocWriteAcceptsLooseButUnambiguousInput(t *testing.T) {
	root := t.TempDir()
	s := &docWriteSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "loose.docx",
		"blocks": ["ย่อหน้าเปล่า ๆ", {"type":"TABLE","columns":["a"],"rows":[["1"]]}]
	}`)); err != nil {
		t.Fatalf("loose input was rejected: %v", err)
	}
	body := readPart(t, filepath.Join(root, "loose.docx"), "word/document.xml")
	if !strings.Contains(body, "ย่อหน้าเปล่า ๆ") {
		t.Error("a bare-string block did not become a paragraph")
	}
	if !strings.Contains(body, "<w:tbl>") {
		t.Error("an upper-case type was not recognised")
	}
}

// A number sent as a JSON number is text in a document — there is no cell type
// for it to be stored as — but it must not arrive in exponent form.
func TestDocWriteRendersNumbersAsTypedNotAsFloats(t *testing.T) {
	root := t.TempDir()
	s := &docWriteSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "nums.docx",
		"blocks": [{"type":"table","columns":["ยอด"],"rows":[[1234567.5],[95]]}]
	}`)); err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	body := readPart(t, filepath.Join(root, "nums.docx"), "word/document.xml")
	if !strings.Contains(body, "1234567.5") {
		t.Errorf("a large number was not rendered as typed:\n%s", body)
	}
	if strings.Contains(body, "e+06") {
		t.Error("a number reached the document in exponent form")
	}
	if !strings.Contains(body, ">95<") {
		t.Errorf("an integer picked up a decimal tail:\n%s", body)
	}
}

func TestDocWriteIsRegisteredAsATool(t *testing.T) {
	r := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	s, ok := r.Get("doc_write")
	if !ok {
		t.Fatal("doc_write is not in the default registry, so the model never sees it")
	}
	tool, ok := s.(Tool)
	if !ok {
		t.Fatal("doc_write is not a Tool, so it has no definition to send")
	}
	if name := tool.ToolDefinition().Function.Name; name != "doc_write" {
		t.Errorf("tool name = %q", name)
	}
}

// A missing price prints as 0.00 — a figure nobody sent, on the one kind of
// document this block exists to keep honest. Refused where it can still be
// corrected, and an explicit zero still allowed: a line given away is a real
// thing to put on an invoice.
func TestALineWithNoPriceIsRefusedButAFreeLineIsNot(t *testing.T) {
	_, err := (&docWriteSkill{}).parseBlocks([]any{map[string]any{
		"type":    "lineitems",
		"columns": []any{"รายการ", "จำนวน", "ราคา", "รวม"},
		"lines":   []any{map[string]any{"text": "ค่าบริการ", "qty": 1.0}},
	}})
	if err == nil {
		t.Fatal("a line with no price was accepted and would print 0.00")
	}
	if !strings.Contains(err.Error(), "ค่าบริการ") {
		t.Errorf("the refusal does not name the line: %v", err)
	}

	if _, err := (&docWriteSkill{}).parseBlocks([]any{map[string]any{
		"type":    "lineitems",
		"columns": []any{"รายการ", "จำนวน", "ราคา", "รวม"},
		"lines":   []any{map[string]any{"text": "ของแถม", "price": 0.0}},
	}}); err != nil {
		t.Errorf("an explicitly free line was refused: %v", err)
	}

	// A price sent as text is the same silent zero by another route.
	if _, err := (&docWriteSkill{}).parseBlocks([]any{map[string]any{
		"type":    "lineitems",
		"columns": []any{"รายการ", "จำนวน", "ราคา", "รวม"},
		"lines":   []any{map[string]any{"text": "ค่าบริการ", "price": "฿15,000"}},
	}}); err == nil {
		t.Error("a price sent as text was accepted and would print 0.00")
	}
}

// The job this was missing on 2026-08-18: an appendix of screenshots, where the
// caption and the picture arrive together and the file travels on its own.
func TestDocWriteEmbedsAPictureAndItsCaption(t *testing.T) {
	root := t.TempDir()
	shot := filepath.Join(root, "หน้าหลัก.png")
	if err := os.WriteFile(shot, pngBytes(t, 1200, 800), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &docWriteSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "ภาคผนวก.docx",
		"blocks": [
			{"type": "heading", "text": "ภาคผนวก ก ภาพหน้าจอ"},
			{"type": "image", "image": "หน้าหลัก.png", "text": "ภาพที่ ก-1 หน้าหลักของเว็บ"}
		]
	}`))
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false: %s", out.Stderr)
	}

	doc := filepath.Join(root, "ภาคผนวก.docx")
	body := readPart(t, doc, "word/document.xml")
	if !strings.Contains(body, "<w:drawing>") {
		t.Error("the document has no drawing, so the picture never arrived")
	}
	if !strings.Contains(body, "ภาพที่ ก-1 หน้าหลักของเว็บ") {
		t.Error("the caption is missing")
	}
	// Embedded, not linked: the .docx has to survive being mailed on its own.
	if media := readPart(t, doc, "word/media/image1.png"); len(media) == 0 {
		t.Error("the picture bytes are not in the package")
	}
}

// A picture the model named and the disk does not have is the one case where
// carrying on is worse than failing: the document opens, looks finished, and is
// missing the figure it exists for.
func TestDocWriteRefusesAPictureItCannotFind(t *testing.T) {
	root := t.TempDir()
	s := &docWriteSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "ภาคผนวก.docx",
		"blocks": [{"type": "image", "image": "ไม่มีจริง.png", "text": "ภาพที่ 1"}]
	}`))
	if err == nil && out.Success {
		t.Fatal("a missing picture was accepted and the document was written without it")
	}
	if !strings.Contains(out.Stderr+errText(err), "ไม่มีจริง.png") {
		t.Errorf("the refusal does not name the file that is missing: %v %s", err, out.Stderr)
	}
}

// `type` and `image` say the same thing, and a call carrying the path and not
// the discriminator is unambiguous. Refusing it costs a whole turn to be told
// something the call already made clear.
func TestAnImagePathIsEnoughToMakeItAnImageBlock(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ก.png"), pngBytes(t, 20, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks, err := (&docWriteSkill{root: root}).parseBlocks([]any{
		map[string]any{"image": "ก.png", "text": "ภาพที่ 1"},
	})
	if err != nil {
		t.Fatalf("a block with a picture and no type was refused: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Kind != ooxml.BlockImage || blocks[0].Image == nil {
		t.Fatalf("blocks = %+v, want one image block", blocks)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
