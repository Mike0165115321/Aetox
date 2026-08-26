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
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/proc"
	"github.com/Mikedev115/Aetox/internal/statereport"
)

type pdfReadSkill struct {
	root string
}

func (*pdfReadSkill) Name() string { return "pdf_read" }

func (*pdfReadSkill) Description() string {
	return "อ่านข้อความจากไฟล์ PDF, ใช้แทน read ซึ่งเปิด PDF ไม่ได้เพราะเป็นไฟล์ไบนารี"
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
			Description: "Extract the text of a PDF document. Use this for any .pdf, `read` cannot open one, a PDF is a binary container rather than text.",
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

	text, runErr := runPdfToText(ctx, targetPath, nil)
	if runErr != nil && errors.Is(runErr, exec.ErrNotFound) && tryAutoInstallPoppler(ctx) {
		text, runErr = runPdfToText(ctx, targetPath, nil) // installed just now — one retry
	}
	if runErr != nil && errors.Is(runErr, errReaderCrashed) {
		text, runErr = runPdfToText(ctx, targetPath, converterEnv()) // see converterEnv
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
		err := errors.New("PDF นี้ไม่มีชั้นข้อความ (น่าจะเป็นไฟล์สแกน), อ่านด้วย pdf_read ไม่ได้")
		return newToolOutput("pdf_read", command, "", start, false, err), err
	}

	truncated, wasTruncated := limitLines(text, defaultToolOutputLineLimit)
	return newToolOutput("pdf_read", command, truncated, start, wasTruncated, nil), nil
}

// runPdfToText runs the converter once. A nil env inherits Aetox's own, which
// is the normal path; converterEnv() is passed only on the retry below.
func runPdfToText(ctx context.Context, pdfPath string, env []string) (string, error) {
	// -layout keeps columns and tables readable instead of interleaving them
	// into one stream, which is what a statement or invoice needs to survive.
	// -enc UTF-8 is explicit rather than assumed: the default is a build-time
	// choice, and Thai text is exactly what a wrong one destroys. "-" is
	// poppler's stdout target.
	cmd := exec.CommandContext(ctx, bundledBinary("poppler", "pdftotext"), "-layout", "-enc", "UTF-8", pdfPath, "-")
	cmd.Env = env
	proc.HideConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", err
		}
		if diedMidRun(err) {
			return "", fmt.Errorf("%w (%v)", errReaderCrashed, err)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// errReaderCrashed is the converter dying partway through rather than judging
// the file.
//
// The distinction matters because of what the caller does next. For a refusal,
// stderr is the reason and handing it to the model is right. For a crash,
// stderr is only whatever had been printed before the process died — often a
// routine warning — and passing that off as the reason tells the model the
// document is corrupt when the reader is the thing that broke. It then tells
// the user so, confidently, which is the exact failure this file's header sets
// out to avoid.
//
// This is observed, not anticipated. On 2026-08-09 the toolbox sweep failed
// with `exit status 0xc0000005` — an access violation, so pdftotext.exe
// crashing rather than reporting anything — on a file that the same binary and
// the same flags handled fine from a shell. That evidence used to live in a
// comment on the test fixture, which is where it was found rather than where it
// is needed.
var errReaderCrashed = errors.New("pdftotext หยุดกลางคัน, ปัญหาอยู่ที่ตัวอ่าน ไม่ใช่ที่ตัวไฟล์ PDF")

func diedMidRun(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	return abnormalExit(exit.ExitCode())
}

// abnormalExit reports whether a status means the process was destroyed rather
// than that it chose to fail. -1 is "killed by a signal" on Unix; Windows
// reports an unhandled exception as its NTSTATUS instead, and those all sit
// above 0xC0000000 — 0xC0000005, the access violation a stray write ends in, is
// the one seen in practice.
func abnormalExit(code int) bool {
	return code == -1 || uint32(code) >= 0xC0000000
}

// converterEnv is the environment a document converter actually needs: where
// its own libraries live, where it may write scratch files, whose settings to
// read, and which locale to decode in. Everything else in Aetox's environment
// is none of the converter's business.
//
// It is used only after a crash, because the reason it helps is not principle
// but damage control. pdftotext builds differ in how carefully they walk the
// environment block they are handed: the Xpdf 4.x build that ships inside Git
// for Windows — which is what the name resolves to on a Windows machine with no
// poppler installed — writes past the end of a fixed buffer for some
// environments and dies with an access violation before it reads a byte of the
// PDF. Handing it a short, plain environment gets the real text out of it, and
// costs a healthy install nothing, since it never reaches this path.
func converterEnv() []string {
	// os.LookupEnv is case-insensitive on Windows, so one spelling covers both
	// PATH and Path.
	keep := []string{
		"PATH",                                // the converter's own DLLs sit beside it
		"SystemRoot", "SystemDrive", "windir", // windows will not start a process without these
		"TEMP", "TMP", "TMPDIR", // scratch space
		"HOME", "USERPROFILE", // whose settings to read
		"APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME",
		"LANG", "LC_ALL", "LC_CTYPE", // how to decode what it finds
	}
	env := make([]string, 0, len(keep))
	for _, name := range keep {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
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

// A state report, not a lesson: what it says about this machine is true or
// false regardless of how the tool was called (see missingTesseractError).
func missingPopplerError() error {
	switch runtime.GOOS {
	case "darwin":
		return statereport.New("ไม่พบ pdftotext และติดตั้งอัตโนมัติไม่สำเร็จ (ต้องมี Homebrew), รันเอง: brew install poppler")
	case "linux":
		if hint := linuxInstallHint("poppler-utils", "poppler-utils", "poppler"); hint != "" {
			return statereport.Newf("ไม่พบโปรแกรม pdftotext ในเครื่อง, ติดตั้งด้วย: %s", hint)
		}
		return statereport.New("ไม่พบโปรแกรม pdftotext ในเครื่อง, ติดตั้งผ่าน package manager ของดิสโทรคุณ (แพ็กเกจ poppler-utils หรือ poppler)")
	default: // windows and anything else
		return statereport.New("ไม่พบโปรแกรม pdftotext ในเครื่อง, ติดตั้งด้วย: scoop install poppler (หรือดาวน์โหลด poppler for Windows) แล้วลองใหม่")
	}
}
