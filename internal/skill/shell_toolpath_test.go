package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/proc"
)

// dataRootWithTools points DataRoot at a fresh directory holding the named
// tool folders, so these tests measure the code rather than whether whoever
// ran them had pressed the install button.
func dataRootWithTools(t *testing.T, rel ...string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	for _, r := range rel {
		if err := os.MkdirAll(filepath.Join(root, "tools", r), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", r, err)
		}
	}
	return filepath.Join(root, "tools")
}

// The whole point: a bare `ffmpeg` used to resolve to nothing, and the video
// agent paid for that by carrying an absolute path on every command.
func TestToolPATHAddsTheInstalledFolders(t *testing.T) {
	tools := dataRootWithTools(t, filepath.Join("ffmpeg-gpl", "bin"), "tesseract")

	cmd := &exec.Cmd{Env: []string{"PATH=/usr/bin"}}
	withToolPATH(proc.Native(), cmd)

	got := pathValue(t, cmd.Env)
	for _, want := range []string{
		filepath.Join(tools, "ffmpeg-gpl", "bin"),
		filepath.Join(tools, "tesseract"),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("PATH %q is missing %q", got, want)
		}
	}
	// Appended, not prepended: a machine's own ffmpeg keeps winning.
	if !strings.HasPrefix(got, "/usr/bin"+string(os.PathListSeparator)) {
		t.Errorf("PATH %q no longer starts with what was already there", got)
	}
}

// A folder that was never installed must not become a dead PATH entry.
func TestToolPATHSkipsWhatIsNotInstalled(t *testing.T) {
	tools := dataRootWithTools(t, filepath.Join("ffmpeg-gpl", "bin"))

	cmd := &exec.Cmd{Env: []string{"PATH=/usr/bin"}}
	withToolPATH(proc.Native(), cmd)

	if got := pathValue(t, cmd.Env); strings.Contains(got, filepath.Join(tools, "poppler")) {
		t.Errorf("PATH %q names poppler, which is not installed", got)
	}
}

// The GPL build holds the same three names as the LGPL one and is the only
// one carrying libx264, so a shell must reach it first (capability.go).
func TestToolPATHPrefersTheGPLFFmpeg(t *testing.T) {
	tools := dataRootWithTools(t, filepath.Join("ffmpeg-gpl", "bin"), filepath.Join("ffmpeg", "bin"))

	cmd := &exec.Cmd{Env: []string{"PATH=/usr/bin"}}
	withToolPATH(proc.Native(), cmd)

	got := pathValue(t, cmd.Env)
	gpl := strings.Index(got, filepath.Join(tools, "ffmpeg-gpl", "bin"))
	lgpl := strings.Index(got, filepath.Join(tools, "ffmpeg", "bin"))
	if gpl < 0 || lgpl < 0 {
		t.Fatalf("PATH %q is missing one of the two ffmpeg folders", got)
	}
	if gpl > lgpl {
		t.Errorf("PATH %q searches the LGPL ffmpeg before the GPL one", got)
	}
}

// Windows spells it `Path`. A second `PATH=` in the block would be two
// spellings of one variable, and which one the child reads is not ours to
// decide.
func TestToolPATHRewritesTheExistingSpelling(t *testing.T) {
	dataRootWithTools(t, filepath.Join("ffmpeg-gpl", "bin"))

	cmd := &exec.Cmd{Env: []string{"Path=/usr/bin", "HOME=/home/x"}}
	withToolPATH(proc.Native(), cmd)

	var seen int
	for _, kv := range cmd.Env {
		if name, _, ok := strings.Cut(kv, "="); ok && strings.EqualFold(name, "PATH") {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("env holds %d PATH entries, want the one that was already there rewritten", seen)
	}
	if !strings.HasPrefix(cmd.Env[0], "Path=") {
		t.Errorf("env[0] = %q, want the original spelling kept", cmd.Env[0])
	}
}

// Our folders name nothing inside a distro's filesystem, so a WSL command must
// come back untouched (proc.IsNative's stated purpose).
func TestToolPATHLeavesWSLAlone(t *testing.T) {
	dataRootWithTools(t, filepath.Join("ffmpeg-gpl", "bin"))

	cmd := &exec.Cmd{Env: []string{"PATH=/usr/bin"}}
	withToolPATH(proc.WSL(""), cmd)

	if got := pathValue(t, cmd.Env); got != "/usr/bin" {
		t.Errorf("PATH = %q, want it untouched for a WSL command", got)
	}
}

func pathValue(t *testing.T, env []string) string {
	t.Helper()
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok && strings.EqualFold(name, "PATH") {
			return value
		}
	}
	t.Fatal("no PATH in env")
	return ""
}
