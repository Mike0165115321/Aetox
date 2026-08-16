//go:build windows

package proc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// clearPythonEncodingVars makes the test process look like an ordinary machine.
// It matters because the shell a developer runs `go test` from may export one
// of these itself (Claude Code's does), and inheriting it would make every
// assertion below pass for a reason the user's Aetox never has.
func clearPythonEncodingVars(t *testing.T) {
	t.Helper()
	t.Setenv("PYTHONUTF8", "")
	t.Setenv("PYTHONIOENCODING", "")
}

// envValue reads a variable the way the child will: last occurrence wins, which
// is how Windows resolves a name that appears twice in an environment block.
func envValue(env []string, name string) (string, bool) {
	value, found := "", false
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, name) {
			value, found = v, true
		}
	}
	return value, found
}

func TestShellCommandTellsPythonToWriteUTF8(t *testing.T) {
	clearPythonEncodingVars(t)

	cmd := ShellCommand(context.Background(), "python --version")
	if got, ok := envValue(cmd.Env, "PYTHONUTF8"); !ok || got != "1" {
		t.Errorf("PYTHONUTF8 = %q (present: %v), want %q — without it a "+
			"console-less Python writes the ANSI codepage, and a script "+
			"printing anything outside it dies with UnicodeEncodeError",
			got, ok, "1")
	}
}

func TestShellCommandKeepsTheUsersOwnEncodingChoice(t *testing.T) {
	// Both variables are an answer to the same question, so either one being
	// set means the machine has already answered it and Aetox must not answer
	// over the top.
	for _, name := range []string{"PYTHONUTF8", "PYTHONIOENCODING"} {
		t.Run(name, func(t *testing.T) {
			clearPythonEncodingVars(t)
			t.Setenv(name, "cp874")

			cmd := ShellCommand(context.Background(), "python --version")
			if slices.Contains(cmd.Env, "PYTHONUTF8=1") && name != "PYTHONUTF8" {
				t.Errorf("added PYTHONUTF8=1 despite %s being set by the user", name)
			}
			if got, _ := envValue(cmd.Env, name); got != "cp874" {
				t.Errorf("%s = %q, want the user's own %q", name, got, "cp874")
			}
		})
	}
}

// TestShellCommandRunsNonASCIIPython is the failure this file's shellEnv was
// written for, run end to end rather than asserted about: on 2026-08-16 an
// agent's calculus script printed "∞", died with UnicodeEncodeError, reported
// `exit status 1`, and rewrote correct maths to get away from it.
func TestShellCommandRunsNonASCIIPython(t *testing.T) {
	if _, err := exec.LookPath("python"); err != nil {
		t.Skip("no python on PATH")
	}
	clearPythonEncodingVars(t)

	// A file, not a here-string: Python reads source files as UTF-8 whatever
	// the locale says, which leaves stdout as the only encoding under test.
	script := filepath.Join(t.TempDir(), "print_non_ascii.py")
	if err := os.WriteFile(script, []byte("print('∞ ² ทดสอบ')\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := ShellCommand(context.Background(), `python "`+script+`"`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("printing non-ASCII failed with %v:\n%s", err, out)
	}
	for _, want := range []string{"∞", "²", "ทดสอบ"} {
		if !strings.Contains(string(out), want) {
			// Mojibake is the quieter half of the bug: cp874 can encode Thai,
			// so this direction exits 0 and the agent believes the garbage.
			t.Errorf("output is missing %q — it arrived as mojibake:\n%s", want, out)
		}
	}
}
