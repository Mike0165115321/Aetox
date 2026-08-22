package main

// The host thread's latch, and the promise that a browser tool ends.
//
// browserHostLazy calls start() on EVERY binding, and start() used to park on a
// channel that only ever closed on the happy path. h.started went up before the
// main-window lookup ran, so a second caller arriving inside that window took
// the "already starting" branch — and when the lookup then failed, the first
// caller un-claimed the flag and walked away from the channel without closing
// it. The second caller waited on it for the life of the process: no error, no
// timeout, nothing in the log. Every browser call after it joined the queue.
//
// That is the shape of the freeze the owner reported on 22 ส.ค., and the rule
// it broke is his: a turn stops when Stop is pressed, and not otherwise.

import (
	"errors"
	"testing"
	"time"
)

func TestAFailedStartWakesEveryoneWaitingOnIt(t *testing.T) {
	h := &win32Host{attempt: newAttempt()}
	att := h.attempt
	h.started = true // the claim a first caller would have taken

	parked := make(chan error, 1)
	go func() { parked <- h.start() }()

	h.abandon(att, errors.New("main window not found"))

	select {
	case err := <-parked:
		if err == nil {
			t.Fatal("a start that failed reported success to the caller waiting on it")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("start() is still parked on an attempt that already failed")
	}
}

// And the next caller tries again rather than waiting on a latch that is
// already spent: one failed lookup must not close the browser for the session.
func TestAFailedStartArmsTheNextAttempt(t *testing.T) {
	h := &win32Host{attempt: newAttempt()}
	att := h.attempt
	h.started = true

	h.abandon(att, errors.New("main window not found"))

	if h.started {
		t.Error("the claim was not released, so nothing can retry")
	}
	if h.attempt == att {
		t.Error("the spent attempt is still the current one")
	}
	select {
	case <-h.attempt.done:
		t.Error("the next attempt starts already closed")
	default:
	}
}

// The bound itself. Nothing in a browser tool may wait without one, whatever
// went wrong underneath it.
func TestWaitingOnTheHostThreadIsBounded(t *testing.T) {
	defer func(prev time.Duration) { browserStartBudget = prev }(browserStartBudget)
	browserStartBudget = 50 * time.Millisecond

	start := time.Now()
	err := await(newAttempt()) // an attempt nobody will ever settle
	if err == nil {
		t.Fatal("waiting on a host thread that never came up reported success")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("the wait took %s, which is not a bound", elapsed)
	}
}
