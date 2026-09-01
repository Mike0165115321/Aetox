package tts

// Gemini's native TTS (gemini-2.5-flash-preview-tts): generateContent with an
// AUDIO response modality, a prebuilt voice name, and the audio coming back as
// base64 PCM inside inlineData — no container, so this file wraps the WAV
// header itself, at whatever rate the response's MIME string declares.
//
// The base URL deliberately does NOT honor the per-provider override the
// OpenAI-shaped engines do: Aetox's Gemini provider row may point its base at
// the OpenAI-COMPATIBLE endpoint for chat, and this call is the NATIVE API —
// reusing that override would aim a generateContent call at a /chat/completions
// host. Tests swap geminiBase instead.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/Mikedev115/Aetox/internal/config"
)

// swapped in tests.
var geminiBase = "https://generativelanguage.googleapis.com/v1beta"

// The prebuilt roster from the TTS docs. Every voice is multilingual — Thai is
// on the model's language list — so Lang stays empty.
var geminiVoices = []string{
	"Achernar", "Achird", "Algenib", "Algieba", "Alnilam", "Aoede", "Autonoe",
	"Callirrhoe", "Charon", "Despina", "Enceladus", "Erinome", "Fenrir",
	"Gacrux", "Iapetus", "Kore", "Laomedeia", "Leda", "Orus", "Puck",
	"Pulcherrima", "Rasalgethi", "Sadachbia", "Sadaltager", "Schedar",
	"Sulafat", "Umbriel", "Vindemiatrix", "Zephyr", "Zubenelgenubi",
}

type geminiSpeech struct {
	apiKey string
	voice  string
	model  string // resolved from Descriptor.Models + Options.Model
}

func newGeminiSpeech(desc Descriptor, opts Options) (Engine, error) {
	key := config.ProviderAPIKey("gemini", "GEMINI_API_KEY", "GOOGLE_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ยังไม่มี API key ของ Gemini — ใส่ได้ที่ ตั้งค่า > โมเดล > Gemini แล้วเสียงอ่านจะใช้ key เดียวกัน")
	}
	model, err := resolveNamedModel(desc, opts.Model)
	if err != nil {
		return nil, err
	}
	return &geminiSpeech{apiKey: key, voice: strings.TrimSpace(opts.Voice), model: model}, nil
}

func (*geminiSpeech) ID() string { return "gemini" }

func (*geminiSpeech) Mime() string { return "audio/wav" }

func (g *geminiSpeech) Voices(_ context.Context) ([]Voice, error) {
	out := make([]Voice, 0, len(geminiVoices))
	for _, name := range geminiVoices {
		out = append(out, Voice{ID: name, Name: name})
	}
	return out, nil
}

func (g *geminiSpeech) Synthesize(ctx context.Context, text, outPath string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("ไม่มีข้อความให้อ่าน")
	}
	voice := g.voice
	if voice == "" {
		voice = "Kore"
	}
	payload, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{{"parts": []map[string]any{{"text": text}}}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"speechConfig": map[string]any{
				"voiceConfig": map[string]any{
					"prebuiltVoiceConfig": map[string]any{"voiceName": voice},
				},
			},
		},
	})
	endpoint := geminiBase + "/models/" + g.model + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)
	body, err := doAudioRequest(req, "Gemini")
	if err != nil {
		return err
	}
	var reply struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return fmt.Errorf("อ่านคำตอบเสียงจาก Gemini ไม่ได้ (%v)", err)
	}
	for _, cand := range reply.Candidates {
		for _, part := range cand.Content.Parts {
			if part.InlineData.Data == "" {
				continue
			}
			pcm, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if err != nil {
				return fmt.Errorf("ถอดข้อมูลเสียงจาก Gemini ไม่ได้ (%v)", err)
			}
			return os.WriteFile(outPath, pcmToWav(pcm, pcmRate(part.InlineData.MimeType)), 0o600)
		}
	}
	return fmt.Errorf("Gemini ตอบกลับโดยไม่มีเสียงมาด้วย")
}

// pcmRate reads the sample rate off Gemini's audio MIME string
// ("audio/L16;codec=pcm;rate=24000"). Missing means the documented 24000.
func pcmRate(mime string) int {
	for _, field := range strings.Split(mime, ";") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(field), "rate="); ok {
			if rate, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && rate > 0 {
				return rate
			}
		}
	}
	return 24000
}

// pcmToWav wraps raw 16-bit mono PCM in the 44-byte RIFF header a player
// needs. The one audio-format fact this package hand-writes, kept in one
// place.
func pcmToWav(pcm []byte, rate int) []byte {
	const (
		channels      = 1
		bitsPerSample = 16
	)
	byteRate := rate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	buf := bytes.NewBuffer(make([]byte, 0, 44+len(pcm)))
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(rate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}
