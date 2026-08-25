package skill

// audio_transcribe gives the agent ears. ffmpeg strips a 16kHz mono WAV out of
// whatever it's handed (audio file or video — one command covers both), an
// internal/stt engine transcribes it, and the segments come back as "[m:ss]
// text": byte-identical in shape to video_ocr, so both tools can be run over
// one clip and read as a single transcript.
//
// This file knows nothing about whisper, ggml or any other engine — picking
// one, finding its binary and its model, and translating its output into
// []stt.Segment all live in internal/stt (ARCHITECTURE.md §33). What is left
// here is the part that is genuinely this skill's: sandboxing the path,
// producing a WAV, and formatting timestamps the way video_ocr does.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/proc"
	"github.com/Mikedev115/Aetox/internal/stt"
)

type audioTranscribeSkill struct {
	root   string
	speech stt.Options
	// newEngine is swapped in tests to exercise this file without a real
	// engine; production always builds from the catalog.
	newEngine func(stt.Options) (stt.Engine, error)
}

func (*audioTranscribeSkill) Name() string { return "audio_transcribe" }

func (*audioTranscribeSkill) Description() string {
	return "ถอดเสียงพูดในไฟล์เสียงหรือวิดีโอเป็นข้อความพร้อมเวลากำกับ (ถอดในเครื่อง ไทย+อังกฤษ) — ใช้เมื่อโมเดลปัจจุบันฟังเสียงไม่ได้"
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
			Description: "Transcribe spoken words from an audio or video file into text, offline (auto-detected language, Thai+English). Use this to hear a file's content when you cannot listen to it — including a video whose screen has no text for video_ocr to read. Returns '[m:ss] text' lines.",
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

	// Resolve the engine before spending ffmpeg time: a missing binary or model
	// is the most likely failure and its error is the same either way.
	build := s.newEngine
	if build == nil {
		build = stt.New
	}
	engine, err := build(s.speech)
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

	segments, err := engine.Transcribe(ctx, wavPath)
	if err != nil {
		return fail(err)
	}

	result := formatSegments(segments)
	if result == "" {
		result = "(ไม่พบเสียงพูดในไฟล์)"
	}
	// Appended to the transcript, not logged: the caution has to travel with the
	// text to whoever reads it. What it says is the engine's to decide — this
	// file still knows nothing about which models exist or what they are called.
	if note := engine.ModelCaution(); note != "" {
		result += "\n\n" + note
	}
	truncated, wasTruncated := limitLines(result, defaultToolOutputLineLimit)
	return newToolOutput("audio_transcribe", command, truncated, start, wasTruncated, nil), nil
}

// formatSegments renders "[m:ss] text" — the same shape video_ocr emits, so a
// clip run through both tools reads as one transcript.
func formatSegments(segments []stt.Segment) string {
	lines := make([]string, 0, len(segments))
	for _, seg := range segments {
		sec := seg.StartMs / 1000
		lines = append(lines, fmt.Sprintf("[%d:%02d] %s", sec/60, sec%60, seg.Text))
	}
	return strings.Join(lines, "\n")
}

// extractAudioTrack normalizes anything ffmpeg can open into the 16kHz mono
// PCM every engine in internal/stt expects. -vn makes a video file just another
// audio source, so audio and video inputs need no branch here.
func extractAudioTrack(ctx context.Context, inputPath, wavPath string) error {
	cmd := exec.CommandContext(ctx, bundledBinary("ffmpeg", "ffmpeg"),
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
