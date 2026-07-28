package model

import (
	"bufio"
	"io"
	"strings"
)

// sseMaxEventBytes bounds one SSE `data:` payload.
//
// bufio.Scanner's default is 64 KB per line, which is fine for the two older
// wire formats — both stream text in small deltas. It is not fine for the
// Responses API, where the final `response.completed` event carries the entire
// output (every message, every tool call and its arguments) on a single line:
// a turn that wrote a large file overruns 64 KB and the stream dies with
// "token too long" *after* the model has already done the work.
const sseMaxEventBytes = 8 << 20 // 8 MiB

// scanSSE walks an event stream and hands each `data:` payload to onData.
//
// It returns when the stream ends, when onData reports stop, or on the first
// error from either side. Comment lines, `event:` lines and the blank lines
// between events are skipped: every format Aetox talks to puts the event type
// inside the JSON as well, so the framing carries no information the payload
// does not already have.
func scanSSE(body io.Reader, onData func(data string) (stop bool, err error)) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), sseMaxEventBytes)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		stop, err := onData(data)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return scanner.Err()
}
