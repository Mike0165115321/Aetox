package skill

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const defaultToolOutputLineLimit = 220

// shellOutputLineLimit is what a command may say, and it is larger than the
// list-shaped tools' ceiling above because a command is not a list.
//
// 220 was never actually the operative limit until 2026-08-22. The turn
// executor cut every tool result to 4,096 characters underneath it — roughly 60
// lines — so nothing above that reached the model whatever this number said.
// "220 has been fine for months" was therefore not evidence of anything; the
// day it became the real ceiling was the day it first had to be justified.
//
// 1,000 puts it between the two implementations worth comparing against:
// OpenCode caps at 2,000 lines, Claude Code at 30,000 characters (roughly 400
// to 700 lines). It is also comfortably inside the executor's own backstop,
// which is what keeps a runaway command from mattering — this number is about
// what is worth reading, not about what fits.
const shellOutputLineLimit = 1000

// toolOutputByteCap bounds what one external command may buffer in RAM.
// limitLines only trims after the process exits, so an unbounded buffer lets
// a runaway producer grow to gigabytes for the full tool timeout and take the
// desktop app down with it — `yes` or a looping log tail under `shell`, or
// `git log`/`git diff`/`git show` on a large repo or a committed binary.
const toolOutputByteCap = 1 << 20 // 1 MiB

// cappedWriter keeps the first toolOutputByteCap bytes and drops the rest —
// the head is what the model needs and limitLines trims it further anyway.
// No mutex: os/exec reuses a single pipe and copy goroutine when Stdout and
// Stderr hold the same interface value, which is how both callers wire it.
type cappedWriter struct {
	buf     bytes.Buffer
	dropped bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	room := toolOutputByteCap - w.buf.Len()
	if room <= 0 {
		w.dropped = true
		return len(p), nil
	}
	if len(p) > room {
		w.buf.Write(p[:room])
		w.dropped = true
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
}

// filterLines keeps the lines of s that match pattern. Shared by shell_output;
// a Go regexp because that is what grep already documents to the model, so one
// syntax covers both.
func filterLines(s, pattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("filter is not a valid regular expression: %w", err)
	}
	kept := make([]string, 0, 16)
	for _, line := range strings.Split(s, "\n") {
		if re.MatchString(line) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n"), nil
}

func newToolOutput(name, command, content string, start time.Time, truncated bool, execErr error) Output {
	if content == "" {
		content = "(no output)"
	}

	success := execErr == nil
	stderr := ""
	if !success {
		stderr = execErr.Error()
	}

	return Output{
		Name:       name,
		Content:    content,
		RawOutput:  content,
		Command:    command,
		Stderr:     stderr,
		Success:    success,
		Truncated:  truncated,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// limitLines is the line ceiling nineteen tools share — shell, grep, glob,
// list, git, diagnostics, symbol, the OCR and transcribe pair, and the rest.
//
// It says how many lines it kept and how many there were, and that is the whole
// change made on 2026-08-22. "(truncated)" on its own cannot tell a result that
// lost its last line from one that lost two thirds of itself, and a reader who
// cannot tell has no way to judge whether the answer in front of them is the
// answer. Measured that day: a 600-line command came back as 220 lines and the
// word "truncated", and nothing in it said 380 were gone.
//
// It is the same rule the turn executor's backstop learned the same morning,
// arrived at from the other end: a cap that does not report its own size is a
// cap that gets read as a complete result.
//
// No advice about how to get the rest, deliberately. Nineteen callers have
// nineteen different answers — `read` hands back an offset, `grep` wants a
// narrower pattern, a foreground `shell` has genuinely lost the rest — and one
// sentence that fits all of them would be true of none.
func limitLines(content string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		maxLines = defaultToolOutputLineLimit
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content, false
	}

	return strings.Join(lines[:maxLines], "\n") +
		fmt.Sprintf("\n... (truncated — showed the first %d of %d lines)", maxLines, len(lines)), true
}

// limitLinesKeepingEnds is limitLines for output whose answer is at the bottom.
//
// Cutting the tail is not a neutral choice — it is a bias aimed squarely at the
// case you most need. `go test ./...` opens with a wall of `ok` lines and closes
// with FAIL and a count; a build opens with compiler chatter and closes with the
// error; almost anything run in a shell says how it went in its last few lines.
// Keeping the first N and dropping the rest throws away the verdict and keeps
// the preamble, every time, on exactly the commands that failed.
//
// Claude Code truncates from the middle for this reason (BASH_MAX_OUTPUT_LENGTH,
// "middle-truncated"), and it is the one idea of theirs worth taking outright.
//
// **An even split between head and tail, deliberately.** There is no evidence
// that either end deserves more, and a ratio like two-thirds-one-third would be
// one more number chosen once and never revisited — which is the failure this
// package spent 2026-08-22 paying off. Half and half is the split that needs no
// justification.
//
// Applied only to `shell` and `shell_output`. The other seventeen callers of
// limitLines hand back ordered material — a file read in order, a list of
// matches, a set of diagnostics — where the front is the part you want and there
// is no verdict at the end to rescue.
func limitLinesKeepingEnds(content string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		maxLines = defaultToolOutputLineLimit
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content, false
	}
	// Below three there is no middle worth naming, so fall back rather than
	// print a marker with one line on either side of it.
	if maxLines < 3 {
		return limitLines(content, maxLines)
	}

	head := maxLines / 2
	tail := maxLines - head
	dropped := len(lines) - maxLines

	return strings.Join(lines[:head], "\n") +
		fmt.Sprintf("\n... (truncated — %d lines from the middle are not shown; the last %d of %d follow)\n",
			dropped, tail, len(lines)) +
		strings.Join(lines[len(lines)-tail:], "\n"), true
}
