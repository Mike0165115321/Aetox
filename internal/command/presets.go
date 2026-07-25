package command

// Prompt presets ("พรอมต์สำเร็จรูป"): a reusable prompt saved under a short
// name, invoked as "/name <args>" in chat. The file's body becomes the prompt,
// with "$ARGUMENTS" replaced by whatever followed the name (appended when the
// body never mentions it).
//
// Two sources, same shape — the same builtin/external split the skill registry
// uses:
//
//   - **Bundled** — presets/*.md, compiled into the binary. They exist on a
//     fresh install with no folder to create and no file to download, which is
//     the whole point: the feature is useful the first time it is opened.
//   - **User** — <DataRoot>/prompts/*.md. Anything the user writes. A user file
//     with the same name as a bundled one **wins**, so a preset can be edited
//     by copying it out and changing it, never by fighting the app.
//
// Aetox's own built-in slash grammar still wins over both — callers try their
// grammar first and only then reach for ExpandPreset.

import (
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
)

//go:embed presets/*.md
var bundledPresets embed.FS

// Preset is one prompt preset, for management UIs.
type Preset struct {
	Name        string `json:"name"`
	Description string `json:"description"` // first non-empty line of the body
	Body        string `json:"body"`        // the prompt itself, so the UI can show and edit it
	Path        string `json:"path"`        // on-disk path, or "" for a bundled preset
	Builtin     bool   `json:"builtin"`     // shipped with Aetox, not written by the user
	Image       string `json:"image"`       // cover as a data URI, "" when none is set
}

// presetImageExts are the cover-image formats a preset may carry, in lookup
// order. The cover lives next to the prompt as <name>.<ext>, so a preset is
// still just files in a folder someone can back up or edit by hand.
var presetImageExts = []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp"}

// maxPresetImageBytes caps what gets inlined as a data URI. A cover is
// decoration; a 20MB camera original would stall the whole settings page for
// no benefit, so oversized files are ignored rather than shipped across.
// ponytail: no downscaling — add it if someone actually hits this.
const maxPresetImageBytes = 4 << 20

// PresetsDir returns <DataRoot>/prompts (not created here).
func PresetsDir() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "prompts"), nil
}

// ListPresets reports every available preset — bundled first, then the user's,
// each group alphabetical. A user preset shadows a bundled one of the same
// name and takes its place in the list.
func ListPresets() []Preset {
	byName := map[string]Preset{}
	var order []string

	for _, name := range bundledNames() {
		body, err := bundledPresets.ReadFile("presets/" + name + ".md")
		if err != nil {
			continue
		}
		byName[name] = Preset{
			Name: name, Description: firstLine(string(body)),
			Body: string(body), Builtin: true, Image: presetImage(name),
		}
		order = append(order, name)
	}

	for _, path := range userPresetFiles() {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if _, shadowed := byName[name]; !shadowed {
			order = append(order, name)
		}
		byName[name] = Preset{
			Name: name, Description: firstLine(string(body)),
			Body: string(body), Path: path, Image: presetImage(name),
		}
	}

	out := make([]Preset, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

// ExpandPreset checks whether input invokes a preset and returns the expanded
// prompt. Anything else — no leading slash, unknown name, unreadable file —
// returns the input unchanged with ok=false.
func ExpandPreset(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return input, false
	}
	name, args, _ := strings.Cut(trimmed[1:], " ")
	name = strings.TrimSpace(name)
	args = strings.TrimSpace(args)
	// A name that could climb out of the presets directory is not a name.
	if name == "" || strings.ContainsAny(name, `\/`) || name == ".." {
		return input, false
	}

	body := strings.TrimSpace(readPresetBody(name))
	if body == "" {
		return input, false
	}
	if strings.Contains(body, "$ARGUMENTS") {
		return strings.ReplaceAll(body, "$ARGUMENTS", args), true
	}
	if args != "" {
		return body + "\n\n" + args, true
	}
	return body, true
}

// readPresetBody prefers the user's file so a bundled preset can be overridden
// by writing one with the same name.
func readPresetBody(name string) string {
	if dir, err := PresetsDir(); err == nil {
		if raw, err := os.ReadFile(filepath.Join(dir, name+".md")); err == nil {
			return string(raw)
		}
	}
	if raw, err := bundledPresets.ReadFile("presets/" + name + ".md"); err == nil {
		return string(raw)
	}
	return ""
}

func bundledNames() []string {
	entries, err := bundledPresets.ReadDir("presets")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		}
	}
	sort.Strings(names)
	return names
}

func userPresetFiles() []string {
	dir, err := PresetsDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // no folder yet is the normal state, not an error
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files
}

// ValidPresetName rejects anything that would not survive being a filename or
// a "/name" in chat. The name is user input that becomes a path, so this is a
// trust boundary, not a formatting nicety.
func ValidPresetName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("ต้องตั้งชื่อพรอมต์ก่อน")
	}
	if len([]rune(name)) > 40 {
		return errors.New("ชื่อยาวเกินไป (ไม่เกิน 40 ตัวอักษร)")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `\/:*?"<>|`) || strings.ContainsAny(name, " \t\n") {
		return errors.New(`ชื่อห้ามมีช่องว่างหรืออักขระเหล่านี้: \ / : * ? " < > |`)
	}
	return nil
}

// SavePreset writes a user preset, creating the prompts folder on demand. It
// always writes to the user directory — saving a bundled preset's name creates
// an override rather than touching what shipped, which is what makes every
// bundled preset editable without being mutable.
func SavePreset(name, body string) error {
	if err := ValidPresetName(name); err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("เนื้อพรอมต์ว่างไม่ได้")
	}
	dir, err := PresetsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, strings.TrimSpace(name)+".md"), []byte(body), 0o644)
}

// DeletePreset removes a user preset and its cover. A bundled preset cannot be
// deleted — deleting an override just restores the one that ships, which is
// the behavior that makes overriding safe to try.
func DeletePreset(name string) error {
	if err := ValidPresetName(name); err != nil {
		return err
	}
	dir, err := PresetsDir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, strings.TrimSpace(name)+".md")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if path, ok := presetImagePath(name); ok {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// SavePresetImage copies srcPath in as the preset's cover, replacing any
// existing one. The cover keeps its source extension so the folder stays
// readable by hand.
func SavePresetImage(name, srcPath string) error {
	if err := ValidPresetName(name); err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(srcPath))
	if !slices.Contains(presetImageExts, ext) {
		return fmt.Errorf("ไฟล์รูปต้องเป็นนามสกุล %s", strings.Join(presetImageExts, " "))
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	if len(data) > maxPresetImageBytes {
		return fmt.Errorf("รูปใหญ่เกิน %d MB — ย่อก่อนแล้วลองใหม่", maxPresetImageBytes>>20)
	}
	dir, err := PresetsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if old, ok := presetImagePath(name); ok {
		_ = os.Remove(old)
	}
	return os.WriteFile(filepath.Join(dir, strings.TrimSpace(name)+ext), data, 0o644)
}

// RemovePresetImage drops the cover, leaving the prompt itself alone.
func RemovePresetImage(name string) error {
	if err := ValidPresetName(name); err != nil {
		return err
	}
	if path, ok := presetImagePath(name); ok {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func presetImagePath(name string) (string, bool) {
	dir, err := PresetsDir()
	if err != nil {
		return "", false
	}
	for _, ext := range presetImageExts {
		path := filepath.Join(dir, strings.TrimSpace(name)+ext)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, true
		}
	}
	return "", false
}

// presetImage returns the cover as a data URI. Empty when there is none, when
// it is unreadable, or when it is too big to be worth inlining — the card
// falls back to a generated cover in every one of those cases.
func presetImage(name string) string {
	path, ok := presetImagePath(name)
	if !ok {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxPresetImageBytes {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "image/png"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// firstLine is the preset's one-line description: the first line with content,
// stripped of markdown heading/bullet punctuation.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(strings.TrimLeft(line, "#-* ")); line != "" {
			line = strings.ReplaceAll(line, "**", "")
			if len([]rune(line)) > 120 {
				return string([]rune(line)[:120]) + "…"
			}
			return line
		}
	}
	return ""
}
