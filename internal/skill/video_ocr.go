package skill

// video_ocr reads on-screen text out of a video the model can't watch:
// ffmpeg samples one frame every few seconds into a temp dir, each frame runs
// through the same Tesseract path image_ocr uses, and the hits come back as
// "[m:ss] text" lines (consecutive duplicates collapsed, so a static title
// doesn't repeat every sample).
// ponytail: fixed-interval sampling misses text shown briefly between samples;
// switch the -vf to select='gt(scene,0.3)' scene detection if that matters.

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

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

const (
	videoOCRDefaultIntervalSec = 5
	// ponytail: hard cap so a long video doesn't OCR thousands of frames;
	// page through offsets if someone actually feeds feature-length videos.
	videoOCRMaxFrames = 120
)

type videoOCRSkill struct {
	root string
}

func (*videoOCRSkill) Name() string { return "video_ocr" }

func (*videoOCRSkill) Description() string {
	return "อ่านข้อความจากในวิดีโอ (แตกเฟรมทุก N วินาทีแล้ว OCR) — ใช้เมื่อโมเดลปัจจุบันดูวิดีโอไม่ได้"
}

func (*videoOCRSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path (under sandbox root) to the video file",
			},
			"interval_seconds": map[string]any{
				"type":        "integer",
				"description": "Sample one frame every this many seconds (default 5, min 1, max 60)",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "video_ocr",
			Description: "Extract on-screen text from a video via frame sampling + OCR (ffmpeg + Tesseract, Thai+English). Use this to read a video's content when you cannot watch it directly. Returns '[m:ss] text' lines.",
			Parameters:  payload,
		},
	}
}

func (s *videoOCRSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: video_ocr <path> [interval_seconds]")
		return newToolOutput("video_ocr", "video_ocr", "", start, false, err), err
	}
	interval := videoOCRDefaultIntervalSec
	if len(args) > 1 {
		if n, err := strconv.Atoi(strings.TrimSpace(args[len(args)-1])); err == nil {
			interval = n
			args = args[:len(args)-1]
		}
	}
	return s.run(ctx, start, strings.TrimSpace(strings.Join(args, " ")), interval)
}

func (s *videoOCRSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		err := errors.New("path is required")
		return newToolOutput("video_ocr", "video_ocr", "", time.Now(), false, err), err
	}
	interval := videoOCRDefaultIntervalSec
	if n, ok := args["interval_seconds"].(float64); ok {
		interval = int(n)
	}
	return s.run(ctx, time.Now(), path, interval)
}

func (s *videoOCRSkill) run(ctx context.Context, start time.Time, requestPath string, intervalSec int) (Output, error) {
	command := "video_ocr " + requestPath
	if intervalSec < 1 {
		intervalSec = 1
	} else if intervalSec > 60 {
		intervalSec = 60
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("video_ocr", command, "", start, false, err), err
	}

	if _, err := exec.LookPath("tesseract"); err != nil {
		if !tryAutoInstallTesseract(ctx) {
			err := missingTesseractError()
			return newToolOutput("video_ocr", command, "", start, false, err), err
		}
	}

	tmpDir, err := os.MkdirTemp("", "aetox-video-ocr-*")
	if err != nil {
		return newToolOutput("video_ocr", command, "", start, false, err), err
	}
	defer os.RemoveAll(tmpDir)

	frames, err := extractFrames(ctx, targetPath, tmpDir, intervalSec)
	if err != nil {
		return newToolOutput("video_ocr", command, "", start, false, err), err
	}
	if len(frames) == 0 {
		err := errors.New("แตกเฟรมจากวิดีโอไม่ได้ — ไฟล์อาจไม่ใช่วิดีโอหรือเสียหาย")
		return newToolOutput("video_ocr", command, "", start, false, err), err
	}

	var lines []string
	lastText := ""
	for i, frame := range frames {
		text, ocrErr := runTesseract(ctx, frame)
		if ocrErr != nil {
			return newToolOutput("video_ocr", command, "", start, false, ocrErr), ocrErr
		}
		if text == "" || text == lastText {
			continue
		}
		lastText = text
		sec := i * intervalSec
		lines = append(lines, fmt.Sprintf("[%d:%02d] %s", sec/60, sec%60, text))
	}

	result := strings.Join(lines, "\n")
	if result == "" {
		result = "(ไม่พบข้อความในวิดีโอ)"
	}
	if len(frames) == videoOCRMaxFrames {
		result += fmt.Sprintf("\n(อ่านถึงเฟรมที่ %d เท่านั้น ≈ วินาทีที่ %d — วิดีโอส่วนท้ายอาจถูกตัด)", videoOCRMaxFrames, videoOCRMaxFrames*intervalSec)
	}
	truncated, wasTruncated := limitLines(result, defaultToolOutputLineLimit)
	return newToolOutput("video_ocr", command, truncated, start, wasTruncated, nil), nil
}

func extractFrames(ctx context.Context, videoPath, outDir string, intervalSec int) ([]string, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=1/%d", intervalSec),
		"-frames:v", strconv.Itoa(videoOCRMaxFrames),
		filepath.Join(outDir, "frame_%04d.png"),
	)
	proc.HideConsole(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, missingFFmpegError()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	frames, err := filepath.Glob(filepath.Join(outDir, "frame_*.png"))
	if err != nil {
		return nil, err
	}
	sort.Strings(frames)
	return frames, nil
}

func missingFFmpegError() error {
	switch runtime.GOOS {
	case "darwin":
		return errors.New("ไม่พบโปรแกรม ffmpeg ในเครื่อง — ติดตั้งด้วย: brew install ffmpeg")
	case "linux":
		return errors.New("ไม่พบโปรแกรม ffmpeg ในเครื่อง — ติดตั้งผ่าน package manager ของดิสโทรคุณ (แพ็กเกจ ffmpeg)")
	default: // windows and anything else
		return errors.New("ไม่พบโปรแกรม ffmpeg ในเครื่อง — ติดตั้งด้วย: winget install ffmpeg (หรือ scoop install ffmpeg) แล้วลองใหม่")
	}
}
