package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// A repository with three branches and one commit, so the branch control has
// something real to answer about. Skips rather than fails where git is missing:
// every function under test is a wrapper around git, and a machine without it
// is a machine where they correctly do nothing.
func repoWithBranches(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH; these functions are wrappers around it")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "first")
	run("branch", "dev")
	run("branch", "feature/x")
	return root
}

// The bare two fields the branch functions read. Deliberately not
// workspace_test.go's focusedApp, which boots a database and an engine: nothing
// here touches either, and paying for both on every case would make a git
// wrapper's tests the slow ones in the package.
func branchApp(root string) *App {
	return seed(&App{cfg: config.Config{SandboxRoot: root}, projectFocused: true}, newConversation())
}

func TestBranchesListsTheLocalOnesWithTheCurrentFirst(t *testing.T) {
	a := branchApp(repoWithBranches(t))

	got := a.GitBranches()
	if len(got) != 3 {
		t.Fatalf("expected three branches, got %d: %+v", len(got), got)
	}
	// Current first: on a repository with forty branches, "where am I" is the
	// question this menu is opened to answer, and the alphabet is no place to
	// look for it.
	if !got[0].Current || got[0].Name != "main" {
		t.Errorf("the checked-out branch must lead the list, got %+v", got[0])
	}
	for _, b := range got[1:] {
		if b.Current {
			t.Errorf("only one branch can be current, %q also claimed it", b.Name)
		}
	}
}

// Every kind of "not applicable" answers the same way, because they mean the
// same thing to the chip: nothing to choose between, so stay a label.
func TestBranchesAreEmptyWhereThereIsNothingToChooseFrom(t *testing.T) {
	root := repoWithBranches(t)

	unfocused := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
	if got := unfocused.GitBranches(); len(got) != 0 {
		t.Errorf("unfocused mode has no project, so no branches: %+v", got)
	}
	notARepo := branchApp(t.TempDir())
	if got := notARepo.GitBranches(); len(got) != 0 {
		t.Errorf("a folder that is not a repository has no branches: %+v", got)
	}
}

func TestSwitchMovesTheRepositoryAndReportsWhereItLanded(t *testing.T) {
	a := branchApp(repoWithBranches(t))

	now, err := a.GitSwitchBranch("dev")
	if err != nil {
		t.Fatalf("switching to an existing branch must work: %v", err)
	}
	if now != "dev" {
		t.Errorf("expected to land on dev, reported %q", now)
	}
	if got := readGitBranch(a.cur().cfg.SandboxRoot); got != "dev" {
		t.Errorf("the repository itself is on %q, so the report was a guess", got)
	}
}

// The property the whole control rests on: git decides, and a refusal leaves
// both the repository and the user's edits exactly where they were. Nothing
// here passes --force, and this is the test that would fail if somebody added
// it to make this case "work".
func TestARefusedSwitchKeepsTheBranchAndTheUnsavedWork(t *testing.T) {
	root := repoWithBranches(t)
	a := branchApp(root)

	// Give dev its own version of a.txt, so switching onto it would have to
	// overwrite the file.
	if _, err := a.GitSwitchBranch("dev"); err != nil {
		t.Fatalf("setup switch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("dev version\n"), 0o644); err != nil {
		t.Fatalf("seed dev file: %v", err)
	}
	commit := exec.Command("git", "-C", root, "commit", "-qam", "dev edits a.txt")
	commit.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("commit on dev: %v\n%s", err, out)
	}
	if _, err := a.GitSwitchBranch("main"); err != nil {
		t.Fatalf("back to main: %v", err)
	}

	const unsaved = "work the user has not committed\n"
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(unsaved), 0o644); err != nil {
		t.Fatalf("dirty the tree: %v", err)
	}

	now, err := a.GitSwitchBranch("dev")
	if err == nil {
		t.Fatal("git refuses this switch; the app must not have gone anyway")
	}
	// git's message names the file in the way, which is the half a summary
	// would drop and the half the user acts on.
	if !strings.Contains(err.Error(), "a.txt") {
		t.Errorf("the refusal must name what is in the way, got %q", err)
	}
	if now != "main" {
		t.Errorf("after a refusal the chip must say where the repository is, got %q", now)
	}
	body, readErr := os.ReadFile(filepath.Join(root, "a.txt"))
	if readErr != nil {
		t.Fatalf("read back: %v", readErr)
	}
	if string(body) != unsaved {
		t.Errorf("the user's uncommitted work was destroyed: %q", body)
	}
}

func TestCreateCutsANewBranchAndSwitchesToIt(t *testing.T) {
	a := branchApp(repoWithBranches(t))

	now, err := a.GitCreateBranch("release/1.0.0")
	if err != nil {
		t.Fatalf("creating a branch must work: %v", err)
	}
	if now != "release/1.0.0" {
		t.Errorf("expected to land on the new branch, reported %q", now)
	}
	// A name already taken is git's to refuse, and its refusal says so plainly.
	if _, err := a.GitCreateBranch("dev"); err == nil {
		t.Error("creating a branch that already exists must fail rather than switch to it")
	}
}

// A leading '-' reaches git as a flag rather than as a branch, and `git switch
// --detach` is a real command. A text field is not where that decision gets
// made, so it is refused here rather than passed through.
func TestANameThatWouldReachGitAsAFlagIsRefused(t *testing.T) {
	root := repoWithBranches(t)
	a := branchApp(root)

	for _, name := range []string{"--detach", "-d", "--orphan"} {
		now, err := a.GitSwitchBranch(name)
		if err == nil {
			t.Errorf("%q must be refused before it reaches git", name)
		}
		if now != "main" {
			t.Errorf("a refused name must leave the branch alone, got %q", now)
		}
	}
	if _, err := a.GitCreateBranch("-c"); err == nil {
		t.Error("create must refuse a flag-shaped name too")
	}
}

// Unfocused mode is rooted at the user's home directory. Even when that sits
// inside a repository it is not the project, and switching its branch from here
// would be acting on something the user never pointed at.
func TestSwitchingRefusesWithoutAFocusedProject(t *testing.T) {
	a := seed(&App{cfg: config.Config{SandboxRoot: repoWithBranches(t)}}, newConversation())

	if _, err := a.GitSwitchBranch("dev"); err == nil {
		t.Error("unfocused mode has no project to switch")
	}
	if _, err := a.GitCreateBranch("whatever"); err == nil {
		t.Error("unfocused mode has no project to branch")
	}
}
