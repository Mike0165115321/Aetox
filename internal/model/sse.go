package model

import (
	"bufio"
	"io"
	"strings"
)

// sseMaxEventBytes bounds one line of a streamed response.
//
// bufio.Scanner's default is 64 KB per line, and every wire format Aetox
// speaks has a case that goes past it: the Responses API puts the entire
// output (every message, every tool call and its arguments) on the single
// `response.completed` line, Ollama sends a tool call's whole arguments object
// in one message, and any gateway that buffers upstream forwards a finished
// turn as one delta. Past the bound the stream dies with "token too long"
// *after* the model has already done the work, so the answer is paid for and
// thrown away.
const sseMaxEventBytes = 8 << 20 // 8 MiB

// newStreamScanner reads a streamed body line by line with that bound in
// place. Every provider's stream goes through it; none may take the 64 KB
// default.
func newStreamScanner(body io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), sseMaxEventBytes)
	return scanner
}

// scanSSE walks an event stream and hands each `data:` payload to onData.
//
// It returns when the stream ends, when onData reports stop, or on the first
// error from either side. Comment lines, `event:` lines and the blank lines
// between events are skipped: every format Aetox talks to puts the event type
// inside the JSON as well, so the framing carries no information the payload
// does not already have.
func scanSSE(body io.Reader, onData func(data string) (stop bool, err error)) error {
	scanner := newStreamScanner(body)

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
