package tts

// Piper (rhasspy/piper): the second read-aloud vendor, a local neural TTS that
// runs entirely offline. Its voices ARE files — one .onnx per voice — so this
// engine's Voices() enumerates disk rather than a registry, and the voice ID
// is the file's absolute path.
//
// §32 discipline, same as speech models: Aetox never downloads a voice. A
// missing one is an error naming where the voices live
// (https://huggingface.co/rhasspy/piper-voices) and where to put the file.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/proc"
)

// swapped in tests — production always runs the real binary.
var runPiper = execPiper

type piperVoice struct {
	binPath string
	voice   string // Voice.ID = absolute path of an .onnx voice; "" = first found
}

func newPiper(desc Descriptor, opts Options) (Engine, error) {
	binPath, err := findPiperBinary(desc)
	if err != nil {
		return nil, err
	}
	return &piperVoice{binPath: binPath, voice: strings.TrimSpace(opts.Voice)}, nil
}

func (*piperVoice) ID() string { return "piper" }

func (*piperVoice) Mime() string { return "audio/wav" }

// Voices lists every .onnx voice in the managed model store. Piper's file
// names carry the locale ("th_TH-...", "en_US-lessac-medium.onnx"), which is
// the only language information a bare file offers.
func (p *piperVoice) Voices(_ context.Context) ([]Voice, error) {
	var voices []Voice
	for _, dir := range piperVoiceDirs() {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.onnx"))
		sort.Strings(matches)
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			name := filepath.Base(match)
			voices = append(voices, Voice{
				ID:   match,
				Name: strings.TrimSuffix(name, ".onnx"),
				Lang: piperLang(name),
			})
		}
	}
	return voices, nil
}

func (p *piperVoice) Synthesize(ctx context.Context, text, wavPath string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("ไม่มีข้อความให้อ่าน")
	}
	model := p.voice
	if model == "" {
		voices, err := p.Voices(ctx)
		if err != nil {
			return err
		}
		if len(voices) == 0 {
			return missingPiperVoiceError()
		}
		model = voices[0].ID
	}
	if info, err := os.Stat(model); err != nil || info.IsDir() {
		return fmt.Errorf("ไม่พบไฟล์เสียง %s แล้ว — เลือกใหม่ในหน้าตั้งค่า > เสียง", model)
	}
	return runPiper(ctx, p.binPath, model, text, wavPath)
}

func execPiper(ctx context.Context, binPath, model, text, wavPath string) error {
	// Text through stdin — piper's own contract, and the one channel with no
	// quoting rules to get wrong.
	cmd := exec.CommandContext(ctx, binPath, "-m", model, "-f", wavPath)
	proc.HideConsole(cmd)
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return missingPiperError()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("piper อ่านออกเสียงไม่สำเร็จ (%s)", msg)
	}
	return nil
}

// piperVoiceDirs is where voices are looked for: piper's own folder in the
// managed store first, then the store root for files dropped loose.
func piperVoiceDirs() []string {
	root, err := config.DataRoot()
	if err != nil {
		return nil
	}
	models := filepath.Join(root, "models")
	return []string{filepath.Join(models, "piper"), models}
}

// piperLang reads the locale off a voice file name ("th_TH-voice-medium.onnx"
// → "th-TH"). No prefix means no claim.
func piperLang(fileName string) string {
	head, _, ok := strings.Cut(fileName, "-")
	if !ok {
		return ""
	}
	head = strings.TrimSpace(head)
	if i := strings.IndexByte(head, '_'); i > 0 {
		return head[:i] + "-" + head[i+1:]
	}
	return ""
}

// findPiperBinary is stt.findBinary's shape for this package: PATH first (a
// piper someone installed is the one they chose), then the copy a capability
// install could place in the managed tools folder.
func findPiperBinary(desc Descriptor) (string, error) {
	for _, name := range desc.Binaries {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	if root, err := config.DataRoot(); err == nil {
		for _, name := range desc.Binaries {
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			candidate := filepath.Join(root, "tools", "piper", name)
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	return "", missingPiperError()
}

func missingPiperError() error {
	return fmt.Errorf("ไม่พบโปรแกรม piper ในเครื่อง — โหลด release ของ Windows จาก https://github.com/rhasspy/piper แล้ววางโฟลเดอร์ไว้ที่ <DataRoot>\\tools\\piper หรือให้มันอยู่บน PATH")
}

func missingPiperVoiceError() error {
	dirs := piperVoiceDirs()
	where := "<DataRoot>\\models\\piper"
	if len(dirs) > 0 {
		where = dirs[0]
	}
	return fmt.Errorf("ยังไม่มีไฟล์เสียงของ piper ในเครื่อง — โหลดไฟล์ .onnx (คู่กับ .onnx.json) จาก https://huggingface.co/rhasspy/piper-voices แล้ววางไว้ที่ %s", where)
}
