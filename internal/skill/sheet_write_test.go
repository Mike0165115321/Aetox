package skill

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// callArgs is the shape a tool call arrives in: the model's JSON, decoded the
// same way the executor decodes it. Building the map in Go by hand would use
// []string and int where the wire gives []any and float64, and that difference
// is exactly what parseSheets has to survive.
func callArgs(t *testing.T, body string) map[string]any {
	t.Helper()
	var args map[string]any
	if err := json.Unmarshal([]byte(body), &args); err != nil {
		t.Fatalf("bad test fixture: %v", err)
	}
	return args
}

func readSheet1(t *testing.T, path string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("result is not a readable workbook: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
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
	t.Fatal("workbook has no first worksheet")
	return ""
}

// The end-to-end shape the flagship example on the website needs: a folder of
// slips becomes a table that opens, with amounts that add up.
func TestSheetWriteProducesAWorkbookExcelCanRead(t *testing.T) {
	root := t.TempDir()
	s := &sheetWriteSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "สรุปสลิป.xlsx",
		"sheets": [{
			"name": "สิงหาคม",
			"columns": ["เลขที่", "ร้าน", "ยอด", "วันที่"],
			"rows": [
				["0012", "ร้านกาแฟ", 185.5, "2026-08-03"],
				["0013", "ค่าไฟ", 1240, "2026-08-01"]
			]
		}]
	}`))
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false: %s", out.Stderr)
	}

	sheet := readSheet1(t, filepath.Join(root, "สรุปสลิป.xlsx"))
	if !strings.Contains(sheet, `<c r="C2"><v>185.5</v></c>`) {
		t.Errorf("amount is not a summable number:\n%s", sheet)
	}
	if !strings.Contains(sheet, `<c r="D2" s="2">`) {
		t.Errorf("date did not become a real date cell:\n%s", sheet)
	}
	if !strings.Contains(sheet, ">0012<") {
		t.Errorf("reference lost its leading zeros:\n%s", sheet)
	}
	if !strings.Contains(sheet, "ร้านกาแฟ") {
		t.Errorf("Thai text did not survive:\n%s", sheet)
	}
	// The receipt is what the model reads back to the user, so it has to name
	// the file rather than just say it worked.
	if !strings.Contains(out.Content, "สรุปสลิป.xlsx") {
		t.Errorf("output does not name the file: %q", out.Content)
	}
	// And the workbook comes back as a thing, not only as a name inside a
	// sentence — this is what the chat puts an open button on.
	if len(out.Artifacts) != 1 || out.Artifacts[0] != "สรุปสลิป.xlsx" {
		t.Errorf("Artifacts = %v, want the workbook's path", out.Artifacts)
	}
}

// The artifact has to be the path that resolves from outside this package, or
// the button the UI puts on it opens nothing. In an unfocused session that is
// the placed path, not the one the model typed.
func TestSheetWriteReportsTheArtifactWhereItActuallyLanded(t *testing.T) {
	root := t.TempDir()
	s := &sheetWriteSkill{root: root, outputSubdir: func() string { return "session-1" }}

	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "report.xlsx",
		"sheets": [{"name":"S","columns":["A"],"rows":[["x"]]}]
	}`))
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if len(out.Artifacts) != 1 {
		t.Fatalf("Artifacts = %v, want one path", out.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(root, out.Artifacts[0])); err != nil {
		t.Errorf("Artifacts[0] = %q does not resolve under the sandbox root: %v", out.Artifacts[0], err)
	}
}

// A failed call must not advertise a file. The card would open nothing, and the
// answer would claim work that did not happen.
func TestSheetWriteReportsNoArtifactWhenItFails(t *testing.T) {
	root := t.TempDir()
	s := &sheetWriteSkill{root: root}

	out, _ := s.ExecuteTool(context.Background(), callArgs(t, `{"path":"a.xlsx","sheets":[]}`))
	if len(out.Artifacts) != 0 {
		t.Errorf("Artifacts = %v on a failed call, want none", out.Artifacts)
	}
}

// A model that types "report" or copies a .csv habit still gets a workbook,
// and the receipt shows the corrected name so it tells the user the right one.
func TestSheetWriteForcesTheXlsxExtension(t *testing.T) {
	root := t.TempDir()
	s := &sheetWriteSkill{root: root}

	for _, given := range []string{"report", "report2.csv", "report3.xls"} {
		args := callArgs(t, `{"sheets":[{"name":"S","columns":["A"],"rows":[["x"]]}]}`)
		args["path"] = given
		out, err := s.ExecuteTool(context.Background(), args)
		if err != nil {
			t.Fatalf("%s: %v", given, err)
		}
		want := strings.TrimSuffix(given, filepath.Ext(given)) + ".xlsx"
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Errorf("%s did not land at %s: %v", given, want, err)
		}
		if !strings.Contains(out.Content, want) {
			t.Errorf("receipt for %s does not name %s: %q", given, want, out.Content)
		}
	}
}

// sheet_write creates files, so it is bound by the sandbox exactly as write is.
// A tool that writes binaries anywhere on disk is a worse hole than one that
// writes text anywhere on disk.
func TestSheetWriteStaysInsideTheSandbox(t *testing.T) {
	root := t.TempDir()
	s := &sheetWriteSkill{root: root}

	for _, escape := range []string{"../escaped.xlsx", "a/../../escaped.xlsx"} {
		args := callArgs(t, `{"sheets":[{"name":"S","columns":["A"],"rows":[["x"]]}]}`)
		args["path"] = escape
		if _, err := s.ExecuteTool(context.Background(), args); err == nil {
			t.Errorf("%s was allowed out of the sandbox", escape)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escaped.xlsx")); err == nil {
		t.Error("a file was written outside the sandbox root")
	}
}

// The rule write follows: a relative path from an unfocused session goes to
// that session's output folder. If sheet_write ignored it, half of what a chat
// produced would be in the folder and half at the sandbox root.
func TestSheetWriteHonoursTheSessionOutputFolder(t *testing.T) {
	root := t.TempDir()
	s := &sheetWriteSkill{root: root, outputSubdir: func() string { return "session-1" }}

	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "report.xlsx",
		"sheets": [{"name":"S","columns":["A"],"rows":[["x"]]}]
	}`))
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "session-1", "report.xlsx")); err != nil {
		t.Fatalf("file did not land in the session folder: %v", err)
	}
	// write hands over the on-disk path in this case for a reason: nothing else
	// in context lets the model answer "where is it on my machine".
	if !strings.Contains(out.Content, "on disk:") {
		t.Errorf("moved file did not report its real location: %q", out.Content)
	}
}

// Failing a tool call costs a whole turn and the model often cannot tell from
// the error what it got wrong, so the shapes with one obvious reading are read
// rather than rejected.
func TestSheetWriteAcceptsLooseButUnambiguousInput(t *testing.T) {
	root := t.TempDir()
	s := &sheetWriteSkill{root: root}

	// A single-column sheet whose rows are bare values instead of arrays.
	out, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "loose.xlsx",
		"sheets": [{"name":"S","columns":["Item"],"rows":["one","two"]}]
	}`))
	if err != nil {
		t.Fatalf("bare-value rows were rejected: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false: %s", out.Stderr)
	}
	sheet := readSheet1(t, filepath.Join(root, "loose.xlsx"))
	if !strings.Contains(sheet, ">one<") || !strings.Contains(sheet, ">two<") {
		t.Errorf("bare-value rows lost their values:\n%s", sheet)
	}

	// A sheet with headers and no rows at all is a legitimate empty template.
	if _, err := s.ExecuteTool(context.Background(), callArgs(t, `{
		"path": "empty.xlsx",
		"sheets": [{"name":"S","columns":["A","B"]}]
	}`)); err != nil {
		t.Errorf("a sheet with no rows was rejected: %v", err)
	}
}

func TestSheetWriteRejectsWhatItCannotRead(t *testing.T) {
	root := t.TempDir()
	s := &sheetWriteSkill{root: root}

	cases := map[string]string{
		"no path":       `{"sheets":[{"name":"S","columns":["A"]}]}`,
		"no sheets":     `{"path":"a.xlsx"}`,
		"empty sheets":  `{"path":"a.xlsx","sheets":[]}`,
		"sheets scalar": `{"path":"a.xlsx","sheets":"one sheet please"}`,
		"sheet scalar":  `{"path":"a.xlsx","sheets":["one sheet please"]}`,
	}
	for name, body := range cases {
		if _, err := s.ExecuteTool(context.Background(), callArgs(t, body)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	// Nothing may be left behind by a rejected call.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a failed call left files behind: %v", entries)
	}
}

func TestSheetWriteIsRegisteredAsATool(t *testing.T) {
	r := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	s, ok := r.Get("sheet_write")
	if !ok {
		t.Fatal("sheet_write is not in the default registry, so the model never sees it")
	}
	tool, ok := s.(Tool)
	if !ok {
		t.Fatal("sheet_write is not a Tool, so it has no definition to send")
	}
	definition := tool.ToolDefinition()
	if definition.Function.Name != "sheet_write" {
		t.Errorf("tool name = %q", definition.Function.Name)
	}
	// The typing rule is the one thing that decides whether the export is
	// useful, and it only reaches the model through this description.
	if !strings.Contains(definition.Function.Description, "1234.5") {
		t.Error("the description no longer tells the model to send numbers as numbers")
	}
}
