package proc

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestEveryExecSiteHidesTheConsole is a source guard, not a behaviour test.
//
// The desktop build is a GUI-subsystem exe, so any console child it spawns
// gets its own console — and on Windows 11 that console is hosted in a visible
// Windows Terminal window that flashes over the app. The fix is one line
// (HideConsole) at every spawn site, which means the failure mode is not a bug
// in the fix but somebody forgetting it on the next site added. With MCP
// servers, skills and language servers all growing over time, that "somebody"
// is a certainty; a reviewer noticing a missing line is not.
//
// So: every exec.Command/exec.CommandContext in non-test code must be followed
// within a few lines by HideConsole on the same command. Fails loudly with the
// file and line when it isn't.
func TestEveryExecSiteHidesTheConsole(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	spawn := regexp.MustCompile(`(\w+)\s*:?=\s*exec\.Command(?:Context)?\(`)
	// A few sites legitimately never pass through HideConsole.
	skipDir := map[string]bool{
		"third_party":  true, // vendored; the conpty patch sets CREATE_NO_WINDOW itself
		"node_modules": true,
		".git":         true,
	}

	var missing []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			m := spawn.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			// The window is deliberately generous: HideConsole usually sits on
			// the next line, but a cmd.Dir or a comment in between is fine.
			want := "HideConsole(" + m[1] + ")"
			hidden := false
			for j := i; j < len(lines) && j < i+8; j++ {
				if strings.Contains(lines[j], want) {
					hidden = true
					break
				}
			}
			if !hidden {
				rel, _ := filepath.Rel(root, path)
				missing = append(missing, rel+":"+strconv.Itoa(i+1)+" — "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(missing) > 0 {
		t.Errorf("exec site(s) without proc.HideConsole — each one flashes a "+
			"terminal window over the desktop app on Windows:\n  %s",
			strings.Join(missing, "\n  "))
	}
}
