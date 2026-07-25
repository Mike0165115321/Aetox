package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupPresetsDir points the data root at a temp dir and creates the user
// presets folder, returning it.
func setupPresetsDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	dir := filepath.Join(root, "prompts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return dir
}

func TestExpandPresetUserFiles(t *testing.T) {
	dir := setupPresetsDir(t)
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("mine", "Review the file $ARGUMENTS carefully.")
	write("standup", "Summarize today's work.")

	got, ok := ExpandPreset("/mine src/main.go")
	if !ok || got != "Review the file src/main.go carefully." {
		t.Fatalf("expand with args = %q, %v", got, ok)
	}

	// No $ARGUMENTS in the body → args appended.
	got, ok = ExpandPreset("/standup ship it")
	if !ok || got != "Summarize today's work.\n\nship it" {
		t.Fatalf("expand append = %q, %v", got, ok)
	}

	if _, ok := ExpandPreset("/nope"); ok {
		t.Error("unknown name must not expand")
	}
	if _, ok := ExpandPreset("hello /mine"); ok {
		t.Error("non-leading slash must not expand")
	}
	if _, ok := ExpandPreset("/../secrets"); ok {
		t.Error("traversal name must not expand")
	}
}

// The whole point of bundling: presets work on a fresh install, with no folder
// created and no file written.
func TestBundledPresetsWorkWithNoUserFolder(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir()) // nothing in it, no prompts/ dir

	list := ListPresets()
	if len(list) < 3 {
		t.Fatalf("expected the bundled presets to be available out of the box, got %d", len(list))
	}
	for _, p := range list {
		if !p.Builtin {
			t.Errorf("preset %q should be marked builtin", p.Name)
		}
		if p.Description == "" {
			t.Errorf("preset %q has no description — the settings page shows it", p.Name)
		}
		if p.Path != "" {
			t.Errorf("bundled preset %q should have no on-disk path, got %q", p.Name, p.Path)
		}
	}

	// Every bundled preset must actually expand, and must take $ARGUMENTS —
	// a preset that ignores its input is a snippet, not a command.
	for _, p := range list {
		body, ok := ExpandPreset("/" + p.Name + " ทดสอบอาร์กิวเมนต์")
		if !ok {
			t.Errorf("bundled preset /%s did not expand", p.Name)
			continue
		}
		if !strings.Contains(body, "ทดสอบอาร์กิวเมนต์") {
			t.Errorf("bundled preset /%s dropped its arguments", p.Name)
		}
		if strings.Contains(body, "$ARGUMENTS") {
			t.Errorf("bundled preset /%s left $ARGUMENTS unreplaced", p.Name)
		}
	}
}

func TestLandingPresetIsBundled(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	for _, p := range ListPresets() {
		if p.Name == "landing" {
			return
		}
	}
	t.Error("the landing-page preset must ship with the app")
}

// A user file with a bundled preset's name replaces it — editing a preset is
// copying it out and changing it, never fighting the app.
func TestUserPresetShadowsBundled(t *testing.T) {
	dir := setupPresetsDir(t)
	if err := os.WriteFile(filepath.Join(dir, "landing.md"), []byte("# ของผมเอง\nทำอย่างที่ผมบอก $ARGUMENTS"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := ExpandPreset("/landing เว็บขายกาแฟ")
	if !ok || !strings.Contains(got, "ทำอย่างที่ผมบอก เว็บขายกาแฟ") {
		t.Fatalf("user file must win over the bundled preset, got %q", got)
	}

	var landing *Preset
	seen := map[string]int{}
	for i := range ListPresets() {
		p := ListPresets()[i]
		seen[p.Name]++
		if p.Name == "landing" {
			landing = &p
		}
	}
	if seen["landing"] != 1 {
		t.Errorf("shadowed preset listed %d times, want exactly 1", seen["landing"])
	}
	if landing == nil || landing.Builtin || landing.Description != "ของผมเอง" {
		t.Errorf("shadowing entry = %+v, want the user's version", landing)
	}
}

func TestListPresetsIncludesUserPresets(t *testing.T) {
	dir := setupPresetsDir(t)
	if err := os.WriteFile(filepath.Join(dir, "zz-mine.md"), []byte("# พรอมต์ของผม\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range ListPresets() {
		if p.Name == "zz-mine" {
			if p.Builtin || p.Description != "พรอมต์ของผม" || p.Path == "" {
				t.Errorf("user preset = %+v, want non-builtin with a path", p)
			}
			return
		}
	}
	t.Error("user preset missing from the list")
}
