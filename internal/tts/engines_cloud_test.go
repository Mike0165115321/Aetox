package tts

// The four cloud vendors, tested without a cloud: the CLI pair through their
// parsers and swapped runners, the API pair against httptest servers reached
// through the same per-provider base-URL override a user would set.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
)

func isolateCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ELEVENLABS_API_KEY", "")
	t.Setenv("ELEVEN_API_KEY", "")
}

func saveBaseURL(t *testing.T, provider, url string) {
	t.Helper()
	pref := config.ModelPreference{}
	pref.SetBaseURLForProvider(provider, url)
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatal(err)
	}
}

func TestParseEdgeVoices(t *testing.T) {
	raw := "Name                               Gender    ContentCategories      VoicePersonalities\n" +
		"---------------------------------  --------  ---------------------  --------------------\n" +
		"th-TH-PremwadeeNeural              Female    General                Friendly, Positive\n" +
		"en-US-AriaNeural                   Female    News, Novel            Positive, Confident\n" +
		"\n"
	got := parseEdgeVoices(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 voices, got %d: %+v", len(got), got)
	}
	if got[0].ID != "th-TH-PremwadeeNeural" || got[0].Lang != "th-TH" || got[0].Gender != "Female" {
		t.Errorf("premwadee parsed wrong: %+v", got[0])
	}
}

func TestParseGTTSLanguages(t *testing.T) {
	raw := "  af: Afrikaans\n  th: Thai\n  en: English\nnot a language line\n"
	got := parseGTTSLanguages(raw)
	if len(got) != 3 {
		t.Fatalf("expected 3 languages, got %d: %+v", len(got), got)
	}
	// Sorted by code, and the code doubles as the Lang the locale-default
	// policy matches on.
	if got[2].ID != "th" || got[2].Lang != "th" || !strings.Contains(got[2].Name, "Thai") {
		t.Errorf("thai row parsed wrong: %+v", got[2])
	}
}

func TestEdgeSynthesizePassesVoiceAndText(t *testing.T) {
	old := runEdgeTTS
	defer func() { runEdgeTTS = old }()
	var gotArgs []string
	runEdgeTTS = func(_ context.Context, _ string, args ...string) (string, error) {
		gotArgs = args
		return "", nil
	}
	engine := &edgeVoice{binPath: "edge-tts", voice: "th-TH-PremwadeeNeural"}
	if err := engine.Synthesize(context.Background(), "สวัสดี", "out.mp3"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "--voice th-TH-PremwadeeNeural") || !strings.Contains(joined, "--text สวัสดี") {
		t.Errorf("edge-tts args wrong: %v", gotArgs)
	}
}

func TestOpenAISpeechWithoutAKeyNamesTheFix(t *testing.T) {
	isolateCredentials(t)
	if _, err := New(Options{Engine: "openai"}); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("no key on the official host must name the fix, got: %v", err)
	}
}

func TestOpenAISpeechSpeaksThroughTheBaseURLOverride(t *testing.T) {
	isolateCredentials(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/speech" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		// A keyless local clone: no Authorization header expected.
		if r.Header.Get("Authorization") != "" {
			t.Error("keyless base-URL override must not send a bearer header")
		}
		_, _ = w.Write([]byte("RIFFfake-wav"))
	}))
	defer server.Close()
	saveBaseURL(t, "openai", server.URL)

	engine, err := New(Options{Engine: "openai", Voice: "nova"})
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "out.wav")
	if err := engine.Synthesize(context.Background(), "สวัสดี", outPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil || string(data) != "RIFFfake-wav" {
		t.Errorf("audio not written through: %q err=%v", data, err)
	}
	if engine.Mime() != "audio/wav" {
		t.Errorf("openai mime = %q", engine.Mime())
	}
}

func TestElevenLabsListsAndSpeaksWithTheAccountsVoices(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("ELEVENLABS_API_KEY", "xi-test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != "xi-test-key" {
			t.Errorf("key header missing on %s", r.URL.Path)
		}
		switch {
		case r.URL.Path == "/voices":
			_, _ = w.Write([]byte(`{"voices":[{"voice_id":"v1","name":"Rachel","labels":{"gender":"female"}}]}`))
		case strings.HasPrefix(r.URL.Path, "/text-to-speech/v1"):
			_, _ = w.Write([]byte("fake-mp3"))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	saveBaseURL(t, "elevenlabs", server.URL)

	engine, err := New(Options{Engine: "elevenlabs"})
	if err != nil {
		t.Fatal(err)
	}
	voices, err := engine.Voices(context.Background())
	if err != nil || len(voices) != 1 || voices[0].Name != "Rachel" {
		t.Fatalf("voices = %+v err=%v", voices, err)
	}
	// No voice pinned: the account's first voice is the engine's default.
	outPath := filepath.Join(t.TempDir(), "out.mp3")
	if err := engine.Synthesize(context.Background(), "hello", outPath); err != nil {
		t.Fatal(err)
	}
	if engine.Mime() != "audio/mpeg" {
		t.Errorf("elevenlabs mime = %q", engine.Mime())
	}
}

func TestElevenLabsWithoutAKeyNamesTheFix(t *testing.T) {
	isolateCredentials(t)
	if _, err := New(Options{Engine: "elevenlabs"}); err == nil || !strings.Contains(err.Error(), "ELEVENLABS_API_KEY") {
		t.Errorf("no key must name the env var, got: %v", err)
	}
}

func TestGroqSpeechUsesItsOwnRosterAndModel(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("GROQ_API_KEY", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "playai-tts" || body["voice"] != "Fritz-PlayAI" {
			t.Errorf("groq payload wrong: %+v", body)
		}
		_, _ = w.Write([]byte("RIFFgroq"))
	}))
	defer server.Close()
	saveBaseURL(t, "groq", server.URL)

	engine, err := New(Options{Engine: "groq"})
	if err != nil {
		t.Fatal(err)
	}
	voices, _ := engine.Voices(context.Background())
	if len(voices) == 0 || voices[0].Lang != "en" {
		t.Errorf("playai voices must be marked English-only: %+v", voices[:1])
	}
	outPath := filepath.Join(t.TempDir(), "out.wav")
	if err := engine.Synthesize(context.Background(), "hello", outPath); err != nil {
		t.Fatal(err)
	}
}

func TestGeminiSpeechWrapsThePCMItIsHanded(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("GEMINI_API_KEY", "g-test-key")
	pcm := []byte{1, 2, 3, 4}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "gemini-2.5-flash-preview-tts") {
			t.Errorf("wrong model path: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "g-test-key" {
			t.Error("gemini key header missing")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)
		if !strings.Contains(string(raw), "AUDIO") || !strings.Contains(string(raw), "Kore") {
			t.Errorf("payload missing modality or default voice: %s", raw)
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/L16;codec=pcm;rate=24000","data":"` +
			base64.StdEncoding.EncodeToString(pcm) + `"}}]}}]}`))
	}))
	defer server.Close()
	oldBase := geminiBase
	geminiBase = server.URL
	defer func() { geminiBase = oldBase }()

	engine, err := New(Options{Engine: "gemini"})
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "out.wav")
	if err := engine.Synthesize(context.Background(), "สวัสดี", outPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "RIFF" || len(data) != 44+len(pcm) {
		t.Errorf("not a WAV wrap of the PCM: %d bytes, head %q", len(data), data[:4])
	}
}

func TestPcmRateReadsTheMime(t *testing.T) {
	if got := pcmRate("audio/L16;codec=pcm;rate=16000"); got != 16000 {
		t.Errorf("rate = %d", got)
	}
	if got := pcmRate("audio/L16"); got != 24000 {
		t.Errorf("missing rate must fall back to the documented 24000, got %d", got)
	}
}
