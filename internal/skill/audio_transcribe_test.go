package skill

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/stt"
)

// fakeEngine stands in for internal/stt so this file tests what is actually
// this skill's job — sandboxing, WAV extraction, formatting, cleanup — without
// dragging a real engine in. The engine's own command line is covered by
// internal/stt's helper-process test.
type fakeEngine struct {
	segments []stt.Segment
	err      error
	gotWav   string
}

func (*fakeEngine) ID() string { return "fake" }

func (f *fakeEngine) Transcribe(_ context.Context, wavPath string) ([]stt.Segment, error) {
	f.gotWav = wavPath
	return f.segments, f.err
}

func withEngine(s *audioTranscribeSkill, engine stt.Engine, err error) *audioTranscribeSkill {
	s.newEngine = func(stt.Options) (stt.Engine, error) { return engine, err }
	return s
}

func TestAudioTranscribeRejectsEscape(t *testing.T) {
	s := &audioTranscribeSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{"../outside.mp3"}}); err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestAudioTranscribeUsageError(t *testing.T) {
	s := &audioTranscribeSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{}}); err == nil {
		t.Fatal("expected usage error for missing path, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected usage error for missing path arg, got nil")
	}
}

func TestAudioTranscribeMissingFile(t *testing.T) {
	s := &audioTranscribeSkill{root: t.TempDir()}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "nope.mp3"})
	if err == nil {
		t.Fatal("expected error for a file that does not exist, got nil")
	}
	if !strings.Contains(err.Error(), "ไม่พบไฟล์") {
		t.Errorf("error should say the file is missing, in Thai; got: %v", err)
	}
	if out.Success {
		t.Error("Success = true on a missing file")
	}
}

// The engine's setup error (no binary, no model) has to reach the user
// unchanged — it is the only thing telling them how to fix the problem.
func TestAudioTranscribeSurfacesEngineSetupError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "voice.mp3"), []byte("not really audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	setupErr := errors.New("ไม่พบโปรแกรม whisper.cpp (ggml) ในเครื่อง — ติดตั้งด้วย: scoop install whisper-cpp")
	s := withEngine(&audioTranscribeSkill{root: root}, nil, setupErr)

	_, err := s.ExecuteTool(context.Background(), map[string]any{"path": "voice.mp3"})
	if err == nil || err.Error() != setupErr.Error() {
		t.Errorf("engine setup error = %v, want it passed through verbatim: %v", err, setupErr)
	}
}

// The whole pipeline on a real file with the engine faked: ffmpeg really
// converts an .mp4 (the branch -vn has to handle), the engine really receives a
// .wav, segments are really formatted as [m:ss], and the temp dir is gone.
func TestAudioTranscribePipeline(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}

	root := t.TempDir()
	clip := filepath.Join(root, "clip.mp4")
	build := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=320x120:d=3:r=5",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-shortest", clip)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test video with audio: %v — %s", err, out)
	}

	engine := &fakeEngine{segments: []stt.Segment{
		{StartMs: 0, EndMs: 2480, Text: "สวัสดีครับ นี่คือเสียงทดสอบ"},
		{StartMs: 64000, EndMs: 67000, Text: "And this line is in English."},
	}}
	tempBefore := countAetoxAudioTempDirs()

	s := withEngine(&audioTranscribeSkill{root: root}, engine, nil)
	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "clip.mp4"})
	if err != nil {
		t.Fatalf("audio_transcribe failed: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false, stderr: %s", out.Stderr)
	}
	want := "[0:00] สวัสดีครับ นี่คือเสียงทดสอบ\n[1:04] And this line is in English."
	if out.Content != want {
		t.Errorf("Content =\n%s\nwant:\n%s", out.Content, want)
	}
	if !strings.HasSuffix(engine.gotWav, ".wav") {
		t.Errorf("engine received %q, want a converted .wav", engine.gotWav)
	}
	if _, err := os.Stat(engine.gotWav); err == nil {
		t.Error("temp WAV still exists after the call — defer os.RemoveAll did not run")
	}
	if after := countAetoxAudioTempDirs(); after > tempBefore {
		t.Errorf("temp dirs leaked: %d before, %d after", tempBefore, after)
	}
}

func TestAudioTranscribeEmptyTranscriptStillSaysSomething(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}
	root := t.TempDir()
	clip := filepath.Join(root, "tone.wav")
	build := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1", clip)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test audio: %v — %s", err, out)
	}

	s := withEngine(&audioTranscribeSkill{root: root}, &fakeEngine{}, nil)
	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "tone.wav"})
	if err != nil {
		t.Fatalf("audio_transcribe failed: %v", err)
	}
	if strings.TrimSpace(out.Content) == "" {
		t.Error("silence must still report the no-speech line, not an empty tool result")
	}
}

func countAetoxAudioTempDirs() int {
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "aetox-audio-*"))
	return len(matches)
}

// End-to-end with the real engine, real binary and real model — skips unless
// this machine has all three. This is the only test that proves whisper.cpp's
// actual output survives the parser.
func TestAudioTranscribeLive(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}
	if _, err := stt.New(stt.Options{}); err != nil {
		t.Skipf("no usable speech engine on this machine: %v", err)
	}

	root := t.TempDir()
	clip := filepath.Join(root, "tone.wav")
	build := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3", clip)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test audio: %v — %s", err, out)
	}

	s := &audioTranscribeSkill{root: root}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "tone.wav"})
	if err != nil {
		t.Fatalf("audio_transcribe failed against the real engine: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false, stderr: %s", out.Stderr)
	}
	if strings.TrimSpace(out.Content) == "" {
		t.Error("content should never be empty — silence still reports the no-speech line")
	}
	t.Logf("real engine output:\n%s", out.Content)
}
