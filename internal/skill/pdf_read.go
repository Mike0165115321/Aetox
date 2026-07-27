package skill

// pdf_read pulls the text layer out of a PDF. It exists because `read` cannot:
// a PDF is a binary container, so read correctly refuses it, and the attachment
// picker has always accepted *.pdf — the user attached a document and the agent
// had nowhere to go with it.
//
// It shells out to poppler's pdftotext rather than parsing PDFs in Go, and the
// reason is the failure mode, not the effort. Getting text out means resolving
// the font's ToUnicode CMap; a parser that gets that wrong does not error, it
// returns plausible-looking wrong text — and the agent then summarises a
// financial statement from it, confidently. pdftotext is the reference
// implementation of exactly that decoding. Same trade as image_ocr shelling out
// to Tesseract (see its header for the install story, which this mirrors).
//
// A PDF with no text layer at all (a scan) is a different problem: the answer
// there is to render the pages and OCR them, which the same poppler install
// makes possible via pdftoppm feeding the Tesseract image_ocr already needs.
// Not built until someone actually hits it — this reports the case instead.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

type pdfReadSkill struct {
	root string
}

func (*pdfReadSkill) Name() string { return "pdf_read" }

func (*pdfReadSkill) Description() string {
	return "อ่านข้อความจากไฟล์ PDF — ใช้แทน read ซึ่งเปิด PDF ไม่ได้เพราะเป็นไฟล์ไบนารี"
}

func (*pdfReadSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path (under sandbox root) to the PDF file",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "pdf_read",
			Description: "Extract the text of a PDF document. Use this for any .pdf — `read` cannot open one, a PDF is a binary container rather than text.",
			Parameters:  payload,
		},
	}
}

func (s *pdfReadSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: pdf_read <path>")
		return newToolOutput("pdf_read", "pdf_read", "", start, false, err), err
	}
	return s.run(ctx, start, strings.TrimSpace(strings.Join(args, " ")))
}

func (s *pdfReadSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		err := errors.New("path is required")
		return newToolOutput("pdf_read", "pdf_read", "", time.Now(), false, err), err
	}
	return s.run(ctx, time.Now(), path)
}

func (s *pdfReadSkill) run(ctx context.Context, start time.Time, requestPath string) (Output, error) {
	command := "pdf_read " + requestPath
	if requestPath == "" {
		err := errors.New("usage: pdf_read <path>")
		return newToolOutput("pdf_read", command, "", start, false, err), err
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("pdf_read", command, "", start, false, err), err
	}

	text, runErr := runPdfToText(ctx, targetPath)
	if runErr != nil && errors.Is(runErr, exec.ErrNotFound) && tryAutoInstallPoppler(ctx) {
		text, runErr = runPdfToText(ctx, targetPath) // installed just now — one retry
	}
	if runErr != nil {
		if errors.Is(runErr, exec.ErrNotFound) {
			runErr = missingPopplerError()
		}
		return newToolOutput("pdf_read", command, "", start, false, runErr), runErr
	}

	// An empty extraction is a real answer, not an empty file: the pages are
	// images. Returned as an error so it reads as "this route is closed",
	// which is the whole point of the read fix that sits alongside this.
	if text == "" {
		err := errors.New("PDF นี้ไม่มีชั้นข้อความ (น่าจะเป็นไฟล์สแกน) — อ่านด้วย pdf_read ไม่ได้")
		return newToolOutput("pdf_read", command, "", start, false, err), err
	}

	truncated, wasTruncated := limitLines(text, defaultToolOutputLineLimit)
	return newToolOutput("pdf_read", command, truncated, start, wasTruncated, nil), nil
}

func runPdfToText(ctx context.Context, pdfPath string) (string, error) {
	// -layout keeps columns and tables readable instead of interleaving them
	// into one stream, which is what a statement or invoice needs to survive.
	// -enc UTF-8 is explicit rather than assumed: the default is a build-time
	// choice, and Thai text is exactly what a wrong one destroys. "-" is
	// poppler's stdout target.
	cmd := exec.CommandContext(ctx, bundledBinary("poppler", "pdftotext"), "-layout", "-enc", "UTF-8", pdfPath, "-")
	proc.HideConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", err
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// tryAutoInstallPoppler mirrors tryAutoInstallTesseract: Homebrew needs no
// sudo, so macOS is the one place installing unattended is safe.
func tryAutoInstallPoppler(ctx context.Context) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	if _, err := exec.LookPath("brew"); err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, "brew", "install", "poppler")
	proc.HideConsole(cmd) // no-op on darwin; keeps the "every exec site" rule exception-free
	return cmd.Run() == nil
}

func missingPopplerError() error {
	switch runtime.GOOS {
	case "darwin":
		return errors.New("ไม่พบ pdftotext และติดตั้งอัตโนมัติไม่สำเร็จ (ต้องมี Homebrew) — รันเอง: brew install poppler")
	case "linux":
		if hint := linuxInstallHint("poppler-utils", "poppler-utils", "poppler"); hint != "" {
			return fmt.Errorf("ไม่พบโปรแกรม pdftotext ในเครื่อง — ติดตั้งด้วย: %s", hint)
		}
		return errors.New("ไม่พบโปรแกรม pdftotext ในเครื่อง — ติดตั้งผ่าน package manager ของดิสโทรคุณ (แพ็กเกจ poppler-utils หรือ poppler)")
	default: // windows and anything else
		return errors.New("ไม่พบโปรแกรม pdftotext ในเครื่อง — ติดตั้งด้วย: scoop install poppler (หรือดาวน์โหลด poppler for Windows) แล้วลองใหม่")
	}
}
