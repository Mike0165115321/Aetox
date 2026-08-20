package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// openTestDB is a bare store with no App around it: these tests are about
// eachRow's contract, not about any binding that uses it.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "q.db")) + "?_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Windows cannot remove an open file, so t.TempDir's cleanup fails without
	// this (Cleanup is LIFO, and TempDir registered its own above).
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// failsOnThirdRow generates four rows and raises inside row three.
//
// abs() of the most negative int64 has no representable result, so SQLite
// raises "integer overflow" while stepping — which is the case this whole file
// exists for and the one no unit test can reach by handing bad input to Scan:
// two rows arrive, the third fails, and Next() then returns false exactly as it
// would at a clean end of results. Only rows.Err() can tell the two apart.
const failsOnThirdRow = `
	WITH t(i) AS (VALUES (1),(2),(3),(4))
	SELECT CASE WHEN i = 3 THEN abs(-9223372036854775808) ELSE i END FROM t`

func TestEachRowReportsAFailureMidIteration(t *testing.T) {
	db := openTestDB(t)

	got := []int64{}
	err := eachRow(db, "test", failsOnThirdRow, nil, func(rows *sql.Rows) error {
		var n int64
		if scanErr := rows.Scan(&n); scanErr != nil {
			return scanErr
		}
		got = append(got, n)
		return nil
	})

	if err == nil {
		t.Fatalf("a query that broke after %d of 4 rows reported success — this is the "+
			"silent short list eachRow exists to stop (ARCHITECTURE.md §6.7)", len(got))
	}
	if len(got) == 4 {
		t.Fatalf("expected the query to fail partway, got all 4 rows — the fixture no " +
			"longer provokes a mid-iteration error, so this test is no longer testing anything")
	}
}

func TestEachRowStopsEarlyWithoutCallingItAFailure(t *testing.T) {
	db := openTestDB(t)

	seen := 0
	err := eachRow(db, "test", `WITH t(i) AS (VALUES (1),(2),(3),(4)) SELECT i FROM t`, nil,
		func(*sql.Rows) error {
			seen++
			if seen == 2 {
				return errStopRows
			}
			return nil
		})
	if err != nil {
		t.Fatalf("a caller that stopped at its page limit is not a failure: %v", err)
	}
	if seen != 2 {
		t.Fatalf("errStopRows did not stop iteration: saw %d rows, want 2", seen)
	}
}

// A row that cannot be read is skipped and the rest still arrive, which is what
// every call site did before eachRow existed. What changed is that it is logged
// rather than silent.
func TestEachRowSkipsAnUnreadableRowAndKeepsGoing(t *testing.T) {
	db := openTestDB(t)

	kept := []int64{}
	err := eachRow(db, "test", `WITH t(i) AS (VALUES (1),(2),(3)) SELECT i FROM t`, nil,
		func(rows *sql.Rows) error {
			var n int64
			if scanErr := rows.Scan(&n); scanErr != nil {
				return scanErr
			}
			if n == 2 {
				return errors.New("pretend this row is malformed")
			}
			kept = append(kept, n)
			return nil
		})
	if err != nil {
		t.Fatalf("a skipped row is not an iteration failure: %v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("kept %v, want the two readable rows", kept)
	}
}

func TestEachRowReportsAQueryThatNeverRan(t *testing.T) {
	db := openTestDB(t)
	err := eachRow(db, "test", `SELECT * FROM a_table_that_does_not_exist`, nil,
		func(*sql.Rows) error { return nil })
	if err == nil {
		t.Fatal("a query that could not run reported success")
	}
}

// queryAll returns [] and not nil, so a binding built on it satisfies the
// jsonSlice rule (§34) without the caller remembering to wrap it.
func TestQueryAllNeverReturnsANilSlice(t *testing.T) {
	db := openTestDB(t)
	out, err := queryAll(db, "test", `WITH t(i) AS (VALUES (1)) SELECT i FROM t WHERE i > 99`, nil,
		func(rows *sql.Rows) (int64, error) {
			var n int64
			err := rows.Scan(&n)
			return n, err
		})
	if err != nil {
		t.Fatalf("empty result is not an error: %v", err)
	}
	if out == nil {
		t.Fatal("queryAll returned a nil slice — marshals to JSON null and crashes the frontend")
	}
}

// directQueryMarker is what a site writes to opt out. It must appear in the
// source, so the exception is reviewable where it lives rather than being a
// name on a list somewhere else.
const directQueryMarker = "query-direct:"

// TestEveryQuerySiteChecksItsRows is a source guard, not a behaviour test.
//
// A *sql.Rows that fails partway through iteration ends its loop exactly like a
// successful one, so the caller sees a short list and no error. Twenty-three
// sites in this package had that shape at once, which says the failure mode is
// not that the check is hard but that nobody can see it is missing — the code
// reads perfectly well without it. A reviewer noticing an absent rows.Err() on
// the twenty-fourth is not something to plan around.
//
// So: every db.Query in non-test code in this package goes through eachRow or
// queryAll, which own the check. The exception is a site that already handles
// its rows more strictly than the helper does — usage.go aborts on a scan error
// rather than skipping the row, because a billing number built from most of the
// data is worse than no number — and it opts out with an explicit marker.
func TestEveryQuerySiteChecksItsRows(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	query := regexp.MustCompile(`\w+\s*:?=\s*\w+\.Query\(`)

	var direct []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// query.go is the one place allowed to hold the loop.
		if name == "query.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			if !query.MatchString(line) {
				continue
			}
			// The marker sits in the comment above the call, which is where the
			// reason for the exception belongs.
			excused := false
			for j := i; j >= 0 && j > i-10; j-- {
				if strings.Contains(lines[j], directQueryMarker) {
					excused = true
					break
				}
			}
			if !excused {
				direct = append(direct, name+":"+strconv.Itoa(i+1)+" — "+strings.TrimSpace(line))
			}
		}
	}

	if len(direct) > 0 {
		t.Errorf("db.Query outside eachRow/queryAll — a query that fails partway through "+
			"iteration returns a short list and no error, and the user cannot tell that "+
			"from an empty week:\n  %s\n\nRoute it through eachRow (query.go), or write "+
			"%q with the reason if this site handles its own rows more strictly.",
			strings.Join(direct, "\n  "), directQueryMarker)
	}
}
