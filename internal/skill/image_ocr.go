package skill

// image_ocr lets the agent read text out of an image it can't otherwise see.
//
// This is the fallback path now, not the only one: since ARCHITECTURE.md §51 a
// model that can actually look at a picture is handed the picture, and this
// tool is what runs for the models that cannot (model.ResolveVision decides,
// and an unrecognized model counts as blind). OCR keeps the letters and loses
// the image, which is the right trade for a model with no eyes and a loss for
// every other one — so reaching for it when vision is available is a mistake,
// and desktop/app.go's visionAttachments is what makes sure that never happens.
//
// It shells out to Tesseract rather than embedding an
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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/proc"
	"github.com/Mike0165115321/Aetox/internal/statereport"

	"github.com/Mike0165115321/Aetox/internal/config"
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

	res, runErr := runTesseract(ctx, targetPath)
	if runErr != nil && errors.Is(runErr, exec.ErrNotFound) && tryAutoInstallTesseract(ctx) {
		res, runErr = runTesseract(ctx, targetPath) // installed just now — one retry
	}
	if runErr != nil {
		if errors.Is(runErr, exec.ErrNotFound) {
			runErr = missingTesseractError()
		}
		return newToolOutput("image_ocr", command, "", start, false, runErr), runErr
	}

	text := res.Text
	if text == "" {
		text = "(ไม่พบข้อความในรูปภาพ)"
	} else {
		// Appended after the text, not before it: the reader should meet what
		// was read first and the doubt about it second, the same order a person
		// looking at the image would.
		text = appendConfidenceNote(text, res.Confidence, res.Words)
	}
	truncated, wasTruncated := limitLines(text, defaultToolOutputLineLimit)
	return newToolOutput("image_ocr", command, truncated, start, wasTruncated, nil), nil
}

// tesseractCommand is the bare name, and what resolveTesseract falls back to
// when it finds nothing: bare, so a genuinely missing Tesseract surfaces as
// exec.ErrNotFound and becomes missingTesseractError() rather than a confusing
// "cannot find the file" about some path the user never typed.
const tesseractCommand = "tesseract"

// resolveTesseract picks the tesseract executable to run: PATH first, then the
// fixed addresses a Windows install is known to sit at.
//
// The order is the opposite of bundledBinary's, deliberately. poppler and
// ffmpeg are copies we unpacked and pinned ourselves, so ours wins over
// whatever else is on the machine. Tesseract is an ordinary system-wide
// install that any application may have put there, and someone who put a
// particular tesseract on PATH chose it — that choice stands.
//
// The fallback is here because our own installer runs the UB-Mannheim setup
// with /S, and that silent install never touches PATH. The machine ends up
// with a working Tesseract, Thai language data and all, at an address
// exec.LookPath cannot see — so image_ocr reported "ไม่พบโปรแกรม Tesseract"
// while tesseract.exe sat in Program Files (found 2026-08-18, on the owner's
// own machine after a clean v1.2.4 install). project.nsi checks the same
// $PROGRAMFILES64\Tesseract-OCR to decide whether to install at all; this
// keeps the two ends agreeing on where the thing lives.
func resolveTesseract() string {
	if path, err := exec.LookPath(tesseractCommand); err == nil {
		return path
	}
	if path := managedTesseract(); path != "" {
		return path
	}
	if path := tesseractInInstallDir(); path != "" {
		return path
	}
	return tesseractCommand
}

// managedTesseract is the copy internal/capability unpacked into
// <DataRoot>/tools/tesseract, which needs no administrator to write and so is
// the only address a normally-launched Aetox can install to at all.
//
// Ahead of Program Files because that one is whatever some past installer
// left; this one is the build the current release pinned. Behind PATH for the
// reason above: a tesseract someone put on PATH was chosen.
func managedTesseract() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	root, err := config.DataRoot()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(root, "tools", "tesseract", "tesseract.exe")
	if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
		return candidate
	}
	return ""
}

// tesseractEnv is the environment for one tesseract run.
//
// The managed copy is a loose tree, not an install: nothing registered it, so
// tesseract.exe cannot find its own tessdata the way it does after a real
// setup, and asked for tha+eng it would fail with "Failed loading language"
// rather than with anything about a missing folder. TESSDATA_PREFIX is the
// documented way to say where the folder is, and it is set only for our own
// copy — overriding it for a system install would break a machine whose
// tessdata sits somewhere we did not put it.
func tesseractEnv(binary string) []string {
	if binary != managedTesseract() || binary == "" {
		return nil
	}
	return append(os.Environ(), "TESSDATA_PREFIX="+filepath.Join(filepath.Dir(binary), "tessdata"))
}

// tesseractAvailable reports whether a tesseract can be found at all, for
// callers that want to bail before doing expensive setup work.
func tesseractAvailable() bool {
	return resolveTesseract() != tesseractCommand
}

// tesseractInInstallDir looks in the fixed locations Windows installers use,
// returning "" when none of them holds a tesseract.exe. Always "" elsewhere:
// Homebrew and every Linux package manager put it on PATH, so there is no
// known address to guess at and a guess would only be a wrong one.
func tesseractInInstallDir() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	// ProgramW6432 before ProgramFiles: it is C:\Program Files even when the
	// process asking is 32-bit, where "ProgramFiles" would silently be the
	// x86 folder. The x86 folder is still checked after, for the 32-bit build
	// of Tesseract itself. LOCALAPPDATA\Programs is UB-Mannheim's per-user
	// install option, which needs no administrator and so is what a user who
	// installed it themselves is most likely to have.
	for _, loc := range []struct{ env, sub string }{
		{"ProgramW6432", ""},
		{"ProgramFiles", ""},
		{"ProgramFiles(x86)", ""},
		{"LOCALAPPDATA", "Programs"},
	} {
		base := os.Getenv(loc.env)
		if base == "" {
			continue
		}
		candidate := filepath.Join(base, loc.sub, "Tesseract-OCR", "tesseract.exe")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// ocrResult is one Tesseract pass: the text, and how sure the engine was of it.
//
// The confidence is here because without it the tool cannot tell the difference
// between reading an image and failing to. Tesseract asked for tha+eng will
// answer for a page of Chinese too — it returns plausible-looking Thai letters
// and exit code 0, and the digits, which are Arabic either way, come through
// correct and lend the surrounding nonsense credibility. A caller handed only
// the text has no way to know which of the two just happened.
type ocrResult struct {
	Text string
	// Confidence is the mean per-word confidence Tesseract reported, 0-100,
	// or -1 when it found no words to be confident about.
	Confidence float64
	Words      int
}

// ocrLowConfidence is the mean below which a receipt says the text may not be
// the image's.
//
// 70 comes from four images measured 2026-08-18 through this exact code path:
// Thai rendered sharp scored 94.2 over 33 words, the same image downscaled and
// blown back up scored 93.2 over 33, an image half Thai and half Chinese scored
// 79.1 over 12, and a page of Chinese read through tha+eng scored 47.7 over 17.
// Degrading the image barely moved the number because it measures how well a
// glyph matched the model rather than how sharp the pixels were, which is
// exactly the separation wanted here: unreadable is not the same as
// photographed badly.
//
// Measured through this path and not by hand: an eyeball count over the same
// TSV put the Chinese page at 64, because it averaged in rows whose text is
// only whitespace, and those score high. Dropping them is what
// meanWordConfidence does and why the real gap is wider than it first looked.
//
// Four synthetic images are not a corpus. A real slip photographed at an angle,
// with glare, over JPEG, will sit below anything measured here — which is the
// whole reason this number produces a sentence in the output and never a
// refusal. Raising it to 80 would also catch the half-and-half case, at the
// price of warning about real photographs; that trade needs real photographs
// to decide, not more synthetic ones.
const ocrLowConfidence = 70

// lowConfidenceNote states the limit as well as the doubt: a reader told only
// "confidence is low" will retry the same unreadable page, where one told which
// two languages exist can stop.
const lowConfidenceNote = "อ่านมาได้ไม่ค่อยมั่นใจ (%.0f%%) ข้อความข้างบนอาจไม่ตรงกับภาพ เครื่องมือนี้อ่านได้เฉพาะภาษาไทยกับอังกฤษ ถ้าภาพเป็นภาษาอื่นผลที่ได้จะไม่มีความหมาย"

// appendConfidenceNote adds that sentence to text when the reading was poor
// enough to doubt, and returns text untouched otherwise.
//
// Separate from both callers so the rule can be tested without Tesseract on the
// machine. The alternative — asserting it through a real OCR run — would make
// the check skip exactly where it is most likely to regress, since the CI
// runners have no Tesseract and the threshold is a number someone will
// eventually want to move.
//
// A confidence of -1 (Tesseract found nothing to be sure about) is not a low
// score, it is the absence of one, and warning on it would put the sentence
// under every image that legitimately holds no text.
func appendConfidenceNote(text string, mean float64, words int) string {
	if text == "" || words <= 0 || mean < 0 || mean >= ocrLowConfidence {
		return text
	}
	return text + "\n\n" + fmt.Sprintf(lowConfidenceNote, mean)
}

// runTesseract OCRs one image.
//
// It writes to a temp basename rather than reading stdout, because that is what
// buys the confidence: `txt tsv` asks one pass for both the text and the
// per-word detail, at a cost measured at 2ms (182ms → 184ms over five runs).
// Reconstructing the text from the TSV instead would save the file and lose
// Tesseract's own line breaking, which for Thai is already the weaker part of
// the output.
func runTesseract(ctx context.Context, imagePath string) (ocrResult, error) {
	dir, err := os.MkdirTemp("", "aetox-ocr-*")
	if err != nil {
		return ocrResult{}, err
	}
	defer os.RemoveAll(dir)

	base := filepath.Join(dir, "page")
	binary := resolveTesseract()
	cmd := exec.CommandContext(ctx, binary, imagePath, base, "-l", "tha+eng", "txt", "tsv")
	cmd.Env = tesseractEnv(binary)
	proc.HideConsole(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ocrResult{}, err
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return ocrResult{}, errors.New(msg)
	}

	textBytes, err := os.ReadFile(base + ".txt")
	if err != nil {
		return ocrResult{}, err
	}
	result := ocrResult{Text: strings.TrimSpace(string(textBytes)), Confidence: -1}

	// A missing or malformed TSV costs the confidence, not the text. The text
	// is what the caller asked for and it is already in hand; failing the whole
	// read because the second file did not parse would trade the answer for the
	// footnote.
	if tsv, err := os.ReadFile(base + ".tsv"); err == nil {
		result.Confidence, result.Words = meanWordConfidence(string(tsv))
	}
	return result, nil
}

// meanWordConfidence averages the conf column of Tesseract's TSV over the rows
// that are actually words, returning -1 when there are none.
//
// The layout is twelve tab-separated columns with conf at index 10 and the word
// at 11. Rows carrying no text are the page/block/paragraph/line levels, which
// report conf -1, and dropping them is what keeps the mean about the reading
// rather than about the page structure.
func meanWordConfidence(tsv string) (float64, int) {
	var sum float64
	var n int
	for i, line := range strings.Split(tsv, "\n") {
		if i == 0 {
			continue // header
		}
		cols := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(cols) < 12 || strings.TrimSpace(cols[11]) == "" {
			continue
		}
		conf, err := strconv.ParseFloat(cols[10], 64)
		if err != nil || conf < 0 {
			continue
		}
		sum += conf
		n++
	}
	if n == 0 {
		return -1, 0
	}
	return sum / float64(n), n
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
	proc.HideConsole(cmd) // no-op on darwin; keeps the "every exec site" rule exception-free
	return cmd.Run() == nil
}

// missingTesseractError reports the machine's state, not anyone's behaviour:
// whether Tesseract is installed here is true or false no matter how the tool
// was called, and nothing the agent does differently next time makes it
// appear. So it carries the statereport mark (turn.ErrorFromWorld), same as an
// n8n that is not running.
//
// Written plain, it was read as a lesson. Three of these in a row on one
// machine whose NSIS install step had been skipped became an approval card
// proposing "เลี่ยงรูปแบบที่ชนเงื่อนไขนี้ตั้งแต่ครั้งแรก" — permanent memory
// teaching the agent to avoid OCR, drafted from a missing install (2026-08-18).
// The same reading applies to every "this machine does not have X" message in
// the tool layer: pdf_read's poppler, video_ocr's ffmpeg, stt's whisper binary
// and model file, git's own absence from PATH.
func missingTesseractError() error {
	switch runtime.GOOS {
	case "darwin":
		return statereport.New("ไม่พบ Tesseract และติดตั้งอัตโนมัติไม่สำเร็จ (ต้องมี Homebrew) — รันเอง: brew install tesseract tesseract-lang")
	case "linux":
		if hint := linuxInstallHint("tesseract-ocr tesseract-ocr-tha", "tesseract tesseract-langpack-tha", "tesseract-data-tha tesseract"); hint != "" {
			return statereport.Newf("ไม่พบโปรแกรม Tesseract ในเครื่อง — ติดตั้งด้วย: %s", hint)
		}
		return statereport.New("ไม่พบโปรแกรม Tesseract ในเครื่อง — ติดตั้งผ่าน package manager ของดิสโทรคุณ (แพ็กเกจ tesseract-ocr หรือ tesseract พร้อมชุดภาษาไทย)")
	default: // windows and anything else
		return statereport.New("ไม่พบโปรแกรม Tesseract ในเครื่อง — ติดตั้งจาก https://github.com/UB-Mannheim/tesseract/wiki แล้วลองใหม่")
	}
}

// linuxInstallHint returns a ready-to-paste install command for whichever
// package manager is present, given each one's package names for the thing
// being installed. Not auto-run — these all need sudo, and running a
// privileged command silently isn't something to do without the user
// watching, same reasoning as not scripting around Windows' UAC.
func linuxInstallHint(aptPkgs, dnfPkgs, pacmanPkgs string) string {
	switch {
	case commandExists("apt-get"):
		return "sudo apt-get install -y " + aptPkgs
	case commandExists("dnf"):
		return "sudo dnf install -y " + dnfPkgs
	case commandExists("pacman"):
		return "sudo pacman -S " + pacmanPkgs
	default:
		return ""
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
