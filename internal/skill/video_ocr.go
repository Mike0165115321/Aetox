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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/proc"
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
	return "อ่านข้อความจากในวิดีโอ (แตกเฟรมทุก N วินาทีแล้ว OCR), ใช้เมื่อโมเดลปัจจุบันดูวิดีโอไม่ได้"
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

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("video_ocr", command, "", start, false, err), err
	}
	result, err := OCRVideoFile(ctx, targetPath, intervalSec)
	if err != nil {
		return newToolOutput("video_ocr", command, "", start, false, err), err
	}
	truncated, wasTruncated := limitLines(result, defaultToolOutputLineLimit)
	return newToolOutput("video_ocr", command, truncated, start, wasTruncated, nil), nil
}

// OCRVideoFile reads the on-screen text of one video file already resolved to
// an absolute path. Exported for `video render` to read its own output back in
// the same reply, so proving a render costs the caller zero extra turns; the
// video_ocr tool above is the same read for a file the model names itself.
func OCRVideoFile(ctx context.Context, targetPath string, intervalSec int) (string, error) {
	if intervalSec < 1 {
		intervalSec = 1
	} else if intervalSec > 60 {
		intervalSec = 60
	}

	// resolveTesseract, not exec.LookPath: our Windows installer leaves
	// Tesseract off PATH (see image_ocr.go), and failing here would waste the
	// frame extraction that follows on a machine that can in fact OCR.
	if !tesseractAvailable() {
		if !tryAutoInstallTesseract(ctx) {
			return "", missingTesseractError()
		}
	}

	tmpDir, err := os.MkdirTemp("", "aetox-video-ocr-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	frames, err := extractFrames(ctx, targetPath, tmpDir, intervalSec)
	if err != nil {
		return "", err
	}
	if len(frames) == 0 {
		return "", errors.New("แตกเฟรมจากวิดีโอไม่ได้, ไฟล์อาจไม่ใช่วิดีโอหรือเสียหาย")
	}

	var lines []string
	lastText := ""
	// Confidence is weighted by word count and reported once for the clip, not
	// per frame: a hundred frames each carrying their own percentage would bury
	// the transcript the tool exists to produce, and the question being answered
	// ("is this text really what is on screen") is about the video, not about
	// frame 37. Weighting by words rather than averaging the averages keeps a
	// frame holding two words from counting as much as one holding forty.
	var confSum float64
	var confWords int
	for i, frame := range frames {
		res, ocrErr := runTesseract(ctx, frame)
		if ocrErr != nil {
			return "", ocrErr
		}
		if res.Words > 0 && res.Confidence >= 0 {
			confSum += res.Confidence * float64(res.Words)
			confWords += res.Words
		}
		if res.Text == "" || res.Text == lastText {
			continue
		}
		lastText = res.Text
		sec := i * intervalSec
		lines = append(lines, fmt.Sprintf("[%d:%02d] %s", sec/60, sec%60, res.Text))
	}

	result := strings.Join(lines, "\n")
	if result == "" {
		result = "(ไม่พบข้อความในวิดีโอ)"
	} else if confWords > 0 {
		result = appendConfidenceNote(result, confSum/float64(confWords), confWords)
	}
	if len(frames) == videoOCRMaxFrames {
		result += fmt.Sprintf("\n(อ่านถึงเฟรมที่ %d เท่านั้น ≈ วินาทีที่ %d, วิดีโอส่วนท้ายอาจถูกตัด)", videoOCRMaxFrames, videoOCRMaxFrames*intervalSec)
	}
	return result, nil
}

func extractFrames(ctx context.Context, videoPath, outDir string, intervalSec int) ([]string, error) {
	cmd := exec.CommandContext(ctx, bundledBinary("ffmpeg", "ffmpeg"),
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
