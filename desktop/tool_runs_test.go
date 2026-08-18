package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

type storedRun struct {
	sessionID   string
	ref         string
	parentRef   string
	agent       string
	tool        string
	args        string
	argsBytes   int
	output      string
	outputBytes int
	outputHash  string
	ok          int
	errText     string
	durationMs  int64
}

func readRuns(t *testing.T, a *App) []storedRun {
	t.Helper()
	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	rows, err := db.Query(`SELECT session_id, ref, parent_ref, agent, tool, args, args_bytes,
		output, output_bytes, output_sha256, ok, error, duration_ms FROM tool_runs ORDER BY id`)
	if err != nil {
		t.Fatalf("query tool_runs: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []storedRun
	for rows.Next() {
		var r storedRun
		if err := rows.Scan(&r.sessionID, &r.ref, &r.parentRef, &r.agent, &r.tool, &r.args,
			&r.argsBytes, &r.output, &r.outputBytes, &r.outputHash, &r.ok, &r.errText, &r.durationMs); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func newRunApp(t *testing.T) *App {
	t.Helper()
	a := seed(&App{cfg: config.Config{}, dbDir: t.TempDir()}, &conversation{id: "20260801-120000.000"})
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	return a
}

// The whole point of the record: the arguments the model sent and the output it
// got back, neither of which ToolEvent carries.
func TestRecordToolRunKeepsArgumentsAndOutput(t *testing.T) {
	a := newRunApp(t)
	a.recordToolRun(a.cur(), turn.ToolRun{
		Ref:      "call_1",
		Name:     "image_ocr",
		Args:     `{"path":"slip.png"}`,
		Output:   "ยอดเงิน 1,250 บาท",
		OK:       true,
		Duration: 1500 * time.Millisecond,
	})

	runs := readRuns(t, a)
	if len(runs) != 1 {
		t.Fatalf("want 1 row, got %d", len(runs))
	}
	got := runs[0]
	if got.tool != "image_ocr" || got.args != `{"path":"slip.png"}` {
		t.Fatalf("arguments not stored verbatim: %+v", got)
	}
	if got.output != "ยอดเงิน 1,250 บาท" {
		t.Fatalf("output not stored: %q", got.output)
	}
	if got.ok != 1 || got.durationMs != 1500 {
		t.Fatalf("outcome/timing wrong: ok=%d duration=%d", got.ok, got.durationMs)
	}
	if got.sessionID != a.cur().id {
		t.Fatalf("row not attributed to the session: %q", got.sessionID)
	}
	// Nothing was truncated, so there is no hash to disambiguate.
	if got.outputHash != "" {
		t.Fatalf("untruncated output should carry no hash, got %q", got.outputHash)
	}
}

// A sub-agent's calls are the ones the command-history panel deliberately
// hides. The store must keep them anyway, with who ran them — otherwise "which
// sub-agent is bad at what" can never be asked.
func TestRecordToolRunKeepsDelegateCallsWithAttribution(t *testing.T) {
	a := newRunApp(t)
	a.recordToolRun(a.cur(), turn.ToolRun{Ref: "call_1", Name: "task", OK: true})
	a.recordToolRun(a.cur(), turn.ToolRun{
		Ref: "call_2", Parent: "call_1", Agent: "explore",
		Name: "grep", Args: `{"pattern":"TODO"}`, Output: "3 matches", OK: true,
	})

	runs := readRuns(t, a)
	if len(runs) != 2 {
		t.Fatalf("a delegate's call was dropped: got %d rows, want 2", len(runs))
	}
	child := runs[1]
	if child.parentRef != "call_1" || child.agent != "explore" {
		t.Fatalf("delegate attribution lost: parent=%q agent=%q", child.parentRef, child.agent)
	}
	if runs[0].parentRef != "" || runs[0].agent != "" {
		t.Fatalf("main agent's own call should have no parent/agent: %+v", runs[0])
	}
}

// Big outputs are measured whole and stored short, so aetox.db stays a history
// rather than a copy of every file the agent read.
func TestRecordToolRunTruncatesLargeOutputButKeepsTrueSize(t *testing.T) {
	a := newRunApp(t)
	huge := strings.Repeat("x", maxStoredOutput*3)
	a.recordToolRun(a.cur(), turn.ToolRun{Ref: "call_1", Name: "read", Output: huge, OK: true})

	got := readRuns(t, a)[0]
	if len(got.output) > maxStoredOutput {
		t.Fatalf("stored %d bytes, cap is %d", len(got.output), maxStoredOutput)
	}
	if got.outputBytes != len(huge) {
		t.Fatalf("true size not recorded: got %d, want %d", got.outputBytes, len(huge))
	}
	sum := sha256.Sum256([]byte(huge))
	if got.outputHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash must cover the whole output, got %q", got.outputHash)
	}
}

// Cutting mid-character would store invalid UTF-8 — for Thai that is most
// cuts, since every character is three bytes.
func TestRecordToolRunTruncatesOnRuneBoundary(t *testing.T) {
	a := newRunApp(t)
	thai := strings.Repeat("ก", maxStoredOutput)
	a.recordToolRun(a.cur(), turn.ToolRun{Ref: "call_1", Name: "read", Output: thai, OK: true})

	got := readRuns(t, a)[0]
	if !utf8.ValidString(got.output) {
		t.Fatal("truncation produced invalid UTF-8")
	}
	if got.output == "" {
		t.Fatal("truncation ate the whole output")
	}
}

// A failed call is the most interesting row a learning pass reads, so the
// reason has to survive.
func TestRecordToolRunKeepsFailureReason(t *testing.T) {
	a := newRunApp(t)
	a.recordToolRun(a.cur(), turn.ToolRun{
		Ref: "call_1", Name: "web_fetch", Args: `{"url":"https://example.invalid"}`,
		OK: false, Error: "dial tcp: no such host",
	})

	got := readRuns(t, a)[0]
	if got.ok != 0 || got.errText != "dial tcp: no such host" {
		t.Fatalf("failure not recorded: ok=%d error=%q", got.ok, got.errText)
	}
}

// Deleting a conversation has to take its tool runs with it: those rows hold
// file contents and page text, so leaving them is the opposite of what the
// delete button promises.
func TestDeleteSessionRemovesItsToolRuns(t *testing.T) {
	a := newRunApp(t)
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "อ่านสลิปให้หน่อย", Time: time.Now().Format(time.RFC3339)},
		SessionMessage{Role: "agent", Text: "ได้ครับ", Time: time.Now().Format(time.RFC3339)},
	)
	a.recordToolRun(a.cur(), turn.ToolRun{Ref: "call_1", Name: "image_ocr", Output: "ยอด 500", OK: true})
	if len(readRuns(t, a)) != 1 {
		t.Fatal("setup: expected one recorded run")
	}

	if err := a.DeleteSession(a.cur().id); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if runs := readRuns(t, a); len(runs) != 0 {
		t.Fatalf("tool runs outlived their session: %+v", runs)
	}
}
