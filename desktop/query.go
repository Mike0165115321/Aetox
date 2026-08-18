package main

import (
	"database/sql"
	"errors"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
)

// errStopRows ends iteration early and successfully. A caller that has filled
// its page has not failed, and rows.Err() is nil after a caller-initiated stop,
// so this composes with the check below rather than fighting it.
var errStopRows = errors.New("stop iterating")

// eachRow owns the whole lifetime of a query's rows: Query, defer Close, the
// Next loop, and the rows.Err() that twenty-three call sites in this package
// used to forget.
//
// Why it exists: a *sql.Rows that fails partway through iteration ends its loop
// exactly like a successful one. A query that broke after ten of two hundred
// rows returned ten rows and no error, and every caller here reported that as
// the answer. In an app whose subject is session history, that is history
// quietly coming back short — the one failure the user cannot tell from an
// empty week. ARCHITECTURE.md §6.7 is the same swallow one level up, where
// SearchSessions returned zero results forever and "that silent swallow is why
// the bug was invisible outside the failing test".
//
// what names the subsystem for the log line, following the house format
// "<subsystem>: <what failed>: %v".
//
// fn returning an error means this row could not be read: it is logged and
// skipped, which is what every call site already did — the change is that it is
// no longer silent, so schema drift shows up in the log instead of as a list
// that is mysteriously one short. Return errStopRows to end early.
//
// The error return is for the callers that have somewhere to put it. The rest
// deliberately report "no history" rather than a failure, for the reason
// workspace.go and workbench.go both state, and may ignore it — the log line
// has already been written by then.
func eachRow(db *sql.DB, what, query string, args []any, fn func(*sql.Rows) error) error {
	if db == nil {
		return sql.ErrConnDone
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		debuglog.Msg("%s: query failed: %v", what, err)
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := fn(rows); err != nil {
			if errors.Is(err, errStopRows) {
				return nil
			}
			debuglog.Msg("%s: row skipped: %v", what, err)
		}
	}

	// The line this whole file exists for.
	if err := rows.Err(); err != nil {
		debuglog.Msg("%s: iteration failed, results are incomplete: %v", what, err)
		return err
	}
	return nil
}

// queryAll is eachRow for the common case — scan each row into a T and collect
// them — so a caller that only wants a slice does not write the closure. The
// slice is never nil, which is the jsonSlice rule (see jsonslice.go) arriving
// one frame earlier for the bindings that return one of these directly.
func queryAll[T any](db *sql.DB, what, query string, args []any, scan func(*sql.Rows) (T, error)) ([]T, error) {
	out := []T{}
	err := eachRow(db, what, query, args, func(rows *sql.Rows) error {
		v, scanErr := scan(rows)
		if scanErr != nil {
			return scanErr
		}
		out = append(out, v)
		return nil
	})
	return out, err
}
