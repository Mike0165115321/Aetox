package tts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiperLang(t *testing.T) {
	cases := map[string]string{
		"th_TH-somevoice-medium.onnx": "th-TH",
		"en_US-lessac-medium.onnx":    "en-US",
		"noprefix.onnx":               "",
		"weird-name.onnx":             "",
	}
	for in, want := range cases {
		if got := piperLang(in); got != want {
			t.Errorf("piperLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPiperVoicesEnumerateTheStore(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	piperDir := filepath.Join(root, "models", "piper")
	if err := os.MkdirAll(piperDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"th_TH-voice-medium.onnx", "en_US-lessac-medium.onnx"} {
		if err := os.WriteFile(filepath.Join(piperDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A stray non-voice file must not become a voice.
	if err := os.WriteFile(filepath.Join(root, "models", "ggml-base.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := &piperVoice{binPath: "piper"}
	voices, err := engine.Voices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got %d: %+v", len(voices), voices)
	}
	if voices[0].Lang != "en-US" && voices[1].Lang != "en-US" {
		t.Errorf("locale not read off the file name: %+v", voices)
	}
}

func TestPiperSynthesizeWithNoVoiceInstalledNamesTheFix(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	engine := &piperVoice{binPath: "piper"}
	err := engine.Synthesize(context.Background(), "ทดสอบ", filepath.Join(t.TempDir(), "out.wav"))
	if err == nil {
		t.Fatal("no installed voice must be an error, not silence")
	}
	if !strings.Contains(err.Error(), "piper-voices") {
		t.Errorf("the error must say where voices come from, got: %v", err)
	}
}

func TestPiperSynthesizeUsesThePinnedVoice(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	voicePath := filepath.Join(root, "models", "piper", "th_TH-voice-medium.onnx")
	if err := os.MkdirAll(filepath.Dir(voicePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(voicePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	old := runPiper
	defer func() { runPiper = old }()
	var gotModel, gotText string
	runPiper = func(_ context.Context, _, model, text, _ string) error {
		gotModel, gotText = model, text
		return nil
	}

	engine := &piperVoice{binPath: "piper", voice: voicePath}
	if err := engine.Synthesize(context.Background(), "สวัสดี", filepath.Join(root, "out.wav")); err != nil {
		t.Fatal(err)
	}
	if gotModel != voicePath || gotText != "สวัสดี" {
		t.Errorf("piper called with model=%q text=%q", gotModel, gotText)
	}
}
