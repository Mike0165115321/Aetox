package tts

// The OpenAI-shaped speech endpoint (POST /audio/speech), worn by two catalog
// rows: OpenAI itself and Groq (playai-tts — same wire format, different host
// and voice roster). Keys come from the credential store the models page
// already fills (config.ProviderAPIKey), and the base URL honors the same
// per-provider override the LLM side uses: point OpenAI's base URL at a
// LocalAI/Speaches box and that row speaks through it, key or no key.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Mikedev115/Aetox/internal/apierr"
	"github.com/Mikedev115/Aetox/internal/config"
)

// apiSpeechSpec is one vendor's wearing of the shared wire format.
type apiSpeechSpec struct {
	defaultBase  string
	provider     string   // credential-store name (the models page's row)
	envVars      []string // key fallbacks
	vendor       string   // shown in errors
	official     string   // keyless calls allowed only off this host
	defaultVoice string
	voices       []string
	voiceLang    string // "" = speaks the text's language; "en" = English only
}

var apiSpeechSpecs = map[string]apiSpeechSpec{
	"openai": {
		defaultBase:  "https://api.openai.com/v1",
		provider:     "openai",
		envVars:      []string{"OPENAI_API_KEY"},
		vendor:       "OpenAI",
		official:     "https://api.openai.com/v1",
		defaultVoice: "alloy",
		// The endpoint's fixed roster — the API has no voice-listing call.
		voices: []string{"alloy", "ash", "ballad", "coral", "echo", "fable", "nova", "onyx", "sage", "shimmer", "verse"},
	},
	"groq": {
		defaultBase:  "https://api.groq.com/openai/v1",
		provider:     "groq",
		envVars:      []string{"GROQ_API_KEY"},
		vendor:       "Groq",
		official:     "https://api.groq.com/openai/v1",
		defaultVoice: "Fritz-PlayAI",
		voices: []string{
			"Arista-PlayAI", "Atlas-PlayAI", "Basil-PlayAI", "Briggs-PlayAI", "Calum-PlayAI",
			"Celeste-PlayAI", "Cheyenne-PlayAI", "Chip-PlayAI", "Cillian-PlayAI", "Deedee-PlayAI",
			"Fritz-PlayAI", "Gail-PlayAI", "Indigo-PlayAI", "Mamaw-PlayAI", "Mason-PlayAI",
			"Mikail-PlayAI", "Mitch-PlayAI", "Quinn-PlayAI", "Thunder-PlayAI",
		},
		voiceLang: "en", // playai-tts speaks English only — the picker should say so
	},
}

type apiSpeech struct {
	id      string
	baseURL string
	apiKey  string
	voice   string
	model   string // resolved from Descriptor.Models + Options.Model
	spec    apiSpeechSpec
}

func newAPISpeech(desc Descriptor, opts Options) (Engine, error) {
	spec, ok := apiSpeechSpecs[desc.ID]
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
	// No key is allowed only off the official host: a local clone usually
	// wants none, the real service always does — and failing here beats
	// failing after the user typed a message they wanted read.
	if key == "" && base == spec.official {
		return nil, fmt.Errorf("ยังไม่มี API key ของ %s — ใส่ได้ที่ ตั้งค่า > โมเดล > %s แล้วเสียงอ่านจะใช้ key เดียวกัน", spec.vendor, spec.vendor)
	}
	return &apiSpeech{id: desc.ID, baseURL: base, apiKey: key, voice: strings.TrimSpace(opts.Voice), model: model, spec: spec}, nil
}

func (a *apiSpeech) ID() string { return a.id }

func (*apiSpeech) Mime() string { return "audio/wav" }

func (a *apiSpeech) Voices(_ context.Context) ([]Voice, error) {
	out := make([]Voice, 0, len(a.spec.voices))
	for _, name := range a.spec.voices {
		// Lang "" means the voice speaks the language of the text it is given,
		// Thai included; "en" marks an English-only roster honestly.
		out = append(out, Voice{ID: name, Name: name, Lang: a.spec.voiceLang})
	}
	return out, nil
}

func (a *apiSpeech) Synthesize(ctx context.Context, text, outPath string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("ไม่มีข้อความให้อ่าน")
	}
	voice := a.voice
	if voice == "" {
		voice = a.spec.defaultVoice
	}
	payload, _ := json.Marshal(map[string]string{
		"model":           a.model,
		"input":           text,
		"voice":           voice,
		"response_format": "wav",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	audio, err := doAudioRequest(req, a.spec.vendor)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, audio, 0o600)
}

// doAudioRequest runs one API call whose success body IS the audio (or JSON to
// parse). A failure body goes through internal/apierr, never onto the page
// verbatim — a 401 quotes the whole redacted key back.
func doAudioRequest(req *http.Request, vendor string) ([]byte, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ต่อ %s ไม่ได้ (%s) — ตัวนี้ต้องต่อเน็ต", vendor, firstLine(err.Error(), err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, apierr.HTTP(vendor, resp.StatusCode, body)
	}
	return body, nil
}
