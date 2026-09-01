package main

// The Settings page's voice section, and the two chat buttons it configures:
// the composer's mic (speak instead of type) and the reply's ฟัง button (read
// the answer aloud).
//
// Nothing here is engine work — internal/stt and internal/tts each hold a
// catalog and one interface, and these bindings are the wiring: enumerate the
// catalogs for the two vendor pickers, persist the picks, turn a mic recording
// into text, and turn a reply into a WAV the webview can play. The one piece
// of judgement that lives here, and deliberately not in internal/tts, is which
// voice to prefer when the user never picked one: this file knows the UI
// locale, the engine does not.

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/proc"
	"github.com/Mikedev115/Aetox/internal/stt"
	"github.com/Mikedev115/Aetox/internal/tts"
)

// VoiceEngineInfo is one vendor row, shaped for either picker — the STT list
// and the TTS list render the same way on purpose.
type VoiceEngineInfo struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Install string `json:"install"`
	Active  bool   `json:"active"`
	// HasModels says whether this vendor reads model files the picker can
	// point at. faster-whisper stores its own weights by name, so drawing a
	// file picker for it would be a control over nothing.
	HasModels bool `json:"hasModels"`
	// InstallCommand is the catalog's runnable install, argv-shaped, for the
	// ติดตั้ง button to display — and for InstallVoiceEngine to run. Empty for
	// a vendor with nothing runnable. Never nil (§34).
	InstallCommand []string `json:"installCommand"`
	// Models are the vendor's NAMED models, first = default, and ActiveModel
	// is the one this vendor would run right now. One entry or none means the
	// page draws no picker — a picker with a single entry is not a choice.
	// Never nil (§34).
	Models      []string `json:"models"`
	ActiveModel string   `json:"activeModel"`
}

// TTSVoiceInfo is one installed voice for the voice picker.
type TTSVoiceInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Lang   string `json:"lang"`
	Gender string `json:"gender"`
	Active bool   `json:"active"`
}

// speechOptions is the one translation from config to internal/stt options —
// SpeechStatus, the mic, and bootstrap-by-way-of-applyConfig must not each
// assemble their own and drift.
func (a *App) speechOptions() stt.Options {
	cfg := a.cur().cfg
	return stt.Options{
		Engine:    strings.TrimSpace(cfg.SpeechEngine),
		Model:     strings.TrimSpace(cfg.SpeechModelName),
		ModelPath: strings.TrimSpace(cfg.SpeechModelPath),
	}
}

// ListSpeechEngines enumerates the STT vendors the catalog knows. Never nil
// (ARCHITECTURE.md §34).
func (a *App) ListSpeechEngines() []VoiceEngineInfo {
	cfg := a.cur().cfg
	return engineRows(sttDescriptors(), cfg.SpeechEngine, cfg.SpeechModelName)
}

// SetSpeechEngine picks the STT vendor. Empty goes back to the catalog
// default. Through applyConfig because audio_transcribe is handed its engine
// at construction — the same path SetSpeechModel takes, for the same reason.
func (a *App) SetSpeechEngine(id string) error {
	id = strings.TrimSpace(id)
	if _, ok := stt.Lookup(id); !ok {
		return fmt.Errorf("ไม่รู้จัก engine ถอดเสียงชื่อ %q", id)
	}
	next := a.cfg
	next.SpeechEngine = id
	// A model name is one vendor's private vocabulary — same rule as the TTS
	// voice on a vendor switch.
	next.SpeechModelName = ""
	a.applyConfig(a.cur(), next)
	return nil
}

// ListTTSEngines enumerates the read-aloud vendors. Never nil.
func (a *App) ListTTSEngines() []VoiceEngineInfo {
	cfg := a.cur().cfg
	return engineRows(ttsDescriptors(), cfg.TTSEngine, cfg.TTSModelName)
}

// SetTTSEngine picks the read-aloud vendor. The voice pick is cleared with it:
// a voice id is one engine's private naming, and keeping it across a vendor
// switch would pin the new engine to a name it has never heard of.
func (a *App) SetTTSEngine(id string) error {
	id = strings.TrimSpace(id)
	if _, ok := tts.Lookup(id); !ok {
		return fmt.Errorf("ไม่รู้จัก engine เสียงอ่านชื่อ %q", id)
	}
	next := a.cfg
	next.TTSEngine = id
	next.TTSVoice = ""
	next.TTSModelName = ""
	a.applyConfig(a.cur(), next)
	return nil
}

// SetSpeechModelName / SetTTSModelName pin a NAMED model on the active vendor
// (whisper-1 vs gpt-4o-transcribe, tts-1 vs tts-1-hd). Empty is a real choice
// — back to the vendor's first. Validation is against the catalog roster, so
// the webview can only ever store a name the vendor actually serves.
func (a *App) SetSpeechModelName(name string) error {
	name = strings.TrimSpace(name)
	if err := validateNamedModel(sttDescriptors(), a.cur().cfg.SpeechEngine, name); err != nil {
		return err
	}
	next := a.cfg
	next.SpeechModelName = name
	a.applyConfig(a.cur(), next)
	return nil
}

func (a *App) SetTTSModelName(name string) error {
	name = strings.TrimSpace(name)
	if err := validateNamedModel(ttsDescriptors(), a.cur().cfg.TTSEngine, name); err != nil {
		return err
	}
	next := a.cfg
	next.TTSModelName = name
	a.applyConfig(a.cur(), next)
	return nil
}

func validateNamedModel(descs []descriptorRow, activeEngine, name string) error {
	if name == "" {
		return nil
	}
	for _, row := range engineRows(descs, activeEngine, "") {
		if !row.Active {
			continue
		}
		for _, m := range row.Models {
			if strings.EqualFold(m, name) {
				return nil
			}
		}
		return fmt.Errorf("engine %s ไม่มีโมเดลชื่อ %q", row.ID, name)
	}
	return fmt.Errorf("ไม่พบ engine ที่กำลังใช้อยู่")
}

// ListTTSVoices enumerates what the active TTS engine can speak with. The
// slice is never nil; the error is the engine's own reason, verbatim, for the
// page to show above an empty list.
func (a *App) ListTTSVoices() ([]TTSVoiceInfo, error) {
	cfg := a.cur().cfg
	voices, err := a.ttsVoices(cfg.TTSEngine)
	out := []TTSVoiceInfo{}
	active := strings.TrimSpace(cfg.TTSVoice)
	for _, v := range voices {
		out = append(out, TTSVoiceInfo{
			ID:     v.ID,
			Name:   v.Name,
			Lang:   v.Lang,
			Gender: v.Gender,
			Active: active != "" && strings.EqualFold(v.ID, active),
		})
	}
	return out, err
}

// SetTTSVoice pins the voice replies are read with. Empty means "the engine
// decides", which resolves through defaultTTSVoice below.
func (a *App) SetTTSVoice(id string) error {
	id = strings.TrimSpace(id)
	if id != "" {
		voices, err := a.ttsVoices(a.cur().cfg.TTSEngine)
		if err != nil {
			return err
		}
		known := false
		for _, v := range voices {
			if strings.EqualFold(v.ID, id) {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("ไม่พบเสียงชื่อ %q ในเครื่อง", id)
		}
	}
	next := a.cfg
	next.TTSVoice = id
	a.applyConfig(a.cur(), next)
	return nil
}

// TTSStatus is what the page shows above the TTS picker: "" when the engine is
// ready, otherwise its own reason it cannot run, in the user's language — the
// same contract as SpeechStatus.
func (a *App) TTSStatus() string {
	cfg := a.cur().cfg
	if _, err := tts.New(tts.Options{Engine: strings.TrimSpace(cfg.TTSEngine), Model: strings.TrimSpace(cfg.TTSModelName)}); err != nil {
		return err.Error()
	}
	return ""
}

// SpeakText reads text aloud: synthesizes with the configured engine and
// voice, and hands back a data: URL the webview plays directly. The WAV never
// touches the workspace — it is a rendering of the reply, not a deliverable.
func (a *App) SpeakText(text string) (string, error) {
	cfg := a.cur().cfg
	voice := strings.TrimSpace(cfg.TTSVoice)
	if voice == "" {
		voice = a.defaultTTSVoice(cfg.TTSEngine, cfg.UILocale)
	}
	engine, err := tts.New(tts.Options{
		Engine: strings.TrimSpace(cfg.TTSEngine),
		Voice:  voice,
		Model:  strings.TrimSpace(cfg.TTSModelName),
	})
	if err != nil {
		return "", err
	}
	// Bounded, because SAPI on a wedged audio driver can sit forever and the
	// button this serves has no other way home. Three minutes covers a very
	// long reply many times over at synthesis speed.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	tmpDir, err := os.MkdirTemp("", "aetox-speak-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	outPath := filepath.Join(tmpDir, "reply.audio")
	if err := engine.Synthesize(ctx, text, outPath); err != nil {
		return "", err
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		return "", err
	}
	// The engine's own MIME, not a hardcoded wav: the cloud vendors hand back
	// MP3 and the player must not be lied to about it.
	return "data:" + engine.Mime() + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// TranscribeMicAudio turns a composer recording (a data: URL from
// MediaRecorder) into plain text for the input box. ffmpeg normalizes whatever
// the webview recorded into the 16kHz mono WAV every internal/stt engine
// expects — the same shape audio_transcribe produces, minus the timestamps: a
// dictated sentence has no use for "[m:ss]" in front of it.
func (a *App) TranscribeMicAudio(dataURL string) (string, error) {
	_, payload, ok := strings.Cut(dataURL, ";base64,")
	if !ok {
		return "", fmt.Errorf("รูปแบบเสียงที่อัดมาไม่ถูกต้อง")
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("อ่านเสียงที่อัดมาไม่ได้ (%v)", err)
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("ไม่มีเสียงในบันทึก ลองพูดอีกครั้ง")
	}
	// The engine first: a missing whisper binary or model is the likeliest
	// failure, and finding out before spending ffmpeg time gives the same
	// error faster — the order audio_transcribe.go settled on.
	engine, err := stt.New(a.speechOptions())
	if err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp("", "aetox-mic-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	// ffmpeg probes the container from content, so the recording needs no
	// correct extension — only a name.
	srcPath := filepath.Join(tmpDir, "mic.audio")
	if err := os.WriteFile(srcPath, raw, 0o600); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	wavPath := filepath.Join(tmpDir, "mic.wav")
	if err := micToWav(ctx, srcPath, wavPath); err != nil {
		return "", err
	}
	segments, err := engine.Transcribe(ctx, wavPath)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(segments))
	for _, seg := range segments {
		if text := strings.TrimSpace(seg.Text); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("ไม่ได้ยินเสียงพูดในบันทึก ลองพูดใกล้ไมค์ขึ้นอีกนิด")
	}
	return strings.Join(parts, " "), nil
}

// micToWav is extractAudioTrack's shape with the desktop's own ffmpeg
// resolution: findProgram already knows every address a real ffmpeg lives at
// on this machine (videotooling.go), including the copy the capability
// install downloads.
func micToWav(ctx context.Context, srcPath, wavPath string) error {
	ffmpeg := findProgram("ffmpeg")
	if ffmpeg == "" {
		return fmt.Errorf("ไม่พบโปรแกรม ffmpeg ซึ่งใช้แปลงเสียงจากไมค์ — ติดตั้งชุดเครื่องมือวิดีโอในหน้าเอเจน หรือ winget install ffmpeg แล้วลองใหม่")
	}
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", srcPath,
		"-vn", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le",
		wavPath,
	)
	proc.HideConsole(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("แปลงเสียงที่อัดมาไม่ได้ (%s)", msg)
	}
	return nil
}

// ttsVoices reads the engine's installed voices through the process cache —
// see the field comment on App.ttsVoiceCache for why a stale-until-restart
// list is the right trade here.
func (a *App) ttsVoices(engineID string) ([]tts.Voice, error) {
	desc, ok := tts.Lookup(strings.TrimSpace(engineID))
	if !ok {
		return nil, fmt.Errorf("ไม่รู้จัก engine เสียงอ่านชื่อ %q", engineID)
	}
	a.ttsVoiceMu.Lock()
	cached, hit := a.ttsVoiceCache[desc.ID]
	a.ttsVoiceMu.Unlock()
	if hit {
		return cached, nil
	}
	engine, err := tts.New(tts.Options{Engine: desc.ID})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	voices, err := engine.Voices(ctx)
	if err != nil {
		return nil, err
	}
	a.ttsVoiceMu.Lock()
	if a.ttsVoiceCache == nil {
		a.ttsVoiceCache = map[string][]tts.Voice{}
	}
	a.ttsVoiceCache[desc.ID] = voices
	a.ttsVoiceMu.Unlock()
	return voices, nil
}

// defaultTTSVoice is the policy internal/tts refuses to hold: with no voice
// picked, prefer one that speaks the UI's language, so a Thai machine's first
// ฟัง press answers in Thai rather than in SAPI's English default. No match —
// or no way to enumerate — falls back to "", the engine's own default, and
// speaking with the wrong accent beats refusing to speak.
func (a *App) defaultTTSVoice(engineID, locale string) string {
	lang := strings.ToLower(strings.TrimSpace(locale))
	if lang == "" {
		return ""
	}
	voices, err := a.ttsVoices(engineID)
	if err != nil {
		return ""
	}
	for _, v := range voices {
		if strings.HasPrefix(strings.ToLower(v.Lang), lang) {
			return v.ID
		}
	}
	return ""
}

func engineRows(descs []descriptorRow, active, activeModel string) []VoiceEngineInfo {
	out := []VoiceEngineInfo{}
	active = strings.TrimSpace(active)
	activeModel = strings.TrimSpace(activeModel)
	for _, d := range descs {
		isActive := strings.EqualFold(d.id, active) || (active == "" && d.def)
		row := VoiceEngineInfo{
			ID:      d.id,
			Label:   d.label,
			Install: d.install,
			// The default engine is active when nothing is picked — a picker
			// with no marked row reads as "not configured", and that is not
			// what an empty value means.
			Active:         isActive,
			HasModels:      d.models,
			InstallCommand: append([]string{}, d.cmd...),
			Models:         append([]string{}, d.modelNames...),
		}
		// The model actually running: the pick when this vendor is the one
		// picked on, its first otherwise — so the page never shows a stale
		// name from another vendor's vocabulary.
		if len(d.modelNames) > 0 {
			row.ActiveModel = d.modelNames[0]
			if isActive && activeModel != "" {
				row.ActiveModel = activeModel
			}
		}
		out = append(out, row)
	}
	return out
}

// descriptorRow is the two catalogs' common shape — internal/stt and
// internal/tts deliberately do not import each other, so the flattening
// happens here.
type descriptorRow struct {
	id, label, install string
	cmd                []string
	modelNames         []string
	def, models        bool
}

func sttDescriptors() []descriptorRow {
	out := make([]descriptorRow, 0)
	for _, d := range stt.Catalog() {
		out = append(out, descriptorRow{id: d.ID, label: d.Label, install: d.Install, cmd: d.InstallCommand, modelNames: d.Models, def: d.Default, models: d.ModelGlob != ""})
	}
	return out
}

func ttsDescriptors() []descriptorRow {
	out := make([]descriptorRow, 0)
	for _, d := range tts.Catalog() {
		out = append(out, descriptorRow{id: d.ID, label: d.Label, install: d.Install, cmd: d.InstallCommand, modelNames: d.Models, def: d.Default})
	}
	return out
}
