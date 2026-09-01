package stt

// Gemini's audio understanding as a transcriber: generateContent with the WAV
// inline and one instruction — transcribe verbatim. What comes back is plain
// text, not segments; a dictated sentence never needed timestamps, and
// audio_transcribe renders an untimed segment honestly as a single line.
//
// Same rule as internal/tts's Gemini engine: the base URL does NOT ride the
// per-provider override, because Aetox's Gemini provider row may point at the
// OpenAI-compatible endpoint for chat and this is the native API. Tests swap
// geminiSTTBase.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Mikedev115/Aetox/internal/apierr"
	"github.com/Mikedev115/Aetox/internal/config"
)

// swapped in tests.
var geminiSTTBase = "https://generativelanguage.googleapis.com/v1beta"

type geminiTranscriber struct {
	apiKey string
	model  string // resolved from Descriptor.Models + Options.Model
}

func newGeminiTranscriber(desc Descriptor, opts Options) (Engine, error) {
	key := config.ProviderAPIKey("gemini", "GEMINI_API_KEY", "GOOGLE_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ยังไม่มี API key ของ Gemini — ใส่ได้ที่ ตั้งค่า > โมเดล > Gemini แล้วตัวถอดเสียงจะใช้ key เดียวกัน")
	}
	model, err := resolveNamedModel(desc, opts.Model)
	if err != nil {
		return nil, err
	}
	return &geminiTranscriber{apiKey: key, model: model}, nil
}

func (*geminiTranscriber) ID() string { return "gemini" }

func (g *geminiTranscriber) ModelPath() string { return g.model }

func (*geminiTranscriber) ModelCaution() string { return "" }

func (g *geminiTranscriber) Transcribe(ctx context.Context, wavPath string) ([]Segment, error) {
	audio, err := os.ReadFile(wavPath)
	if err != nil {
		return nil, err
	}
	// generateContent carries inline data up to ~20MB of request — a mic clip
	// or a normalized 16kHz mono track fits many minutes in that. Refusing
	// beyond it beats a confusing 400 from the API.
	if len(audio) > 19<<20 {
		return nil, fmt.Errorf("ไฟล์เสียงใหญ่เกินที่ Gemini รับแบบแนบตรง (~20MB) — ใช้เอนจินอื่นกับไฟล์ยาวขนาดนี้")
	}
	payload, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]any{
				{"text": "Transcribe this audio verbatim, in its original language. Output only the spoken words, nothing else."},
				{"inlineData": map[string]string{
					"mimeType": "audio/wav",
					"data":     base64.StdEncoding.EncodeToString(audio),
				}},
			},
		}},
	})
	endpoint := geminiSTTBase + "/models/" + g.model + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ต่อ Gemini ไม่ได้ (%s) — ตัวนี้ต้องต่อเน็ต", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, apierr.HTTP("Gemini", resp.StatusCode, body)
	}
	var reply struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, fmt.Errorf("อ่านคำตอบของ Gemini ไม่ได้ (%v)", err)
	}
	var parts []string
	for _, cand := range reply.Candidates {
		for _, part := range cand.Content.Parts {
			if text := strings.TrimSpace(part.Text); text != "" {
				parts = append(parts, text)
			}
		}
		break // the first candidate is the answer; the rest are alternatives
	}
	text := strings.TrimSpace(strings.Join(parts, " "))
	if text == "" {
		return nil, nil
	}
	return []Segment{{Text: text}}, nil
}
