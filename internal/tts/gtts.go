package tts

// Google Translate's voice, through the gtts-cli CLI (pip install gTTS) — the
// second free cloud vendor. It has no named voices, only languages, so its
// "voices" are language rows ("th: Thai") and the picked voice ID is a
// language code. Same honesty as edge: the text goes to Google.

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// swapped in tests — reuses the edge runner's shape.
var runGTTS = execEdgeTTS

type gttsVoice struct {
	binPath string
	lang    string // "th", "en", ... ; "" = the CLI's default (en)
}

func newGTTS(desc Descriptor, opts Options) (Engine, error) {
	binPath, err := lookBinary(desc)
	if err != nil {
		return nil, err
	}
	return &gttsVoice{binPath: binPath, lang: strings.TrimSpace(opts.Voice)}, nil
}

func (*gttsVoice) ID() string { return "gtts" }

func (*gttsVoice) Mime() string { return "audio/mpeg" }

// Voices runs `gtts-cli --all` and reads its "  th: Thai" lines. One voice per
// language is the vendor's own shape, not a simplification.
func (g *gttsVoice) Voices(ctx context.Context) ([]Voice, error) {
	out, err := runGTTS(ctx, g.binPath, "--all")
	if err != nil {
		return nil, err
	}
	return parseGTTSLanguages(out), nil
}

func (g *gttsVoice) Synthesize(ctx context.Context, text, outPath string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("ไม่มีข้อความให้อ่าน")
	}
	args := []string{text, "--output", outPath}
	if g.lang != "" {
		args = append(args, "--lang", g.lang)
	}
	if _, err := runGTTS(ctx, g.binPath, args...); err != nil {
		return fmt.Errorf("gTTS ไม่สำเร็จ — ตัวนี้ต้องต่อเน็ต (%s)", firstLine(err.Error(), err.Error()))
	}
	return nil
}

func parseGTTSLanguages(raw string) []Voice {
	var voices []Voice
	for _, line := range strings.Split(raw, "\n") {
		code, name, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		code = strings.TrimSpace(code)
		name = strings.TrimSpace(name)
		if code == "" || name == "" || strings.ContainsAny(code, " \t") {
			continue
		}
		voices = append(voices, Voice{ID: code, Name: name + " (" + code + ")", Lang: code})
	}
	// Alphabetical by code so th sits where a Thai eye scans for it.
	sort.Slice(voices, func(i, j int) bool { return voices[i].ID < voices[j].ID })
	return voices
}
