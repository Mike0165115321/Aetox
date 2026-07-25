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
	"os"
	"path/filepath"
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
	Path        string `json:"path"`        // on-disk path, or "" for a bundled preset
	Builtin     bool   `json:"builtin"`     // shipped with Aetox, not written by the user
}

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
		byName[name] = Preset{Name: name, Description: firstLine(string(body)), Builtin: true}
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
		byName[name] = Preset{Name: name, Description: firstLine(string(body)), Path: path}
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
