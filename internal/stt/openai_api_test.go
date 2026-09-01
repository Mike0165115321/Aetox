package stt

// The cloud transcription rows, tested without a cloud: httptest wearing the
// wire format, reached through the same per-provider base-URL override a user
// would set — which is also the mechanism that makes local OpenAI-compatible
// servers work, so this test covers that path, not a lookalike.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
)

func isolateCloudCreds(t *testing.T) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "")
}

func TestParseVerboseJSON(t *testing.T) {
	body := []byte(`{"text":"สวัสดีครับ ทดสอบ","segments":[
		{"start":0.0,"end":1.5,"text":" สวัสดีครับ"},
		{"start":1.5,"end":3.0,"text":" ทดสอบ"},
		{"start":3.0,"end":3.2,"text":"   "}]}`)
	segments, err := parseVerboseJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d: %+v", len(segments), segments)
	}
	if segments[1].StartMs != 1500 || segments[1].EndMs != 3000 || segments[1].Text != "ทดสอบ" {
		t.Errorf("segment translated wrong: %+v", segments[1])
	}
}

func TestParseVerboseJSONFallsBackToBareText(t *testing.T) {
	segments, err := parseVerboseJSON([]byte(`{"text":"a server that ignored response_format"}`))
	if err != nil || len(segments) != 1 {
		t.Fatalf("segments=%+v err=%v", segments, err)
	}
	if segments[0].Text == "" || segments[0].StartMs != 0 {
		t.Errorf("bare text must survive as one untimed segment: %+v", segments[0])
	}
}

func TestCloudTranscriberWithoutAKeyNamesTheFix(t *testing.T) {
	isolateCloudCreds(t)
	for _, id := range []string{"openai", "groq"} {
		if _, err := New(Options{Engine: id}); err == nil || !strings.Contains(err.Error(), "API key") {
			t.Errorf("%s: no key on the official host must name the fix, got: %v", id, err)
		}
	}
}

func TestCloudTranscriberSpeaksTheWireFormat(t *testing.T) {
	isolateCloudCreds(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("not multipart: %v", err)
		}
		if got := r.FormValue("model"); got != "whisper-1" {
			t.Errorf("model = %q", got)
		}
		if got := r.FormValue("response_format"); got != "verbose_json" {
			t.Errorf("response_format = %q", got)
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Errorf("file part missing: %v", err)
		}
		_, _ = w.Write([]byte(`{"segments":[{"start":0,"end":1,"text":"สวัสดี"}]}`))
	}))
	defer server.Close()
	pref := config.ModelPreference{}
	pref.SetBaseURLForProvider("openai", server.URL)
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatal(err)
	}

	engine, err := New(Options{Engine: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	wavPath := filepath.Join(t.TempDir(), "in.wav")
	if err := os.WriteFile(wavPath, []byte("RIFFfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	segments, err := engine.Transcribe(context.Background(), wavPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].Text != "สวัสดี" {
		t.Errorf("segments = %+v", segments)
	}
	if engine.ModelPath() != "whisper-1" {
		t.Errorf("ModelPath = %q, want the model name", engine.ModelPath())
	}
}

func TestMistralRowSpeaksItsOwnDialect(t *testing.T) {
	isolateCloudCreds(t)
	t.Setenv("MISTRAL_API_KEY", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("not multipart: %v", err)
		}
		if got := r.FormValue("model"); got != "voxtral-mini-latest" {
			t.Errorf("model = %q", got)
		}
		// Mistral's spelling of "give me segment times" — and NOT the OpenAI
		// one, which a strict parser could refuse.
		if got := r.FormValue("timestamp_granularities"); got != "segment" {
			t.Errorf("timestamp_granularities = %q", got)
		}
		if got := r.FormValue("response_format"); got != "" {
			t.Errorf("response_format leaked into the mistral dialect: %q", got)
		}
		_, _ = w.Write([]byte(`{"text":"สวัสดี","segments":[{"start":0,"end":1,"text":"สวัสดี"}]}`))
	}))
	defer server.Close()
	pref := config.ModelPreference{}
	pref.SetBaseURLForProvider("mistral", server.URL)
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatal(err)
	}

	engine, err := New(Options{Engine: "mistral"})
	if err != nil {
		t.Fatal(err)
	}
	wavPath := filepath.Join(t.TempDir(), "in.wav")
	if err := os.WriteFile(wavPath, []byte("RIFFfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	segments, err := engine.Transcribe(context.Background(), wavPath)
	if err != nil || len(segments) != 1 {
		t.Fatalf("segments=%+v err=%v", segments, err)
	}
}

func TestElevenLabsRowSpeaksItsOwnDialect(t *testing.T) {
	isolateCloudCreds(t)
	t.Setenv("ELEVENLABS_API_KEY", "xi-test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/speech-to-text" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		if r.Header.Get("xi-api-key") != "xi-test-key" {
			t.Error("xi-api-key header missing")
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("bearer header leaked into the elevenlabs dialect")
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("not multipart: %v", err)
		}
		if got := r.FormValue("model_id"); got != "scribe_v1" {
			t.Errorf("model_id = %q", got)
		}
		// Scribe answers text + words, no segments — the fallback must carry
		// the words through as one untimed segment.
		_, _ = w.Write([]byte(`{"text":"hello from scribe","words":[]}`))
	}))
	defer server.Close()
	pref := config.ModelPreference{}
	pref.SetBaseURLForProvider("elevenlabs", server.URL)
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatal(err)
	}

	engine, err := New(Options{Engine: "elevenlabs"})
	if err != nil {
		t.Fatal(err)
	}
	wavPath := filepath.Join(t.TempDir(), "in.wav")
	if err := os.WriteFile(wavPath, []byte("RIFFfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	segments, err := engine.Transcribe(context.Background(), wavPath)
	if err != nil || len(segments) != 1 || segments[0].Text != "hello from scribe" {
		t.Fatalf("segments=%+v err=%v", segments, err)
	}
}

func TestGeminiTranscriberSendsInlineAudioAndReadsText(t *testing.T) {
	isolateCloudCreds(t)
	t.Setenv("GEMINI_API_KEY", "g-test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "gemini-2.5-flash") {
			t.Errorf("wrong model path: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "g-test-key" {
			t.Error("gemini key header missing")
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "inlineData") || !strings.Contains(string(raw), "audio/wav") {
			t.Error("audio did not travel inline")
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"สวัสดีครับ ทดสอบ"}]}}]}`))
	}))
	defer server.Close()
	oldBase := geminiSTTBase
	geminiSTTBase = server.URL
	defer func() { geminiSTTBase = oldBase }()

	engine, err := New(Options{Engine: "gemini"})
	if err != nil {
		t.Fatal(err)
	}
	wavPath := filepath.Join(t.TempDir(), "in.wav")
	if err := os.WriteFile(wavPath, []byte("RIFFfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	segments, err := engine.Transcribe(context.Background(), wavPath)
	if err != nil || len(segments) != 1 || segments[0].Text != "สวัสดีครับ ทดสอบ" {
		t.Fatalf("segments=%+v err=%v", segments, err)
	}
}

func TestEveryCloudRowRefusesToRunKeyless(t *testing.T) {
	isolateCloudCreds(t)
	t.Setenv("MISTRAL_API_KEY", "")
	t.Setenv("ELEVENLABS_API_KEY", "")
	t.Setenv("ELEVEN_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	for _, id := range []string{"openai", "groq", "mistral", "gemini", "elevenlabs"} {
		if _, err := New(Options{Engine: id}); err == nil {
			t.Errorf("%s: keyless construction on the official host must fail loudly", id)
		}
	}
}

// gpt-4o-transcribe rejects verbose_json outright — the OpenAI dialect's own
// asymmetry. Only whisper models may ask for segment times.
func TestOpenAIModelPickSwitchesTheResponseFormat(t *testing.T) {
	isolateCloudCreds(t)
	var gotFormat, gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(16 << 20)
		gotModel = r.FormValue("model")
		gotFormat = r.FormValue("response_format")
		_, _ = w.Write([]byte(`{"text":"ok"}`))
	}))
	defer server.Close()
	pref := config.ModelPreference{}
	pref.SetBaseURLForProvider("openai", server.URL)
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatal(err)
	}
	wavPath := filepath.Join(t.TempDir(), "in.wav")
	if err := os.WriteFile(wavPath, []byte("RIFFfake"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine, err := New(Options{Engine: "openai", Model: "gpt-4o-mini-transcribe"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Transcribe(context.Background(), wavPath); err != nil {
		t.Fatal(err)
	}
	if gotModel != "gpt-4o-mini-transcribe" || gotFormat != "" {
		t.Errorf("model=%q format=%q — gpt-4o must not be asked for verbose_json", gotModel, gotFormat)
	}
	if engine.ModelPath() != "gpt-4o-mini-transcribe" {
		t.Errorf("ModelPath = %q", engine.ModelPath())
	}

	engine, err = New(Options{Engine: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Transcribe(context.Background(), wavPath); err != nil {
		t.Fatal(err)
	}
	if gotModel != "whisper-1" || gotFormat != "verbose_json" {
		t.Errorf("model=%q format=%q — the default whisper-1 still carries segment times", gotModel, gotFormat)
	}
}

func TestNamedModelRosters(t *testing.T) {
	if _, err := New(Options{Engine: "groq", Model: "no-such-model"}); err == nil || !strings.Contains(err.Error(), "whisper-large-v3") {
		t.Errorf("an unknown pin must fail loudly and name the roster, got: %v", err)
	}
	// Every roster's first entry is the shipped default — the settings page
	// renders it as the pre-selected option.
	for _, d := range Catalog() {
		if len(d.Models) == 1 {
			continue
		}
		for i, m := range d.Models {
			for j := i + 1; j < len(d.Models); j++ {
				if strings.EqualFold(m, d.Models[j]) {
					t.Errorf("engine %s lists model %q twice", d.ID, m)
				}
			}
		}
	}
}
