package skill

// image_ocr lets the agent read text out of an image it can't otherwise see
// (most chat models have no vision at all, or the current provider path
// doesn't send images). It shells out to Tesseract rather than embedding an
// OCR engine — the only real Go options are CGo-bound to a system Tesseract
// install anyway, or an abandoned pure-Go WASM port, so a plain subprocess is
// the least fragile choice.
//
// Getting Tesseract onto the machine differs by OS (see
// docs/architecture/tesseract-ocr-bundling-2026-07-22.md for the full story):
//   - Windows: the NSIS installer downloads+installs it silently at Aetox
//     install time (project.nsi) — the fallback message below only fires if
//     that step was skipped (offline install, checksum mismatch, ...).
//   - macOS: Homebrew doesn't need sudo, so a missing Tesseract is worth one
//     automatic `brew install` attempt right here, on first use.
//   - Linux: package managers need sudo, so auto-running one isn't safe to
//     do silently (mirrors why Windows doesn't bypass its own UAC prompt) —
//     this just tells the user the right one-liner for their distro.
// This is intentionally the lightweight version: Aetox only really targets
// Windows today (desktop/browser.go is raw Win32), so mac/Linux just needs
// to not leave the user stuck, not a fully engineered multi-distro installer.

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
)

type imageOCRSkill struct {
	root string
}

func (*imageOCRSkill) Name() string { return "image_ocr" }

func (*imageOCRSkill) Description() string {
	return "อ่านข้อความจากในรูปภาพ (OCR) — ใช้เมื่อโมเดลปัจจุบันมองไม่เห็นรูปภาพโดยตรง"
}

func (*imageOCRSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path (under sandbox root) to the image file",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "image_ocr",
			Description: "Extract text from an image via OCR (Tesseract, Thai+English). Use this to read an attached image's content when you have no direct vision of it.",
			Parameters:  payload,
		},
	}
}

func (s *imageOCRSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: image_ocr <path>")
		return newToolOutput("image_ocr", "image_ocr", "", start, false, err), err
	}
	return s.run(ctx, start, strings.TrimSpace(strings.Join(args, " ")))
}

func (s *imageOCRSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		err := errors.New("path is required")
		return newToolOutput("image_ocr", "image_ocr", "", time.Now(), false, err), err
	}
	return s.run(ctx, time.Now(), path)
}

func (s *imageOCRSkill) run(ctx context.Context, start time.Time, requestPath string) (Output, error) {
	command := "image_ocr " + requestPath
	if requestPath == "" {
		err := errors.New("usage: image_ocr <path>")
		return newToolOutput("image_ocr", command, "", start, false, err), err
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("image_ocr", command, "", start, false, err), err
	}

	text, runErr := runTesseract(ctx, targetPath)
	if runErr != nil && errors.Is(runErr, exec.ErrNotFound) && tryAutoInstallTesseract(ctx) {
		text, runErr = runTesseract(ctx, targetPath) // installed just now — one retry
	}
	if runErr != nil {
		if errors.Is(runErr, exec.ErrNotFound) {
			runErr = missingTesseractError()
		}
		return newToolOutput("image_ocr", command, "", start, false, runErr), runErr
	}

	if text == "" {
		text = "(ไม่พบข้อความในรูปภาพ)"
	}
	truncated, wasTruncated := limitLines(text, defaultToolOutputLineLimit)
	return newToolOutput("image_ocr", command, truncated, start, wasTruncated, nil), nil
}

func runTesseract(ctx context.Context, imagePath string) (string, error) {
	cmd := exec.CommandContext(ctx, "tesseract", imagePath, "stdout", "-l", "tha+eng")
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

// tryAutoInstallTesseract attempts a same-process install where that's safe
// to do unattended (macOS via Homebrew, no sudo needed). Returns false for
// anything that would need a password prompt (Linux package managers,
// Windows) — those are left to missingTesseractError()'s instructions.
func tryAutoInstallTesseract(ctx context.Context) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	if _, err := exec.LookPath("brew"); err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, "brew", "install", "tesseract", "tesseract-lang")
	return cmd.Run() == nil
}

func missingTesseractError() error {
	switch runtime.GOOS {
	case "darwin":
		return errors.New("ไม่พบ Tesseract และติดตั้งอัตโนมัติไม่สำเร็จ (ต้องมี Homebrew) — รันเอง: brew install tesseract tesseract-lang")
	case "linux":
		if hint := linuxInstallHint(); hint != "" {
			return fmt.Errorf("ไม่พบโปรแกรม Tesseract ในเครื่อง — ติดตั้งด้วย: %s", hint)
		}
		return errors.New("ไม่พบโปรแกรม Tesseract ในเครื่อง — ติดตั้งผ่าน package manager ของดิสโทรคุณ (แพ็กเกจ tesseract-ocr หรือ tesseract พร้อมชุดภาษาไทย)")
	default: // windows and anything else
		return errors.New("ไม่พบโปรแกรม Tesseract ในเครื่อง — ติดตั้งจาก https://github.com/UB-Mannheim/tesseract/wiki แล้วลองใหม่")
	}
}

// linuxInstallHint returns a ready-to-paste install command for whichever
// package manager is present. Not auto-run — these all need sudo, and
// running a privileged command silently isn't something to do without the
// user watching, same reasoning as not scripting around Windows' UAC.
func linuxInstallHint() string {
	switch {
	case commandExists("apt-get"):
		return "sudo apt-get install -y tesseract-ocr tesseract-ocr-tha"
	case commandExists("dnf"):
		return "sudo dnf install -y tesseract tesseract-langpack-tha"
	case commandExists("pacman"):
		return "sudo pacman -S tesseract-data-tha tesseract"
	default:
		return ""
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
