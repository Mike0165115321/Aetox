//go:build windows

package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestParseKeyCombo(t *testing.T) {
	mods, key, err := parseKeyCombo("ctrl+c")
	if err != nil || len(mods) != 1 || mods[0] != 0x11 || key != 0x43 {
		t.Fatalf("ctrl+c = mods %v key %#x err %v", mods, key, err)
	}
	mods, key, err = parseKeyCombo("enter")
	if err != nil || len(mods) != 0 || key != 0x0D {
		t.Fatalf("enter = mods %v key %#x err %v", mods, key, err)
	}
	mods, key, err = parseKeyCombo("ctrl+shift+s")
	if err != nil || len(mods) != 2 || mods[0] != 0x11 || mods[1] != 0x10 || key != 0x53 {
		t.Fatalf("ctrl+shift+s = mods %v key %#x err %v", mods, key, err)
	}
	if _, _, err := parseKeyCombo("banana"); err == nil {
		t.Fatal("expected error for unknown key name")
	}
	if _, _, err := parseKeyCombo("c+ctrl"); err == nil {
		t.Fatal("expected error for non-modifier prefix")
	}
}

func TestBuildTypeInputsThaiUnicode(t *testing.T) {
	text := "กข"
	inputs := buildTypeInputs(text)
	units := utf16.Encode([]rune(text))
	if len(inputs) != len(units)*2 {
		t.Fatalf("got %d inputs, want %d (down+up per UTF-16 unit)", len(inputs), len(units)*2)
	}
	for i, in := range inputs {
		if in.dwFlags&keyeventfUnicode == 0 {
			t.Errorf("input %d missing KEYEVENTF_UNICODE", i)
		}
		wantUp := i%2 == 1
		if (in.dwFlags&keyeventfKeyUp != 0) != wantUp {
			t.Errorf("input %d key-up flag = %v, want %v", i, !wantUp, wantUp)
		}
		if in.wScan != units[i/2] {
			t.Errorf("input %d wScan = %#x, want %#x", i, in.wScan, units[i/2])
		}
	}
}

func TestBuildClickAndComboInputs(t *testing.T) {
	double := buildClickInputs(mouseeventfLeftDown, mouseeventfLeftUp, true)
	if len(double) != 4 {
		t.Fatalf("double click = %d events, want 4", len(double))
	}
	combo := buildComboInputs([]uint16{0x11, 0x10}, 0x53) // ctrl+shift+s
	wantFlags := []uint32{0, 0, 0, keyeventfKeyUp, keyeventfKeyUp, keyeventfKeyUp}
	wantVk := []uint16{0x11, 0x10, 0x53, 0x53, 0x10, 0x11} // release in reverse order
	if len(combo) != len(wantVk) {
		t.Fatalf("combo = %d events, want %d", len(combo), len(wantVk))
	}
	for i := range combo {
		if combo[i].wVk != wantVk[i] || combo[i].dwFlags != wantFlags[i] {
			t.Errorf("event %d = vk %#x flags %#x, want vk %#x flags %#x",
				i, combo[i].wVk, combo[i].dwFlags, wantVk[i], wantFlags[i])
		}
	}
}

// Live on a real desktop session: reads the screen, moves the actual cursor
// (and puts it back), and takes a real screenshot. Mouse restore keeps the
// disturbance to a couple of milliseconds.
func TestComputerSkillLiveCursorAndScreenshot(t *testing.T) {
	_, _, origX, origY, err := computerScreenInfo()
	if err != nil {
		t.Skipf("no interactive desktop session: %v", err)
	}
	defer func() { _ = computerMouseMove(origX, origY) }()

	root := t.TempDir()
	s := &computerSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"action": "screen_info"})
	if err != nil || !strings.Contains(out.Content, "screen ") {
		t.Fatalf("screen_info = %q, err %v", out.Content, err)
	}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{"action": "mouse_move", "x": float64(50), "y": float64(60)}); err != nil {
		t.Fatalf("mouse_move failed: %v", err)
	}
	_, _, nowX, nowY, err := computerScreenInfo()
	if err != nil {
		t.Fatalf("cursor read-back failed: %v", err)
	}
	if nowX != 50 || nowY != 60 {
		t.Errorf("cursor at (%d, %d), want (50, 60)", nowX, nowY)
	}

	out, err = s.ExecuteTool(context.Background(), map[string]any{"action": "screenshot"})
	if err != nil {
		t.Fatalf("screenshot failed: %v", err)
	}
	shots, _ := filepath.Glob(filepath.Join(root, "screenshots", "*.png"))
	if len(shots) != 1 {
		t.Fatalf("expected 1 screenshot in sandbox, found %d (output: %s)", len(shots), out.Content)
	}
	data, err := os.ReadFile(shots[0])
	if err != nil || len(data) < 8 || string(data[1:4]) != "PNG" {
		t.Fatalf("screenshot is not a PNG (len %d, err %v)", len(data), err)
	}
	if !strings.Contains(out.Content, "image_ocr") {
		t.Errorf("screenshot output should point the model at image_ocr, got %q", out.Content)
	}
}
