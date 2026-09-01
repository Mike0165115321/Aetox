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
// Cloud rows exist since 2026-09-01 (owner's amendment to §31, recorded in
// §216): the LOCAL engines stay the default and recordings still never leave
// the machine unless the user picks a cloud vendor by name — every cloud row's
// Install text says outright that the audio goes out. What §31 still forbids
// is Aetox sending audio anywhere on its own judgement.

import (
	"context"
	"fmt"
	"runtime"
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
	// Models are the NAMED models this vendor accepts, first = the default —
	// for vendors whose models are API names rather than files on disk. One
	// entry means there is no real choice, and the page draws no picker
	// (a picker with a single entry is not a choice). Empty means models are
	// files (ModelGlob) or the engine has no model concept.
	Models []string
	// InstallCommand is the exact argv the settings page's ติดตั้ง button
	// runs, and also the command it displays — one value serving both, so the
	// command on screen can never differ from the command that runs. Empty for
	// an engine that has no runnable install (needs an API key, or is a manual
	// download).
	InstallCommand []string
	// Default marks the engine chosen when config says nothing.
	Default bool
}

// Options is the per-call configuration a caller (today: the audio_transcribe
// skill, fed from RegistryOptions) passes down. The zero value is valid and
// resolves to the default engine with auto-discovered model.
type Options struct {
	// Engine is a Descriptor.ID. Empty picks the catalog default.
	Engine string
	// Model pins one of the engine's NAMED models (Descriptor.Models). Empty
	// takes the first. Ignored by file-based engines, which read ModelPath.
	Model string
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

// whisperCPPInstall is whisper-cpp's runnable install for THIS machine, or
// nil where there is none.
var whisperCPPInstall = map[string][]string{
	"windows": {"scoop", "install", "whisper-cpp"},
	"darwin":  {"brew", "install", "whisper-cpp"},
}[runtime.GOOS]

// catalog is the single list of engines Aetox knows how to speak to. A new
// engine is an entry here plus its constructor in newEngine.
var catalog = []Descriptor{
	{
		ID:        "whisper-cpp",
		Label:     "whisper.cpp (ggml)",
		Binaries:  []string{"whisper-cli", "whisper-cpp"},
		ModelGlob: "ggml-*.bin",
		Install:   "ติดตั้งด้วย: scoop install whisper-cpp (Windows) · brew install whisper-cpp (macOS) · หรือ build จาก https://github.com/ggml-org/whisper.cpp",
		// Per-OS: the package manager differs, and a Linux machine builds from
		// source, which is not a button.
		InstallCommand: whisperCPPInstall,
		Default:        true,
	},
	{
		ID:       "faster-whisper",
		Label:    "faster-whisper (CTranslate2)",
		Binaries: []string{"whisper-ctranslate2"},
		// No ModelGlob: this runtime fetches and stores its own weights by
		// name — there is no file for the picker to point at.
		Install:        "ติดตั้งด้วย: pip install whisper-ctranslate2 (ต้องมี Python) — ครั้งแรกที่ถอด โปรแกรมจะโหลดโมเดล " + fasterWhisperModel + " (~150 MB) ให้ตัวเอง",
		InstallCommand: []string{"pip", "install", "whisper-ctranslate2"},
		// Size names, not files: the runtime fetches whichever is asked for.
		Models: []string{fasterWhisperModel, "tiny", "small", "medium", "large-v3"},
	},
	{
		ID:      "openai",
		Label:   "OpenAI Whisper (คลาวด์, ใช้ API key)",
		Install: "ใช้ API key ของ OpenAI จาก ตั้งค่า > โมเดล > OpenAI — เสียงที่อัดจะถูกส่งไปถอดบนคลาวด์ เปลี่ยน Base URL ได้เพื่อชี้เซิร์ฟเวอร์อื่นที่พูด API เดียวกัน",
		Models:  []string{"whisper-1", "gpt-4o-transcribe", "gpt-4o-mini-transcribe"},
	},
	{
		ID:      "groq",
		Label:   "Groq Whisper (คลาวด์, ใช้ API key)",
		Install: "ใช้ API key ของ Groq จาก ตั้งค่า > โมเดล > Groq — Whisper บนคลาวด์ เร็วมาก และเสียงที่อัดจะถูกส่งขึ้นไปถอด",
		// distil-whisper is deliberately absent: English-only, and a Thai
		// machine picking it from a bare name would find out the hard way.
		Models: []string{"whisper-large-v3", "whisper-large-v3-turbo"},
	},
	{
		ID:      "mistral",
		Label:   "Mistral Voxtral (คลาวด์, ใช้ API key)",
		Install: "ใช้ API key ของ Mistral จาก ตั้งค่า > โมเดล > Mistral — voxtral-mini บนคลาวด์ และเสียงที่อัดจะถูกส่งขึ้นไปถอด",
		// One entry: the transcription endpoint serves exactly one model, and
		// a single-entry picker is not drawn.
		Models: []string{"voxtral-mini-latest"},
	},
	{
		ID:      "gemini",
		Label:   "Gemini (คลาวด์, ใช้ API key)",
		Install: "ใช้ API key ของ Gemini จาก ตั้งค่า > โมเดล > Gemini — ให้ Gemini ฟังแล้วถอดเป็นข้อความ และเสียงที่อัดจะถูกส่งขึ้นคลาวด์ของ Google",
		Models:  []string{"gemini-2.5-flash", "gemini-2.5-pro", "gemini-2.5-flash-lite"},
	},
	{
		ID:      "elevenlabs",
		Label:   "ElevenLabs Scribe (คลาวด์, ใช้ API key)",
		Install: "ตั้ง environment variable ELEVENLABS_API_KEY (สมัครที่ elevenlabs.io) — Scribe ถอดได้ 90+ ภาษา และเสียงที่อัดจะถูกส่งขึ้นคลาวด์",
		Models:  []string{"scribe_v1", "scribe_v2"},
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
	case "faster-whisper":
		return newFasterWhisper(desc, opts)
	case "openai", "groq", "mistral", "elevenlabs":
		return newAPITranscriber(desc, opts)
	case "gemini":
		return newGeminiTranscriber(desc, opts)
	default:
		return nil, fmt.Errorf("engine %q อยู่ในรายการแต่ยังไม่มีตัวรัน", desc.ID)
	}
}

// resolveNamedModel picks the named model an engine runs: the pinned one when
// the roster knows it, the roster's first when nothing is pinned. An unknown
// pin is a loud error, not a silent fallback — a setting that quietly stops
// applying is a setting that lies.
func resolveNamedModel(desc Descriptor, pinned string) (string, error) {
	pinned = strings.TrimSpace(pinned)
	if len(desc.Models) == 0 {
		return pinned, nil
	}
	if pinned == "" {
		return desc.Models[0], nil
	}
	for _, m := range desc.Models {
		if strings.EqualFold(m, pinned) {
			return m, nil
		}
	}
	return "", fmt.Errorf("engine %s ไม่มีโมเดลชื่อ %q — ที่มี: %s", desc.ID, pinned, strings.Join(desc.Models, ", "))
}

func engineIDs() []string {
	ids := make([]string, 0, len(catalog))
	for _, d := range catalog {
		ids = append(ids, d.ID)
	}
	return ids
}
