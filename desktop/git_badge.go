package main

// The git letter on file-touching tool rows (owner, 29 ส.ค.: "อันไหนที่มัน
// ทำปฏิกิริยากับ git อ่ะ แสดงปกตินะ" — the โค้ด desk should wear the same
// marks an editor wears). A read or an edit of a file that git considers
// modified shows M beside the row, untracked shows U, exactly the vocabulary
// every editor has taught for years. Clean shows nothing: the badge marks the
// noteworthy, and a row wearing a "clean" sticker would be noise stapled to
// the ordinary.
//
// Stamped by the host in recordToolAction — the same place and the same
// reasoning as the browser Tab stamp: the engine cannot know it, the tool
// should not reach forward for it, the host holds both halves.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mikedev115/Aetox/internal/proc"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// fileBadgeTools is every tool whose Subject names a file this could honestly
// describe. Not shell — its subject is a command line, not a path.
var fileBadgeTools = map[string]bool{
	"read": true, "write": true, "edit": true, "edits": true, "delete": true,
	"doc_write": true, "sheet_write": true,
}

// repoRoots remembers which sandbox roots are git repositories, so the common
// case (they are, on the โค้ด desk) costs one stat ever instead of one per
// tool call. Forgotten never: a root that GAINS a .git mid-session starts
// showing badges on the next app start, which is a fair price for zero
// rechecking on every read of a long session.
var (
	repoRootsMu sync.Mutex
	repoRoots   = map[string]bool{}
)

func isGitRepo(root string) bool {
	repoRootsMu.Lock()
	defer repoRootsMu.Unlock()
	if known, ok := repoRoots[root]; ok {
		return known
	}
	info, err := os.Stat(filepath.Join(root, ".git"))
	is := err == nil && info.IsDir()
	repoRoots[root] = is
	return is
}

// gitBadgeTimeout bounds what the stamp may cost a tool result: a status of
// one path answers in a few milliseconds, and a repository where it does not
// is a repository whose rows go unbadged rather than a turn that stutters.
const gitBadgeTimeout = 600 * time.Millisecond

// stampGitBadge fills ev.Git for a file tool's result, or leaves it alone.
func (a *App) stampGitBadge(conv *conversation, ev *turn.ToolEvent) {
	if ev.Action != "result" || !fileBadgeTools[ev.Name] || ev.Subject == "" {
		return
	}
	root := strings.TrimSpace(conv.cfg.SandboxRoot)
	if root == "" || !isGitRepo(root) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitBadgeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain", "--", ev.Subject)
	// Or a console window flashes over the app on every stamped read (the
	// coverage test that caught this forgetting names the symptom).
	proc.HideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return
	}
	ev.Git = badgeFromPorcelain(string(out))
}

// badgeFromPorcelain reduces `git status --porcelain` for one path to the one
// letter an editor would show. Empty input is a clean tracked file (or an
// ignored one) and stays empty on purpose.
func badgeFromPorcelain(out string) string {
	line := strings.TrimRight(strings.SplitN(out, "\n", 2)[0], "\r")
	if len(line) < 2 {
		return ""
	}
	if strings.HasPrefix(line, "??") {
		return "U"
	}
	// Two columns, staged and unstaged; either being marked is the fact worth
	// a letter, and the unstaged column is the fresher of the two.
	for _, c := range []byte{line[1], line[0]} {
		switch c {
		case 'M', 'A', 'D', 'R', 'C', 'T':
			return string(c)
		}
	}
	return ""
}
