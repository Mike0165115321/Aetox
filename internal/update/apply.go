package update

// Apply is the button's second half: Check says a newer Aetox exists, Apply
// makes this machine run it. The design VS Code and every Squirrel app share,
// built on the one Windows fact that makes self-update possible at all: a
// running exe cannot be overwritten, but it CAN be renamed.
//
// Per channel:
//
//   - portable — the rename trick. Download the zip, verify it, extract the
//     new exe next to the running one, rename the running exe aside, rename
//     the new one into its place, and relaunch after this process exits. No
//     elevation: the folder is the user's own.
//   - installer — hand the job to the new installer, exactly as re-running it
//     by hand would (project.nsi already taskkills the app). It runs silent
//     (/S), shows UAC if it needs it, and the app is relaunched after.
//   - scoop — never touched from here. Scoop owns that directory; the UI keeps
//     offering `scoop update aetox` instead of a button.
//
// Both flows relaunch through a tiny waiter that blocks on this process's exit
// first, so the new instance never overlaps the old one on the database.
//
// Trust: the download is verified against checksums.txt from the same release,
// fetched over TLS from the same GitHub API the check trusts — the same
// guarantee as downloading from the releases page by hand, which is what this
// replaces. A signing key (CI signs, the binary verifies) is the next rung on
// that ladder and deliberately not faked here with anything weaker.

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// ErrUpToDate means Apply had nothing to do — the caller raced a fresh check,
// or the user pressed the button twice.
var ErrUpToDate = errors.New("ไม่มีเวอร์ชันใหม่ให้อัปเดต")

// Progress reports one download's motion. total is 0 when the server did not
// say how big the file is.
type Progress func(done, total int64)

// relaunch and handOff are seams over the two OS-specific endings, so the swap
// logic is testable without spawning a real waiter process. Assigned by the
// per-OS files; tests substitute their own.
var (
	relaunch = relaunchAfterExit
	handOff  = handOffToInstaller
)

// canAuto reports whether Apply knows how to finish the job on this channel,
// with the files this release actually carries. It needs checksums.txt AND its
// signature — an unverifiable download is a download this package refuses to
// run, and "unverifiable" includes "unsigned": a release from before signing
// existed simply never lights the one-click button.
func canAuto(c Channel, assets []Asset) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	if _, ok := findAsset(assets, checksumsName); !ok {
		return false
	}
	if _, ok := findAsset(assets, signatureName); !ok {
		return false
	}
	_, ok := channelAsset(c, assets)
	return ok
}

const checksumsName = "checksums.txt"

// channelAsset picks the file this install would need from the release.
func channelAsset(c Channel, assets []Asset) (Asset, bool) {
	switch c {
	case ChannelPortable:
		return findAssetSuffix(assets, "-portable.zip")
	case ChannelInstaller:
		return findAssetSuffix(assets, "-installer.exe")
	default:
		return Asset{}, false
	}
}

func findAsset(assets []Asset, name string) (Asset, bool) {
	for _, a := range assets {
		if strings.EqualFold(a.Name, name) {
			return a, true
		}
	}
	return Asset{}, false
}

func findAssetSuffix(assets []Asset, suffix string) (Asset, bool) {
	for _, a := range assets {
		if strings.HasSuffix(strings.ToLower(a.Name), suffix) {
			return a, true
		}
	}
	return Asset{}, false
}

// Apply downloads the newer release, verifies it against the release's own
// checksums, and swaps this install over to it. On a nil error the process
// must exit soon after: the relaunch waiter is already waiting on it, and for
// the portable channel the exe on disk is already the new one.
func Apply(ctx context.Context, current string, progress Progress) error {
	st, assets, err := checkWithAssets(ctx, current)
	if err != nil {
		return err
	}
	if !st.Available {
		return ErrUpToDate
	}
	channel := Channel(st.Channel)
	if !canAuto(channel, assets) {
		return fmt.Errorf("ช่องทางติดตั้งนี้อัปเดตอัตโนมัติไม่ได้ — เปิดหน้า release แทน: %s", st.URL)
	}
	asset, _ := channelAsset(channel, assets)
	sums, _ := findAsset(assets, checksumsName)
	sig, _ := findAsset(assets, signatureName)

	dir, err := stagingDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, asset.Name)
	if err := download(ctx, asset.URL, path, progress); err != nil {
		return fmt.Errorf("ดาวน์โหลด %s ไม่สำเร็จ: %w", asset.Name, err)
	}
	sumData, err := downloadSmall(ctx, sums.URL)
	if err != nil {
		return fmt.Errorf("ดาวน์โหลด %s ไม่สำเร็จ: %w", checksumsName, err)
	}
	sigData, err := downloadSmall(ctx, sig.URL)
	if err != nil {
		return fmt.Errorf("ดาวน์โหลด %s ไม่สำเร็จ: %w", signatureName, err)
	}
	// Origin first, then integrity: the signature says checksums.txt is ours,
	// the checksums say the download matches it. Order matters — a hash from
	// an unverified checksums file proves nothing.
	if err := verifySignature(sumData, sigData); err != nil {
		return err
	}
	if err := verifySHA256(path, asset.Name, sumData); err != nil {
		return err
	}

	switch channel {
	case ChannelPortable:
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return applyPortableTo(exe, path)
	case ChannelInstaller:
		return handOff(path)
	default:
		return fmt.Errorf("ช่องทาง %s อัปเดตอัตโนมัติไม่ได้", channel)
	}
}

// applyPortableTo swaps exePath over to the exe inside zipPath. The extraction
// lands next to the target — same directory, same volume — so both renames are
// metadata moves that either happen or don't; there is no state where the exe
// is half of each.
func applyPortableTo(exePath, zipPath string) error {
	newPath := exePath + ".new"
	if err := extractExe(zipPath, newPath); err != nil {
		return err
	}
	oldPath := exePath + ".old"
	// A leftover from the previous update would make the rename fail; it is
	// dead weight by definition (this build booted without it).
	_ = os.Remove(oldPath)
	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("ย้ายไฟล์เดิมไม่สำเร็จ: %w", err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		// Put the old exe back — a machine left with NO aetox.exe is the one
		// outcome strictly worse than a failed update.
		_ = os.Rename(oldPath, exePath)
		return fmt.Errorf("วางไฟล์ใหม่ไม่สำเร็จ: %w", err)
	}
	return relaunch(exePath)
}

// extractExe pulls the single exe out of the portable zip. By name-suffix, not
// by position: the zip carries exactly one exe today (release.yml packs only
// aetox.exe), and matching the suffix keeps this working if a README ever
// rides along.
func extractExe(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("เปิดไฟล์ zip ไม่สำเร็จ: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".exe") {
			continue
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, src); err != nil {
			out.Close()
			_ = os.Remove(dest)
			return err
		}
		return out.Close()
	}
	return fmt.Errorf("ไม่พบไฟล์ .exe ในชุดอัปเดต")
}

// verifySHA256 checks the downloaded file against its line in checksums.txt
// (release.yml's format: `<HASH>  <filename>` per line).
func verifySHA256(path, name string, sums []byte) error {
	want, ok := expectedSum(sums, name)
	if !ok {
		return fmt.Errorf("%s ไม่มีบรรทัดของ %s — ปฏิเสธไฟล์ที่ตรวจไม่ได้", checksumsName, name)
	}
	got, err := sha256File(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(want, got) {
		return fmt.Errorf("แฮชของ %s ไม่ตรงกับ %s — ไฟล์อาจเสียหรือถูกแก้ ระบบไม่ติดตั้งต่อ", name, checksumsName)
	}
	return nil
}

func expectedSum(sums []byte, name string) (string, bool) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && strings.EqualFold(fields[1], name) {
			return fields[0], true
		}
	}
	return "", false
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// stagingDir is where downloads land: under the app's own data root, never the
// install directory — the one place guaranteed writable on every channel.
func stagingDir() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "updates")
	return dir, os.MkdirAll(dir, 0o700)
}

// RemoveLeftovers sweeps what an update leaves behind: the renamed-aside old
// exe and the staging directory. Called on startup — by definition, a build
// that is running no longer needs either. Best-effort: an .old still locked by
// the exiting previous instance is simply caught on the next boot.
func RemoveLeftovers() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
	if root, err := config.DataRoot(); err == nil {
		_ = os.RemoveAll(filepath.Join(root, "updates"))
	}
}

func download(ctx context.Context, url, dest string, progress Progress) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Aetox-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	var done int64
	buf := make([]byte, 128<<10)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				out.Close()
				return writeErr
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			return readErr
		}
	}
	return out.Close()
}

func downloadSmall(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Aetox-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}
	// checksums.txt is a few lines; a megabyte of it means something is wrong.
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
