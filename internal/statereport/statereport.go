// Package statereport marks errors that describe the state of the world
// rather than a mistake by whoever made the call.
//
// The two read identically as text — "n8n ปฏิเสธ API key นี้" and "this command
// builds a path while it runs, so it cannot be checked" are both one sentence
// carrying a remedy — but they teach opposite things. A refusal is about the
// caller's behaviour: do it differently next time, every time. A state report
// is about a machine at a moment: the server was down, the key had expired,
// the network was out — true tonight, false tomorrow morning, and "avoid this
// pattern from the first time" is exactly the wrong lesson to carve into
// permanent memory (three such cards reached the approval queue on 2026-08-12,
// all three describing one n8n that simply was not running).
//
// The learning floor's summarizer is today's reader, but the mark is not
// written for it specifically — it records a fact about the error that only
// its author knows, the same way *exec.ExitError records that a process ran
// and returned a status. Whoever writes "the server did not answer" knows at
// that moment they are reporting weather, not correcting behaviour.
//
// The line an author draws: mark the error when what it reports would be true
// or false regardless of how the caller behaves (a server's reachability, a
// credential's validity, a service left unconfigured). Leave it unmarked when
// the remedy is a change in the caller's own next action (a malformed payload,
// a missing argument, a wrong id). Unmarked stays the conservative default —
// an unmarked failure is still offered as a possible lesson, and the user is
// the one who says no.
//
// This package sits at the bottom of the import graph on purpose: error
// authors live in leaf packages (internal/automation/n8n, desktop) that the
// turn executor's own package cannot be imported from without a cycle.
package statereport

import (
	"errors"
	"fmt"
)

type reportErr struct{ err error }

func (e *reportErr) Error() string { return e.err.Error() }
func (e *reportErr) Unwrap() error { return e.err }

// New is errors.New for a state report.
func New(text string) error { return &reportErr{errors.New(text)} }

// Newf is fmt.Errorf for a state report. %w works as it does there.
func Newf(format string, a ...any) error { return &reportErr{fmt.Errorf(format, a...)} }

// Mark wraps an existing error as a state report; nil stays nil.
func Mark(err error) error {
	if err == nil {
		return nil
	}
	return &reportErr{err}
}

// Is reports whether err carries the mark anywhere in its chain.
func Is(err error) bool {
	var r *reportErr
	return errors.As(err, &r)
}
