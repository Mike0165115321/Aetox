package stt

// whisper.cpp: the first engine, and the reference for what a Descriptor's
// constructor owes the rest of the package — resolve the binary, resolve the
// model, translate stdout into []Segment. Everything whisper-specific stops
// here (ARCHITECTURE.md §31, §33).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/proc"
	"github.com/Mike0165115321/Aetox/internal/statereport"
)

// preferredWhisperModel is what the missing-model error tells the user to
// download: the best accuracy-per-megabyte for Thai. Any other ggml-*.bin
// already present is used as-is rather than nagging for this exact one.
const preferredWhisperModel = "ggml-base.bin"

// swapped in tests — production always resolves through PATH.
var lookPath = exec.LookPath

type whisperCPP struct {
	binPath   string
	modelPath string
}

func newWhisperCPP(desc Descriptor, opts Options) (Engine, error) {
	binPath, err := findBinary(desc)
	if err != nil {
		return nil, err
	}
	modelPath, err := resolveModel(desc, opts)
	if err != nil {
		return nil, err
	}
	return &whisperCPP{binPath: binPath, modelPath: modelPath}, nil
}

func (*whisperCPP) ID() string { return "whisper-cpp" }

func (w *whisperCPP) ModelPath() string { return w.modelPath }

// ModelCaution flags the tiny models. The Windows installer ships one so a
// fresh install can transcribe at all, and tiny is the least accurate whisper
// offers — on Thai and on noisy audio it guesses. A wrong transcript nobody
// thought to doubt is worse than a slow one.
//
// ponytail: matches whisper.cpp's own file naming (ggml-tiny*). Its convention,
// checked inside its own file — but still a name check, so a rename upstream
// would quietly stop the warning.
func (w *whisperCPP) ModelCaution() string {
	if strings.HasPrefix(strings.ToLower(filepath.Base(w.modelPath)), "ggml-tiny") {
		return "(ถอดด้วยโมเดล tiny ซึ่งเล็กและแม่นน้อยที่สุด — ถ้าข้อความไม่ตรงกับที่ได้ยิน เปลี่ยนเป็นโมเดลใหญ่กว่าได้ที่ ตั้งค่า → เครื่องมือ → audio_transcribe)"
	}
	return ""
}

func (w *whisperCPP) Transcribe(ctx context.Context, wavPath string) ([]Segment, error) {
	// -l auto detects Thai vs English per file; -np drops whisper's banner and
	// progress lines so stdout is nothing but segments.
	cmd := exec.CommandContext(ctx, w.binPath, "-m", w.modelPath, "-f", wavPath, "-l", "auto", "-np")
	proc.HideConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, missingBinaryError(catalog[0])
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return parseWhisperOutput(stdout.String()), nil
}

// parseWhisperOutput translates
// "[00:00:03.000 --> 00:00:06.000]   สวัสดีครับ" into a Segment, dropping
// anything that is not a segment line. Consecutive repeats collapse — whisper
// loops the same phrase over long silences.
func parseWhisperOutput(raw string) []Segment {
	var segments []Segment
	lastText := ""
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		closing := strings.IndexByte(line, ']')
		arrow := strings.Index(line, "-->")
		if closing < 0 || arrow < 0 || arrow > closing {
			continue
		}
		start, ok := parseWhisperTimestamp(line[1:arrow])
		if !ok {
			continue
		}
		end, endOK := parseWhisperTimestamp(line[arrow+3 : closing])
		if !endOK {
			end = start
		}
		text := strings.TrimSpace(line[closing+1:])
		if text == "" || text == lastText {
			continue
		}
		lastText = text
		segments = append(segments, Segment{StartMs: start, EndMs: end, Text: text})
	}
	return segments
}

// parseWhisperTimestamp reads "HH:MM:SS.mmm" (or "MM:SS.mmm") as milliseconds.
func parseWhisperTimestamp(ts string) (int, bool) {
	ts = strings.TrimSpace(ts)
	millis := 0
	if dot := strings.IndexByte(ts, '.'); dot >= 0 {
		frac := ts[dot+1:]
		ts = ts[:dot]
		for len(frac) < 3 {
			frac += "0"
		}
		n, err := strconv.Atoi(frac[:3])
		if err != nil || n < 0 {
			return 0, false
		}
		millis = n
	}
	parts := strings.Split(ts, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	seconds := 0
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 {
			return 0, false
		}
		seconds = seconds*60 + n
	}
	return seconds*1000 + millis, true
}

func findBinary(desc Descriptor) (string, error) {
	for _, name := range desc.Binaries {
		if path, err := lookPath(name); err == nil {
			return path, nil
		}
	}
	return "", missingBinaryError(desc)
}

// resolveModel prefers an explicitly configured file, then the model the error
// message asks for, then any model matching the engine's glob — someone who
// downloaded tiny or small should not be told to fetch base as well.
func resolveModel(desc Descriptor, opts Options) (string, error) {
	if desc.ModelGlob == "" {
		return "", nil
	}
	if pinned := strings.TrimSpace(opts.ModelPath); pinned != "" {
		if !isRegularFile(pinned) {
			return "", fmt.Errorf("ไฟล์โมเดลที่ตั้งค่าไว้ไม่มีอยู่จริง: %s — เลือกใหม่ในหน้าตั้งค่า", pinned)
		}
		return pinned, nil
	}
	dir, err := ModelDir(opts)
	if err != nil {
		return "", err
	}
	if preferred := filepath.Join(dir, preferredWhisperModel); isRegularFile(preferred) {
		return preferred, nil
	}
	// Managed store first (Stores() orders it that way), so a model Aetox
	// downloaded wins over an identically-named one in someone else's folder.
	for _, found := range InstalledModels(desc, opts) {
		return found.Path, nil
	}
	return "", missingModelError(dir)
}

// ModelDir is where downloaded models live: <DataRoot>/models (ARCHITECTURE.md
// §14), unless the caller pinned another directory.
func ModelDir(opts Options) (string, error) {
	if dir := strings.TrimSpace(opts.ModelDir); dir != "" {
		return dir, nil
	}
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "models"), nil
}

// InstalledModels lists every model file this engine can use, across the
// managed store and the user's own — managed first, then alphabetical within
// each store. The settings UI renders this list directly.
func InstalledModels(desc Descriptor, opts Options) []InstalledModel {
	if desc.ModelGlob == "" {
		return nil
	}
	var found []InstalledModel
	for _, store := range Stores(opts) {
		matches, _ := filepath.Glob(filepath.Join(store.Dir, desc.ModelGlob))
		sort.Strings(matches)
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			found = append(found, InstalledModel{
				Path:    match,
				Name:    filepath.Base(match),
				Bytes:   info.Size(),
				Store:   store.Label,
				Managed: store.Managed,
			})
		}
	}
	return found
}

func isRegularFile(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Both of these are state reports (internal/statereport), not lessons: a binary
// that is not installed and a model file that was never downloaded are facts
// about this machine, true or false no matter how the tool was called, so the
// learning summarizer must not read them as behaviour to correct.
func missingBinaryError(desc Descriptor) error {
	return statereport.Newf("ไม่พบโปรแกรม %s ในเครื่อง — %s", desc.Label, desc.Install)
}

func missingModelError(dir string) error {
	return statereport.Newf("ยังไม่มีไฟล์โมเดลถอดเสียงในเครื่อง — Aetox ไม่โหลดให้เองเพราะไฟล์ใหญ่ ~141 MB ต้องให้คุณตัดสินใจก่อน\n"+
		"1) โหลดไฟล์: https://huggingface.co/ggerganov/whisper.cpp/resolve/main/%s\n"+
		"2) วางไว้ที่: %s\n"+
		"ถ้าพื้นที่หรือเน็ตจำกัด ใช้ ggml-tiny-q5_1.bin (~31 MB) แทนได้ในโฟลเดอร์เดียวกัน แลกกับความแม่นที่ลดลง",
		preferredWhisperModel, filepath.Join(dir, preferredWhisperModel))
}
