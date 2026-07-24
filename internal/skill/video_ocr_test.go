package skill

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoOCRSkillRejectsEscape(t *testing.T) {
	s := &videoOCRSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"../outside.mp4"}})
	if err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestVideoOCRSkillUsageError(t *testing.T) {
	s := &videoOCRSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{}}); err == nil {
		t.Fatal("expected usage error for missing path, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected usage error for missing path arg, got nil")
	}
}

func TestMissingFFmpegErrorNeverEmpty(t *testing.T) {
	if err := missingFFmpegError(); err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Errorf("missingFFmpegError() = %v, want a non-empty actionable message", err)
	}
}

// End-to-end on machines that have ffmpeg + tesseract (the dev box does):
// synthesizes a 12s video showing one text for the first half and another for
// the second, then asserts both come back OCR'd, in order, with the first
// timestamped at 0:00 — and only once each despite being sampled repeatedly.
func TestVideoOCRSkillLiveExtractsTimedText(t *testing.T) {
	for _, bin := range []string{"ffmpeg", "tesseract"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed on this machine", bin)
		}
	}
	font := testFontFile()
	if font == "" {
		t.Skip("no known system font available for drawtext")
	}

	root := t.TempDir()
	video := filepath.Join(root, "clip.mp4")
	font = strings.Replace(font, ":", `\:`, 1) // C: would read as a filtergraph separator
	drawtext := func(label, text string) string {
		return fmt.Sprintf("drawtext=fontfile='%s':text='%s':fontsize=48:fontcolor=white:x=40:y=90%s", font, text, label)
	}
	filter := "[0:v]" + drawtext("[a]", "HELLO AETOX") + ";[1:v]" + drawtext("[b]", "SECOND SCENE") + ";[a][b]concat=n=2:v=1:a=0"
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=640x240:d=6:r=5",
		"-f", "lavfi", "-i", "color=c=black:s=640x240:d=6:r=5",
		"-filter_complex", filter, video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test video (ffmpeg without lavfi/drawtext?): %v — %s", err, out)
	}

	s := &videoOCRSkill{root: root}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "clip.mp4", "interval_seconds": float64(3)})
	if err != nil {
		t.Fatalf("video_ocr failed: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false, stderr: %s", out.Stderr)
	}
	first := strings.Index(out.Content, "HELLO AETOX")
	second := strings.Index(out.Content, "SECOND SCENE")
	if first < 0 || second < 0 {
		t.Fatalf("expected both texts in output, got:\n%s", out.Content)
	}
	if first > second {
		t.Errorf("texts out of order (frame timestamps wrong?):\n%s", out.Content)
	}
	if !strings.Contains(out.Content, "[0:00] HELLO AETOX") {
		t.Errorf("first text should be timestamped [0:00], got:\n%s", out.Content)
	}
	if strings.Count(out.Content, "HELLO AETOX") != 1 {
		t.Errorf("consecutive duplicate frames not collapsed:\n%s", out.Content)
	}
}

func testFontFile() string {
	candidates := []string{
		`C:\Windows\Fonts\arial.ttf`,
		"/System/Library/Fonts/Helvetica.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return filepath.ToSlash(c)
		}
	}
	return ""
}
