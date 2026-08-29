package prompt

// The git snapshot folded into a focused session's prompt — see the call site
// in BuildWithReport for why it sits where it sits. Everything here is capped
// and labeled a snapshot: the prompt is built at bootstrap and does not track
// the tree, and a snapshot that presents itself as live would be the one kind
// of wrong this layer must never be. The git tool stays the live answer.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/proc"
)

const (
	// gitLayerTimeout bounds all three commands together. Bootstrap runs once
	// per session and already does file IO; three local git reads answer in
	// tens of milliseconds, and a repository where they do not (a network
	// mount, a corrupt index) costs the layer, not the session.
	gitLayerTimeout = 2 * time.Second
	// gitLayerMaxDirty is how many uncommitted paths are named. Fifty dirty
	// files is this owner's NORMAL working state (they stay uncommitted for
	// weeks), so the list must summarize past a point, never enumerate.
	gitLayerMaxDirty = 12
)

func gitLayer(root string) string {
	if info, err := os.Stat(filepath.Join(root, ".git")); err != nil || !info.IsDir() {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitLayerTimeout)
	defer cancel()
	run := func(args ...string) string {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
		proc.HideConsole(cmd)
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	branch := run("rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Git, as this session opened\n\n")
	b.WriteString("A snapshot, not a feed — the tree may have moved since; the git tool answers live.\n")
	fmt.Fprintf(&b, "Branch: %s\n", branch)

	if status := run("status", "--porcelain"); status != "" {
		lines := strings.Split(status, "\n")
		fmt.Fprintf(&b, "Uncommitted (%d):\n", len(lines))
		for i, line := range lines {
			if i == gitLayerMaxDirty {
				fmt.Fprintf(&b, "  ... and %d more\n", len(lines)-i)
				break
			}
			b.WriteString("  " + strings.TrimRight(line, "\r") + "\n")
		}
	} else {
		b.WriteString("Working tree clean.\n")
	}
	if log := run("log", "--oneline", "-5"); log != "" {
		b.WriteString("Recent commits:\n")
		for _, line := range strings.Split(log, "\n") {
			b.WriteString("  " + strings.TrimRight(line, "\r") + "\n")
		}
	}
	return b.String()
}
