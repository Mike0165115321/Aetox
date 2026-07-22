package rtk

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAssetNameForKnownPlatforms(t *testing.T) {
	cases := map[[2]string]string{
		{"windows", "amd64"}: "rtk-x86_64-pc-windows-msvc.zip",
		{"darwin", "amd64"}:  "rtk-x86_64-apple-darwin.tar.gz",
		{"darwin", "arm64"}:  "rtk-aarch64-apple-darwin.tar.gz",
		{"linux", "amd64"}:   "rtk-x86_64-unknown-linux-musl.tar.gz",
		{"linux", "arm64"}:   "rtk-aarch64-unknown-linux-gnu.tar.gz",
	}
	for platform, want := range cases {
		if got := assetNameFor(platform[0], platform[1]); got != want {
			t.Errorf("assetNameFor(%q, %q) = %q, want %q", platform[0], platform[1], got, want)
		}
	}
}

func TestAssetNameForUnsupportedPlatformIsEmpty(t *testing.T) {
	if got := assetNameFor("plan9", "386"); got != "" {
		t.Errorf("assetNameFor(plan9, 386) = %q, want \"\"", got)
	}
}

func TestParseSha256DigestValid(t *testing.T) {
	got, ok := parseSha256Digest("sha256:abc123")
	if !ok || got != "abc123" {
		t.Errorf("parseSha256Digest: got (%q, %v), want (\"abc123\", true)", got, ok)
	}
}

func TestParseSha256DigestMissingPrefix(t *testing.T) {
	if _, ok := parseSha256Digest("abc123"); ok {
		t.Error("expected ok=false for a digest with no sha256: prefix")
	}
}

func TestParseSha256DigestEmpty(t *testing.T) {
	if _, ok := parseSha256Digest(""); ok {
		t.Error("expected ok=false for an empty digest")
	}
}

func TestExtractSingleFileZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fw, err := zw.Create("rtk.exe")
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := fw.Write([]byte("fake-binary-content")); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "rtk.exe")
	if err := extractSingleFileZip(buf.Bytes(), dest); err != nil {
		t.Fatalf("extractSingleFileZip: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != "fake-binary-content" {
		t.Errorf("extracted content = %q, want %q", got, "fake-binary-content")
	}
}

func TestExtractSingleFileTarGz(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	content := []byte("fake-unix-binary")
	if err := tw.WriteHeader(&tar.Header{Name: "rtk", Typeflag: tar.TypeReg, Size: int64(len(content)), Mode: 0o755}); err != nil {
		t.Fatalf("tar.WriteHeader: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip.Close: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "rtk")
	if err := extractSingleFileTarGz(buf.Bytes(), dest); err != nil {
		t.Fatalf("extractSingleFileTarGz: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("extracted content = %q, want %q", got, content)
	}
}

// The two tests below hit the real GitHub API — same "test against the real
// thing, not a mock" discipline as rtk_test.go's TestRewriteRealBinary (which
// caught a real exit-code bug a mock would have missed). Kept cheap: they
// fetch checksums.txt (838 bytes), never the multi-MB platform binaries.
// Skip gracefully on any failure — indistinguishable here from "offline,"
// and a network hiccup must never fail the suite.

func TestFetchAssetInfoRealAPI(t *testing.T) {
	asset, ok := fetchAssetInfo("checksums.txt")
	if !ok {
		t.Skip("could not reach GitHub API (offline, or rtk-ai/rtk renamed its assets)")
	}
	if asset.URL == "" {
		t.Error("asset URL is empty")
	}
	if _, ok := parseSha256Digest(asset.Digest); !ok {
		t.Errorf("asset digest %q doesn't look like sha256:<hex>", asset.Digest)
	}
}

func TestDownloadAndVerifyRealAsset(t *testing.T) {
	asset, ok := fetchAssetInfo("checksums.txt")
	if !ok {
		t.Skip("could not reach GitHub API (offline, or rtk-ai/rtk renamed its assets)")
	}
	data, ok := downloadAndVerify(asset.URL, asset.Digest)
	if !ok {
		t.Fatal("downloadAndVerify failed for an asset the API just confirmed exists")
	}
	sum := sha256.Sum256(data)
	wantHex, _ := parseSha256Digest(asset.Digest)
	if hex.EncodeToString(sum[:]) != wantHex {
		t.Error("downloaded content doesn't match the digest that was already checked internally")
	}
}

func TestDownloadAndVerifyRejectsWrongChecksum(t *testing.T) {
	asset, ok := fetchAssetInfo("checksums.txt")
	if !ok {
		t.Skip("could not reach GitHub API (offline, or rtk-ai/rtk renamed its assets)")
	}
	if _, ok := downloadAndVerify(asset.URL, "sha256:0000000000000000000000000000000000000000000000000000000000000000"); ok {
		t.Error("expected downloadAndVerify to reject a deliberately wrong checksum")
	}
}

// TestTryAutoInstallEndToEnd downloads the real, full-size platform binary
// and confirms the resulting file actually runs — the piece-tests above
// prove fetch/verify/extract each work in isolation, but not that they
// compose correctly end to end (real directory creation, real chmod, the
// binary genuinely being runnable afterward). Skipped by default (opt in
// with AETOX_TEST_RTK_E2E=1) since it downloads several MB on every run —
// too slow/network-heavy for the routine `go test ./...` sweep.
func TestTryAutoInstallEndToEnd(t *testing.T) {
	if os.Getenv("AETOX_TEST_RTK_E2E") == "" {
		t.Skip("set AETOX_TEST_RTK_E2E=1 to run the real download+install end-to-end check")
	}
	path, ok := tryAutoInstall()
	if !ok {
		t.Fatal("tryAutoInstall failed")
	}
	t.Cleanup(func() { os.Remove(path) })

	if !isExecutableFile(path) {
		t.Fatalf("installed path %q is not a file", path)
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("running the installed binary failed: %v\noutput: %s", err, out)
	}
	if !bytes.Contains(out, []byte("rtk")) {
		t.Errorf("--version output = %q, want it to mention \"rtk\"", out)
	}
}
