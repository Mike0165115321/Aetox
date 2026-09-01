package stt

// faster-whisper, through the whisper-ctranslate2 CLI — the second vendor, and
// the proof that adding one really is a Descriptor plus a file. The CLI prints
// the same "[MM:SS.mmm --> MM:SS.mmm] text" segment lines the original OpenAI
// client does (it is written to be drop-in compatible), so even the parser is
// shared with whisper.cpp.
//
// One honest difference from whisper.cpp: this runtime fetches its own model
// by name on first run — the CTranslate2 weights, from Hugging Face — rather
// than reading a file the user placed. §32 still holds for Aetox itself
// (nothing here downloads anything); the descriptor's Install text says the
// tool will, and how much, so the bandwidth is still spent knowingly.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// fasterWhisperModel is the size handed to --model: the same accuracy-for-
// size point preferredWhisperModel picks for whisper.cpp.
const fasterWhisperModel = "base"

type fasterWhisper struct {
	binPath string
	model   string // a size name from Descriptor.Models, not a file
	desc    Descriptor
}

func newFasterWhisper(desc Descriptor, opts Options) (Engine, error) {
	binPath, err := findBinary(desc)
	if err != nil {
		return nil, err
	}
	model, err := resolveNamedModel(desc, opts.Model)
	if err != nil {
		return nil, err
	}
	return &fasterWhisper{binPath: binPath, model: model, desc: desc}, nil
}

func (*fasterWhisper) ID() string { return "faster-whisper" }

// ModelPath is the model NAME here, not a file: the runtime resolves and
// stores the weights itself, and pretending to know a path would be a lie.
func (f *fasterWhisper) ModelPath() string { return f.model }

func (*fasterWhisper) ModelCaution() string { return "" }

func (f *fasterWhisper) Transcribe(ctx context.Context, wavPath string) ([]Segment, error) {
	// The CLI insists on writing transcript files; --output_dir points them at
	// a throwaway so the segment lines on stdout are the only thing kept.
	tmpDir, err := os.MkdirTemp("", "aetox-fw-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	// No --language: absent means autodetect, the same behaviour -l auto gives
	// whisper.cpp.
	cmd := exec.CommandContext(ctx, f.binPath, wavPath,
		"--model", f.model,
		"--output_dir", tmpDir,
		"--output_format", "txt",
	)
	proc.HideConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, missingBinaryError(f.desc)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return parseWhisperOutput(stdout.String()), nil
}
