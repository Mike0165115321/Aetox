// Package rtk optionally shrinks a tool call's raw output before it re-enters
// the conversation, by piping it through the user's own `rtk` CLI (a
// token-optimizing output filter, unrelated to this repo — see ARCHITECTURE.md
// §13). Purely additive: if the rtk binary isn't installed, or a tool call has
// no matching filter, callers fall back to the original content untouched.
package rtk

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	availableOnce sync.Once
	isAvailable   bool
)

// Available reports whether the rtk binary is on PATH. Checked once per
// process — rtk being installed or removed mid-session isn't worth handling.
func Available() bool {
	availableOnce.Do(func() {
		_, err := exec.LookPath("rtk")
		isAvailable = err == nil
	})
	return isAvailable
}

// pipeFilters is exactly what `rtk pipe -f <name>` accepts (confirmed live by
// running `rtk pipe -f <bogus-name>`, which lists them in its own error text).
var pipeFilters = map[string]bool{
	"cargo-test": true, "pytest": true, "go-test": true, "go-build": true,
	"tsc": true, "vitest": true, "grep": true, "rg": true, "find": true,
	"fd": true, "git-log": true, "git-diff": true, "git-status": true,
	"log": true, "mypy": true, "ruff-check": true, "ruff-format": true,
	"prettier": true,
}

// FilterForTool maps an Aetox tool call to an rtk pipe filter name, or "" if
// there's no good match. Covers only `git` (via its "status/diff/log/show"
// subcommand) — Aetox's git skill already validates and parses the exact
// subcommand itself (internal/skill/git.go), so a direct name→filter mapping
// is simpler and more precise than going through Rewrite's string interface.
// `shell` uses Rewrite instead (see shell.go) — an arbitrary command string
// is exactly what Rewrite (rtk's own registry) is for, not a hand-maintained
// guess here. `read` and everything else pass through untouched (see
// ARCHITECTURE.md §13.4: rtk's file-reading filter is a different mechanism,
// `rtk read --level`, and folding it in risked dropping doc comments the
// agent needs).
func FilterForTool(toolName string, args map[string]any) string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "git":
		sub := strings.ToLower(firstArg(args))
		switch sub {
		case "status":
			return "git-status"
		case "diff", "show":
			return "git-diff"
		case "log":
			return "git-log"
		}
		return ""
	default:
		return ""
	}
}

// Rewrite asks the real `rtk rewrite <command>` for its RTK-equivalent
// command — the exact mechanism rtk's own OpenCode/Claude Code hook plugins
// use (confirmed live via `rtk init -g --opencode --dry-run`: their plugin is
// a thin wrapper that does nothing but call this). Returns ("", false) if rtk
// isn't installed or has no equivalent for this command — the caller must
// then run the original command unchanged. The underlying side effects are
// the same either way (rtk actually runs the real command and compacts its
// output); only the caller's exec.Command target changes.
//
// Success is judged by stdout content, not the process exit code: `rtk
// rewrite`'s own --help documents "exits 0 on success," but a live check
// (v0.34.3) showed a successful rewrite exiting 3 — its exit code doesn't
// reliably follow its own documented convention, while stdout does.
func Rewrite(command string) (string, bool) {
	command = strings.TrimSpace(command)
	if command == "" || !Available() {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rtk", "rewrite", command)
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()
	rewritten := strings.TrimSpace(out.String())
	if rewritten == "" || rewritten == command {
		return "", false
	}
	return rewritten, true
}

// Filter pipes content through `rtk pipe -f <filter>`. Returns the original
// content unchanged (ok=false) on any error, timeout, missing binary, or
// unknown filter — filtering must never be the reason a tool result is lost.
func Filter(filter, content string) (string, bool) {
	if filter == "" || !pipeFilters[filter] || strings.TrimSpace(content) == "" || !Available() {
		return content, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rtk", "pipe", "-f", filter)
	cmd.Stdin = strings.NewReader(content)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return content, false
	}
	filtered := strings.TrimSpace(out.String())
	if filtered == "" {
		return content, false
	}
	return filtered, true
}

func firstArg(args map[string]any) string {
	list := stringSliceArg(args)
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

// stringSliceArg reads args["args"], the shape internal/skill's git and shell
// tools both use (see internal/skill/git.go, shell.go).
func stringSliceArg(args map[string]any) []string {
	raw, ok := args["args"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
