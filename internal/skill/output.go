package skill

import (
	"strings"
	"time"
)

const defaultToolOutputLineLimit = 220

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
