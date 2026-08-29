// Command tokenaudit reads the local history and reports what of it was WASTE.
//
// The app already answers "what did I spend": desktop/usage.go serves the usage
// page from token_usage, per model and per day, priced where a price is known.
// This does not repeat that and must never grow into it — a second place
// answering the same question is the debt this repository keeps refusing.
//
// What it answers instead is the question the spend page structurally cannot:
// **how much of that spend bought nothing.** The two live in different tables
// and that is why they are different programs. token_usage counts tokens after
// the fact and cannot see what was inside them; tool_runs (schema v2) records
// every tool call with the sha256 of its output, which is exactly enough to
// tell a document read once from the same document read four times.
//
// Six sections, in the order they are worth acting on:
//
//   - REPEAT WASTE — byte-identical tool output sent into one session more than
//     once. A tool that returns a document already sitting in the conversation
//     is charging for it twice, and neither the model nor the usage page can
//     see it happen. Measured 2026-08-20: 10.1% of all tool output bytes.
//   - DELEGATION CONTAINMENT — how many of those bytes landed in a delegate's
//     context instead of the chat's. This is the number the whole sub-agent
//     mechanism exists to move, and nothing else reports it.
//   - CACHE HEALTH — fresh input tokens per call, by model. The raw counts are
//     on the usage page; the ranking is not, and the ranking is the finding: on
//     2026-08-20 one model carried 80% of all fresh input tokens ever from 34%
//     of the calls, purely because its cache hit 46% where another hit 95%.
//   - CACHE BREAKS — the moments cacheHealth's averages are made of: calls where
//     the cached prefix shrank against the previous call in the same session.
//     Each break is classified as far as this table can honestly see — a model
//     switch mid-session, an idle gap long enough for a provider TTL, or "the
//     prefix changed", which is everything the rest (a prompt or tool-block
//     edit) and cannot be split further without the app recording a prefix
//     hash per call, which it deliberately does not yet do.
//   - OUTPUT VOLUME — bytes per tool. What actually fills a context, as opposed
//     to what feels like it does.
//
// Read-only, always: opened with mode=ro so a run while the app is live cannot
// touch the history it is measuring. Nothing here writes, migrates or vacuums.
//
// Not built by build.ps1 on purpose. It is a tool for reading this machine's
// own history, like cmd/relsign is a tool for signing a release — neither
// belongs in what a user installs.
//
// Usage:
//
//	go run ./cmd/tokenaudit             # all time
//	go run ./cmd/tokenaudit -days 7     # the last week
//	go run ./cmd/tokenaudit -db PATH    # a copy, or another machine's history
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mikedev115/Aetox/internal/config"
	_ "modernc.org/sqlite"
)

func main() {
	days := flag.Int("days", 0, "only the last N days (0 = all time)")
	dbPath := flag.String("db", "", "path to aetox.db (default: the app's own)")
	flag.Parse()

	path := *dbPath
	if path == "" {
		root, err := config.DataRoot()
		if err != nil {
			fail("cannot find the data root: %v", err)
		}
		path = filepath.Join(root, "aetox.db")
	}
	if _, err := os.Stat(path); err != nil {
		fail("no history at %s: %v", path, err)
	}

	// mode=ro rather than a copy: SQLite in WAL mode admits readers while the
	// app writes, and a copy taken without its -wal file reads as a database
	// missing everything since the last checkpoint — which would understate
	// exactly the recent work somebody runs this to look at.
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		fail("cannot open %s: %v", path, err)
	}
	defer db.Close()

	// One window expression, built once and pasted into every query, so no
	// section can quietly measure a different period from its neighbours.
	window := "1=1"
	label := "all time"
	if *days > 0 {
		window = fmt.Sprintf("time >= date('now','-%d day')", *days)
		label = fmt.Sprintf("the last %d days", *days)
	}

	fmt.Printf("aetox token audit — %s\n%s\n\n", label, path)

	repeatWaste(db, window)
	delegationContainment(db, window)
	cacheHealth(db, window)
	cacheBreaks(db, window)
	trialReport(db, window)
	outputVolume(db, window)
}

// trialReport is the seven-day follow-up the owner asked to run after the
// aider-plan mechanics landed (30 ส.ค.): are the new tools being used, do
// they succeed, and is the number they were built to move actually moving.
// The baselines are the measured 2026-08-28 figures from
// docs/aider-study/BASELINE.md, inlined so one command answers the trial
// without a second document open.
func trialReport(db *sql.DB, window string) {
	section("TRIAL — aider-plan mechanics vs their 28 ส.ค. baseline")

	// The number repo_map exists to move: whole reads of 50KB or more.
	// Baseline 18 per 7 days; the pass line in the plan is <= 10.
	var bigReads, bigBytes int64
	err := db.QueryRow(`SELECT count(*), coalesce(sum(output_bytes),0) FROM tool_runs
	  WHERE tool='read' AND output_bytes >= 51200 AND `+window).Scan(&bigReads, &bigBytes)
	if err == nil {
		fmt.Printf("  reads >=50KB: %d (%d bytes) — baseline 18/7d, pass line <=10\n", bigReads, bigBytes)
	}

	// The new tools themselves: reached for at all, and working when reached.
	// symbol's own baseline is the finding that started this: 0 calls ever.
	rows, err := db.Query(`
	  SELECT tool, count(*), sum(CASE WHEN ok=1 THEN 1 ELSE 0 END), avg(duration_ms)
	  FROM tool_runs WHERE tool IN ('repo_map','symbol','rename','diagnostics') AND ` + window + `
	  GROUP BY tool ORDER BY count(*) DESC`)
	if err != nil {
		fmt.Println("  unavailable:", err)
		return
	}
	defer rows.Close()
	fmt.Printf("  %-14s %7s %7s %10s\n", "tool", "calls", "ok", "avg ms")
	any := false
	for rows.Next() {
		var tool string
		var calls, ok int64
		var avg float64
		if err := rows.Scan(&tool, &calls, &ok, &avg); err != nil {
			continue
		}
		any = true
		fmt.Printf("  %-14s %7d %7d %10.0f\n", tool, calls, ok, avg)
	}
	stopEarly(rows)
	if !any {
		fmt.Println("  none of the new tools ran in this window — the trial has not started")
	}
	fmt.Println()
	fmt.Println("  The other two trial numbers live above: qwen's fresh/call and cache% (CACHE")
	fmt.Println("  HEALTH, baseline 10,962 at 77.1%) and its prefix-changed breaks (CACHE BREAKS,")
	fmt.Println("  baseline 96 breaks / 2.49M tokens in 7 days).")
	fmt.Println()
}

// repeatWaste is the section the usage page cannot have: identical bytes, same
// session, more than once.
//
// Keyed on (session_id, tool, output_sha256) because a repeat only wastes
// context if it lands in the SAME conversation — the same file read in two
// different sessions is two honest reads. Rows with no sha (written before
// schema v2) are left out rather than guessed at.
func repeatWaste(db *sql.DB, window string) {
	section("REPEAT WASTE — identical tool output re-sent into one session")
	rows, err := db.Query(`
	  WITH d AS (
	    SELECT session_id, tool, output_sha256, output_bytes,
	           row_number() OVER (PARTITION BY session_id, tool, output_sha256 ORDER BY id) rn
	    FROM tool_runs WHERE output_sha256 <> '' AND output_bytes > 0 AND ` + window + `)
	  SELECT tool, count(*), sum(output_bytes),
	         sum(CASE WHEN rn > 1 THEN output_bytes ELSE 0 END),
	         sum(CASE WHEN rn > 1 THEN 1 ELSE 0 END)
	  FROM d GROUP BY tool HAVING sum(CASE WHEN rn > 1 THEN output_bytes ELSE 0 END) > 0
	  ORDER BY 4 DESC LIMIT 15`)
	if err != nil {
		fmt.Println("  unavailable:", err)
		return
	}
	defer rows.Close()
	fmt.Printf("  %-28s %7s %12s %12s %7s %8s\n", "tool", "calls", "bytes", "repeated", "%", "re-runs")
	var totalBytes, totalRepeat int64
	for rows.Next() {
		var tool string
		var calls, bytes, repeat, reruns int64
		if err := rows.Scan(&tool, &calls, &bytes, &repeat, &reruns); err != nil {
			continue
		}
		totalBytes += bytes
		totalRepeat += repeat
		fmt.Printf("  %-28s %7d %12d %12d %6.1f%% %8d\n", tool, calls, bytes, repeat, pct(repeat, bytes), reruns)
	}
	stopEarly(rows)
	if totalBytes == 0 {
		fmt.Println("  nothing repeated in this window")
	} else {
		fmt.Printf("  %-28s %7s %12d %12d %6.1f%%\n", "— listed tools —", "", totalBytes, totalRepeat, pct(totalRepeat, totalBytes))
		fmt.Println()
		fmt.Println("  Every repeated byte is a document the conversation already held. The fix is")
		fmt.Println("  at the tool: return a pointer to what was already sent, not the bytes again.")
	}
	fmt.Println()
}

// delegationContainment answers whether the sub-agents are earning their keep.
//
// tool_runs.agent is empty for the assistant's own calls and carries the
// worker's name for a delegate's, so the split is exact rather than inferred. A
// high share here is the mechanism working: those bytes were paid for in a
// context that ended when the delegate did, instead of riding every later round
// of the conversation the user is still sitting in.
func delegationContainment(db *sql.DB, window string) {
	section("DELEGATION CONTAINMENT — whose context paid for the output")
	rows, err := db.Query(`
	  SELECT CASE WHEN agent = '' THEN '(the chat itself)' ELSE agent END,
	         count(*), sum(output_bytes)
	  FROM tool_runs WHERE ` + window + `
	  GROUP BY 1 ORDER BY 3 DESC LIMIT 15`)
	if err != nil {
		fmt.Println("  unavailable:", err)
		return
	}
	defer rows.Close()
	fmt.Printf("  %-28s %7s %12s %12s\n", "who ran it", "calls", "bytes", "bytes/call")
	var chat, delegated int64
	for rows.Next() {
		var who string
		var calls, bytes int64
		if err := rows.Scan(&who, &calls, &bytes); err != nil {
			continue
		}
		if who == "(the chat itself)" {
			chat += bytes
		} else {
			delegated += bytes
		}
		per := int64(0)
		if calls > 0 {
			per = bytes / calls
		}
		fmt.Printf("  %-28s %7d %12d %12d\n", who, calls, bytes, per)
	}
	stopEarly(rows)
	if total := chat + delegated; total > 0 {
		fmt.Println()
		fmt.Printf("  kept out of the chat: %.1f%% of tool output (%d of %d bytes)\n",
			pct(delegated, total), delegated, total)
	}
	fmt.Println()
}

// cacheHealth ranks models by the only number that survives prompt caching.
//
// Fresh tokens per call, not cache percentage: a model with a 90% hit rate on a
// huge prompt can still cost more fresh tokens per round than one with 60% on a
// small one, and it is the fresh tokens that are charged at full price. Rows
// where the provider reports no cache accounting at all (NULL) are excluded,
// because counting their whole prompt as fresh would rank a provider that
// merely does not tell us below one that does.
func cacheHealth(db *sql.DB, window string) {
	section("CACHE HEALTH — fresh input tokens per call, by model")
	rows, err := db.Query(`
	  SELECT model, count(*), sum(prompt_tokens), sum(cached_prompt_tokens)
	  FROM token_usage WHERE cached_prompt_tokens IS NOT NULL AND ` + window + `
	  GROUP BY model HAVING count(*) >= 20
	  ORDER BY sum(prompt_tokens - cached_prompt_tokens) DESC LIMIT 12`)
	if err != nil {
		fmt.Println("  unavailable:", err)
		return
	}
	defer rows.Close()
	fmt.Printf("  %-26s %7s %12s %12s %7s %11s\n", "model", "calls", "prompt", "fresh", "cache%", "fresh/call")
	any := false
	for rows.Next() {
		var m string
		var calls, prompt, cached int64
		if err := rows.Scan(&m, &calls, &prompt, &cached); err != nil {
			continue
		}
		any = true
		fresh := prompt - cached
		per := int64(0)
		if calls > 0 {
			per = fresh / calls
		}
		fmt.Printf("  %-26s %7d %12d %12d %6.1f%% %11d\n", m, calls, prompt, fresh, pct(cached, prompt), per)
	}
	stopEarly(rows)
	if !any {
		fmt.Println("  no model has 20+ calls with cache accounting in this window")
	} else {
		fmt.Println()
		fmt.Println("  A model at the top of this list is the largest cost in the product, and no")
		fmt.Println("  amount of tuning the tool loop reaches it. Changing it is one setting.")
	}
	fmt.Println()
}

// cacheBreaks finds the calls where the cached prefix went backwards.
//
// cacheHealth says a model averaged 77% cached; it cannot say whether that is
// a steady 77% (an incompressible dynamic tail) or 95% punctuated by breaks
// (something keeps invalidating the prefix). The distinction decides where the
// fix lives — the first is prompt layout, the second is whatever moves — so
// this section exists to tell them apart.
//
// A break is: within one session and one stretch of one model, this call's
// cached_prompt_tokens fell at least 2,000 tokens AND at least 5% below the
// previous call's. Both thresholds, not either: a big prompt jitters by
// thousands of tokens at under 1%, and a tiny session crosses 5% on noise.
//
// Causes, by elimination — which is all token_usage supports:
//   - model switch: the previous row in this session ran a different model.
//     Cache is per model everywhere that matters, so this break was bought
//     knowingly with the switch.
//   - idle gap: same model, but more than 5 minutes since the previous call —
//     the shortest provider TTL in play; a break after such a gap says
//     "expired", not "invalidated".
//   - prefix changed: same model, no gap. Something in the sent prefix moved.
//     WHAT moved is not in this table (that needs a prefix hash per call, an
//     app change deliberately deferred), so it is named honestly as the
//     remainder rather than guessed at.
//
// Rows with an empty session_id (written before sessions were recorded) are
// excluded: consecutive-call comparison inside "no particular session" would
// manufacture breaks out of interleaved conversations.
func cacheBreaks(db *sql.DB, window string) {
	section("CACHE BREAKS — calls where the cached prefix shrank, by cause")
	rows, err := db.Query(`
	  WITH seq AS (
	    SELECT session_id, model, time, prompt_tokens, cached_prompt_tokens,
	           lag(model)                OVER w AS prev_model,
	           lag(cached_prompt_tokens) OVER w AS prev_cached,
	           lag(time)                 OVER w AS prev_time
	    FROM token_usage
	    WHERE session_id <> '' AND cached_prompt_tokens IS NOT NULL AND ` + window + `
	    WINDOW w AS (PARTITION BY session_id ORDER BY id)),
	  breaks AS (
	    SELECT model,
	           prev_cached - cached_prompt_tokens AS drop_tokens,
	           CASE
	             WHEN model <> prev_model THEN 'model switch'
	             WHEN (julianday(time) - julianday(prev_time)) * 86400 > 300 THEN 'idle gap (TTL)'
	             ELSE 'prefix changed'
	           END AS cause
	    FROM seq
	    WHERE prev_cached IS NOT NULL
	      AND prev_cached - cached_prompt_tokens >= 2000
	      AND prev_cached - cached_prompt_tokens >= prev_cached / 20)
	  SELECT cause, model, count(*), sum(drop_tokens)
	  FROM breaks GROUP BY cause, model
	  ORDER BY sum(drop_tokens) DESC LIMIT 15`)
	if err != nil {
		fmt.Println("  unavailable:", err)
		return
	}
	defer rows.Close()
	fmt.Printf("  %-16s %-26s %7s %14s\n", "cause", "model", "breaks", "tokens dropped")
	var totalBreaks, totalDrop int64
	for rows.Next() {
		var cause, m string
		var n, drop int64
		if err := rows.Scan(&cause, &m, &n, &drop); err != nil {
			continue
		}
		totalBreaks += n
		totalDrop += drop
		fmt.Printf("  %-16s %-26s %7d %14d\n", cause, m, n, drop)
	}
	stopEarly(rows)
	if totalBreaks == 0 {
		fmt.Println("  no cache breaks in this window")
	} else {
		fmt.Printf("  %-16s %-26s %7d %14d\n", "— total —", "", totalBreaks, totalDrop)
		fmt.Println()
		fmt.Println("  Every dropped token here was re-sent fresh at full price on the very next")
		fmt.Println("  call. 'prefix changed' with no gap is the row worth chasing: something in")
		fmt.Println("  the prompt or tool block moved mid-conversation.")
	}
	fmt.Println()
}

// outputVolume is the plain answer to "what fills a context", which is reliably
// not what it feels like: reading one skill twice can outweigh a day of web
// research.
func outputVolume(db *sql.DB, window string) {
	section("OUTPUT VOLUME — bytes each tool put into a context")
	rows, err := db.Query(`
	  SELECT tool, count(*), sum(output_bytes), sum(args_bytes)
	  FROM tool_runs WHERE ` + window + `
	  GROUP BY tool ORDER BY sum(output_bytes) DESC LIMIT 15`)
	if err != nil {
		fmt.Println("  unavailable:", err)
		return
	}
	defer rows.Close()
	fmt.Printf("  %-28s %7s %12s %12s\n", "tool", "calls", "out bytes", "arg bytes")
	for rows.Next() {
		var tool string
		var calls, out, args int64
		if err := rows.Scan(&tool, &calls, &out, &args); err != nil {
			continue
		}
		fmt.Printf("  %-28s %7d %12d %12d\n", tool, calls, out, args)
	}
	stopEarly(rows)
	fmt.Println()
}

// stopEarly says so when the read did not reach the end of the result.
//
// rows.Next() returns false for "no more rows" and for "the read broke", and
// this whole program exists to put a number in front of the owner and let him
// decide something with it. A table that quietly lost its tail is the one
// failure an audit must not have: it does not look wrong, it looks smaller.
func stopEarly(rows *sql.Rows) {
	if err := rows.Err(); err != nil {
		fmt.Println("  ! the read stopped early, so these totals are short:", err)
	}
}

func section(title string) {
	fmt.Println(title)
	fmt.Println(strings.Repeat("-", len([]rune(title))))
}

func pct(part, whole int64) float64 {
	if whole == 0 {
		return 0
	}
	return 100 * float64(part) / float64(whole)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "tokenaudit: "+format+"\n", args...)
	os.Exit(1)
}
