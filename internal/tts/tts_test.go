package tts

import (
	"context"
	"encoding/base64"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestCatalogDefaultFirst(t *testing.T) {
	got := Catalog()
	if len(got) == 0 {
		t.Fatal("catalog is empty")
	}
	if !got[0].Default {
		t.Errorf("first catalog entry %q is not the default", got[0].ID)
	}
	// A picker rendered from this list shows Install as the user's next step —
	// an entry without one is a dead end on screen.
	for _, d := range got {
		if strings.TrimSpace(d.Install) == "" {
			t.Errorf("engine %q has no install hint", d.ID)
		}
		if strings.TrimSpace(d.Label) == "" {
			t.Errorf("engine %q has no label", d.ID)
		}
	}
}

func TestLookup(t *testing.T) {
	if d, ok := Lookup(""); !ok || !d.Default {
		t.Errorf("empty id should resolve to the default engine, got %+v ok=%v", d, ok)
	}
	if d, ok := Lookup(" Windows "); !ok || d.ID != "windows" {
		t.Errorf("lookup should trim and case-fold, got %+v ok=%v", d, ok)
	}
	if _, ok := Lookup("no-such-engine"); ok {
		t.Error("unknown id should not resolve")
	}
}

func TestNewUnknownEngineNamesTheSupported(t *testing.T) {
	// "elevenlabs" was the fake name here until 2026-09-01, when it became a
	// real row — the test outlived its own hypothetical.
	_, err := New(Options{Engine: "no-such-vendor"})
	if err == nil {
		t.Fatal("expected an error for an unknown engine")
	}
	if !strings.Contains(err.Error(), "windows") {
		t.Errorf("error should name the supported engines, got: %v", err)
	}
}

func TestNewWindowsEngineOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this case only exists off Windows")
	}
	if _, err := New(Options{}); err == nil {
		t.Fatal("the windows engine must refuse to build on a non-Windows machine")
	}
}

func TestParseVoiceLines(t *testing.T) {
	raw := "Preparing modules for first use.\n" +
		"V|Microsoft Pattara - Thai (Thailand)|41E|Male\n" +
		"V|Microsoft Zira Desktop - English (United States)|409;9|Female\n" +
		"V|Odd Voice||\n" +
		"V|\n" +
		"not a voice line\n"
	got := parseVoiceLines(raw)
	if len(got) != 3 {
		t.Fatalf("expected 3 voices, got %d: %+v", len(got), got)
	}
	if got[0].ID != "Microsoft Pattara - Thai (Thailand)" || got[0].Lang != "th-TH" || got[0].Gender != "Male" {
		t.Errorf("pattara parsed wrong: %+v", got[0])
	}
	if got[1].Lang != "en-US" {
		t.Errorf("multi-part LCID should keep its first code: %+v", got[1])
	}
	if got[2].Lang != "" || got[2].Gender != "" {
		t.Errorf("missing attributes should stay empty: %+v", got[2])
	}
}

func TestLcidToTag(t *testing.T) {
	cases := map[string]string{
		"41E":    "th-TH",
		"409":    "en-US",
		"409;9":  "en-US",
		" 0409 ": "en-US",
		"7FFE":   "7FFE", // unknown stays verbatim rather than guessed
		"":       "",
	}
	for in, want := range cases {
		if got := lcidToTag(in); got != want {
			t.Errorf("lcidToTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindScriptError(t *testing.T) {
	if got := findScriptError("V|a|409|Male\n"); got != "" {
		t.Errorf("no marker should mean no error, got %q", got)
	}
	raw := "noise\nAETOX_TTS_ERR|voice-not-found\n"
	if got := findScriptError(raw); got != "voice-not-found" {
		t.Errorf("marker message lost: %q", got)
	}
}

func TestPsQuote(t *testing.T) {
	if got := psQuote(`C:\a'b`); got != `'C:\a''b'` {
		t.Errorf("quote escaping wrong: %s", got)
	}
	// A newline inside -EncodedCommand ends the statement — it must never
	// survive quoting.
	if got := psQuote("a\r\nb"); strings.ContainsAny(got, "\r\n") {
		t.Errorf("newline survived quoting: %q", got)
	}
}

func TestEncodeCommandIsUTF16LE(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(encodeCommand("สวัสดี"))
	if err != nil {
		t.Fatal(err)
	}
	codes := make([]uint16, len(raw)/2)
	for i := range codes {
		codes[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	if got := string(utf16.Decode(codes)); got != "สวัสดี" {
		t.Errorf("round-trip lost the text: %q", got)
	}
}

func TestWindowsVoicesParsesScriptOutput(t *testing.T) {
	old := runPowerShell
	defer func() { runPowerShell = old }()
	runPowerShell = func(ctx context.Context, script string) (string, error) {
		if !strings.Contains(script, "Speech_OneCore") {
			t.Error("enumeration must read the OneCore registry — System.Speech alone cannot see Thai voices")
		}
		return "V|Microsoft Pattara - Thai (Thailand)|41E|Male\n", nil
	}
	engine := &windowsVoice{}
	voices, err := engine.Voices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(voices) != 1 || voices[0].Lang != "th-TH" {
		t.Errorf("unexpected voices: %+v", voices)
	}
}

func TestWindowsSynthesizeVoiceNotFound(t *testing.T) {
	old := runPowerShell
	defer func() { runPowerShell = old }()
	runPowerShell = func(ctx context.Context, script string) (string, error) {
		if !strings.Contains(script, "'Microsoft Gone'") {
			t.Errorf("the pinned voice must reach the script quoted, script: %s", script)
		}
		return "AETOX_TTS_ERR|voice-not-found\n", nil
	}
	engine := &windowsVoice{voice: "Microsoft Gone"}
	err := engine.Synthesize(context.Background(), "ทดสอบ", t.TempDir()+"/out.wav")
	if err == nil || !strings.Contains(err.Error(), "Microsoft Gone") {
		t.Errorf("a vanished voice should be named in the error, got: %v", err)
	}
}

func TestWindowsSynthesizeRefusesEmptyText(t *testing.T) {
	old := runPowerShell
	defer func() { runPowerShell = old }()
	called := false
	runPowerShell = func(ctx context.Context, script string) (string, error) {
		called = true
		return "", nil
	}
	engine := &windowsVoice{}
	if err := engine.Synthesize(context.Background(), "   ", t.TempDir()+"/out.wav"); err == nil {
		t.Error("empty text should be refused")
	}
	if called {
		t.Error("no PowerShell run should be spent on empty text")
	}
}
