//go:build windows

package skill

// Win32 input synthesis, same NewLazyDLL pattern as desktop/browser.go.
// Mouse/keyboard go through SendInput (the supported way to synthesize input);
// the screenshot shells out to PowerShell's CopyFromScreen instead of ~100
// lines of GDI BitBlt syscalls.

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	compUser32           = syscall.NewLazyDLL("user32.dll")
	procSetCursorPos     = compUser32.NewProc("SetCursorPos")
	procGetCursorPos     = compUser32.NewProc("GetCursorPos")
	procSendInput        = compUser32.NewProc("SendInput")
	procGetSystemMetrics = compUser32.NewProc("GetSystemMetrics")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseeventfLeftDown  = 0x0002
	mouseeventfLeftUp    = 0x0004
	mouseeventfRightDown = 0x0008
	mouseeventfRightUp   = 0x0010

	keyeventfKeyUp   = 0x0002
	keyeventfUnicode = 0x0004

	smCxScreen = 0
	smCyScreen = 1
)

// Both structs are a full 40-byte INPUT (type + union) so one SendInput call
// can take a batch of either kind.
type mouseInputW struct {
	typ         uint32
	_           uint32
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type keybdInputW struct {
	typ         uint32
	_           uint32
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
	_           [8]byte
}

func sendMouseInputs(inputs []mouseInputW) error {
	return sendInputBatch(len(inputs), unsafe.Pointer(&inputs[0]), unsafe.Sizeof(inputs[0]))
}

func sendKeybdInputs(inputs []keybdInputW) error {
	return sendInputBatch(len(inputs), unsafe.Pointer(&inputs[0]), unsafe.Sizeof(inputs[0]))
}

func sendInputBatch(count int, first unsafe.Pointer, size uintptr) error {
	sent, _, callErr := procSendInput.Call(uintptr(count), uintptr(first), size)
	if int(sent) != count {
		return fmt.Errorf("SendInput delivered %d of %d events: %v", sent, count, callErr)
	}
	return nil
}

func computerScreenInfo() (width, height, cursorX, cursorY int, err error) {
	w, _, _ := procGetSystemMetrics.Call(smCxScreen)
	h, _, _ := procGetSystemMetrics.Call(smCyScreen)
	var pt struct{ x, y int32 }
	ok, _, callErr := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	if ok == 0 {
		return 0, 0, 0, 0, fmt.Errorf("GetCursorPos failed: %v", callErr)
	}
	return int(w), int(h), int(pt.x), int(pt.y), nil
}

func computerMouseMove(x, y int) error {
	ok, _, callErr := procSetCursorPos.Call(uintptr(x), uintptr(y))
	if ok == 0 {
		return fmt.Errorf("SetCursorPos(%d, %d) failed: %v", x, y, callErr)
	}
	return nil
}

func computerClick(x, y int, button string) error {
	if x >= 0 && y >= 0 {
		if err := computerMouseMove(x, y); err != nil {
			return err
		}
	}
	down, up := uint32(mouseeventfLeftDown), uint32(mouseeventfLeftUp)
	if button == "right" {
		down, up = mouseeventfRightDown, mouseeventfRightUp
	}
	inputs := buildClickInputs(down, up, button == "double")
	return sendMouseInputs(inputs)
}

func buildClickInputs(down, up uint32, double bool) []mouseInputW {
	press := []mouseInputW{
		{typ: inputMouse, dwFlags: down},
		{typ: inputMouse, dwFlags: up},
	}
	if double {
		press = append(press, press...)
	}
	return press
}

// computerType synthesizes each UTF-16 unit as a KEYEVENTF_UNICODE press, so
// any text works — Thai included — regardless of keyboard layout.
func computerType(text string) error {
	inputs := buildTypeInputs(text)
	if len(inputs) == 0 {
		return nil
	}
	return sendKeybdInputs(inputs)
}

func buildTypeInputs(text string) []keybdInputW {
	units := utf16.Encode([]rune(text))
	inputs := make([]keybdInputW, 0, len(units)*2)
	for _, unit := range units {
		inputs = append(inputs,
			keybdInputW{typ: inputKeyboard, wScan: unit, dwFlags: keyeventfUnicode},
			keybdInputW{typ: inputKeyboard, wScan: unit, dwFlags: keyeventfUnicode | keyeventfKeyUp},
		)
	}
	return inputs
}

// ponytail: modest key map — letters, digits, F-keys, navigation, modifiers.
// Extend the map when a real task needs a missing key.
var vkByName = map[string]uint16{
	"ctrl": 0x11, "control": 0x11, "alt": 0x12, "shift": 0x10, "win": 0x5B,
	"enter": 0x0D, "return": 0x0D, "tab": 0x09, "esc": 0x1B, "escape": 0x1B,
	"space": 0x20, "backspace": 0x08, "delete": 0x2E, "del": 0x2E, "insert": 0x2D,
	"home": 0x24, "end": 0x23, "pageup": 0x21, "pagedown": 0x22,
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
}

func vkForName(name string) (uint16, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if vk, ok := vkByName[name]; ok {
		return vk, true
	}
	if len(name) == 1 {
		c := name[0]
		if c >= 'a' && c <= 'z' {
			return uint16(c - 'a' + 0x41), true
		}
		if c >= '0' && c <= '9' {
			return uint16(c - '0' + 0x30), true
		}
	}
	if strings.HasPrefix(name, "f") && len(name) <= 3 {
		var n int
		if _, err := fmt.Sscanf(name, "f%d", &n); err == nil && n >= 1 && n <= 12 {
			return uint16(0x70 + n - 1), true
		}
	}
	return 0, false
}

// parseKeyCombo turns "ctrl+shift+s" into VK codes: leading parts must be
// modifiers, the last part is the key itself.
func parseKeyCombo(combo string) (modifiers []uint16, key uint16, err error) {
	parts := strings.Split(combo, "+")
	for i, part := range parts {
		vk, ok := vkForName(part)
		if !ok {
			return nil, 0, fmt.Errorf("unknown key %q in combo %q", part, combo)
		}
		if i == len(parts)-1 {
			key = vk
			continue
		}
		switch vk {
		case 0x10, 0x11, 0x12, 0x5B: // shift, ctrl, alt, win
			modifiers = append(modifiers, vk)
		default:
			return nil, 0, fmt.Errorf("%q in combo %q is not a modifier (only ctrl/alt/shift/win can prefix)", part, combo)
		}
	}
	return modifiers, key, nil
}

func computerKey(combo string) error {
	modifiers, key, err := parseKeyCombo(combo)
	if err != nil {
		return err
	}
	inputs := buildComboInputs(modifiers, key)
	return sendKeybdInputs(inputs)
}

func buildComboInputs(modifiers []uint16, key uint16) []keybdInputW {
	inputs := make([]keybdInputW, 0, len(modifiers)*2+2)
	for _, mod := range modifiers {
		inputs = append(inputs, keybdInputW{typ: inputKeyboard, wVk: mod})
	}
	inputs = append(inputs,
		keybdInputW{typ: inputKeyboard, wVk: key},
		keybdInputW{typ: inputKeyboard, wVk: key, dwFlags: keyeventfKeyUp},
	)
	for i := len(modifiers) - 1; i >= 0; i-- {
		inputs = append(inputs, keybdInputW{typ: inputKeyboard, wVk: modifiers[i], dwFlags: keyeventfKeyUp})
	}
	return inputs
}

func computerScreenshot(ctx context.Context, pngPath string) error {
	script := fmt.Sprintf(
		"Add-Type -AssemblyName System.Windows.Forms,System.Drawing; "+
			"$b=[System.Windows.Forms.SystemInformation]::VirtualScreen; "+
			"$bmp=New-Object System.Drawing.Bitmap $b.Width,$b.Height; "+
			"$g=[System.Drawing.Graphics]::FromImage($bmp); "+
			"$g.CopyFromScreen($b.X,$b.Y,0,0,$bmp.Size); "+
			"$bmp.Save('%s',[System.Drawing.Imaging.ImageFormat]::Png); "+
			"$g.Dispose(); $bmp.Dispose()",
		strings.ReplaceAll(pngPath, "'", "''"))
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("screenshot failed: %s", msg)
	}
	return nil
}
