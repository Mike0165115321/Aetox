package skill

import (
	"bytes"
	"strings"
	"time"
)

const defaultToolOutputLineLimit = 220

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

func limitLines(content string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		maxLines = defaultToolOutputLineLimit
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content, false
	}

	return strings.Join(lines[:maxLines], "\n") + "\n... (truncated)", true
}
