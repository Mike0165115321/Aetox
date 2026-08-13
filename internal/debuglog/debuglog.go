package debuglog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	writer io.Writer
	indent int
)

func Init(baseDir string) {
	if writer != nil {
		return
	}
	dir := filepath.Join(baseDir, "logs")
	os.MkdirAll(dir, 0755)

	name := "aetox-" + time.Now().Format("20060102-150405") + ".log"
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	writer = f
	timestamp("=== AETOX DEBUG LOG ===")
	timestamp("file: " + path)
}

func Enable(filepath string) error {
	if writer != nil {
		_ = Disable()
	}
	f, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	writer = f
	timestamp("=== AETOX DEBUG LOG ===")
	timestamp("file: " + filepath)
	return nil
}

func Disable() error {
	if writer == nil {
		return nil
	}
	var err error
	if closer, ok := writer.(io.Closer); ok {
		err = closer.Close()
	}
	writer = nil
	return err
}

// Msg records a line even with no log file open — the file is one sink, the
// in-memory ring behind Recent() is the other, and the ring must work on the
// production installs where nobody enabled logging (they are where the bug
// reports come from).
func Msg(format string, args ...any) {
	prefix := strings.Repeat("  ", indent)
	line := prefix + fmt.Sprintf(format, args...)
	timestamp(line)
}

// Block times a scoped phase: it logs entry, and on the returned func's call
// logs exit with the elapsed wall time (e.g. "--- bootstrapFromConfig (812.4ms) ---").
// Usage: defer debuglog.Block("phase")(). The printed duration turns the log
// into a profiler you can read top-to-bottom — no subtracting timestamps by hand.
func Block(title string) func() {
	if writer == nil {
		return func() {}
	}
	start := time.Now()
	timestamp(strings.Repeat("  ", indent) + "=== " + title + " ===")
	indent++
	return func() {
		if indent > 0 {
			indent--
		}
		elapsed := time.Since(start)
		timestamp(fmt.Sprintf("%s--- %s (%.1fms) ---", strings.Repeat("  ", indent), title, float64(elapsed.Microseconds())/1000))
	}
}

func Info(label, value string) {
	Msg("%-20s = %s", label+":", value)
}

// Secrets registered with Redact, scrubbed out of every line on the way to the
// file. A set rather than a slice so registering the same key on each config
// reload does not grow the list forever.
var (
	secretsMu sync.RWMutex
	secrets   = map[string]bool{}
)

// Redact registers a value that must never reach the log, whatever a caller
// does with it. internal/config calls it for every API key it reads or writes.
//
// The scrub happens in timestamp() — the single funnel every line goes through
// — rather than at each call site, because a rule enforced at call sites is a
// rule the next call site can be written without. A debug log is pasted into
// bug reports and screenshots by definition, so "we remembered everywhere" is
// not a standard that survives contact with a new logging line.
//
// Short values are ignored. A four-character secret cannot be told apart from
// ordinary text, and replacing every occurrence of it would shred the log into
// something nobody can read — losing the log entirely is its own failure.
func Redact(secret string) {
	secret = strings.TrimSpace(secret)
	if len(secret) < 8 {
		return
	}
	secretsMu.Lock()
	secrets[secret] = true
	secretsMu.Unlock()
}

// Scrub returns text with every registered secret replaced. Exported because
// the debug log is not the only place a key can come to rest: the shell audit
// log records the command line, and tool_runs records arguments and output — a
// `curl -H "Authorization: Bearer …"` lands in both.
//
// One registry, several sinks. A second list of secrets kept by whoever writes
// to the database would be a list that drifts, and the drift would be silent
// until the day someone reads the file it failed on.
//
// This package rather than a new one because Redact already lives here and a
// secret registry with two homes is the thing it exists to prevent. Nothing in
// here logs, so the dependency is one way and cheap.
func Scrub(text string) string { return scrub(text) }

// scrub replaces every registered secret with a marker that still says what was
// there, so a reader can tell "the key was present and correct" from "the key
// was missing" — which is the one thing a credentials bug needs the log for.
func scrub(msg string) string {
	secretsMu.RLock()
	defer secretsMu.RUnlock()
	for secret := range secrets {
		if strings.Contains(msg, secret) {
			msg = strings.ReplaceAll(msg, secret, "[redacted]")
		}
	}
	return msg
}

func timestamp(msg string) {
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05.000"), scrub(msg))
	remember(line)
	if writer != nil {
		fmt.Fprintln(writer, line)
	}
}

// ---------------------------------------------------------------------------
// The last lines, kept in memory whether or not a log file is open.
//
// For the About page's "ส่งปัญหาให้นักพัฒนา": a bug report with no evidence is
// a conversation, not a report, and the evidence was always here — this
// package sees every internal complaint the app makes about itself. Most
// installs run with file logging off, so the file cannot be the source; the
// ring can, at the cost of a few kilobytes that exist anyway the moment
// logging is on. Lines pass through scrub() first, same as the file — the
// whole point of the single funnel is that a new sink cannot forget the rule.
// ---------------------------------------------------------------------------

const recentCap = 200

var (
	recentMu sync.Mutex
	recent   []string // oldest first
)

func remember(line string) {
	recentMu.Lock()
	recent = append(recent, line)
	if len(recent) > recentCap {
		recent = recent[len(recent)-recentCap:]
	}
	recentMu.Unlock()
}

// Recent returns up to max of the newest remembered lines, oldest first.
func Recent(max int) []string {
	recentMu.Lock()
	defer recentMu.Unlock()
	if max <= 0 || max > len(recent) {
		max = len(recent)
	}
	out := make([]string, max)
	copy(out, recent[len(recent)-max:])
	return out
}
