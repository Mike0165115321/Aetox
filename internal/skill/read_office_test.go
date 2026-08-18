package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/ooxml"
)

func writeDoc(t *testing.T, path string, blocks []ooxml.Block) {
	t.Helper()
	parts, err := ooxml.BuildDOCX(blocks)
	if err != nil {
		t.Fatal(err)
	}
	if err := ooxml.WriteFile(path, parts); err != nil {
		t.Fatal(err)
	}
}

// The refusal this replaces was true about the bytes and useless about the
// file: *"read target is a binary file — there is no text to read"*, said to
// the agent whose entire job is documents, about a document.
func TestReadOpensAWordDocumentInsteadOfRefusingIt(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, filepath.Join(root, "รายงาน.docx"), []ooxml.Block{
		{Kind: ooxml.BlockHeading, Text: "ผลการทดสอบ", Level: 2},
		{Kind: ooxml.BlockParagraph, Text: "ระบบผ่านทุกกรณี"},
		{Kind: ooxml.BlockTable, Columns: []string{"หมวด", "ผล"}, Rows: [][]string{{"ความเร็ว", "ผ่าน"}}},
	})

	s := &readSkill{root: root}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "รายงาน.docx"})
	if err != nil {
		t.Fatalf("read refused a .docx: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false: %s", out.Stderr)
	}
	for _, want := range []string{"ผลการทดสอบ", "ระบบผ่านทุกกรณี", "ความเร็ว", "Heading2", "table"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("the rendered document is missing %q:\n%s", want, out.Content)
		}
	}
	// Numbered, because the number is how the next instruction names the thing
	// to change — which is the whole reason this returns structure rather than
	// a wall of text.
	if !strings.Contains(out.Content, "[1]") || !strings.Contains(out.Content, "[3]") {
		t.Errorf("blocks are not addressable:\n%s", out.Content)
	}
}

// The figures are the question somebody actually asks of an appendix, so the
// count is stated rather than left to be inferred from the listing.
func TestReadingADocumentSaysHowManyPicturesItHolds(t *testing.T) {
	root := t.TempDir()
	shot := filepath.Join(root, "จอ.png")
	if err := os.WriteFile(shot, pngBytes(t, 60, 40), 0o644); err != nil {
		t.Fatal(err)
	}
	picture, err := loadPicture(root, nil, "จอ.png")
	if err != nil {
		t.Fatal(err)
	}
	writeDoc(t, filepath.Join(root, "ภาคผนวก.docx"), []ooxml.Block{
		{Kind: ooxml.BlockImage, Text: "ภาพที่ 1", Image: picture},
		{Kind: ooxml.BlockImage, Text: "ภาพที่ 2", Image: picture},
	})

	s := &readSkill{root: root}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "ภาคผนวก.docx"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "2 picture(s)") {
		t.Errorf("the picture count is not reported:\n%s", out.Content)
	}
	if !strings.Contains(out.Content, "จอ.png") {
		t.Errorf("a figure does not name the file it came from:\n%s", out.Content)
	}
}

func TestReadOpensADeckAndAWorkbook(t *testing.T) {
	root := t.TempDir()
	deck, err := ooxml.BuildPPTX([]ooxml.Slide{{Title: "หัวข้อ", Bullets: []string{"ประเด็น"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ooxml.WriteFile(filepath.Join(root, "สไลด์.pptx"), deck); err != nil {
		t.Fatal(err)
	}
	book, err := ooxml.BuildXLSX([]ooxml.Sheet{{Name: "ยอดขาย", Columns: []string{"เดือน", "ยอด"}, Rows: [][]ooxml.Cell{{ooxml.TextCell("ม.ค."), ooxml.NumberCell(95)}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ooxml.WriteFile(filepath.Join(root, "ตัวเลข.xlsx"), book); err != nil {
		t.Fatal(err)
	}

	s := &readSkill{root: root}
	for path, want := range map[string]string{
		"สไลด์.pptx": "ประเด็น",
		"ตัวเลข.xlsx": "ยอดขาย",
	} {
		out, err := s.ExecuteTool(context.Background(), map[string]any{"path": path})
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if !strings.Contains(out.Content, want) {
			t.Errorf("reading %s did not return %q:\n%s", path, want, out.Content)
		}
	}
}

// A .docx that is not one has to fail as itself. Renaming a zip is the
// ordinary way this happens, and "binary file" would send the model looking for
// a different tool rather than a different file.
func TestReadNamesWhatIsWrongWithABrokenDocument(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ปลอม.docx"), []byte("not a zip at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &readSkill{root: root}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "ปลอม.docx"})
	if err == nil && out.Success {
		t.Fatal("a file that is not a Word document was read as one")
	}
	if !strings.Contains(out.Stderr+errText(err), "Word") {
		t.Errorf("the refusal does not say what it tried to open: %v %s", err, out.Stderr)
	}
}
