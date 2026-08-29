package version

import (
	"strings"
	"testing"
)

// codeLines drops comment lines. Both files below explain the old, wrong flag
// form by name in a comment, and a test that cannot tell an explanation from an
// instruction would fail on the very sentence that documents the fix.
func codeLines(body string) string {
	var b strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// The gosec report has to actually be gosec. golangci-lint v2's --enable
// APPENDS to the enable list in .golangci.yml, so `--default=none
// --enable=gosec` re-ran the thirteen gating linters instead of replacing them,
// and verify.sh's one-line summary printed their last bullet — "* wastedassign:
// 2" standing where a gosec count belonged. The report ran on every push for
// eleven days and never said what it found (docs/DECISIONS.md §141.5).
//
// It lives beside the release-checklist tests because it asks the same kind of
// question — do the repo's own files still agree with each other — and reuses
// their repoFile helper rather than growing a second one.
func TestGosecReportIsGosecOnly(t *testing.T) {
	for _, f := range []string{"verify.sh", ".github/workflows/ci.yml"} {
		body := codeLines(repoFile(t, f))
		if !strings.Contains(body, "--enable-only=gosec") {
			t.Errorf("%s: no --enable-only=gosec — the gosec report must replace the enable list, not append to it", f)
		}
		if strings.Contains(body, "--enable=gosec") {
			t.Errorf("%s: --enable=gosec is back; v2 appends it to .golangci.yml's list, so what runs is every linter and what the summary reports is not gosec", f)
		}
	}
}
