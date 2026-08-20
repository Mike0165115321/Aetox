package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/ooxml"
)

// A gallery app rooted at one folder, with a file already sitting in its
// output/ — the shape every card on the ผลงาน page has.
func previewApp(t *testing.T, name string, body []byte) (*App, string) {
	t.Helper()
	// A subdirectory rather than t.TempDir() itself, because the escape test
	// below writes to this root's PARENT on purpose. Handing it the directory
	// Go promised to clean means that write lands somewhere Go owns; when the
	// root was t.TempDir() directly, the parent was outside that promise, and
	// on a build where the conversation carried no sandbox root at all it
	// resolved to "." and left a secrets.txt in the repo.
	root := filepath.Join(t.TempDir(), "workspace")
	dir := filepath.Join(root, outputDir, "20260808-010203.000")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation()), path
}

func TestArtifactPreviewReadsWhatIsInside(t *testing.T) {
	t.Run("markdown keeps its source for the renderer", func(t *testing.T) {
		a, path := previewApp(t, "file-scan-report.md", []byte("# รายงาน\n\n- 516,374 ไฟล์\n"))
		got, err := a.ArtifactPreview(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != "markdown" {
			t.Errorf("Kind = %q, want markdown", got.Kind)
		}
		if !strings.Contains(got.Text, "516,374") {
			t.Errorf("Text = %q, want the file's own words", got.Text)
		}
	})

	// The card renders this one rather than reading it, and half a document
	// renders as a broken document.
	t.Run("html comes back whole", func(t *testing.T) {
		body := []byte(`<h1>Northstar</h1><p style="color:red">แบรนด์</p>`)
		a, path := previewApp(t, "northstar-brand.html", body)
		got, err := a.ArtifactPreview(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != "html" {
			t.Fatalf("Kind = %q, want html", got.Kind)
		}
		if got.Text != string(body) {
			t.Errorf("Text = %q, want the whole document", got.Text)
		}
	})

	// An extension nobody listed is still usually text — .ps1, .toml, a file
	// with no extension at all. Judging by the bytes is what keeps a coding
	// session's output from coming back as a grid of blank cards.
	t.Run("an unlisted extension is judged by its bytes", func(t *testing.T) {
		a, path := previewApp(t, "scan_files.ps1", []byte("Get-ChildItem -Recurse | Measure-Object\n"))
		got, err := a.ArtifactPreview(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != "text" || !strings.Contains(got.Text, "Get-ChildItem") {
			t.Errorf("got %+v, want the script's text", got)
		}
	})

	t.Run("binary refuses instead of printing mojibake", func(t *testing.T) {
		a, path := previewApp(t, "mystery.bin", []byte{0x00, 0x01, 0x02, 0xff, 0xfe})
		got, err := a.ArtifactPreview(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != "none" {
			t.Errorf("Kind = %q, want none for binary", got.Kind)
		}
	})

	t.Run("an image comes back as something the card can draw", func(t *testing.T) {
		// The smallest real PNG there is.
		png := []byte{
			0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
			0, 0, 0, 0x0d, 'I', 'H', 'D', 'R',
			0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
		}
		a, path := previewApp(t, "chart.png", png)
		got, err := a.ArtifactPreview(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != "image" || !strings.HasPrefix(got.DataURL, "data:image/png;base64,") {
			t.Errorf("got Kind=%q DataURL=%.40q, want a png data URL", got.Kind, got.DataURL)
		}
	})
}

// The three office formats the agent actually produces. Built with the same
// writers that made the files in the gallery, so the readers are tested against
// real output rather than a hand-written fixture that agrees with them.
func TestArtifactPreviewReadsOfficeFiles(t *testing.T) {
	t.Run("xlsx comes back as a grid", func(t *testing.T) {
		parts, err := ooxml.BuildXLSX([]ooxml.Sheet{{
			Name: "สรุป",
			Rows: [][]ooxml.Cell{
				{ooxml.TextCell("เดือน"), ooxml.TextCell("ยอด")},
				{ooxml.TextCell("ม.ค."), ooxml.NumberCell(1200)},
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		body, err := ooxml.WritePackage(parts)
		if err != nil {
			t.Fatal(err)
		}
		a, path := previewApp(t, "สรุปค่าใช้จ่าย-สาธิต.xlsx", body)

		got, err := a.ArtifactPreview(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != "sheet" {
			t.Fatalf("Kind = %q, want sheet", got.Kind)
		}
		if got.Sheet != "สรุป" {
			t.Errorf("Sheet = %q, want สรุป", got.Sheet)
		}
		if len(got.Rows) < 2 || got.Rows[0][0] != "เดือน" || got.Rows[1][0] != "ม.ค." {
			t.Errorf("Rows = %v, want the workbook's own cells", got.Rows)
		}
	})

	t.Run("docx gives up its words", func(t *testing.T) {
		parts, err := ooxml.BuildDOCX([]ooxml.Block{
			{Kind: ooxml.BlockHeading, Level: 1, Text: "นิสัยการทำงานที่ดี"},
			{Kind: ooxml.BlockParagraph, Text: "ทีมเล็กชนะด้วยรอบสั้น"},
		})
		if err != nil {
			t.Fatal(err)
		}
		body, err := ooxml.WritePackage(parts)
		if err != nil {
			t.Fatal(err)
		}
		a, path := previewApp(t, "นิสัยการทำงานที่ดีของทีมเล็ก.docx", body)

		got, err := a.ArtifactPreview(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != "text" {
			t.Fatalf("Kind = %q, want text", got.Kind)
		}
		for _, want := range []string{"นิสัยการทำงานที่ดี", "ทีมเล็กชนะด้วยรอบสั้น"} {
			if !strings.Contains(got.Text, want) {
				t.Errorf("Text = %q, missing %q", got.Text, want)
			}
		}
	})

	t.Run("pptx reads its slides in deck order", func(t *testing.T) {
		slides := make([]ooxml.Slide, 0, 11)
		for i := 1; i <= 11; i++ {
			slides = append(slides, ooxml.Slide{Title: "หัวข้อ", Bullets: []string{"สไลด์" + itoa(i)}})
		}
		parts, err := ooxml.BuildPPTX(slides)
		if err != nil {
			t.Fatal(err)
		}
		body, err := ooxml.WritePackage(parts)
		if err != nil {
			t.Fatal(err)
		}
		a, path := previewApp(t, "แนะนำแอสซิสแทนต์.pptx", body)

		got, err := a.ArtifactPreview(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != "text" {
			t.Fatalf("Kind = %q, want text", got.Kind)
		}
		// A plain string sort puts slide10 before slide2, which would open the
		// preview in the middle of the deck.
		two := strings.Index(got.Text, "สไลด์2")
		ten := strings.Index(got.Text, "สไลด์10")
		if two < 0 || ten < 0 {
			t.Fatalf("Text = %q, want every slide's words", got.Text)
		}
		if two > ten {
			t.Errorf("slide 10 came before slide 2 — deck order lost")
		}
	})
}

// The window hands this method an absolute path, which is a request to read
// any file on the machine unless something says otherwise. The gallery's own
// roots are that something.
func TestArtifactPreviewRefusesFilesOutsideTheGallery(t *testing.T) {
	a, _ := previewApp(t, "ok.md", []byte("# fine"))

	outside := filepath.Join(t.TempDir(), "secrets.txt")
	if err := os.WriteFile(outside, []byte("token=hunter2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ArtifactPreview(outside); err == nil {
		t.Error("read a file from outside the output folder")
	}

	// Climbing out of output/ with .. is the same request wearing a relative
	// path — the gallery's own root is not the gallery.
	//
	// The root is read off the CONVERSATION, which is where a chat's sandbox
	// has lived since §155. An empty one here would not fail this test, it
	// would hollow it out: every path below collapses to a relative one, the
	// climb is refused for the wrong reason, and the decoy is written to the
	// process's working directory instead. Assert it rather than discover it
	// later as a stray file in the repository.
	sandbox := a.cur().cfg.SandboxRoot
	if sandbox == "" {
		t.Fatal("the conversation has no sandbox root, so the escape below is not being tested")
	}
	escape := filepath.Join(sandbox, outputDir, "..", "..", "secrets.txt")
	if err := os.WriteFile(filepath.Join(filepath.Dir(sandbox), "secrets.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("could not plant the file the climb is meant to reach: %v", err)
	}
	if _, err := a.ArtifactPreview(escape); err == nil {
		t.Error("climbed out of the output folder with ..")
	}

	if _, err := a.ArtifactPreview(""); err == nil {
		t.Error("accepted an empty path")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}
