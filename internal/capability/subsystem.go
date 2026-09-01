package capability

// The renderer's browser is a CONSOLE program, and that fact reached the
// user's screen.
//
// chrome-headless-shell ships from Chrome-for-Testing with the PE subsystem
// set to CONSOLE (3). On Windows, a console-subsystem child whose spawner does
// not manage to suppress it gets a console window — and the suppression chain
// here is long and not ours: Aetox hides its own children (internal/proc), but
// the browser is a GRANDchild, spawned by node out of the hyperframes bundle,
// where Node's `windowsHide` is an SW_HIDE request that Windows Terminal (the
// Win11 default host) has a history of ignoring. The owner watched the result
// on 1 ก.ย.: black terminal windows titled with the AppData path, popping up
// "เอง", each holding one Chromium GPU log line — and was right to fear every
// installed user seeing the same.
//
// The fix is at the only layer that cannot be undone by anybody's spawn
// options: the binary itself. A GUI-subsystem (2) process never allocates a
// console, whoever launches it and however carelessly; stdout/stderr stay
// ordinary handles, so the pipes puppeteer talks CDP over are untouched.
// Verified on the owner's machine before this was written: the patched binary
// answers --version and renders --dump-dom exactly as before.
//
// Patched at install (Component.NoConsoleWindow) and re-checked where the path
// is composed for use (hyperframesEnvironment), so machines that installed the
// bundle before this existed are healed without a re-download.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Subsystem values from the PE optional header, the only two this cares about.
const (
	peSubsystemGUI     = 2
	peSubsystemConsole = 3
)

// EnsureNoConsoleWindow flips a console-subsystem Windows executable to the
// GUI subsystem, in place. Already-GUI binaries are left untouched, so calling
// this on every launch path costs two reads. Anything that is not a PE with
// one of the two expected subsystems is refused rather than written to —
// better a browser that still flashes a window than one this code corrupted.
func EnsureNoConsoleWindow(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	// MZ header: magic, and the offset of the PE header at 0x3C.
	var dosHead [0x40]byte
	if _, err := io.ReadFull(f, dosHead[:]); err != nil {
		return fmt.Errorf("อ่านหัวไฟล์ไม่ได้: %w", err)
	}
	if dosHead[0] != 'M' || dosHead[1] != 'Z' {
		return fmt.Errorf("%s ไม่ใช่ไฟล์ .exe (ไม่มีหัว MZ)", path)
	}
	peOff := int64(binary.LittleEndian.Uint32(dosHead[0x3C:]))

	var sig [4]byte
	if _, err := f.ReadAt(sig[:], peOff); err != nil {
		return fmt.Errorf("อ่าน PE signature ไม่ได้: %w", err)
	}
	if !bytes.Equal(sig[:], []byte{'P', 'E', 0, 0}) {
		return fmt.Errorf("%s ไม่ใช่ไฟล์ PE", path)
	}

	// Signature (4) + COFF header (20), then Subsystem lives 68 bytes into the
	// optional header — same offset for PE32 and PE32+.
	subOff := peOff + 4 + 20 + 68
	var sub [2]byte
	if _, err := f.ReadAt(sub[:], subOff); err != nil {
		return fmt.Errorf("อ่าน subsystem ไม่ได้: %w", err)
	}
	switch binary.LittleEndian.Uint16(sub[:]) {
	case peSubsystemGUI:
		return nil // already the shape we want — the every-launch path ends here
	case peSubsystemConsole:
		binary.LittleEndian.PutUint16(sub[:], peSubsystemGUI)
		if _, err := f.WriteAt(sub[:], subOff); err != nil {
			return fmt.Errorf("เขียน subsystem ไม่ได้: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("%s มี subsystem %d ซึ่งไม่ใช่ console/GUI — ไม่แตะ", path, binary.LittleEndian.Uint16(sub[:]))
	}
}
