package rtk

// Runtime install fallback (ARCHITECTURE.md §13.6): if rtk isn't already on
// PATH, download it once from its official GitHub release — mirrors the
// judgment already made for Tesseract on macOS
// (docs/architecture/tesseract-ocr-bundling-2026-07-22.md §3: no
// elevation/sudo needed, so a single automatic attempt is safe; unlike
// Tesseract's Windows story, rtk ships a portable zip/tar.gz with no
// installer wizard, so this works the same way on every OS, not just macOS).
//
// rtk-ai/rtk is a real public project (github.com/rtk-ai/rtk, Apache 2.0,
// 72k+ stars — confirmed via `gh api repos/rtk-ai/rtk`), not the project
// owner's private tool, so bundling/redistributing it is not a licensing
// concern. What IS deliberately not done: patching it into the NSIS
// installer (owner's explicit choice, 2026-07-23) — this whole file only
// runs lazily, in-process, the first time a tool call actually needs rtk.

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
)

const releaseAPIURL = "https://api.github.com/repos/rtk-ai/rtk/releases/latest"

var (
	resolveOnce  sync.Once
	resolvedPath string
)

// resolve finds (or installs) the rtk binary once per process:
//  1. already on PATH — respects an existing system/dev install
//  2. already downloaded by a previous run's auto-install on this machine
//  3. one-time download from the official GitHub release
//
// Any failure at any step (offline, unsupported OS/arch, checksum mismatch,
// rate-limited API, ...) leaves this returning "" — RTK integration stays
// exactly what it always was: optional, fails open to "just don't use it."
func resolve() string {
	resolveOnce.Do(func() {
		if p, err := exec.LookPath("rtk"); err == nil {
			resolvedPath = p
			return
		}
		if p := privateBinaryPath(); isExecutableFile(p) {
			resolvedPath = p
			return
		}
		if p, ok := tryAutoInstall(); ok {
			resolvedPath = p
		}
	})
	return resolvedPath
}

// privateBinaryPath is where a downloaded rtk lands: <UserConfigDir>/aetox/bin/,
// the same directory family as everything else in internal/config (kept
// separate from the config *files* there since this is a binary, not JSON).
func privateBinaryPath() string {
	userGlobalContextFile, err := config.UserGlobalContextPath() // reuse its dir resolution
	if err != nil {
		return ""
	}
	name := "rtk"
	if runtime.GOOS == "windows" {
		name = "rtk.exe"
	}
	return filepath.Join(filepath.Dir(userGlobalContextFile), "bin", name)
}

func isExecutableFile(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// releaseAsset is the subset of GitHub's release-asset JSON this needs.
type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"` // "sha256:<hex>"
}

type release struct {
	Assets []releaseAsset `json:"assets"`
}

// assetNameFor maps GOOS/GOARCH to the exact asset name rtk-ai/rtk publishes
// (confirmed live against the v0.43.0 release — re-verify if a future rtk
// release renames its targets). "" means unsupported.
func assetNameFor(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "windows/amd64":
		return "rtk-x86_64-pc-windows-msvc.zip"
	case "darwin/amd64":
		return "rtk-x86_64-apple-darwin.tar.gz"
	case "darwin/arm64":
		return "rtk-aarch64-apple-darwin.tar.gz"
	case "linux/amd64":
		return "rtk-x86_64-unknown-linux-musl.tar.gz"
	case "linux/arm64":
		return "rtk-aarch64-unknown-linux-gnu.tar.gz"
	default:
		return ""
	}
}

func tryAutoInstall() (string, bool) {
	assetName := assetNameFor(runtime.GOOS, runtime.GOARCH)
	if assetName == "" {
		return "", false
	}
	asset, ok := fetchAssetInfo(assetName)
	if !ok {
		return "", false
	}
	archiveBytes, ok := downloadAndVerify(asset.URL, asset.Digest)
	if !ok {
		return "", false
	}
	binPath := privateBinaryPath()
	if binPath == "" {
		return "", false
	}
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return "", false
	}
	var extractErr error
	if filepath.Ext(assetName) == ".zip" {
		extractErr = extractSingleFileZip(archiveBytes, binPath)
	} else {
		extractErr = extractSingleFileTarGz(archiveBytes, binPath)
	}
	if extractErr != nil {
		return "", false
	}
	if err := os.Chmod(binPath, 0o755); err != nil {
		return "", false
	}
	return binPath, true
}

func fetchAssetInfo(assetName string) (releaseAsset, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPIURL, nil)
	if err != nil {
		return releaseAsset{}, false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return releaseAsset{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseAsset{}, false
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return releaseAsset{}, false
	}
	for _, a := range rel.Assets {
		if a.Name == assetName {
			return a, true
		}
	}
	return releaseAsset{}, false
}

// downloadAndVerify fetches url and checks it against digest (GitHub's own
// "sha256:<hex>" release-asset digest field — confirmed live to match a
// locally-computed sha256sum, so no separately-pinned hash to maintain by
// hand, unlike the Tesseract installer's digest-was-null case).
func downloadAndVerify(url, digest string) ([]byte, bool) {
	wantHex, ok := parseSha256Digest(digest)
	if !ok {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != wantHex {
		return nil, false
	}
	return data, true
}

func parseSha256Digest(digest string) (string, bool) {
	const prefix = "sha256:"
	if len(digest) <= len(prefix) || digest[:len(prefix)] != prefix {
		return "", false
	}
	return digest[len(prefix):], true
}

// extractSingleFileZip pulls the one file rtk's Windows release contains
// (confirmed live: a bare "rtk.exe" at archive root, no subfolder) and writes
// it to destPath.
func extractSingleFileZip(data []byte, destPath string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if len(r.File) == 0 {
		return errors.New("rtk: empty zip archive")
	}
	f := r.File[0]
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return writeExecutable(destPath, rc)
}

// extractSingleFileTarGz pulls the one file rtk's macOS/Linux releases
// contain (confirmed live: a bare executable named "rtk" at archive root).
func extractSingleFileTarGz(data []byte, destPath string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		return err
	}
	if hdr.Typeflag != tar.TypeReg {
		return fmt.Errorf("rtk: unexpected tar entry type %v", hdr.Typeflag)
	}
	return writeExecutable(destPath, tr)
}

func writeExecutable(destPath string, src io.Reader) error {
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}
