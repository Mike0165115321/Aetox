package main

// The voice page's bindings: two vendor pickers, a voice picker, and the mic.
// TTS voice enumeration costs a real PowerShell run, so every test that needs
// voices seeds App.ttsVoiceCache instead — the cache is the seam, and it keeps
// the suite off SAPI on Windows and runnable at all everywhere else.

import (
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/tts"
)

func seedTTSVoices(a *App, voices ...tts.Voice) {
	a.ttsVoiceCache = map[string][]tts.Voice{"windows": voices}
}

// A picker with no marked row reads as "not configured" — the default engine
// must come back active before anybody has picked anything.
func TestListEnginesMarkTheDefaultWhenNothingIsPicked(t *testing.T) {
	a, _ := newSpeechTestApp(t)

	for name, rows := range map[string][]VoiceEngineInfo{
		"stt": a.ListSpeechEngines(),
		"tts": a.ListTTSEngines(),
	} {
		if len(rows) == 0 {
			t.Fatalf("%s: no engines listed", name)
		}
		active := 0
		for _, r := range rows {
			if r.Active {
				active++
			}
			if strings.TrimSpace(r.Label) == "" || strings.TrimSpace(r.Install) == "" {
				t.Errorf("%s: engine %q missing label or install hint", name, r.ID)
			}
		}
		if active != 1 {
			t.Errorf("%s: %d engines marked active, want exactly 1", name, active)
		}
	}
}

func TestSetSpeechEnginePersistsAndRejectsUnknown(t *testing.T) {
	a, _ := newSpeechTestApp(t)

	if err := a.SetSpeechEngine("no-such-vendor"); err == nil {
		t.Fatal("an unknown engine id must be refused")
	}
	if err := a.SetSpeechEngine("whisper-cpp"); err != nil {
		t.Fatalf("SetSpeechEngine: %v", err)
	}
	if a.cur().cfg.SpeechEngine != "whisper-cpp" {
		t.Errorf("cfg.SpeechEngine = %q", a.cur().cfg.SpeechEngine)
	}
	pref, ok, err := config.LoadModelPreference()
	if err != nil || !ok {
		t.Fatalf("preference not saved: ok=%v err=%v", ok, err)
	}
	if pref.SpeechEngine != "whisper-cpp" {
		t.Errorf("saved preference = %q — the pick would not survive a restart", pref.SpeechEngine)
	}
}

// Switching vendor clears the voice pick: a voice id is one engine's private
// naming, and the new engine has never heard of it.
func TestSetTTSEngineClearsTheVoicePick(t *testing.T) {
	a, _ := newSpeechTestApp(t)
	seedTTSVoices(a, tts.Voice{ID: "Microsoft Pattara - Thai (Thailand)", Lang: "th-TH"})

	if err := a.SetTTSVoice("Microsoft Pattara - Thai (Thailand)"); err != nil {
		t.Fatalf("SetTTSVoice: %v", err)
	}
	if err := a.SetTTSEngine("windows"); err != nil {
		t.Fatalf("SetTTSEngine: %v", err)
	}
	if got := a.cur().cfg.TTSVoice; got != "" {
		t.Errorf("voice pick survived a vendor switch: %q", got)
	}
	pref, _, _ := config.LoadModelPreference()
	if pref.TTSEngine != "windows" || pref.TTSVoice != "" {
		t.Errorf("saved preference engine=%q voice=%q", pref.TTSEngine, pref.TTSVoice)
	}
}

func TestSetTTSVoiceValidatesAgainstInstalledVoices(t *testing.T) {
	a, _ := newSpeechTestApp(t)
	seedTTSVoices(a,
		tts.Voice{ID: "Microsoft Zira Desktop - English (United States)", Lang: "en-US"},
		tts.Voice{ID: "Microsoft Pattara - Thai (Thailand)", Lang: "th-TH"},
	)

	if err := a.SetTTSVoice("Microsoft Gone - Nowhere"); err == nil {
		t.Fatal("a voice not on this machine must be refused")
	}
	if err := a.SetTTSVoice("Microsoft Pattara - Thai (Thailand)"); err != nil {
		t.Fatalf("SetTTSVoice: %v", err)
	}
	voices, err := a.ListTTSVoices()
	if err != nil {
		t.Fatalf("ListTTSVoices: %v", err)
	}
	active := 0
	for _, v := range voices {
		if v.Active {
			active++
		}
	}
	if active != 1 {
		t.Errorf("%d voices marked active, want exactly 1", active)
	}
	// Empty is a real choice — back to the engine deciding.
	if err := a.SetTTSVoice(""); err != nil {
		t.Fatalf("SetTTSVoice(\"\"): %v", err)
	}
	if got := a.cur().cfg.TTSVoice; got != "" {
		t.Errorf("clearing the voice did not stick: %q", got)
	}
}

// The policy internal/tts refuses to hold: no pick + Thai UI = the Thai voice,
// and a locale with no matching voice falls back to the engine's own default
// rather than refusing to speak.
func TestDefaultTTSVoicePrefersTheUILanguage(t *testing.T) {
	a, _ := newSpeechTestApp(t)
	seedTTSVoices(a,
		tts.Voice{ID: "Microsoft Zira Desktop - English (United States)", Lang: "en-US"},
		tts.Voice{ID: "Microsoft Pattara - Thai (Thailand)", Lang: "th-TH"},
	)

	if got := a.defaultTTSVoice("", "th"); got != "Microsoft Pattara - Thai (Thailand)" {
		t.Errorf("th locale picked %q", got)
	}
	if got := a.defaultTTSVoice("", "en"); got != "Microsoft Zira Desktop - English (United States)" {
		t.Errorf("en locale picked %q", got)
	}
	if got := a.defaultTTSVoice("", "ja"); got != "" {
		t.Errorf("an unmatched locale must fall back to the engine default, got %q", got)
	}
}

func TestTranscribeMicAudioRefusesAMalformedRecording(t *testing.T) {
	a, _ := newSpeechTestApp(t)

	if _, err := a.TranscribeMicAudio("not a data url"); err == nil {
		t.Error("a payload without ;base64, must be refused")
	}
	if _, err := a.TranscribeMicAudio("data:audio/webm;base64,%%%"); err == nil {
		t.Error("invalid base64 must be refused")
	}
	if _, err := a.TranscribeMicAudio("data:audio/webm;base64,"); err == nil {
		t.Error("an empty recording must be refused")
	}
}

// The ติดตั้ง button may run only what the catalog says. An engine with no
// runnable install, an unknown engine, and an unknown side each refuse with a
// sentence — there is no path from the webview to an arbitrary command.
func TestInstallVoiceEngineRunsOnlyCatalogCommands(t *testing.T) {
	if _, err := voiceInstallArgv("tts", "windows"); err == nil {
		t.Error("windows TTS has no install command but was allowed")
	}
	if _, err := voiceInstallArgv("stt", "no-such-engine"); err == nil {
		t.Error("unknown engine was allowed")
	}
	if _, err := voiceInstallArgv("chat", "edge"); err == nil {
		t.Error("unknown side was allowed")
	}
	argv, err := voiceInstallArgv("tts", "edge")
	if err != nil || strings.Join(argv, " ") != "pip install edge-tts" {
		t.Errorf("edge argv = %v, %v — must be exactly the catalog's command", argv, err)
	}
}

// Rows carry the command for the page to display next to the button. Absent
// means empty, never null (ARCHITECTURE.md §34 — the frontend does .length).
func TestEngineRowsCarryNonNilInstallCommand(t *testing.T) {
	a, _ := newSpeechTestApp(t)
	for _, rows := range [][]VoiceEngineInfo{a.ListSpeechEngines(), a.ListTTSEngines()} {
		for _, r := range rows {
			if r.InstallCommand == nil {
				t.Errorf("engine %q: InstallCommand is nil", r.ID)
			}
		}
	}
}

// The named-model pick: sticks, validates against the active vendor's roster,
// and clears on a vendor switch — a model name is one vendor's vocabulary.
func TestSetSpeechModelNameValidatesPersistsAndClears(t *testing.T) {
	a, _ := newSpeechTestApp(t)

	if err := a.SetSpeechEngine("openai"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetSpeechModelName("no-such-model"); err == nil {
		t.Error("a model the vendor does not serve must be refused")
	}
	if err := a.SetSpeechModelName("gpt-4o-mini-transcribe"); err != nil {
		t.Fatalf("SetSpeechModelName: %v", err)
	}
	pref, _, _ := config.LoadModelPreference()
	if pref.SpeechModelName != "gpt-4o-mini-transcribe" {
		t.Errorf("saved preference = %q — the pick would not survive a restart", pref.SpeechModelName)
	}
	for _, row := range a.ListSpeechEngines() {
		if row.ID == "openai" && row.ActiveModel != "gpt-4o-mini-transcribe" {
			t.Errorf("row.ActiveModel = %q", row.ActiveModel)
		}
		if row.Models == nil {
			t.Errorf("engine %q: Models is nil", row.ID)
		}
	}
	// Switching vendor clears the pick.
	if err := a.SetSpeechEngine("whisper-cpp"); err != nil {
		t.Fatal(err)
	}
	if got := a.cur().cfg.SpeechModelName; got != "" {
		t.Errorf("model pick survived a vendor switch: %q", got)
	}
}

func TestSetTTSModelNameFollowsTheSameRules(t *testing.T) {
	a, _ := newSpeechTestApp(t)

	if err := a.SetTTSEngine("openai"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetTTSModelName("tts-1-hd"); err != nil {
		t.Fatalf("SetTTSModelName: %v", err)
	}
	if err := a.SetTTSModelName("whisper-1"); err == nil {
		t.Error("an STT model name on the TTS side must be refused")
	}
	if err := a.SetTTSEngine("windows"); err != nil {
		t.Fatal(err)
	}
	if got := a.cur().cfg.TTSModelName; got != "" {
		t.Errorf("model pick survived a vendor switch: %q", got)
	}
}
