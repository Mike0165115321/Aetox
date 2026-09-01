package stt

// The OpenAI-shaped transcription endpoint (POST /audio/transcriptions), worn
// by two catalog rows: OpenAI itself (whisper-1) and Groq (whisper-large-v3,
// same wire format, different host). The per-provider base-URL override the
// LLM side already has works here too, so any local server that clones the
// API — Speaches, LocalAI, a future LM Studio — is the OpenAI row pointed at
// another address, not a new engine.
//
// These rows are the owner's 2026-09-01 amendment to §31: local engines stay
// the default and the recordings-never-leave rule still describes them, but a
// cloud row chosen by name, whose Install text says the audio goes out, is an
// informed trade the user is allowed to make. The catalog no longer refuses
// it on their behalf.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/Mikedev115/Aetox/internal/apierr"
	"github.com/Mikedev115/Aetox/internal/config"
)

// apiTranscriberSpec is one vendor's wearing of the shared multipart shape.
// The dialects differ only in field names and headers — Mistral's Voxtral and
// ElevenLabs' Scribe are the same POST with different spellings, which is a
// spec entry, not an engine.
type apiTranscriberSpec struct {
	defaultBase string
	provider    string   // credential-store name (the models page's row)
	envVars     []string // key fallbacks
	vendor      string   // shown in errors
	// keyOptional allows a keyless call off the official host — a local clone
	// usually wants none.
	official string
	// path is the endpoint under the base; empty means the OpenAI spelling.
	path string
	// modelField is the multipart field the model name travels in; empty
	// means the OpenAI spelling ("model").
	modelField string
	// keyHeader is the header the key travels in; empty means bearer auth.
	keyHeader string
	// extraFields ride the form as-is — how a vendor is asked for segment
	// timestamps in its own dialect. Absent fields are absent on the wire, so
	// a strict parser is never shown a spelling it does not know.
	extraFields map[string]string
}

var apiTranscribers = map[string]apiTranscriberSpec{
	"openai": {
		defaultBase: "https://api.openai.com/v1",
		provider:    "openai",
		envVars:     []string{"OPENAI_API_KEY"},
		vendor:      "OpenAI",
		official:    "https://api.openai.com/v1",
		extraFields: map[string]string{"response_format": "verbose_json"},
	},
	"groq": {
		defaultBase: "https://api.groq.com/openai/v1",
		provider:    "groq",
		envVars:     []string{"GROQ_API_KEY"},
		vendor:      "Groq",
		official:    "https://api.groq.com/openai/v1",
		extraFields: map[string]string{"response_format": "verbose_json"},
	},
	"mistral": {
		defaultBase: "https://api.mistral.ai/v1",
		provider:    "mistral",
		envVars:     []string{"MISTRAL_API_KEY"},
		vendor:      "Mistral",
		official:    "https://api.mistral.ai/v1",
		extraFields: map[string]string{"timestamp_granularities": "segment"},
	},
	"elevenlabs": {
		defaultBase: "https://api.elevenlabs.io/v1",
		provider:    "elevenlabs",
		envVars:     []string{"ELEVENLABS_API_KEY", "ELEVEN_API_KEY"},
		vendor:      "ElevenLabs",
		official:    "https://api.elevenlabs.io/v1",
		path:        "/speech-to-text",
		modelField:  "model_id",
		keyHeader:   "xi-api-key",
	},
}

type apiTranscriber struct {
	id      string
	baseURL string
	apiKey  string
	model   string // resolved from Descriptor.Models + Options.Model
	spec    apiTranscriberSpec
}

func newAPITranscriber(desc Descriptor, opts Options) (Engine, error) {
	spec, ok := apiTranscribers[desc.ID]
	if !ok {
		return nil, fmt.Errorf("engine %q อยู่ในรายการแต่ยังไม่มีตัวรัน", desc.ID)
	}
	model, err := resolveNamedModel(desc, opts.Model)
	if err != nil {
		return nil, err
	}
	key := config.ProviderAPIKey(spec.provider, spec.envVars...)
	base := strings.TrimRight(config.ProviderBaseURL(spec.provider), "/")
	if base == "" {
		base = spec.defaultBase
	}
	if key == "" && base == spec.official {
		// ElevenLabs is not a models-page row, so pointing there would lie.
		if spec.provider == "elevenlabs" {
			return nil, fmt.Errorf("ยังไม่มี API key ของ ElevenLabs — ตั้ง environment variable ELEVENLABS_API_KEY (สมัครที่ elevenlabs.io) แล้วเปิดแอปใหม่")
		}
		return nil, fmt.Errorf("ยังไม่มี API key ของ %s — ใส่ได้ที่ ตั้งค่า > โมเดล > %s แล้วตัวถอดเสียงจะใช้ key เดียวกัน", spec.vendor, spec.vendor)
	}
	return &apiTranscriber{id: desc.ID, baseURL: base, apiKey: key, model: model, spec: spec}, nil
}

func (a *apiTranscriber) ID() string { return a.id }

// ModelPath is the model NAME — there is no file on this machine, and saying
// so is more honest than inventing a path.
func (a *apiTranscriber) ModelPath() string { return a.model }

func (a *apiTranscriber) ModelCaution() string { return "" }

func (a *apiTranscriber) Transcribe(ctx context.Context, wavPath string) ([]Segment, error) {
	audio, err := os.ReadFile(wavPath)
	if err != nil {
		return nil, err
	}
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(audio); err != nil {
		return nil, err
	}
	modelField := a.spec.modelField
	if modelField == "" {
		modelField = "model"
	}
	_ = writer.WriteField(modelField, a.model)
	// The vendor's own spelling of "give me segment times" — verbose_json for
	// the OpenAI dialect, timestamp_granularities for Mistral, nothing for
	// ElevenLabs, whose reply is text plus words.
	for field, value := range a.spec.extraFields {
		// One asymmetry inside the OpenAI dialect itself: gpt-4o-transcribe
		// and -mini reject verbose_json outright (only whisper models carry
		// segment times). Their default json still returns text, which the
		// parser carries through as one untimed segment.
		if field == "response_format" && !strings.HasPrefix(strings.ToLower(a.model), "whisper") {
			continue
		}
		_ = writer.WriteField(field, value)
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	path := a.spec.path
	if path == "" {
		path = "/audio/transcriptions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, &form)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if a.apiKey != "" {
		if a.spec.keyHeader != "" {
			req.Header.Set(a.spec.keyHeader, a.apiKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+a.apiKey)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ต่อ %s ไม่ได้ (%s) — ตัวนี้ต้องต่อเน็ต", a.spec.vendor, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, apierr.HTTP(a.spec.vendor, resp.StatusCode, body)
	}
	return parseVerboseJSON(body)
}

// parseVerboseJSON reads verbose_json into []Segment. A server that ignored
// response_format and sent only {"text": ...} still yields its words as one
// untimed segment — text that arrived must never be dropped over a missing
// timestamp.
func parseVerboseJSON(body []byte) ([]Segment, error) {
	var payload struct {
		Text     string `json:"text"`
		Segments []struct {
			Start float64 `json:"start"`
			End   float64 `json:"end"`
			Text  string  `json:"text"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("อ่านคำตอบของตัวถอดเสียงไม่ได้ (%v)", err)
	}
	var segments []Segment
	for _, s := range payload.Segments {
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}
		segments = append(segments, Segment{
			StartMs: int(s.Start * 1000),
			EndMs:   int(s.End * 1000),
			Text:    text,
		})
	}
	if len(segments) == 0 {
		if text := strings.TrimSpace(payload.Text); text != "" {
			segments = append(segments, Segment{Text: text})
		}
	}
	return segments, nil
}
