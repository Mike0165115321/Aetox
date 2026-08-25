package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
)

// repoAt builds a real repository with one committed file, because every claim
// this panel makes is about the difference between HEAD and the disk, and a
// fake of that is a fake of the whole feature.
func repoAt(t *testing.T) (string, *App) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "kept.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	run("add", "kept.txt")
	run("commit", "-m", "first")

	a := seed(&App{cfg: config.Config{SandboxRoot: root}, projectFocused: true}, newConversation())
	return root, a
}

func TestGitWorkingTreeCountsWhatChanged(t *testing.T) {
	root, a := repoAt(t)
	if err := os.WriteFile(filepath.Join(root, "kept.txt"), []byte("one\nTWO\nthree\n"), 0o644); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fresh.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatalf("add: %v", err)
	}

	rows := map[string]GitFileChange{}
	for _, f := range a.GitWorkingTree() {
		rows[f.Path] = f
	}
	if len(rows) != 2 {
		t.Fatalf("GitWorkingTree() = %+v, want two rows", rows)
	}
	if got := rows["kept.txt"]; got.Status != "M" || got.Added != 1 || got.Removed != 1 {
		t.Errorf("kept.txt = %+v, want M +1 -1", got)
	}
	// An untracked file has no numstat row to read, so its own length is the
	// addition — which is what git reports the moment it is added.
	if got := rows["fresh.txt"]; got.Status != "U" || got.Added != 2 || got.Removed != 0 {
		t.Errorf("fresh.txt = %+v, want U +2 -0", got)
	}
}

func TestGitWorkingTreeCleanTreeHasNoRows(t *testing.T) {
	_, a := repoAt(t)
	if got := a.GitWorkingTree(); len(got) != 0 {
		t.Errorf("GitWorkingTree() on a clean tree = %+v, want empty", got)
	}
}

// Unfocused mode roams the machine, and home may well sit inside somebody's
// repository. That repository's status is not this window's business.
func TestGitWorkingTreeEmptyWhenUnfocused(t *testing.T) {
	root, a := repoAt(t)
	a.projectFocused = false
	if err := os.WriteFile(filepath.Join(root, "kept.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if got := a.GitWorkingTree(); len(got) != 0 {
		t.Errorf("GitWorkingTree() unfocused = %+v, want empty", got)
	}
}

func TestGitWorkingTreeOutsideARepo(t *testing.T) {
	a := seed(&App{cfg: config.Config{SandboxRoot: t.TempDir()}, projectFocused: true}, newConversation())
	if got := a.GitWorkingTree(); len(got) != 0 {
		t.Errorf("GitWorkingTree() outside a repo = %+v, want empty", got)
	}
}

func TestGitFileDiffShowsTheChangedLine(t *testing.T) {
	root, a := repoAt(t)
	if err := os.WriteFile(filepath.Join(root, "kept.txt"), []byte("one\nTWO\nthree\n"), 0o644); err != nil {
		t.Fatalf("edit: %v", err)
	}

	got := a.GitFileDiff("kept.txt")
	if !strings.HasPrefix(got, "+++ kept.txt\n@@ ") {
		t.Fatalf("GitFileDiff = %q, want the path then a hunk", got)
	}
	if !strings.Contains(got, "-two") || !strings.Contains(got, "+TWO") {
		t.Errorf("GitFileDiff =\n%s\nwant the swap of line 2", got)
	}
	// Line-numbered from the file, which is the whole reason to show hunks
	// rather than the counts the row already carries.
	if !strings.Contains(got, "@@ -1,3 +1,3 @@") {
		t.Errorf("hunk header missing the file's own numbering:\n%s", got)
	}
}

// A file HEAD never had has no `git show` answer, and "" is exactly what that
// means: everything in it is an addition.
func TestGitFileDiffOnANewFileIsAllAdded(t *testing.T) {
	root, a := repoAt(t)
	if err := os.WriteFile(filepath.Join(root, "fresh.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatalf("add: %v", err)
	}
	got := a.GitFileDiff("fresh.txt")
	if !strings.Contains(got, "+a") || !strings.Contains(got, "+b") || strings.Contains(got, "\n-") {
		t.Errorf("GitFileDiff on a new file =\n%s\nwant every line added", got)
	}
}

// A binding is a public door. A path that climbs out of the project is refused
// here rather than trusted because of where it usually comes from.
func TestGitFileDiffRefusesAPathOutsideTheProject(t *testing.T) {
	_, a := repoAt(t)
	if got := a.GitFileDiff("../../../etc/passwd"); got != "" {
		t.Errorf("GitFileDiff escaped the project: %q", got)
	}
}

func TestGitFileDiffEmptyWhenNothingChanged(t *testing.T) {
	_, a := repoAt(t)
	if got := a.GitFileDiff("kept.txt"); got != "" {
		t.Errorf("GitFileDiff on an unchanged file = %q, want empty", got)
	}
}
