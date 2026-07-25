package skill

// audio_transcribe gives the agent ears. ffmpeg strips a 16kHz mono WAV out of
// whatever it's handed (audio file or video — the same command covers both),
// whisper.cpp transcribes it locally, and the segments come back as "[m:ss]
// text" lines: byte-identical in shape to video_ocr, so both tools can be run
// over one clip and read as a single transcript.
//
// Local binary, not a cloud API, on purpose — audio leaving the machine would
// contradict the one promise Aetox actually makes.
//
// The ggml model is deliberately NOT bundled: base is ~142MB against a ~12MB
// installer, and downloading it silently on first use would spend the user's
// bandwidth without asking. Missing model = an error that says what to fetch,
// how big it is, and where to put it.
// ponytail: whisper.cpp's own timestamps are taken as-is; no VAD, no diarization.

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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

// whisper.cpp renamed its CLI from `main` to `whisper-cli`; Homebrew ships it
// as `whisper-cpp`. Both take the same flags. `main` is too generic a name to
// go looking for on PATH.
var whisperBinaryNames = []string{"whisper-cli", "whisper-cpp"}

// whisperDefaultModel is what the error message tells users to download: the
// best accuracy-per-megabyte for Thai. Any other ggml-*.bin already sitting in
// the models dir is used as-is rather than nagging for this exact one.
const whisperDefaultModel = "ggml-base.bin"

// swapped in tests — production always resolves through PATH.
var whisperLookPath = exec.LookPath

type audioTranscribeSkill struct {
	root string
}

func (*audioTranscribeSkill) Name() string { return "audio_transcribe" }

func (*audioTranscribeSkill) Description() string {
	return "ถอดเสียงพูดในไฟล์เสียงหรือวิดีโอเป็นข้อความพร้อมเวลากำกับ (whisper.cpp ในเครื่อง ไทย+อังกฤษ) — ใช้เมื่อโมเดลปัจจุบันฟังเสียงไม่ได้"
}

func (*audioTranscribeSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path (under sandbox root) to an audio or video file",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "audio_transcribe",
			Description: "Transcribe spoken words from an audio or video file into text, offline (whisper.cpp, auto-detected language, Thai+English). Use this to hear a file's content when you cannot listen to it — including a video whose screen has no text for video_ocr to read. Returns '[m:ss] text' lines.",
			Parameters:  payload,
		},
	}
}

func (s *audioTranscribeSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: audio_transcribe <path>")
		return newToolOutput("audio_transcribe", "audio_transcribe", "", start, false, err), err
	}
	return s.run(ctx, start, strings.TrimSpace(strings.Join(args, " ")))
}

func (s *audioTranscribeSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		err := errors.New("path is required")
		return newToolOutput("audio_transcribe", "audio_transcribe", "", time.Now(), false, err), err
	}
	return s.run(ctx, time.Now(), path)
}

func (s *audioTranscribeSkill) run(ctx context.Context, start time.Time, requestPath string) (Output, error) {
	command := "audio_transcribe " + requestPath
	fail := func(err error) (Output, error) {
		return newToolOutput("audio_transcribe", command, "", start, false, err), err
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return fail(err)
	}
	if info, statErr := os.Stat(targetPath); statErr != nil || info.IsDir() {
		return fail(fmt.Errorf("ไม่พบไฟล์ %s ใน workspace — ตรวจชื่อไฟล์และที่อยู่อีกครั้ง", requestPath))
	}

	binPath, err := whisperBinary()
	if err != nil {
		return fail(err)
	}
	modelPath, err := whisperModelPath()
	if err != nil {
		return fail(err)
	}

	tmpDir, err := os.MkdirTemp("", "aetox-audio-*")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(tmpDir)

	wavPath := filepath.Join(tmpDir, "audio.wav")
	if err := extractAudioTrack(ctx, targetPath, wavPath); err != nil {
		return fail(err)
	}

	raw, err := runWhisper(ctx, binPath, modelPath, wavPath)
	if err != nil {
		return fail(err)
	}

	result := strings.Join(parseWhisperSegments(raw), "\n")
	if result == "" {
		result = "(ไม่พบเสียงพูดในไฟล์)"
	}
	truncated, wasTruncated := limitLines(result, defaultToolOutputLineLimit)
	return newToolOutput("audio_transcribe", command, truncated, start, wasTruncated, nil), nil
}

// extractAudioTrack normalizes anything ffmpeg can open into the 16kHz mono
// PCM whisper.cpp expects. -vn makes a video file just another audio source,
// so audio and video inputs need no branch here.
func extractAudioTrack(ctx context.Context, inputPath, wavPath string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", inputPath,
		"-vn", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le",
		wavPath,
	)
	proc.HideConsole(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return missingFFmpegError()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ดึงเสียงออกจากไฟล์ไม่ได้ — ไฟล์อาจไม่มีแทร็กเสียงหรือเสียหาย (%s)", msg)
	}
	return nil
}

func runWhisper(ctx context.Context, binPath, modelPath, wavPath string) (string, error) {
	// -l auto detects Thai vs English per file; -np drops whisper's banner and
	// progress lines so stdout is nothing but segments.
	cmd := exec.CommandContext(ctx, binPath, "-m", modelPath, "-f", wavPath, "-l", "auto", "-np")
	proc.HideConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", missingWhisperError()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return stdout.String(), nil
}

// parseWhisperSegments turns whisper.cpp's
// "[00:00:03.000 --> 00:00:06.000]   สวัสดีครับ" into "[0:03] สวัสดีครับ",
// dropping anything that isn't a segment line. Consecutive repeats collapse —
// whisper loops the same phrase over long silences.
func parseWhisperSegments(raw string) []string {
	var lines []string
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
		sec, ok := parseWhisperTimestamp(line[1:arrow])
		if !ok {
			continue
		}
		text := strings.TrimSpace(line[closing+1:])
		if text == "" || text == lastText {
			continue
		}
		lastText = text
		lines = append(lines, fmt.Sprintf("[%d:%02d] %s", sec/60, sec%60, text))
	}
	return lines
}

// parseWhisperTimestamp reads "HH:MM:SS.mmm" (or "MM:SS.mmm") as whole seconds.
func parseWhisperTimestamp(ts string) (int, bool) {
	ts = strings.TrimSpace(ts)
	if dot := strings.IndexByte(ts, '.'); dot >= 0 {
		ts = ts[:dot]
	}
	parts := strings.Split(ts, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	total := 0
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 {
			return 0, false
		}
		total = total*60 + n
	}
	return total, true
}

func whisperBinary() (string, error) {
	for _, name := range whisperBinaryNames {
		if path, err := whisperLookPath(name); err == nil {
			return path, nil
		}
	}
	return "", missingWhisperError()
}

// whisperModelPath prefers the model the error message asks for, but accepts
// any ggml-*.bin already in the models dir — someone who downloaded tiny or
// small should not be told to fetch base as well.
func whisperModelPath() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "models")
	if preferred := filepath.Join(dir, whisperDefaultModel); isRegularFile(preferred) {
		return preferred, nil
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "ggml-*.bin"))
	sort.Strings(matches)
	for _, match := range matches {
		if isRegularFile(match) {
			return match, nil
		}
	}
	return "", missingWhisperModelError(dir)
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func missingWhisperModelError(dir string) error {
	return fmt.Errorf("ยังไม่มีไฟล์โมเดลถอดเสียงในเครื่อง — Aetox ไม่โหลดให้เองเพราะไฟล์ใหญ่ ~142 MB ต้องให้คุณตัดสินใจก่อน\n"+
		"1) โหลดไฟล์: https://huggingface.co/ggerganov/whisper.cpp/resolve/main/%s\n"+
		"2) วางไว้ที่: %s\n"+
		"ถ้าพื้นที่หรือเน็ตจำกัด ใช้ ggml-tiny.bin (~75 MB) แทนได้ในโฟลเดอร์เดียวกัน แลกกับความแม่นที่ลดลง",
		whisperDefaultModel, filepath.Join(dir, whisperDefaultModel))
}

func missingWhisperError() error {
	switch runtime.GOOS {
	case "darwin":
		return errors.New("ไม่พบโปรแกรม whisper.cpp ในเครื่อง — ติดตั้งด้วย: brew install whisper-cpp")
	case "linux":
		return errors.New("ไม่พบโปรแกรม whisper.cpp ในเครื่อง — build จาก https://github.com/ggml-org/whisper.cpp แล้วให้คำสั่ง whisper-cli อยู่ใน PATH")
	default: // windows and anything else
		return errors.New("ไม่พบโปรแกรม whisper.cpp ในเครื่อง — โหลด binary จาก https://github.com/ggml-org/whisper.cpp/releases แตกไฟล์แล้ววาง whisper-cli.exe ไว้ในโฟลเดอร์ที่อยู่ใน PATH")
	}
}
