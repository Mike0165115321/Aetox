package tts

// ElevenLabs — the premium voice vendor. Not on the models page's provider
// list, so its key comes from the environment (ELEVENLABS_API_KEY, or the
// older ELEVEN_API_KEY) or a manually added provider entry; the Install text
// says which. Voices are the account's own roster (GET /v1/voices), so what
// the picker shows is what this key can actually speak with.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Mikedev115/Aetox/internal/config"
)

const elevenDefaultBase = "https://api.elevenlabs.io/v1"

type elevenLabs struct {
	baseURL string
	apiKey  string
	voice   string // voice_id
	model   string // resolved from Descriptor.Models + Options.Model
}

func newElevenLabs(desc Descriptor, opts Options) (Engine, error) {
	key := config.ProviderAPIKey("elevenlabs", "ELEVENLABS_API_KEY", "ELEVEN_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ยังไม่มี API key ของ ElevenLabs — ตั้ง environment variable ELEVENLABS_API_KEY (สมัครที่ elevenlabs.io) แล้วเปิดแอปใหม่")
	}
	model, err := resolveNamedModel(desc, opts.Model)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(config.ProviderBaseURL("elevenlabs"), "/")
	if base == "" {
		base = elevenDefaultBase
	}
	return &elevenLabs{baseURL: base, apiKey: key, voice: strings.TrimSpace(opts.Voice), model: model}, nil
}

func (*elevenLabs) ID() string { return "elevenlabs" }

func (*elevenLabs) Mime() string { return "audio/mpeg" }

func (e *elevenLabs) Voices(ctx context.Context) ([]Voice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/voices", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", e.apiKey)
	body, err := doAudioRequest(req, "ElevenLabs")
	if err != nil {
		return nil, err
	}
	var listing struct {
		Voices []struct {
			VoiceID string            `json:"voice_id"`
			Name    string            `json:"name"`
			Labels  map[string]string `json:"labels"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("อ่านรายชื่อเสียงจาก ElevenLabs ไม่ได้ (%v)", err)
	}
	out := make([]Voice, 0, len(listing.Voices))
	for _, v := range listing.Voices {
		out = append(out, Voice{
			ID:     v.VoiceID,
			Name:   v.Name,
			Lang:   v.Labels["language"],
			Gender: v.Labels["gender"],
		})
	}
	return out, nil
}

func (e *elevenLabs) Synthesize(ctx context.Context, text, outPath string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("ไม่มีข้อความให้อ่าน")
	}
	voice := e.voice
	if voice == "" {
		voices, err := e.Voices(ctx)
		if err != nil {
			return err
		}
		if len(voices) == 0 {
			return fmt.Errorf("บัญชี ElevenLabs นี้ยังไม่มีเสียงเลย — เพิ่มเสียงใน Voice Library ของ elevenlabs.io ก่อน")
		}
		voice = voices[0].ID
	}
	payload, _ := json.Marshal(map[string]string{"text": text, "model_id": e.model})
	endpoint := e.baseURL + "/text-to-speech/" + url.PathEscape(voice) + "?output_format=mp3_44100_128"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", e.apiKey)
	audio, err := doAudioRequest(req, "ElevenLabs")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, audio, 0o600)
}
