package tts

// Microsoft Edge's online voices, through the edge-tts CLI (pip install
// edge-tts) — the free cloud vendor with real Thai neural voices
// (th-TH-PremwadeeNeural, th-TH-NiwatNeural), no key, no account. Cloud means
// cloud: the TEXT goes to Microsoft and MP3 comes back. The descriptor's
// Install text says so; this file just does the work.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// swapped in tests.
var runEdgeTTS = execEdgeTTS

type edgeVoice struct {
	binPath string
	voice   string // ShortName like "th-TH-PremwadeeNeural"; "" = the CLI's default
}

func newEdge(desc Descriptor, opts Options) (Engine, error) {
	binPath, err := lookBinary(desc)
	if err != nil {
		return nil, err
	}
	return &edgeVoice{binPath: binPath, voice: strings.TrimSpace(opts.Voice)}, nil
}

func (*edgeVoice) ID() string { return "edge" }

func (*edgeVoice) Mime() string { return "audio/mpeg" }

// Voices runs `edge-tts --list-voices` and reads its table: ShortName first,
// Gender second, the rest is prose. The ShortName carries the locale
// ("th-TH-PremwadeeNeural"), which is all the language information needed.
func (e *edgeVoice) Voices(ctx context.Context) ([]Voice, error) {
	out, err := runEdgeTTS(ctx, e.binPath, "--list-voices")
	if err != nil {
		return nil, err
	}
	return parseEdgeVoices(out), nil
}

func (e *edgeVoice) Synthesize(ctx context.Context, text, outPath string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("ไม่มีข้อความให้อ่าน")
	}
	// --text as a plain argv entry: exec.Command passes it without a shell, so
	// there is nothing to quote and nothing to inject.
	args := []string{"--text", text, "--write-media", outPath}
	if e.voice != "" {
		args = append([]string{"--voice", e.voice}, args...)
	}
	_, err := runEdgeTTS(ctx, e.binPath, args...)
	return err
}

func parseEdgeVoices(raw string) []Voice {
	var voices []Voice
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		// The table's header and rule lines have no locale-shaped first column.
		if !strings.Contains(name, "-") || strings.HasPrefix(name, "-") {
			continue
		}
		lang := name
		if parts := strings.SplitN(name, "-", 3); len(parts) >= 2 {
			lang = parts[0] + "-" + parts[1]
		}
		voices = append(voices, Voice{ID: name, Name: name, Lang: lang, Gender: fields[1]})
	}
	return voices
}

func execEdgeTTS(ctx context.Context, binPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, binPath, args...)
	proc.HideConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("ไม่พบโปรแกรม edge-tts — ติดตั้งด้วย: pip install edge-tts")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("edge-tts ไม่สำเร็จ (%s) — ตัวนี้ต้องต่อเน็ต เพราะเสียงสังเคราะห์บนคลาวด์ของ Microsoft", firstLine(msg, msg))
	}
	return stdout.String(), nil
}

// lookBinary resolves a CLI vendor's program: PATH first, then the pip --user
// Scripts folders, which pip fills WITHOUT putting them on PATH — measured on
// the owner's machine 2026-09-01, where `pip install edge-tts` landed in
// %APPDATA%\Python\Python313\Scripts and LookPath alone called it missing.
// Shared by the edge and gtts engines.
func lookBinary(desc Descriptor) (string, error) {
	for _, name := range desc.Binaries {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		for _, name := range desc.Binaries {
			matches, _ := filepath.Glob(filepath.Join(appData, "Python", "*", "Scripts", name+".exe"))
			sort.Sort(sort.Reverse(sort.StringSlice(matches))) // newest Python first
			for _, match := range matches {
				if info, err := os.Stat(match); err == nil && !info.IsDir() {
					return match, nil
				}
			}
		}
	}
	return "", fmt.Errorf("ไม่พบโปรแกรม %s ในเครื่อง — %s", strings.Join(desc.Binaries, "/"), desc.Install)
}
