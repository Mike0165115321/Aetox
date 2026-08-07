package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoFile reads a file by its path from the repository root. Tests run in
// their own package directory, so everything here is two levels up.
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// The version the Windows build stamps into the exe's own version resource,
// and the one the installer's VIProductVersion carries. release.yml overwrites
// this field from the pushed tag before building — so if it disagrees with
// Current here, the tag and the source are about to disagree too.
func TestWailsJSONCarriesCurrent(t *testing.T) {
	var cfg struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal([]byte(repoFile(t, "desktop/wails.json")), &cfg); err != nil {
		t.Fatalf("desktop/wails.json is not valid JSON: %v", err)
	}
	if cfg.Info.ProductVersion != Current {
		t.Errorf("desktop/wails.json info.productVersion = %q, want %q — bump it", cfg.Info.ProductVersion, Current)
	}
}

// Scoop reads this manifest to decide whether an installed copy is stale, so a
// wrong version or a URL pointing at another tag is a broken upgrade path for
// every Scoop user.
//
// `hash` is deliberately not checked: it is the SHA256 of a zip that does not
// exist until CI publishes the tag, so it legitimately lags one release behind
// until the post-release step fills it in (ARCHITECTURE.md §58).
func TestScoopManifestCarriesCurrent(t *testing.T) {
	var m struct {
		Version string `json:"version"`
		URL     string `json:"url"`
	}
	if err := json.Unmarshal([]byte(repoFile(t, "scoop/aetox.json")), &m); err != nil {
		t.Fatalf("scoop/aetox.json is not valid JSON: %v", err)
	}
	if m.Version != Current {
		t.Errorf("scoop/aetox.json version = %q, want %q — bump it", m.Version, Current)
	}
	if want := "/download/v" + Current + "/"; !strings.Contains(m.URL, want) {
		t.Errorf("scoop/aetox.json url = %q, want it to point at %q", m.URL, want)
	}
}

// The two places a *reader* is told which release this is. Only presence of
// the current marker is asserted — older version numbers elsewhere in these
// files are history, not staleness.
func TestPublishedDocsCarryCurrent(t *testing.T) {
	for _, c := range []struct{ file, marker, what string }{
		{"README.md", "## Status — v" + Current, "status heading"},
		{"docs/index.html", ">v" + Current + " · Windows<", "version badge"},
	} {
		if !strings.Contains(repoFile(t, c.file), c.marker) {
			t.Errorf("%s: %s does not say v%s — bump it (or fix this test if the markup changed)", c.file, c.what, Current)
		}
	}
}
