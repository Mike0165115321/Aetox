package tts

// Package tts is the mirror of internal/stt on the speaking side: one language
// every speech-synthesis engine is translated into. Engines disagree about
// everything here too — Windows SAPI is a COM surface with two voice registries
// that cannot see each other, Piper is a binary with ONNX model files, cloud
// vendors are HTTP APIs. Nothing above this package is allowed to care. An
// Engine takes text and a path, writes a WAV there, and that is the whole
// contract.
//
// Same shape as internal/stt and internal/model (ARCHITECTURE.md §33): a
// catalog describes what exists, New() switches on it and hands back one
// interface, and callers never name a concrete engine. Adding a vendor is a
// Descriptor plus a file — the settings picker renders from Catalog() and
// needs no change.
//
// Policy this package does NOT hold: which voice to prefer for the user's
// language. An engine speaks with the voice it is told, or its own default;
// choosing a Thai voice because the UI is in Thai is the caller's judgement
// (desktop/voice.go), because that is where the locale lives.

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Voice is one installed voice, shaped for a settings picker. ID is the
// engine's own stable identity for it (for SAPI, the token description) and is
// what Options.Voice pins; everything else is display.
type Voice struct {
	ID     string
	Name   string
	Lang   string // "th-TH"-style tag, or the engine's raw code when unmappable
	Gender string // "Male", "Female", or ""
}

// Engine turns text into an audio file. Implementations own nothing: the
// caller picks the path, plays or ships the file, and deletes it.
type Engine interface {
	// ID matches the Descriptor it was built from.
	ID() string
	// Voices enumerates what is installed, in the engine's own order.
	Voices(ctx context.Context) ([]Voice, error)
	// Synthesize speaks text into an audio file at outPath, with the voice the
	// engine was built with (Options.Voice) or the engine's default when none
	// was.
	Synthesize(ctx context.Context, text, outPath string) error
	// Mime is the type of what Synthesize writes — "audio/wav" for the local
	// engines, "audio/mpeg" for the ones that hand back MP3. The player never
	// guesses.
	Mime() string
}

// Descriptor is what the settings UI renders and what New() builds from — one
// entry per supported engine, data only, no behavior.
type Descriptor struct {
	ID    string // stable config value, e.g. "windows"
	Label string // shown to the user
	// Binaries are the PATH candidates that count as this engine being
	// installed, in preference order. Empty for an engine the OS ships.
	Binaries []string
	// Install is a ready-to-follow Thai instruction for getting this engine —
	// or, for one that ships with the OS, for getting more voices into it.
	Install string
	// InstallCommand is the exact argv the settings page's ติดตั้ง button
	// runs, and also the command it displays — one value serving both, so the
	// command on screen can never differ from the command that runs. Empty for
	// an engine that has no runnable install (ships with the OS, needs an API
	// key, or is a manual download).
	InstallCommand []string
	// Models are the NAMED models this vendor accepts, first = the default.
	// One entry means there is no real choice and the page draws no picker;
	// empty means the engine has no model concept (its voices ARE the choice).
	Models []string
	// Default marks the engine chosen when config says nothing.
	Default bool
}

// Options is the per-call configuration. The zero value is valid and resolves
// to the default engine speaking with its default voice.
type Options struct {
	// Engine is a Descriptor.ID. Empty picks the catalog default.
	Engine string
	// Voice is a Voice.ID. Empty lets the engine use its own default.
	Voice string
	// Model pins one of the engine's named models (Descriptor.Models). Empty
	// takes the first.
	Model string
}

// catalog is the single list of engines Aetox knows how to speak with. A new
// vendor is an entry here plus its constructor in newEngine.
var catalog = []Descriptor{
	{
		ID:      "windows",
		Label:   "Windows (เสียงในเครื่อง)",
		Install: "เสียงมากับ Windows อยู่แล้ว เพิ่มภาษาอื่นได้ที่ Settings > Time & language > Speech > Manage voices",
		Default: true,
	},
	{
		ID:       "piper",
		Label:    "Piper (neural, ออฟไลน์)",
		Binaries: []string{"piper"},
		Install:  "โหลด release จาก https://github.com/rhasspy/piper วางที่ <DataRoot>\\tools\\piper และโหลดไฟล์เสียง .onnx จาก https://huggingface.co/rhasspy/piper-voices ไว้ที่ <DataRoot>\\models\\piper",
	},
	{
		ID:             "edge",
		Label:          "Microsoft Edge (คลาวด์, ฟรี)",
		Binaries:       []string{"edge-tts"},
		Install:        "ติดตั้งด้วย: pip install edge-tts (ต้องมี Python) — เสียง Neural ของ Microsoft รวมเสียงไทย Premwadee/Niwat ฟรี ไม่ใช้ key แต่ข้อความจะถูกส่งไปสังเคราะห์บนคลาวด์",
		InstallCommand: []string{"pip", "install", "edge-tts"},
	},
	{
		ID:             "gtts",
		Label:          "Google Translate (คลาวด์, ฟรี)",
		Binaries:       []string{"gtts-cli"},
		Install:        "ติดตั้งด้วย: pip install gTTS (ต้องมี Python) — เสียงของ Google Translate ฟรี ไม่ใช้ key เลือกเป็นรายภาษา และข้อความจะถูกส่งไปสังเคราะห์บนคลาวด์",
		InstallCommand: []string{"pip", "install", "gTTS"},
	},
	{
		ID:      "openai",
		Label:   "OpenAI (คลาวด์, ใช้ API key)",
		Install: "ใช้ API key ของ OpenAI จาก ตั้งค่า > โมเดล > OpenAI — เปลี่ยน Base URL ของ OpenAI ได้เพื่อชี้ไปเซิร์ฟเวอร์อื่นที่พูด API เดียวกัน (LocalAI, Speaches ฯลฯ) ข้อความจะถูกส่งไปสังเคราะห์ปลายทาง",
		Models:  []string{"gpt-4o-mini-tts", "tts-1", "tts-1-hd"},
	},
	{
		ID:      "groq",
		Label:   "Groq PlayAI (คลาวด์, อังกฤษเท่านั้น)",
		Install: "ใช้ API key ของ Groq จาก ตั้งค่า > โมเดล > Groq — เสียง playai-tts พูดได้เฉพาะภาษาอังกฤษ และข้อความจะถูกส่งไปสังเคราะห์บนคลาวด์",
		// One entry, no picker: playai-tts-arabic exists but reads Arabic
		// only, and a bare name in a dropdown would not say that.
		Models: []string{"playai-tts"},
	},
	{
		ID:      "gemini",
		Label:   "Gemini (คลาวด์, ใช้ API key)",
		Install: "ใช้ API key ของ Gemini จาก ตั้งค่า > โมเดล > Gemini — เสียง TTS ของ Gemini พูดได้หลายภาษารวมภาษาไทย ข้อความจะถูกส่งไปสังเคราะห์บนคลาวด์ของ Google",
		Models:  []string{"gemini-2.5-flash-preview-tts", "gemini-2.5-pro-preview-tts"},
	},
	{
		ID:      "elevenlabs",
		Label:   "ElevenLabs (คลาวด์, ใช้ API key)",
		Install: "ตั้ง environment variable ELEVENLABS_API_KEY (สมัครที่ elevenlabs.io มีชั้นฟรี) — ข้อความจะถูกส่งไปสังเคราะห์บนคลาวด์ของ ElevenLabs",
		Models:  []string{"eleven_multilingual_v2", "eleven_v3", "eleven_flash_v2_5", "eleven_turbo_v2_5"},
	},
}

// Catalog returns every known engine — default first, then alphabetical — for
// the settings picker to enumerate.
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

// New resolves opts into a ready engine, or returns an actionable Thai error —
// a wrong engine name, or an engine this machine cannot run. Callers hand that
// error straight to the user.
func New(opts Options) (Engine, error) {
	desc, ok := Lookup(opts.Engine)
	if !ok {
		return nil, fmt.Errorf("ไม่รู้จัก engine เสียงอ่านชื่อ %q — ที่รองรับตอนนี้: %s", opts.Engine, strings.Join(engineIDs(), ", "))
	}
	return newEngine(desc, opts)
}

// newEngine is the one switch that maps a descriptor to a concrete runtime.
func newEngine(desc Descriptor, opts Options) (Engine, error) {
	switch desc.ID {
	case "windows":
		return newWindowsVoice(desc, opts)
	case "piper":
		return newPiper(desc, opts)
	case "edge":
		return newEdge(desc, opts)
	case "gtts":
		return newGTTS(desc, opts)
	case "openai", "groq":
		return newAPISpeech(desc, opts)
	case "gemini":
		return newGeminiSpeech(desc, opts)
	case "elevenlabs":
		return newElevenLabs(desc, opts)
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
