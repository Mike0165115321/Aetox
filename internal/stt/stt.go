package stt

// Package stt is the one language everything speech-related is translated into.
//
// Speech recognition engines disagree about everything: whisper.cpp is a C++
// binary printing "[HH:MM:SS.mmm --> ...] text" on stdout, faster-whisper is a
// Python CLI with its own format, Vosk and sherpa-onnx are different runtimes
// again with different model formats (ggml, CTranslate2, ONNX, Kaldi). Nothing
// above this package is allowed to care. An Engine takes a 16kHz mono WAV and
// returns []Segment — start, end, text — and that is the whole contract.
//
// Same shape as internal/model: a catalog describes what exists, New() switches
// on it and hands back one interface, and callers never name a concrete engine.
// Adding an engine is a Descriptor plus a file — no caller changes.
//
// Deliberately absent: any cloud transcription service. Not a technical limit —
// a product one (ARCHITECTURE.md §31). The interface would accept one; the
// catalog will not ship one.

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Segment is the normalized unit every engine is translated into. Milliseconds
// rather than a formatted timestamp: formatting is the caller's business, and
// "[m:ss]" is only one of the shapes this could be rendered as.
type Segment struct {
	StartMs int
	EndMs   int
	Text    string
}

// Engine transcribes a 16kHz mono WAV. Implementations must never mutate or
// delete the file they are given — the caller owns it.
type Engine interface {
	// ID matches the Descriptor it was built from.
	ID() string
	// ModelPath is the model file this engine resolved to — for showing the
	// user what the transcript came off, not for judging it.
	ModelPath() string
	// ModelCaution is what the user needs told about that model's accuracy, in
	// their language, or "" when there is nothing to say. It lives here because
	// only an engine knows what its own models are called and which of them
	// guess: a caller matching on file names would be this package's business
	// leaking one level up, which is the thing this package exists to stop.
	ModelCaution() string
	Transcribe(ctx context.Context, wavPath string) ([]Segment, error)
}

// Descriptor is what the settings UI renders and what New() builds from — one
// entry per supported engine, data only, no behavior.
type Descriptor struct {
	ID    string // stable config value, e.g. "whisper-cpp"
	Label string // shown to the user
	// Binaries are the PATH candidates that count as this engine being
	// installed, in preference order.
	Binaries []string
	// ModelGlob matches model files in the model directory. Empty means the
	// engine needs no separate model file.
	ModelGlob string
	// Install is a ready-to-follow Thai instruction for getting the engine
	// itself (not its model) onto this machine.
	Install string
	// Default marks the engine chosen when config says nothing.
	Default bool
}

// Options is the per-call configuration a caller (today: the audio_transcribe
// skill, fed from RegistryOptions) passes down. The zero value is valid and
// resolves to the default engine with auto-discovered model.
type Options struct {
	// Engine is a Descriptor.ID. Empty picks the catalog default.
	Engine string
	// ModelPath pins an exact model file. Empty auto-discovers inside ModelDir.
	ModelPath string
	// ModelDir is the Aetox-managed model directory — the one place Aetox
	// downloads into and may delete from. Empty resolves to <DataRoot>/models.
	ModelDir string
	// ExtraModelDirs are the user's own model folders. Read-only to Aetox: a
	// model found here is used, never moved, replaced or deleted.
	ExtraModelDirs []string
}

// InstalledModel is one model file on disk, with enough context for the
// settings UI to render it: which store it came from, and whether Aetox is
// allowed to delete it.
type InstalledModel struct {
	Path    string
	Name    string
	Bytes   int64
	Store   string // Store.Label it was found in
	Managed bool   // Aetox downloaded it and may remove it
}

// catalog is the single list of engines Aetox knows how to speak to. A new
// engine is an entry here plus its constructor in newEngine.
var catalog = []Descriptor{
	{
		ID:        "whisper-cpp",
		Label:     "whisper.cpp (ggml)",
		Binaries:  []string{"whisper-cli", "whisper-cpp"},
		ModelGlob: "ggml-*.bin",
		Install:   "ติดตั้งด้วย: scoop install whisper-cpp (Windows) · brew install whisper-cpp (macOS) · หรือ build จาก https://github.com/ggml-org/whisper.cpp",
		Default:   true,
	},
}

// Catalog returns every known engine. The settings UI enumerates this to build
// its picker, so order is stable (default first, then alphabetical).
func Catalog() []Descriptor {
	out := make([]Descriptor, len(catalog))
	copy(out, catalog)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Default != out[j].Default {
			return out[i].Default
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Lookup finds an engine descriptor by ID. An empty id resolves to the default.
func Lookup(id string) (Descriptor, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, d := range catalog {
		if id == "" && d.Default {
			return d, true
		}
		if d.ID == id {
			return d, true
		}
	}
	return Descriptor{}, false
}

// New resolves opts into a ready engine, or returns an actionable Thai error
// explaining exactly what is missing — a wrong engine name, a missing binary,
// or a missing model file. Callers hand that error straight to the user.
func New(opts Options) (Engine, error) {
	desc, ok := Lookup(opts.Engine)
	if !ok {
		return nil, fmt.Errorf("ไม่รู้จัก engine ถอดเสียงชื่อ %q — ที่รองรับตอนนี้: %s", opts.Engine, strings.Join(engineIDs(), ", "))
	}
	return newEngine(desc, opts)
}

// newEngine is the one switch that maps a descriptor to a concrete runtime.
func newEngine(desc Descriptor, opts Options) (Engine, error) {
	switch desc.ID {
	case "whisper-cpp":
		return newWhisperCPP(desc, opts)
	default:
		return nil, fmt.Errorf("engine %q อยู่ในรายการแต่ยังไม่มีตัวรัน", desc.ID)
	}
}

func engineIDs() []string {
	ids := make([]string, 0, len(catalog))
	for _, d := range catalog {
		ids = append(ids, d.ID)
	}
	return ids
}
