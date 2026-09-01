package tts

// Windows SAPI: the first engine, and the reference for what a Descriptor's
// constructor owes the rest of the package — enumerate voices, speak text into
// a WAV, and keep everything Windows-specific behind the Engine interface.
//
// The mechanics are a PowerShell script driving SAPI COM, because that is the
// one speech surface every Windows has with no install step. The wrinkle that
// shaped this file: Windows keeps voices in TWO registries that cannot see
// each other. System.Speech / plain SAPI only enumerates the old "Desktop"
// voices (David, Zira); the modern OneCore voices — including every Thai voice
// a Thai Windows 11 actually has (Pattara) — live under Speech_OneCore and are
// only reachable by pointing SpObjectTokenCategory at that registry key
// directly. Measured on the owner's machine 2026-09-01: System.Speech in
// powershell.exe 5.1 saw 2 voices and no Thai; the token-category route saw
// all 6 including Pattara. So this file enumerates both registries, OneCore
// first, and never goes through System.Speech.
//
// powershell.exe (5.1) explicitly, not pwsh: it is the one guaranteed present,
// and everything here is COM, which both speak identically.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// errMarker is how the script reports a failure on stdout — stderr carries
// CLIXML noise from -EncodedCommand, so it cannot be the channel.
const errMarker = "AETOX_TTS_ERR|"

// runPowerShell is swapped in tests; production always runs powershell.exe.
var runPowerShell = execPowerShell

type windowsVoice struct {
	voice string // Voice.ID to speak with; "" = SAPI's own default
}

func newWindowsVoice(_ Descriptor, opts Options) (Engine, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("เอนจินเสียงอ่านของ Windows ใช้ได้เฉพาะบน Windows — เครื่องนี้คือ %s", runtime.GOOS)
	}
	return &windowsVoice{voice: strings.TrimSpace(opts.Voice)}, nil
}

func (*windowsVoice) ID() string { return "windows" }

func (*windowsVoice) Mime() string { return "audio/wav" }

// tokenPrelude collects voice tokens from both registries into $aetoxTokens —
// OneCore first, so the modern copy of a voice wins the dedupe downstream.
// Each registry is its own try/catch: a machine where one category is missing
// still speaks with the other.
const tokenPrelude = `$ErrorActionPreference = 'Stop'
$aetoxTokens = @()
try {
  $cat = New-Object -ComObject SAPI.SpObjectTokenCategory
  $cat.SetId('HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Speech_OneCore\Voices', $false)
  foreach ($t in $cat.EnumerateTokens()) { $aetoxTokens += $t }
} catch {}
try {
  $sv = New-Object -ComObject SAPI.SpVoice
  foreach ($t in $sv.GetVoices()) { $aetoxTokens += $t }
} catch {}
`

func (w *windowsVoice) Voices(ctx context.Context) ([]Voice, error) {
	script := tokenPrelude + `try {
  $seen = @{}
  foreach ($t in $aetoxTokens) {
    $d = $t.GetDescription()
    if ($seen.ContainsKey($d)) { continue }
    $seen[$d] = $true
    $lang = ''; $gender = ''
    try { $lang = $t.GetAttribute('Language') } catch {}
    try { $gender = $t.GetAttribute('Gender') } catch {}
    Write-Output ('V|' + $d + '|' + $lang + '|' + $gender)
  }
} catch { Write-Output ('` + errMarker + `' + $_.Exception.Message); exit 1 }`
	out, err := runPowerShell(ctx, script)
	if scriptErr := findScriptError(out); scriptErr != "" {
		return nil, fmt.Errorf("อ่านรายชื่อเสียงในเครื่องไม่ได้ (%s)", scriptErr)
	}
	if err != nil {
		return nil, err
	}
	return parseVoiceLines(out), nil
}

func (w *windowsVoice) Synthesize(ctx context.Context, text, wavPath string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("ไม่มีข้อความให้อ่าน")
	}
	// Through a file, not through the command line: the text is arbitrary user
	// content, and a file has no quoting problem to get wrong.
	tmpDir, err := os.MkdirTemp("", "aetox-tts-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	txtPath := filepath.Join(tmpDir, "say.txt")
	if err := os.WriteFile(txtPath, []byte(text), 0o600); err != nil {
		return err
	}
	// SSFMCreateForWrite = 3. Speak flag 0 = synchronous plain text, which is
	// what makes the exit of the process mean the WAV is complete.
	script := tokenPrelude + `try {
  $want = ` + psQuote(w.voice) + `
  $tok = $null
  if ($want -ne '') {
    foreach ($t in $aetoxTokens) { if ($t.GetDescription() -eq $want) { $tok = $t; break } }
    if ($null -eq $tok) { Write-Output '` + errMarker + `voice-not-found'; exit 1 }
  }
  $v = New-Object -ComObject SAPI.SpVoice
  if ($tok) { $v.Voice = $tok }
  $f = New-Object -ComObject SAPI.SpFileStream
  $f.Open(` + psQuote(wavPath) + `, 3, $false)
  $v.AudioOutputStream = $f
  $sr = New-Object IO.StreamReader(` + psQuote(txtPath) + `, [Text.Encoding]::UTF8)
  $text = $sr.ReadToEnd(); $sr.Close()
  $v.Speak($text, 0) | Out-Null
  $f.Close()
} catch { Write-Output ('` + errMarker + `' + $_.Exception.Message); exit 1 }`
	out, runErr := runPowerShell(ctx, script)
	if scriptErr := findScriptError(out); scriptErr != "" {
		if scriptErr == "voice-not-found" {
			return fmt.Errorf("ไม่พบเสียงชื่อ %q ในเครื่องแล้ว — เลือกเสียงใหม่ในหน้าตั้งค่า > เสียง", w.voice)
		}
		return fmt.Errorf("อ่านออกเสียงไม่สำเร็จ (%s)", scriptErr)
	}
	return runErr
}

// parseVoiceLines translates "V|desc|langLCID|gender" lines into []Voice,
// dropping anything else — progress noise, blank lines, whatever a profile
// script might have printed despite -NoProfile.
func parseVoiceLines(raw string) []Voice {
	var voices []Voice
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "V|") {
			continue
		}
		parts := strings.SplitN(line[2:], "|", 3)
		if len(parts) < 1 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		v := Voice{ID: strings.TrimSpace(parts[0]), Name: strings.TrimSpace(parts[0])}
		if len(parts) > 1 {
			v.Lang = lcidToTag(parts[1])
		}
		if len(parts) > 2 {
			v.Gender = strings.TrimSpace(parts[2])
		}
		voices = append(voices, v)
	}
	return voices
}

// findScriptError pulls the message out of the first errMarker line, "" when
// the script never printed one.
func findScriptError(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, errMarker) {
			return strings.TrimSpace(strings.TrimPrefix(line, errMarker))
		}
	}
	return ""
}

// lcidToTag maps SAPI's hex LCID attribute ("41E", sometimes "409;9") to a
// readable tag. Unknown codes come back verbatim — a raw hex code the picker
// shows honestly beats a guessed language.
func lcidToTag(raw string) string {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if i := strings.IndexByte(code, ';'); i >= 0 {
		code = code[:i]
	}
	code = strings.TrimLeft(code, "0")
	tags := map[string]string{
		"409": "en-US", "809": "en-GB", "C09": "en-AU", "1009": "en-CA", "4009": "en-IN",
		"41E": "th-TH",
		"804": "zh-CN", "404": "zh-TW", "C04": "zh-HK",
		"411": "ja-JP", "412": "ko-KR",
		"40C": "fr-FR", "C0C": "fr-CA", "407": "de-DE",
		"40A": "es-ES", "C0A": "es-ES", "80A": "es-MX",
		"410": "it-IT", "416": "pt-BR", "816": "pt-PT",
		"419": "ru-RU", "42A": "vi-VN", "421": "id-ID", "43E": "ms-MY",
		"439": "hi-IN", "401": "ar-SA",
	}
	if tag, ok := tags[code]; ok {
		return tag
	}
	return code
}

// psQuote wraps s as a PowerShell single-quoted literal, where the only
// escape is a doubled quote. Newlines are stripped rather than escaped —
// nothing quoted here (a path, a voice name) legitimately contains one, and a
// literal newline inside -EncodedCommand would end the statement early.
func psQuote(s string) string {
	s = strings.NewReplacer("\r", "", "\n", " ", "\x00", "").Replace(s)
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// execPowerShell runs a script through powershell.exe -EncodedCommand — the
// one invocation shape that needs no quoting rules at all. Stdout is the
// script's own output; stderr under CLIXML is noise unless the process fails.
func execPowerShell(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell",
		"-NoProfile", "-NonInteractive", "-EncodedCommand", encodeCommand(script))
	proc.HideConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return stdout.String(), fmt.Errorf("ไม่พบ PowerShell ในเครื่อง ซึ่งเอนจินเสียงของ Windows ต้องใช้")
		}
		// The marker line, when present, is the better message — leave the
		// decision to the caller, who checks stdout first.
		return stdout.String(), fmt.Errorf("เรียกเสียงของ Windows ไม่สำเร็จ (%s)", strings.TrimSpace(firstLine(stderr.String(), err.Error())))
	}
	return stdout.String(), nil
}

func firstLine(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// encodeCommand is -EncodedCommand's wire format: UTF-16LE, base64.
func encodeCommand(script string) string {
	codes := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(codes)*2)
	for _, c := range codes {
		buf = append(buf, byte(c), byte(c>>8))
	}
	return base64.StdEncoding.EncodeToString(buf)
}
