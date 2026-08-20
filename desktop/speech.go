package main

// The Settings page's speech-model picker. audio_transcribe runs whatever
// model file it finds; these bindings let the user say which one, so the
// accuracy-for-size trade (ggml-tiny ~31MB against ggml-base ~141MB, and
// larger) is theirs to make and a machine with several models is not a
// coin toss.
//
// internal/stt already had every piece of this — Options.ModelPath to pin one,
// InstalledModels to enumerate them across Aetox's own folder and the user's
// (Ollama's and LM Studio's stores are scanned too). Nothing here is engine
// work; it is the wiring that was missing, plus somewhere to persist the answer.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/stt"
)

// SpeechModelInfo is one installed model, shaped for the picker.
type SpeechModelInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
	// SizeMB rather than bytes: the number is only ever read by a human
	// choosing what to spend disk on.
	SizeMB int64 `json:"sizeMB"`
	// Store is where it came from ("Aetox", "Ollama", "LM Studio"), and
	// Managed says whether Aetox may delete it — a model the user downloaded
	// into their own tool's folder is theirs, and the UI must not offer to
	// remove it.
	Store   string `json:"store"`
	Managed bool   `json:"managed"`
	Active  bool   `json:"active"`
	// Where is the containing folder with the machine-specific head folded back
	// into its variable — what the UI shows. Path stays exact, for opening.
	Where string `json:"where"`
}

// ListSpeechModels enumerates every model the speech engine could use. Never
// nil: a nil slice serializes to JSON null, which the picker's .length crashes
// on mid-render.
func (a *App) ListSpeechModels() []SpeechModelInfo {
	out := []SpeechModelInfo{}
	desc, ok := stt.Lookup("")
	if !ok {
		return out
	}
	active := strings.TrimSpace(a.cur().cfg.SpeechModelPath)
	for _, m := range stt.InstalledModels(desc, stt.Options{}) {
		out = append(out, SpeechModelInfo{
			Path:    m.Path,
			Name:    m.Name,
			SizeMB:  m.Bytes >> 20,
			Store:   m.Store,
			Managed: m.Managed,
			Active:  active != "" && strings.EqualFold(m.Path, active),
			Where:   shortenPath(filepath.Dir(m.Path)),
		})
	}
	return out
}

// SetSpeechModel pins the model audio_transcribe will use. An empty path means
// "go back to picking whatever is on disk" — the original behaviour, and the
// only way to undo a choice — so it is accepted rather than rejected as blank.
//
// Re-bootstraps through applyConfig because the skill is handed its options at
// construction: the same path SwitchModel and friends take, rather than a
// second, partial reload that would only ever be used here.
func (a *App) SetSpeechModel(path string) error {
	path = strings.TrimSpace(path)
	if path != "" {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("ไม่พบไฟล์โมเดลนี้แล้ว (%s) — อาจถูกย้ายหรือลบไป", path)
		}
		if info.IsDir() {
			return fmt.Errorf("%q เป็นโฟลเดอร์ ไม่ใช่ไฟล์โมเดล", path)
		}
	}
	next := a.cfg
	next.SpeechModelPath = path
	a.applyConfig(a.cur(), next)
	return nil
}

// RevealSpeechModel opens the folder a model file sits in, so "where did this
// come from" is answerable by looking rather than by reading a long path.
//
// The path must be one InstalledModels actually reported. That is not
// ceremony: it turns an "open any folder on this machine" binding, callable
// from the webview, into one that can only reach a file the scan already
// found.
func (a *App) RevealSpeechModel(path string) error {
	path = strings.TrimSpace(path)
	known := false
	for _, m := range a.ListSpeechModels() {
		if strings.EqualFold(m.Path, path) {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("ไม่รู้จักไฟล์โมเดลนี้: %s", path)
	}
	return a.revealInFileManager(filepath.Dir(path))
}

// SpeechDirInfo is one scanned folder: the real path to open, and a label to
// show. They differ on purpose — the real one contains the account name, and a
// settings screen has no reason to put that on display.
type SpeechDirInfo struct {
	Path  string `json:"path"`
	Label string `json:"label"`
}

// SpeechModelDirs are the folders the scan looks in, in order. The picker shows
// them so a missing model is a question with an answer ("put it in one of
// these") instead of a dead end.
func (a *App) SpeechModelDirs() []SpeechDirInfo {
	out := []SpeechDirInfo{}
	for _, s := range stt.Stores(stt.Options{}) {
		out = append(out, SpeechDirInfo{Path: s.Dir, Label: shortenPath(s.Dir)})
	}
	return out
}

// shortenPath rewrites the machine-specific head of a path back into the
// variable it came from: %APPDATA%\aetox\models rather than a path with the
// user's account name in it. Anything outside those roots (a drive the user
// pointed OLLAMA_MODELS at, say) is already theirs and is left alone.
func shortenPath(p string) string {
	type root struct{ dir, label string }
	var roots []root
	if runtime.GOOS == "windows" {
		for _, v := range []string{"APPDATA", "LOCALAPPDATA"} {
			if dir := strings.TrimSpace(os.Getenv(v)); dir != "" {
				roots = append(roots, root{dir, "%" + v + "%"})
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		roots = append(roots, root{home, "~"})
	}
	for _, r := range roots {
		if strings.EqualFold(p, r.dir) {
			return r.label
		}
		prefix := r.dir + string(filepath.Separator)
		if len(p) > len(prefix) && strings.EqualFold(p[:len(prefix)], prefix) {
			return r.label + string(filepath.Separator) + p[len(prefix):]
		}
	}
	return p
}

// OpenSpeechModelDir opens one of the scanned folders, creating Aetox's own if
// it does not exist yet — that is where a downloaded model is meant to go.
func (a *App) OpenSpeechModelDir(dir string) error {
	dir = strings.TrimSpace(dir)
	for _, known := range a.SpeechModelDirs() {
		if strings.EqualFold(known.Path, dir) {
			_ = os.MkdirAll(dir, 0o755)
			return a.revealInFileManager(dir)
		}
	}
	return fmt.Errorf("ไม่ใช่โฟลเดอร์ที่ Aetox ค้นหาโมเดล: %s", dir)
}

// openInFileManager reveals a directory in the OS file manager. The one place
// every "open folder" button in the app goes through.
//
// Deliberately NOT wrapped in proc.HideConsole. That helper sets HideWindow and
// CREATE_NO_WINDOW so a background console process (git, a shell) does not flash
// a black box — but explorer.exe is a GUI program whose window is the entire
// point, and those flags suppress it. Every folder button in the app was hiding
// the window it had just asked for, which reads as the button doing nothing.
//
// explorer.exe also exits non-zero on success, so Start() (not Run()) is what
// this wants regardless: launch it and stop caring.
func openInFileManager(dir string) error {
	// proc-show-window: launching a GUI program — see the comment above and
	// TestEveryExecSiteHidesTheConsole. HideConsole here would hide the very
	// window this function exists to open.
	// proc-detached: the file manager belongs to the user, not to this call.
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}

// SpeechStatus is what the Settings page shows above the picker: either the
// engine is ready, or the exact reason it is not, in the user's language. It
// is stt's own error verbatim — that text is the only thing telling the user
// which of the two missing pieces (the program, or a model file) to go get.
func (a *App) SpeechStatus() string {
	if _, err := stt.New(stt.Options{ModelPath: strings.TrimSpace(a.cur().cfg.SpeechModelPath)}); err != nil {
		return err.Error()
	}
	return ""
}
