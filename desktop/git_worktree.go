package main

// The working tree, as a room you can open (§161.4).
//
// The chat timeline says what *this turn's* calls changed, hunk by hunk, under
// the row that made each one. That is the right answer to "what did you just
// do" and the wrong one to "where does my repository stand" — a turn is not a
// session, a session is not an afternoon, and by the third turn the honest
// answer to the second question lives nowhere in the window.
//
// So: one panel, the branch and the working tree at the top, a row per changed
// file with its own `+N -M`, and a diff behind each row. The diff is built by
// internal/skill's differ rather than by parsing `git diff` output, for one
// reason that is worth the extra call: the fold-out in the chat and the fold-out
// here then draw the *same format*, capped the same way, from the same code. Two
// renderers for one thing is how they drift.
//
// Every "not applicable" answers the same way — unfocused, not a repo, git not
// installed — because all three mean "there is no working tree to show", and a
// panel that distinguishes them would be reporting on itself rather than on the
// user's code.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/proc"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// GitFileChange is one row of the panel.
type GitFileChange struct {
	Path string `json:"path"`
	// "M" changed, "U" untracked or newly added, "D" gone. The same three the
	// file tree's badges use, so a file means the same thing in both places.
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

// GitWorkingTree lists what the focused project's working tree has that HEAD
// does not, newest information first: status from `git status --porcelain`,
// line counts from `git diff --numstat HEAD`.
//
// Counts come from a second call rather than being computed here, because
// numstat is what git itself reports and a panel that disagreed with
// `git diff --stat` about the same file would be a bug nobody could explain.
// An untracked file has no numstat row — nothing to compare against — so its
// own line count is the addition, which is what git shows once it is added.
func (a *App) GitWorkingTree() []GitFileChange {
	out := []GitFileChange{}
	root, ok := a.gitRoot()
	if !ok {
		return out
	}

	status, err := gitOut(root, "status", "--porcelain")
	if err != nil {
		return out
	}
	counts := numstat(root)

	for _, line := range strings.Split(strings.TrimRight(status, "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		// A rename arrives as "old -> new"; the new name is the one that exists.
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		path = strings.Trim(path, `"`)

		row := GitFileChange{Path: path, Status: "M"}
		switch {
		case strings.Contains(code, "D"):
			row.Status = "D"
		case strings.Contains(code, "?"), strings.Contains(code, "A"):
			row.Status = "U"
		}
		if c, hit := counts[path]; hit {
			row.Added, row.Removed = c[0], c[1]
		} else if row.Status == "U" {
			row.Added = fileLineCount(filepath.Join(root, filepath.FromSlash(path)))
		}
		out = append(out, row)
	}
	return out
}

// GitFileDiff is what one row unfolds into: the same git-style hunks the chat
// draws under an edit, for this file's difference from HEAD.
//
// Empty for anything that has no readable answer — outside a project, not a
// repo, a path that leaves the root, a binary file. The panel draws "no diff to
// show" rather than an error, because every one of those is an ordinary state
// of a working tree and none of them is the user's mistake.
func (a *App) GitFileDiff(path string) string {
	root, ok := a.gitRoot()
	if !ok {
		return ""
	}
	clean := strings.TrimSpace(path)
	if clean == "" {
		return ""
	}
	// The panel's own rows are the only source of these paths, but a binding is
	// a public door: a path that climbs out of the project is refused here
	// rather than trusted because of where it usually comes from.
	full := filepath.Join(root, filepath.FromSlash(clean))
	if rel, relErr := filepath.Rel(root, full); relErr != nil || strings.HasPrefix(rel, "..") {
		return ""
	}

	// HEAD's copy, or nothing when the file is new to git — `git show` fails
	// for a path HEAD never had, and "" is exactly what that means here.
	before, _ := gitOut(root, "show", "HEAD:"+filepath.ToSlash(clean))

	after := ""
	if data, readErr := os.ReadFile(full); readErr == nil {
		if strings.ContainsRune(string(data), 0) {
			return "" // binary; hunks would be noise, not information
		}
		after = string(data)
	}
	return skill.FileDiff(clean, before, after)
}

// gitRoot is the project this panel reports on, and whether there is one at
// all. Unfocused mode has no project: home may well sit inside somebody's
// repository, and that repository's status is not this window's business.
func (a *App) gitRoot() (string, bool) {
	if !a.projectFocused {
		return "", false
	}
	root := a.cur().cfg.SandboxRoot
	if strings.TrimSpace(root) == "" {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return "", false
	}
	return root, true
}

func gitOut(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	proc.HideConsole(cmd)
	raw, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// numstat maps path -> {added, removed} against HEAD, staged and unstaged
// together, which is what the working tree actually holds.
func numstat(root string) map[string][2]int {
	counts := map[string][2]int{}
	raw, err := gitOut(root, "diff", "--numstat", "HEAD")
	if err != nil {
		return counts
	}
	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		cols := strings.SplitN(line, "\t", 3)
		if len(cols) != 3 {
			continue
		}
		// git writes "-" for both counts on a binary file. Zeroes are the right
		// answer there: there are no lines to have changed.
		added, _ := strconv.Atoi(cols[0])
		removed, _ := strconv.Atoi(cols[1])
		counts[strings.Trim(cols[2], `"`)] = [2]int{added, removed}
	}
	return counts
}

func fileLineCount(full string) int {
	data, err := os.ReadFile(full)
	if err != nil || len(data) == 0 {
		return 0
	}
	text := strings.TrimSuffix(string(data), "\n")
	return strings.Count(text, "\n") + 1
}
