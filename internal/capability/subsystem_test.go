package capability

// The patch that keeps the renderer's browser off the user's screen
// (subsystem.go) edits two bytes of a PE header in place. These tests build a
// minimal PE by hand — offsets written from the format, not from any real
// binary — because the property that matters is exactly byte-level: flip a
// console subsystem, leave a GUI one alone, and refuse to write into anything
// else.

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// fakePE is the smallest file the patcher accepts: an MZ header pointing at a
// PE signature, a COFF header of zeros, and an optional header long enough to
// hold the subsystem field.
func fakePE(t *testing.T, subsystem uint16) string {
	t.Helper()
	const peOff = 0x80
	buf := make([]byte, peOff+4+20+96)
	buf[0], buf[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(buf[0x3C:], peOff)
	copy(buf[peOff:], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(buf[peOff+4+20+68:], subsystem)
	path := filepath.Join(t.TempDir(), "fake.exe")
	if err := os.WriteFile(path, buf, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func readSubsystem(t *testing.T, path string) uint16 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	peOff := binary.LittleEndian.Uint32(data[0x3C:])
	return binary.LittleEndian.Uint16(data[peOff+4+20+68:])
}

func TestConsoleSubsystemIsFlippedToGUI(t *testing.T) {
	path := fakePE(t, peSubsystemConsole)
	if err := EnsureNoConsoleWindow(path); err != nil {
		t.Fatalf("EnsureNoConsoleWindow: %v", err)
	}
	if got := readSubsystem(t, path); got != peSubsystemGUI {
		t.Fatalf("subsystem = %d, want %d (GUI)", got, peSubsystemGUI)
	}
	// The every-launch path: already GUI must stay a cheap no-op, not an error.
	if err := EnsureNoConsoleWindow(path); err != nil {
		t.Fatalf("second call on a patched binary: %v", err)
	}
}

func TestUnexpectedFilesAreRefusedUnwritten(t *testing.T) {
	// A subsystem that is neither console nor GUI (a driver, say) is refused —
	// and, the half that matters, left byte-for-byte alone.
	path := fakePE(t, 1)
	if err := EnsureNoConsoleWindow(path); err == nil {
		t.Fatal("a native-subsystem binary was accepted")
	}
	if got := readSubsystem(t, path); got != 1 {
		t.Fatalf("refused file was still written: subsystem = %d", got)
	}

	// Not an executable at all.
	text := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(text, make([]byte, 256), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureNoConsoleWindow(text); err == nil {
		t.Fatal("a non-PE file was accepted")
	}
}
