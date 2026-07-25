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

// TestMain doubles as a stand-in for whisper.cpp. TestAudioTranscribePipeline
// re-execs this very test binary in the binary's place, and the env var below
// tells the child to behave like whisper instead of running the suite — the
// os/exec helper-process idiom, so the pipeline can be exercised end to end on
// a machine with no whisper.cpp and no 142MB model downloaded.
func TestMain(m *testing.M) {
	if canned, ok := os.LookupEnv("AETOX_TEST_FAKE_WHISPER"); ok {
		os.Exit(fakeWhisperMain(canned))
	}
	os.Exit(m.Run())
}

// fakeWhisperMain asserts it was invoked the way whisper.cpp expects before
// printing canned segments: if the production code ever passes a model that
// isn't there, an input that isn't a converted WAV, or drops -l auto / -np,
// this exits non-zero and the calling test fails.
func fakeWhisperMain(canned string) int {
	values, flags := map[string]string{}, map[string]bool{}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
			values[arg] = os.Args[i+1]
			i++
			continue
		}
		flags[arg] = true
	}
	switch {
	case !isRegularFile(values["-m"]):
		fmt.Fprintf(os.Stderr, "fake whisper: -m %q is not an existing model file\n", values["-m"])
	case !isRegularFile(values["-f"]) || !strings.HasSuffix(values["-f"], ".wav"):
		fmt.Fprintf(os.Stderr, "fake whisper: -f %q is not an existing .wav\n", values["-f"])
	case values["-l"] != "auto":
		fmt.Fprintf(os.Stderr, "fake whisper: -l = %q, want auto (Thai+English detection)\n", values["-l"])
	case !flags["-np"]:
		fmt.Fprintln(os.Stderr, "fake whisper: -np missing, banner noise would reach the parser")
	default:
		fmt.Print(canned)
		return 0
	}
	return 1
}

// stubWhisperLookPath swaps the PATH lookup so the binary-missing and
// model-missing branches can each be reached on any machine, with or without
// whisper.cpp actually installed.
func stubWhisperLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	previous := whisperLookPath
	whisperLookPath = fn
	t.Cleanup(func() { whisperLookPath = previous })
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

func TestAudioTranscribeMissingBinaryGivesThaiInstructions(t *testing.T) {
	stubWhisperLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "voice.mp3"), []byte("not really audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &audioTranscribeSkill{root: root}
	_, err := s.ExecuteTool(context.Background(), map[string]any{"path": "voice.mp3"})
	if err == nil {
		t.Fatal("expected error when whisper.cpp is not installed, got nil")
	}
	if strings.Contains(err.Error(), exec.ErrNotFound.Error()) {
		t.Errorf("raw exec error leaked to the model instead of install instructions: %v", err)
	}
	if !strings.Contains(err.Error(), "whisper") || !strings.Contains(err.Error(), "ไม่พบโปรแกรม") {
		t.Errorf("error should name whisper.cpp and how to install it, in Thai; got: %v", err)
	}
}

func TestAudioTranscribeMissingModelGivesDownloadInstructions(t *testing.T) {
	stubWhisperLookPath(t, func(name string) (string, error) { return name, nil })
	dataRoot := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dataRoot)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "voice.mp3"), []byte("not really audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &audioTranscribeSkill{root: root}
	_, err := s.ExecuteTool(context.Background(), map[string]any{"path": "voice.mp3"})
	if err == nil {
		t.Fatal("expected error when the ggml model is absent, got nil")
	}
	msg := err.Error()
	for _, want := range []string{whisperDefaultModel, "142 MB", filepath.Join(dataRoot, "models")} {
		if !strings.Contains(msg, want) {
			t.Errorf("model error should mention %q so the user knows what to fetch and where; got: %v", want, msg)
		}
	}
	// The whole point of the message: nothing was downloaded behind their back.
	if entries, _ := os.ReadDir(filepath.Join(dataRoot, "models")); len(entries) > 0 {
		t.Errorf("nothing should have been downloaded automatically, found %d files", len(entries))
	}
}

// A model file that is present is used even when it isn't the default one.
func TestAudioTranscribeAcceptsAnyGGMLModel(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dataRoot)
	modelsDir := filepath.Join(dataRoot, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tiny := filepath.Join(modelsDir, "ggml-tiny.bin")
	if err := os.WriteFile(tiny, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := whisperModelPath()
	if err != nil {
		t.Fatalf("whisperModelPath() with ggml-tiny.bin present = %v, want it accepted", err)
	}
	if got != tiny {
		t.Errorf("whisperModelPath() = %q, want %q", got, tiny)
	}
}

func TestParseWhisperSegments(t *testing.T) {
	raw := strings.Join([]string{
		"whisper_init_from_file_with_params_no_state: loading model",
		"[00:00:00.000 --> 00:00:03.480]   สวัสดีครับ ยินดีต้อนรับ",
		"[00:00:03.480 --> 00:00:07.000]  This is the second line.",
		"[00:00:07.000 --> 00:00:11.000]  This is the second line.",
		"[00:01:05.120 --> 00:01:09.000]   หลังจากผ่านไปหนึ่งนาที",
		"[00:02:00.000 --> 00:02:04.000]   ",
		"[malformed line without an arrow]",
		"",
	}, "\n")

	got := parseWhisperSegments(raw)
	want := []string{
		"[0:00] สวัสดีครับ ยินดีต้อนรับ",
		"[0:03] This is the second line.",
		"[1:05] หลังจากผ่านไปหนึ่งนาที",
	}
	if len(got) != len(want) {
		t.Fatalf("parseWhisperSegments() returned %d lines, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseWhisperTimestamp(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"00:00:00.000", 0, true},
		{"00:00:07.480", 7, true},
		{"00:01:05.120", 65, true},
		{"01:02:03.000", 3723, true},
		{"02:05", 125, true},
		{"12", 0, false},
		{"00:00:00:00:00", 0, false},
		{"aa:bb.000", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseWhisperTimestamp(c.in)
		if ok != c.wantOK || got != c.want {
			t.Errorf("parseWhisperTimestamp(%q) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

// The whole pipeline on a real file, with whisper itself stubbed out: ffmpeg
// really converts, the real flags are really passed (the stub fails the test
// otherwise), the real output is parsed, and the temp dir is really cleaned up.
// Input is an .mp4 on purpose — the video branch is the one -vn has to handle.
func TestAudioTranscribePipelineWithStubWhisper(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}

	dataRoot := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dataRoot)
	modelsDir := filepath.Join(dataRoot, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, whisperDefaultModel), []byte("stub model"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AETOX_TEST_FAKE_WHISPER", strings.Join([]string{
		"[00:00:00.000 --> 00:00:02.480]   สวัสดีครับ นี่คือเสียงทดสอบ",
		"[00:01:04.000 --> 00:01:07.000]  And this line is in English.",
		"",
	}, "\n"))
	stubWhisperLookPath(t, func(string) (string, error) { return os.Args[0], nil })

	root := t.TempDir()
	clip := filepath.Join(root, "clip.mp4")
	build := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=320x120:d=3:r=5",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-shortest", clip)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test video with audio: %v — %s", err, out)
	}

	tempBefore := countAetoxAudioTempDirs()

	s := &audioTranscribeSkill{root: root}
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
	if after := countAetoxAudioTempDirs(); after > tempBefore {
		t.Errorf("temp dirs leaked: %d before, %d after", tempBefore, after)
	}
}

func countAetoxAudioTempDirs() int {
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "aetox-audio-*"))
	return len(matches)
}

// End-to-end on a machine that has ffmpeg, whisper.cpp and a model: a
// synthesized tone has no speech in it, so the assertion is that the whole
// pipeline (ffmpeg conversion, whisper flags, output parsing) runs clean rather
// than that any particular words come back. Wrong flags fail here, not in prod.
func TestAudioTranscribeLiveRunsCleanPipeline(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed on this machine")
	}
	if _, err := whisperBinary(); err != nil {
		t.Skip("whisper.cpp not installed on this machine")
	}
	if _, err := whisperModelPath(); err != nil {
		t.Skip("no ggml model downloaded on this machine")
	}

	root := t.TempDir()
	clip := filepath.Join(root, "tone.wav")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3", clip)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test audio: %v — %s", err, out)
	}

	s := &audioTranscribeSkill{root: root}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "tone.wav"})
	if err != nil {
		t.Fatalf("audio_transcribe failed: %v", err)
	}
	if !out.Success {
		t.Fatalf("Success = false, stderr: %s", out.Stderr)
	}
	if strings.TrimSpace(out.Content) == "" {
		t.Error("content should never be empty — silence still reports the no-speech line")
	}
}
