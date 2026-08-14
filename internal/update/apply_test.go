package update

// The self-update's moving parts, tested where they can hurt: picking the
// wrong asset installs the wrong thing, a checksum that doesn't gate is a
// remote-code door, and the swap is the two renames everything hangs on.

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestChannelAssetPicksTheRightFile(t *testing.T) {
	assets := []Asset{
		{Name: "aetox-amd64-installer.exe"},
		{Name: "aetox-windows-amd64-portable.zip"},
		{Name: "checksums.txt"},
	}
	if a, ok := channelAsset(ChannelPortable, assets); !ok || a.Name != "aetox-windows-amd64-portable.zip" {
		t.Errorf("portable picked %q, want the portable zip", a.Name)
	}
	if a, ok := channelAsset(ChannelInstaller, assets); !ok || a.Name != "aetox-amd64-installer.exe" {
		t.Errorf("installer picked %q, want the installer exe", a.Name)
	}
	// Scoop and unknown must never resolve to a file: those channels are not
	// ours to write.
	if _, ok := channelAsset(ChannelScoop, assets); ok {
		t.Error("scoop resolved to an asset — that directory belongs to Scoop")
	}
	if _, ok := channelAsset(ChannelUnknown, assets); ok {
		t.Error("unknown resolved to an asset")
	}
}

func TestVerifySHA256GatesTheDownload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aetox-windows-amd64-portable.zip")
	if err := os.WriteFile(path, []byte("release bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	// release.yml's format: `<HASH>  <name>` — uppercase hex, because that is
	// what PowerShell's Get-FileHash writes and the compare must not care.
	sums := []byte(strings.ToUpper(hash) + "  aetox-windows-amd64-portable.zip\r\n")
	if err := verifySHA256(path, "aetox-windows-amd64-portable.zip", sums); err != nil {
		t.Errorf("a matching hash was refused: %v", err)
	}

	bad := []byte("0000000000000000000000000000000000000000000000000000000000000000  aetox-windows-amd64-portable.zip")
	if err := verifySHA256(path, "aetox-windows-amd64-portable.zip", bad); err == nil {
		t.Error("a wrong hash was accepted — this line is the entire gate between a download and running it")
	}

	missing := []byte("1111  some-other-file.zip")
	if err := verifySHA256(path, "aetox-windows-amd64-portable.zip", missing); err == nil {
		t.Error("a file with no checksum line was accepted — unverifiable must mean refused")
	}
}

// The rename trick itself: extract beside the target, move the old aside, move
// the new in. After it, the exe path holds the new build and the old one still
// exists as .old — the rollback if anything downstream goes wrong.
//
// And it restarts nothing. That is the whole point of the split: the swap is
// safe to do while the user keeps working, so it must not be the thing that
// closes their window.
func TestSwapPortableReplacesTheExeAndRestartsNothing(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "aetox.exe")
	if err := os.WriteFile(exe, []byte("old build"), 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(dir, "portable.zip")
	writeZip(t, zipPath, "aetox.exe", []byte("new build"))

	relaunched := ""
	orig := relaunch
	relaunch = func(path string) error { relaunched = path; return nil }
	defer func() { relaunch = orig }()

	if err := swapPortable(exe, zipPath); err != nil {
		t.Fatalf("swapPortable: %v", err)
	}

	if got, _ := os.ReadFile(exe); string(got) != "new build" {
		t.Errorf("exe holds %q, want the new build", got)
	}
	if got, _ := os.ReadFile(exe + ".old"); string(got) != "old build" {
		t.Errorf(".old holds %q, want the old build kept aside", got)
	}
	if relaunched != "" {
		t.Errorf("swapPortable relaunched %q — staging must never close the user's window", relaunched)
	}
}

// The second half, on its own, on the moment the user picked.
func TestStagedRestartRelaunchesThePortableExe(t *testing.T) {
	relaunched := ""
	orig := relaunch
	relaunch = func(path string) error { relaunched = path; return nil }
	defer func() { relaunch = orig }()

	s := Staged{Version: "0.9.7", channel: ChannelPortable, exe: `C:\Aetox\aetox.exe`}
	if !s.Ready() {
		t.Fatal("a staged update reports itself as nothing")
	}
	if err := s.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if relaunched != `C:\Aetox\aetox.exe` {
		t.Errorf("relaunched %q, want the swapped exe", relaunched)
	}
}

// The installer channel stages only the download — the installer itself needs
// the app closed, so all of its work waits for the user's word.
func TestStagedRestartHandsTheInstallerOver(t *testing.T) {
	handed := ""
	orig := handOff
	handOff = func(path string) error { handed = path; return nil }
	defer func() { handOff = orig }()

	s := Staged{Version: "0.9.7", channel: ChannelInstaller, installer: `C:\tmp\aetox-installer.exe`}
	if err := s.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if handed != `C:\tmp\aetox-installer.exe` {
		t.Errorf("handed over %q, want the verified installer", handed)
	}
}

// Nothing staged, nothing to restart into — and in particular, no relaunch of
// an unchanged build dressed up as an update.
func TestZeroStagedRestartsNothing(t *testing.T) {
	called := false
	origR, origH := relaunch, handOff
	relaunch = func(string) error { called = true; return nil }
	handOff = func(string) error { called = true; return nil }
	defer func() { relaunch, handOff = origR, origH }()

	if (Staged{}).Ready() {
		t.Error("the zero Staged claims to be ready")
	}
	if err := (Staged{}).Restart(); err == nil {
		t.Error("restarting nothing returned nil")
	}
	if called {
		t.Error("an empty Staged reached a real ending")
	}
}

func TestSwapPortableRefusesAZipWithNoExe(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "aetox.exe")
	if err := os.WriteFile(exe, []byte("old build"), 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(dir, "empty.zip")
	writeZip(t, zipPath, "README.md", []byte("not an exe"))

	if err := swapPortable(exe, zipPath); err == nil {
		t.Fatal("a zip with no exe was applied")
	}
	// And the running build was never touched.
	if got, _ := os.ReadFile(exe); string(got) != "old build" {
		t.Errorf("exe holds %q after a refused update, want it untouched", got)
	}
}

func TestCanAutoNeedsChecksumsSignatureAndAnAsset(t *testing.T) {
	full := []Asset{
		{Name: "aetox-windows-amd64-portable.zip"},
		{Name: "aetox-amd64-installer.exe"},
		{Name: "checksums.txt"},
		{Name: "checksums.txt.sig"},
	}
	if runtime.GOOS != "windows" {
		if canAuto(ChannelPortable, full) {
			t.Error("canAuto off Windows — nothing ships there yet")
		}
		return
	}
	if !canAuto(ChannelPortable, full) || !canAuto(ChannelInstaller, full) {
		t.Error("a release with all its files should auto-update portable and installer")
	}
	if canAuto(ChannelScoop, full) {
		t.Error("scoop must stay Scoop's")
	}
	// No checksums = no verification = no update. The unverifiable download is
	// the one this feature must never run — and "unverifiable" includes a
	// release from before signing existed.
	noSums := []Asset{{Name: "aetox-windows-amd64-portable.zip"}, {Name: "checksums.txt.sig"}}
	if canAuto(ChannelPortable, noSums) {
		t.Error("canAuto without checksums.txt — an unverifiable download was offered as one-click")
	}
	noSig := []Asset{{Name: "aetox-windows-amd64-portable.zip"}, {Name: "checksums.txt"}}
	if canAuto(ChannelPortable, noSig) {
		t.Error("canAuto without checksums.txt.sig — an unsigned release was offered as one-click")
	}
}

// The signature is trust's first link: sig → checksums → artifact. relsign
// (cmd/relsign) writes base64 of ed25519.Sign over the file; this is the
// verifying half, against a throwaway pair.
func TestVerifySignatureRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	orig := releasePublicKey
	releasePublicKey = base64.StdEncoding.EncodeToString(pub)
	defer func() { releasePublicKey = orig }()

	sums := []byte("AAAA  aetox-windows-amd64-portable.zip\n")
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sums)) + "\n")

	if err := verifySignature(sums, sig); err != nil {
		t.Errorf("a genuine signature was refused: %v", err)
	}
	if err := verifySignature([]byte("BBBB  tampered\n"), sig); err == nil {
		t.Error("a signature over different bytes was accepted — the whole gate is this refusal")
	}
	if err := verifySignature(sums, []byte("bm90LWEtc2ln")); err == nil {
		t.Error("garbage accepted as a signature")
	}

	// A build with no usable key must fail closed, never skip the check.
	releasePublicKey = ""
	if err := verifySignature(sums, sig); err == nil {
		t.Error("an unkeyed build verified a signature — it has nothing to verify WITH")
	}
}

func writeZip(t *testing.T, path, name string, content []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
