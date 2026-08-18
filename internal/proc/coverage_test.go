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
//
// The exception is a spawn whose window IS the point, and there are two kinds.
//
// One is launching a GUI program — the OS file manager. HideConsole sets
// HideWindow and CREATE_NO_WINDOW, which on a GUI program suppresses the window
// it was asked to show; every "open folder" button in the app did nothing until
// that was removed.
//
// The other is starting a long-lived server the user asked for (an n8n or a
// Windmill, from the settings page). There the console is the only place the
// server can say why it refused to start, and closing it is how the user stops
// the thing — hidden, a port clash would surface as Aetox timing out with no
// way to find out more, and a running server with no window to close.
//
// Both opt out with the same explicit marker rather than by being skipped
// silently, so the exception is as visible in the source as the rule.
// showWindowMarker is what a spawn writes to opt out: it must appear in the
// source, so the exception is reviewable rather than a name on a skip list
// somewhere else.
const showWindowMarker = "proc-show-window: launching a GUI program"

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
				if strings.Contains(lines[j], want) || strings.Contains(lines[j], showWindowMarker) {
					hidden = true
					break
				}
			}
			// The marker may also sit just above the switch that builds the
			// command, which is where a file-manager launcher naturally puts it.
			if !hidden {
				for j := i; j >= 0 && j > i-12; j-- {
					if strings.Contains(lines[j], showWindowMarker) {
						hidden = true
						break
					}
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

// detachedMarker is what a context-less spawn writes to opt out. Like
// showWindowMarker it must appear in the source, so the exception is reviewable
// where it lives rather than being a name on a list somewhere else.
const detachedMarker = "proc-detached:"

// TestEverySpawnCanBeStopped is a source guard, not a behaviour test.
//
// exec.Command without a context produces a process nothing can stop. That is
// correct for a spawn meant to outlive us — the user's browser, a server they
// started from Settings, the relaunch that replaces this exe — and it is a leak
// everywhere else, because the only thing left to reap it is §24's Job Object,
// which fires when the app exits and not before.
//
// It went unnoticed at the two sites where it mattered most, and the reason is
// worth stating: an MCP server and a language server are both started once,
// kept on a struct, and closed by something far away from the spawn, so the
// missing teardown is invisible at both ends. What made it visible was pressing
// Settings' Test button five times and counting five node.exe.
//
// KillOnCancel is the fix and it needs a cancellable context to work at all —
// os/exec only calls cmd.Cancel from the watcher it starts for a context that
// can be done, so exec.Command cannot carry it and context.Background() would
// take it and silently never fire. Hence the rule is about the context: a spawn
// with none says so, in the source, with the reason.
func TestEverySpawnCanBeStopped(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	// Deliberately not matching CommandContext: a spawn that has a context can
	// be stopped, and whether it also needs a tree kill on top is a judgement
	// about whether the program forks, which no regex can make.
	spawn := regexp.MustCompile(`exec\.Command\(`)
	skipDir := map[string]bool{"third_party": true, "node_modules": true, ".git": true}

	var loose []string
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
			if !spawn.MatchString(line) || strings.Contains(line, "exec.CommandContext(") {
				continue
			}
			excused := false
			// Wide enough that one marker above a switch covers every case
			// in it, which is how the file-manager and browser launchers
			// already document their other exception.
			for j := i; j >= 0 && j > i-14; j-- {
				if strings.Contains(lines[j], detachedMarker) {
					excused = true
					break
				}
			}
			if !excused {
				rel, _ := filepath.Rel(root, path)
				loose = append(loose, rel+":"+strconv.Itoa(i+1)+" — "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(loose) > 0 {
		t.Errorf("exec.Command with no context — nothing can stop these, so a server "+
			"that forks leaves its children running until the app exits:\n  %s\n\nUse "+
			"exec.CommandContext with a context the owner cancels, plus proc.KillOnCancel, "+
			"or write %q with the reason this one is meant to outlive us.",
			strings.Join(loose, "\n  "), detachedMarker)
	}
}
