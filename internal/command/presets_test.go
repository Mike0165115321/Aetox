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

// Every bundled preset ships its own cover art. A gallery where some cards
// have a picture and others have a coloured rectangle looks broken, not
// minimal — so this is all-or-nothing by test.
func TestEveryBundledPresetShipsACover(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	for _, p := range ListPresets() {
		if !p.Builtin {
			continue
		}
		if !strings.HasPrefix(p.Image, "data:image/svg+xml;base64,") {
			t.Errorf("bundled preset %q has no cover — add internal/command/presets/covers/%s.svg", p.Name, p.Name)
		}
	}
}

// A user cover replaces the shipped one rather than fighting it.
func TestUserCoverWinsOverBundledCover(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	src := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(src, []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SavePresetImage("landing", src); err != nil {
		t.Fatal(err)
	}
	for _, p := range ListPresets() {
		if p.Name == "landing" {
			if !strings.HasPrefix(p.Image, "data:image/png;base64,") {
				t.Errorf("user cover should win, got %.40q", p.Image)
			}
			return
		}
	}
	t.Fatal("landing preset disappeared")
}

func TestValidPresetNameRejectsWhatCannotBeAFilename(t *testing.T) {
	for _, bad := range []string{"", "  ", "..", ".", "a/b", `a\b`, "a b", "a:b", "a*b", strings.Repeat("ก", 41)} {
		if err := ValidPresetName(bad); err == nil {
			t.Errorf("ValidPresetName(%q) = nil, want an error — this name becomes a path", bad)
		}
	}
	for _, ok := range []string{"landing", "my-prompt", "สรุป", "a_b.c"} {
		if err := ValidPresetName(ok); err != nil {
			t.Errorf("ValidPresetName(%q) = %v, want it accepted", ok, err)
		}
	}
}

// Saving creates the folder on demand — a user should never have to make it.
func TestSavePresetCreatesFolderAndIsUsableImmediately(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)

	if err := SavePreset("mine", "ทำสิ่งนี้ให้: $ARGUMENTS"); err != nil {
		t.Fatalf("SavePreset: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "prompts", "mine.md")); err != nil {
		t.Fatalf("preset file not written: %v", err)
	}
	got, ok := ExpandPreset("/mine เรื่องนี้")
	if !ok || got != "ทำสิ่งนี้ให้: เรื่องนี้" {
		t.Errorf("saved preset not usable: %q, %v", got, ok)
	}
	if err := SavePreset("mine", "   "); err == nil {
		t.Error("an empty body must be refused")
	}
}

// Overriding a bundled preset writes a separate file; deleting the override
// brings the bundled one back. That round trip is what makes editing a
// shipped preset safe to try.
func TestOverrideAndDeleteRestoresBundled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)

	original, ok := ExpandPreset("/landing ร้านกาแฟ")
	if !ok {
		t.Fatal("bundled /landing should expand before any override")
	}
	if err := SavePreset("landing", "ของผม $ARGUMENTS"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ExpandPreset("/landing ร้านกาแฟ"); got != "ของผม ร้านกาแฟ" {
		t.Fatalf("override not in effect: %q", got)
	}
	if err := DeletePreset("landing"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ExpandPreset("/landing ร้านกาแฟ"); got != original {
		t.Error("deleting an override must restore the bundled preset, not remove the command")
	}
}

func TestPresetCoverImageRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	if err := SavePreset("mine", "body $ARGUMENTS"); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(src, []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SavePresetImage("mine", src); err != nil {
		t.Fatalf("SavePresetImage: %v", err)
	}

	var mine *Preset
	for _, p := range ListPresets() {
		if p.Name == "mine" {
			cp := p
			mine = &cp
		}
	}
	if mine == nil || !strings.HasPrefix(mine.Image, "data:image/png;base64,") {
		t.Fatalf("cover should come back as a png data URI, got %+v", mine)
	}
	if mine.Body != "body $ARGUMENTS" {
		t.Errorf("Body = %q, want the prompt itself so the UI can edit it", mine.Body)
	}

	// A cover is not a prompt: the .md glob must not pick it up as one.
	for _, p := range ListPresets() {
		if strings.HasSuffix(p.Name, ".png") {
			t.Errorf("image file leaked into the preset list as %q", p.Name)
		}
	}

	if err := SavePresetImage("mine", filepath.Join(t.TempDir(), "notes.txt")); err == nil {
		t.Error("a non-image extension must be refused")
	}
	if err := DeletePreset("mine"); err != nil {
		t.Fatal(err)
	}
	if _, ok := presetImagePath("mine"); ok {
		t.Error("deleting a preset must take its cover with it")
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
