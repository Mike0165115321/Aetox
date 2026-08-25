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
// Four sections, in the order they are worth acting on:
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
	outputVolume(db, window)
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
