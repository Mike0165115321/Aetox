package skill

import (
	"fmt"
	"strconv"
	"strings"
)

// Exact-match editing against files the model reads but cannot fully see.
//
// `read` hands back what is on disk, byte for byte. On Windows — the reference
// platform, and with `core.autocrlf=true` the default checkout there — every
// line in that text ends `\r\n`, and a `\r` is invisible in a way a tab or a
// space is not: nothing in the rendering of the file distinguishes it. So a
// model composes its `find` text by joining the lines it saw with `\n`, and
// `strings.Count` reports zero.
//
// The old answer was "re-read the file and match the text exactly", which asks
// the model to observe the one thing it cannot observe. It re-reads, sees the
// same characters, composes the same `\n`, and fails identically — the same
// error, repeatedly, for a reason that is not the model's mistake. Single-line
// edits are unaffected, which is why this reads as flakiness rather than as the
// structural failure it is.
//
// Two rules follow, and neither is a leniency:
//
//   - **Match on what the model can see.** A line ending is the file's business,
//     not the caller's. If the `find` text identifies exactly one place in the file
//     once line endings are set aside, that is the place it meant.
//   - **Keep the file's own line endings.** Replacement text arrives with
//     whatever the model typed; writing it verbatim into a CRLF file leaves a
//     file that is half one convention and half the other, which is a worse bug
//     than the one being fixed and shows up in everybody's diff.

// dominantEOL reports the line ending the file is written in. A file is treated
// as CRLF when at least half its newlines are, so a mostly-CRLF file with a few
// stray LF lines still round-trips as CRLF instead of being converted line by
// line by successive edits.
func dominantEOL(content string) string {
	crlf := strings.Count(content, "\r\n")
	if crlf > 0 && crlf*2 >= strings.Count(content, "\n") {
		return "\r\n"
	}
	return "\n"
}

// toLF normalises CRLF to LF. Lone `\r` is left alone on purpose: it is a
// legitimate character inside a string literal, and classic-Mac line endings
// have not been a thing for twenty years — guessing there would corrupt real
// content to serve a case that does not arrive.
func toLF(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// newlinesLike rewrites s's line endings to whatever content uses.
func newlinesLike(content, s string) string {
	if eol := dominantEOL(content); eol == "\r\n" {
		return strings.ReplaceAll(toLF(s), "\n", "\r\n")
	}
	return toLF(s)
}

// resolveFindText returns the text that actually appears in content for the
// find text the caller asked for, and how many times it appears.
//
// The literal string is tried first and wins whenever it matches, so a caller
// that did send the file's own line endings is never second-guessed. Only when
// that finds nothing are the two conversions tried — and the count that comes
// back is a count of the *converted* text, so the "matches N times" guard
// upstream keeps meaning what it meant.
func resolveFindText(content, findText string) (string, int) {
	if n := strings.Count(content, findText); n > 0 {
		return findText, n
	}
	lf := toLF(findText)
	for _, candidate := range []string{lf, strings.ReplaceAll(lf, "\n", "\r\n")} {
		if candidate == findText {
			continue
		}
		if n := strings.Count(content, candidate); n > 0 {
			return candidate, n
		}
	}
	return findText, 0
}

// whyNoMatch says what is wrong with a find text that matched nothing, in
// place of telling the caller to go and read the file again.
//
// Re-reading is the most expensive recovery available and the least likely to
// work: the failure is almost never "I misremembered the text", it is one
// invisible difference — a line-number prefix carried over from `read`, an
// indentation that is tabs where the caller wrote spaces, a line that has moved
// on since. All three are answerable from the file, which this function is
// holding, so answering here turns a whole re-read into a one-line correction.
//
// Deliberately short. Every character of it is a tool result the model pays for
// on the turn it failed, and a diagnosis nobody can act on is worse than none.
func whyNoMatch(content, findText string) string {
	lines := strings.Split(toLF(findText), "\n")
	first := ""
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = l
			break
		}
	}
	if first == "" {
		return "find text is only whitespace"
	}
	if numbered, stripped := stripReadPrefix(first); numbered {
		return fmt.Sprintf("it still carries read's line-number prefix — strip %q down to %q; the number and tab are not in the file",
			truncateForError(first), truncateForError(stripped))
	}

	trimmed := strings.TrimSpace(first)
	for i, line := range strings.Split(toLF(content), "\n") {
		if strings.TrimSpace(line) != trimmed {
			continue
		}
		if line == first {
			return fmt.Sprintf("its first line matches at line %d, so a later line of the find text is what differs — compare from line %d down",
				i+1, i+1)
		}
		return fmt.Sprintf("line %d holds the same text with different leading whitespace: the file has %q, find has %q",
			i+1, truncateForError(line), truncateForError(first))
	}
	return "no line of the find text appears in this file — check the path, or that the text has not already been changed"
}

// stripReadPrefix reports whether s starts with the `%6d\t` prefix read puts on
// every line, and returns s without it.
func stripReadPrefix(s string) (bool, string) {
	tab := strings.IndexByte(s, '\t')
	if tab <= 0 {
		return false, s
	}
	if _, err := strconv.Atoi(strings.TrimSpace(s[:tab])); err != nil {
		return false, s
	}
	return true, s[tab+1:]
}

// truncateForError keeps a quoted line short enough to read in an error.
func truncateForError(s string) string {
	const max = 60
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
