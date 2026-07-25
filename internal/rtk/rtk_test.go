package rtk

import (
	"context"
	"testing"
)

func TestFilterForToolGitSubcommands(t *testing.T) {
	cases := map[string]string{
		"status": "git-status",
		"diff":   "git-diff",
		"show":   "git-diff",
		"log":    "git-log",
		"branch": "", // no matching filter — passthrough
	}
	for sub, want := range cases {
		args := map[string]any{"args": []any{sub}}
		if got := FilterForTool("git", args); got != want {
			t.Errorf("git %s: got %q, want %q", sub, got, want)
		}
	}
}

func TestFilterForToolShellUsesRewriteNotFilterForTool(t *testing.T) {
	// shell no longer goes through FilterForTool at all — it uses Rewrite
	// directly (see shell.go) — so any shell args must map to "".
	if got := FilterForTool("shell", map[string]any{"args": []any{"go", "test", "./..."}}); got != "" {
		t.Errorf("shell: got %q, want \"\" (shell uses Rewrite, not FilterForTool)", got)
	}
}

func TestFilterForToolUnknownToolPassesThrough(t *testing.T) {
	if got := FilterForTool("read", map[string]any{"path": "foo.go"}); got != "" {
		t.Errorf("read: got %q, want \"\"", got)
	}
}

func TestRewriteUnavailableOrEmptyIsNoop(t *testing.T) {
	if got, ok := Rewrite(context.Background(), ""); ok || got != "" {
		t.Errorf("Rewrite(\"\"): got (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestRewriteRealBinary only runs meaningfully when rtk is actually
// installed — the closest thing to an integration check for the live
// subprocess call, mirroring rtk's own OpenCode plugin (confirmed via
// `rtk init -g --opencode --dry-run`): it does nothing but call this.
func TestRewriteRealBinary(t *testing.T) {
	if !Available() {
		t.Skip("rtk not installed on PATH")
	}
	got, ok := Rewrite(context.Background(), "git status")
	if !ok {
		t.Fatal("expected rtk to have an equivalent for \"git status\"")
	}
	if got == "" || got == "git status" {
		t.Fatalf("Rewrite(\"git status\") = %q, want a real rewritten command", got)
	}
}

func TestRewriteNoEquivalentIsNoop(t *testing.T) {
	if !Available() {
		t.Skip("rtk not installed on PATH")
	}
	// A command rtk has no registry entry for — confirmed live: exits 1, no output.
	got, ok := Rewrite(context.Background(), "aetox-definitely-not-a-real-command --xyz")
	if ok || got != "" {
		t.Errorf("Rewrite with no equivalent: got (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestFilterUnknownFilterNameIsNoop(t *testing.T) {
	got, ok := Filter(context.Background(), "not-a-real-filter", "some output")
	if ok || got != "some output" {
		t.Errorf("Filter with bad name: got (%q, %v), want (\"some output\", false)", got, ok)
	}
}

func TestFilterEmptyContentIsNoop(t *testing.T) {
	got, ok := Filter(context.Background(), "git-status", "")
	if ok || got != "" {
		t.Errorf("Filter with empty content: got (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestFilterRealBinary only runs meaningfully when rtk is actually installed —
// it's the closest thing to an integration check for the live subprocess call.
func TestFilterRealBinary(t *testing.T) {
	if !Available() {
		t.Skip("rtk not installed on PATH")
	}
	got, ok := Filter(context.Background(), "git-status", "On branch main\nnothing to commit, working tree clean\n")
	if !ok {
		t.Fatal("expected rtk to successfully filter well-formed git-status input")
	}
	if got == "" {
		t.Fatal("filtered output was empty")
	}
}

func TestStripBanner(t *testing.T) {
	const banner = "[rtk] ⚠ No hook installed — run `rtk init -g` for automatic token savings"
	cases := []struct{ name, in, want string }{
		{"banner then content", banner + "\nGo build: Success", "Go build: Success"},
		{"banner only", banner, ""},
		{"no banner is untouched", "M  internal/rtk/rtk.go\n?? new.go", "M  internal/rtk/rtk.go\n?? new.go"},
		{"indented banner still goes", "  " + banner + "\nok", "ok"},
		{"a line merely mentioning rtk stays", "rtk saved 20%", "rtk saved 20%"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := StripBanner(c.in); got != c.want {
			t.Errorf("%s: StripBanner(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// An allowlist, so an unknown command runs raw — the worst case being "no
// better than before" rather than the silent bloat measured on go build/test.
func TestWorthRewriting(t *testing.T) {
	yes := []string{"git log", "git diff", "git status", "git show HEAD", "GIT LOG --oneline"}
	no := []string{
		"go build ./...", "go test ./...", "go vet ./...",
		"npm install -g typescript", "npm ci",
		"git", "", "git commit -m x", "ls",
	}
	for _, c := range yes {
		if !worthRewriting(c) {
			t.Errorf("worthRewriting(%q) = false, want true", c)
		}
	}
	for _, c := range no {
		if worthRewriting(c) {
			t.Errorf("worthRewriting(%q) = true, want false", c)
		}
	}
}
