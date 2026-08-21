package skill

import (
	"fmt"
	"strings"
	"testing"
)

func TestUnifiedDiffNoChange(t *testing.T) {
	if got := UnifiedDiff("a\nb\nc\n", "a\nb\nc\n"); got != "" {
		t.Errorf("UnifiedDiff of identical text = %q, want empty", got)
	}
}

func TestUnifiedDiffOneLineInTheMiddle(t *testing.T) {
	before := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\n"
	after := "one\ntwo\nthree\nFOUR\nfive\nsix\nseven\neight\n"

	got := UnifiedDiff(before, after)
	want := strings.Join([]string{
		"@@ -1,7 +1,7 @@",
		" one",
		" two",
		" three",
		"-four",
		"+FOUR",
		" five",
		" six",
		" seven",
	}, "\n")
	if got != want {
		t.Errorf("UnifiedDiff =\n%s\n\nwant\n%s", got, want)
	}
}

// The line numbers are the whole point of showing a diff at all: "it changed
// four lines" is what the counts already said.
func TestUnifiedDiffNumbersTheHunkFromTheRealLine(t *testing.T) {
	var before strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&before, "line %d\n", i)
	}
	after := strings.Replace(before.String(), "line 20\n", "line twenty\n", 1)

	got := UnifiedDiff(before.String(), after)
	if !strings.HasPrefix(got, "@@ -17,7 +17,7 @@\n") {
		t.Errorf("hunk header = %q, want it to start at line 17", strings.SplitN(got, "\n", 2)[0])
	}
	if !strings.Contains(got, "\n-line 20\n+line twenty\n") {
		t.Errorf("UnifiedDiff =\n%s\nwant the swap of line 20", got)
	}
}

// Two edits far apart are two hunks, not one hunk with thirty untouched lines
// inside it.
func TestUnifiedDiffSplitsDistantChanges(t *testing.T) {
	var before strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&before, "line %d\n", i)
	}
	after := strings.Replace(before.String(), "line 5\n", "FIVE\n", 1)
	after = strings.Replace(after, "line 50\n", "FIFTY\n", 1)

	got := UnifiedDiff(before.String(), after)
	if n := strings.Count(got, "@@ -"); n != 2 {
		t.Errorf("hunks = %d, want 2:\n%s", n, got)
	}
	if strings.Contains(got, " line 30") {
		t.Errorf("untouched middle of the file leaked into the diff:\n%s", got)
	}
}

// Nearby changes share a hunk, on git's own rule — otherwise a renamed symbol
// draws a header per line.
func TestUnifiedDiffMergesNearbyChanges(t *testing.T) {
	before := "a\nb\nc\nd\ne\nf\ng\nh\n"
	after := "a\nB\nc\nd\ne\nF\ng\nh\n"

	got := UnifiedDiff(before, after)
	if n := strings.Count(got, "@@ -"); n != 1 {
		t.Errorf("hunks = %d, want 1:\n%s", n, got)
	}
}

func TestUnifiedDiffNewFileIsAllAdded(t *testing.T) {
	got := UnifiedDiff("", "hello\nworld\n")
	want := "@@ -1,0 +1,2 @@\n+hello\n+world"
	if got != want {
		t.Errorf("UnifiedDiff of a new file =\n%s\nwant\n%s", got, want)
	}
}

// CRLF is the reference platform's default checkout. A diff that renders the
// `\r` is reporting the file's convention, not the change.
func TestUnifiedDiffIgnoresLineEndingStyle(t *testing.T) {
	if got := UnifiedDiff("a\r\nb\r\n", "a\nb\n"); got != "" {
		t.Errorf("CRLF vs LF of the same text = %q, want empty", got)
	}
	got := UnifiedDiff("a\r\nb\r\n", "a\r\nB\r\n")
	if strings.Contains(got, "\r") {
		t.Errorf("diff carries carriage returns:\n%q", got)
	}
}

// A cut that will not say it is a cut reads as the whole change.
func TestUnifiedDiffMarksWhatItDropped(t *testing.T) {
	var after strings.Builder
	for i := 0; i < diffMaxLines+50; i++ {
		fmt.Fprintf(&after, "line %d\n", i)
	}

	got := UnifiedDiff("", after.String())
	lines := strings.Split(got, "\n")
	if len(lines) != diffMaxLines+1 {
		t.Fatalf("diff lines = %d, want %d plus one marker", len(lines), diffMaxLines)
	}
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "~") {
		t.Errorf("last line = %q, want the ~N marker", last)
	}
	if last != "~51" { // 400 kept of 401 rendered (one header + 450 adds)
		t.Errorf("marker = %q, want ~51", last)
	}
}

func TestFileDiffNamesItsFile(t *testing.T) {
	got := FileDiff("cmd/main.go", "a\n", "b\n")
	if !strings.HasPrefix(got, "+++ cmd/main.go\n@@ ") {
		t.Errorf("FileDiff = %q, want it headed by the path", got)
	}
	if FileDiff("cmd/main.go", "a\n", "a\n") != "" {
		t.Error("FileDiff of an unchanged file should be empty, not a bare header")
	}
}

func TestJoinDiffsDropsEmptyOnes(t *testing.T) {
	got := JoinDiffs([]string{"", FileDiff("a.go", "x\n", "y\n"), ""})
	if strings.Count(got, "+++ ") != 1 {
		t.Errorf("JoinDiffs = %q, want exactly the one real diff", got)
	}
	if JoinDiffs([]string{"", ""}) != "" {
		t.Error("JoinDiffs of nothing should be empty")
	}
}

// The fallback exists so a huge rewrite still reports its content instead of
// spending a hundred megabytes proving which lines paired up.
func TestUnifiedDiffFallsBackToBlockOnHugeRegions(t *testing.T) {
	var before, after strings.Builder
	for i := 0; i < 1200; i++ {
		fmt.Fprintf(&before, "old %d\n", i)
		fmt.Fprintf(&after, "new %d\n", i)
	}
	got := UnifiedDiff(before.String(), after.String())
	if !strings.HasPrefix(got, "@@ -1,") {
		t.Errorf("diff = %q, want a hunk from line 1", strings.SplitN(got, "\n", 2)[0])
	}
	if !strings.HasSuffix(got, fmt.Sprintf("~%d", 1200+1200+1-diffMaxLines)) {
		t.Errorf("block fallback lost its truncation marker:\n%s", got[len(got)-40:])
	}
}
