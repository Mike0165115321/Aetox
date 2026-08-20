package main

// The branch control behind the chip in the composer's context row.
//
// The chip has drawn the current branch since project focus existed, as a
// `<span>` — a label, never a control. This file is the other half: the list to
// choose from, and the two ways to move between them.
//
// **Why this shells out to git when readGitBranch deliberately does not.** That
// function answers a question that must never fail — what is on the chip — so it
// reads .git/HEAD directly and a machine with no git still gets a correct label.
// Listing could be done the same way (refs/heads plus packed-refs), but
// *switching* cannot: there is no honest way to move HEAD, update the index and
// rewrite the working tree without git. A picker that lists branches it cannot
// switch to would be worse than one that says git is missing, so both halves ask
// git and both fail the same way.

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/proc"
)

// GitBranch is one row of the picker.
type GitBranch struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

// GitBranches lists the local branches of the focused project, current first
// and the rest in git's own order.
//
// **Local only.** A remote branch in this list would look like somewhere to go
// and be somewhere else entirely: switching to one means creating a local
// tracking branch, which is a different act with a different failure mode and
// belongs behind its own control rather than hidden inside this one.
//
// An empty list is the answer for every kind of "not applicable" — unfocused,
// not a repo, no git on PATH — because all three mean the same thing to the
// chip: nothing to choose between, so stay a label.
func (a *App) GitBranches() []GitBranch {
	out := []GitBranch{}
	// Unfocused mode is rooted at the user's home directory. Even when that sits
	// inside a repository it is not the project, and offering to switch its
	// branch from here would be acting on something the user never pointed at.
	if !a.projectFocused {
		return out
	}
	root := strings.TrimSpace(a.cur().cfg.SandboxRoot)
	if root == "" {
		return out
	}
	// proc-detached: one git read, bounded by its own completion a few lines down
	cmd := exec.Command("git", "-C", root, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	proc.HideConsole(cmd)
	raw, err := cmd.Output()
	if err != nil {
		return out
	}
	current := readGitBranch(root)
	for _, name := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, GitBranch{Name: name, Current: name == current})
	}
	// Current first: it is the row the user is looking for confirmation of, and
	// on a repository with forty branches it is otherwise wherever the alphabet
	// put it. The rest keep git's order rather than being sorted here — that
	// order is stable, and re-sorting would only be a second opinion.
	for i, b := range out {
		if b.Current && i != 0 {
			out = append(out[:i], out[i+1:]...)
			out = append([]GitBranch{b}, out...)
			break
		}
	}
	return out
}

// GitSwitchBranch moves the focused project onto an existing branch and reports
// the branch actually in force afterwards.
//
// **git decides whether the switch is allowed, and this never overrules it.**
// A working tree holding changes that the switch would overwrite makes git
// refuse, and its refusal is handed back word for word rather than summarised —
// it names the files, which is the part the user needs. Nothing here passes
// --force, --discard-changes or -f: every one of them answers "your work is in
// the way" by destroying the work, and no click in a branch menu is consent to
// that.
//
// The branch is returned rather than assumed, because a refused switch leaves
// the caller drawing a chip that says somewhere it never went.
func (a *App) GitSwitchBranch(name string) (string, error) {
	return a.runGitSwitch(name, false)
}

// GitCreateBranch cuts a new branch from where HEAD is now and switches to it —
// the picker's "create and switch to new branch".
//
// Cut from HEAD and not from a named base, deliberately: a base picker is a
// second decision, and the overwhelmingly common one is "branch off what I am
// looking at". Naming a base is `git switch -c <name> <start>` when somebody
// asks for it.
func (a *App) GitCreateBranch(name string) (string, error) {
	return a.runGitSwitch(name, true)
}

func (a *App) runGitSwitch(name string, create bool) (string, error) {
	if !a.projectFocused {
		return "", errors.New("ยังไม่ได้โฟกัสโปรเจกต์ จึงยังไม่มีรีโปให้สลับสาขา")
	}
	root := strings.TrimSpace(a.cur().cfg.SandboxRoot)
	if root == "" {
		return "", errors.New("ยังไม่ได้โฟกัสโปรเจกต์ จึงยังไม่มีรีโปให้สลับสาขา")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return readGitBranch(root), errors.New("ต้องบอกชื่อสาขา")
	}
	// A name beginning with '-' would reach git as a flag rather than as a
	// branch. exec.Command already rules out a shell, so this is not about
	// quoting — it is that `git switch --detach` is a real command, and a text
	// field is not where that decision should be made. git's own name rules
	// (spaces, ~, ^, :, ..) are left to git, which refuses with a message that
	// says which rule was broken.
	if strings.HasPrefix(name, "-") {
		return readGitBranch(root), fmt.Errorf("ชื่อสาขาขึ้นต้นด้วย - ไม่ได้: %q", name)
	}

	// `switch` rather than `checkout`, and the difference is not cosmetic.
	// `git checkout <name>` means "restore this path from HEAD" when <name>
	// happens to match a file and no branch — which silently throws away the
	// user's edits to that file. `git switch` only ever means a branch: given a
	// name that is not one, it fails and changes nothing.
	args := []string{"-C", root, "switch"}
	if create {
		args = append(args, "-c")
	}
	args = append(args, name)
	// proc-detached: one git read, bounded by its own completion a few lines down
	cmd := exec.Command("git", args...)
	proc.HideConsole(cmd)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = err.Error()
		}
		return readGitBranch(root), errors.New(msg)
	}
	return readGitBranch(root), nil
}
