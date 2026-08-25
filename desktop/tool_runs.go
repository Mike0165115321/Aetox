package main

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// Caps on what one row may carry. Everything a tool returns is *measured* in
// full (the byte counts and the hash are over the whole thing) but only the
// head is stored, because the store lives on the user's own disk and some tools
// return a whole file: one `read` of a 2 MB source file or a `web_fetch` of a
// long page would put more into aetox.db in a single row than the entire
// application weighs.
//
// The head is what the later summarizing pass reads — "OCR came back with
// under 3 lines", "this fetch returned a login page" are both decidable from
// the first few KB — so the truncation costs the learning loop nothing it was
// going to use, and buys a database that stays a chat history rather than
// becoming a copy of every file the agent ever opened.
const (
	maxStoredArgs   = 4 << 10
	maxStoredOutput = 8 << 10
)

// recordToolRun persists one finished tool call. Called from the turn executor
// (and, for a delegate's calls, through the sub-agent relay that stamps who
// ran it), on the turn's own goroutine.
//
// Failures only log, for the same reason recordTokenUsage's do: a chat turn
// must not break because the history could not be written.
func (a *App) recordToolRun(conv *conversation, run turn.ToolRun) {
	db, err := a.database()
	if err != nil {
		debuglog.Msg("tool_runs: db unavailable: %v", err)
		return
	}
	// Scrubbed before storing, not before displaying. This row outlives the
	// session — it is searchable, it is in every backup of the database, and
	// session_search reads it back into a later conversation. A shell command
	// carrying `-H "Authorization: Bearer …"`, or a tool that echoed a key into
	// its output, would otherwise be persisted in the clear and then handed to
	// a model in some future turn.
	//
	// Before clamping, so the replacement cannot be cut in half and leave the
	// tail of a real key at the boundary.
	args, argsBytes := clampStored(debuglog.Scrub(run.Args), maxStoredArgs)
	output, outputBytes := clampStored(debuglog.Scrub(run.Output), maxStoredOutput)
	// The hash is over the whole output, never the stored head: it is what
	// tells two runs that truncate to the same prefix apart, which is the one
	// question a stored prefix cannot answer on its own.
	var outputHash string
	if outputBytes > len(output) {
		sum := sha256.Sum256([]byte(run.Output))
		outputHash = hex.EncodeToString(sum[:])
	}
	_, err = db.Exec(
		`INSERT INTO tool_runs(
			session_id, ref, parent_ref, agent, tool,
			args, args_bytes, output, output_bytes, output_sha256,
			ok, error, error_kind, duration_ms, time)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		// The conversation this engine was built for — a tool run belongs to the
		// chat that made it, same as its messages, and the callback carries that
		// rather than asking anybody afterwards.
		conv.id, run.Ref, run.Parent, run.Agent, run.Name,
		args, argsBytes, output, outputBytes, outputHash,
		boolToInt(run.OK), run.Error, run.ErrorKind, run.Duration.Milliseconds(),
		time.Now().Format(time.RFC3339),
	)
	if err != nil {
		debuglog.Msg("tool_runs: insert failed: %v", err)
	}
}

// clampStored returns the head of s that may be stored, plus the true byte
// length of s. Cutting is on a rune boundary: half a Thai character stored as
// invalid UTF-8 would be worse than one character less.
func clampStored(s string, max int) (string, int) {
	total := len(s)
	if total <= max {
		return s, total
	}
	cut := max
	for cut > 0 && !utf8RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], total
}

// utf8RuneStart reports whether b begins a rune (i.e. is not a continuation
// byte). Continuation bytes are 10xxxxxx.
func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
