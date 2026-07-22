package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildIncludesIdentityAndEnvironment(t *testing.T) {
	got := Build(SurfaceCLI, "/tmp/proj")
	if !strings.Contains(got, "terminal conversation") {
		t.Fatalf("missing CLI identity: %s", got)
	}
	if !strings.Contains(got, "/tmp/proj") {
		t.Fatalf("missing sandbox root: %s", got)
	}
}

func TestBuildDesktopIdentity(t *testing.T) {
	got := Build(SurfaceDesktop, "/tmp/proj")
	if !strings.Contains(got, "desktop chat UI") {
		t.Fatalf("missing desktop identity: %s", got)
	}
}

func TestProjectContextFilePrefersAetoxOverAgents(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "agents")
	mustWrite(t, filepath.Join(dir, "AETOX.md"), "aetox")
	if got := ProjectContextFile(dir); filepath.Base(got) != "AETOX.md" {
		t.Fatalf("want AETOX.md, got %q", got)
	}
}

func TestProjectContextFileFallsBackToAgents(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "agents")
	if got := ProjectContextFile(dir); filepath.Base(got) != "AGENTS.md" {
		t.Fatalf("want AGENTS.md fallback, got %q", got)
	}
}

func TestProjectContextFileMissingReturnsEmpty(t *testing.T) {
	if got := ProjectContextFile(t.TempDir()); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestBuildWithReportFoldsInProjectLayerAndReportsPath(t *testing.T) {
	dir := t.TempDir()
	rulePath := filepath.Join(dir, "AETOX.md")
	mustWrite(t, rulePath, "always answer in haiku")

	text, loaded := BuildWithReport(SurfaceCLI, dir)
	if !strings.Contains(text, "always answer in haiku") {
		t.Fatalf("project rules not folded in: %s", text)
	}
	if loaded.ProjectPath != rulePath {
		t.Fatalf("loaded.ProjectPath = %q, want %q", loaded.ProjectPath, rulePath)
	}
}

func TestReadCappedTruncatesOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.md")
	mustWrite(t, path, strings.Repeat("a", maxLayerBytes+500))
	if got := readCapped(path); len(got) > maxLayerBytes {
		t.Fatalf("readCapped did not truncate: len=%d", len(got))
	}
}

func TestReadCappedMissingFileReturnsEmpty(t *testing.T) {
	if got := readCapped(filepath.Join(t.TempDir(), "nope.md")); got != "" {
		t.Fatalf("want empty for missing file, got %q", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
