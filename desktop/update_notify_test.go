package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/update"
)

// captureEmit swaps the App's event seam for a recorder, and puts the real one
// back when the test ends.
func captureEmit(t *testing.T, a *App) *[]string {
	t.Helper()
	var events []string
	a.emit = func(event string, _ ...any) { events = append(events, event) }
	return &events
}

// stubCheck replaces the package-level check for one test.
func stubCheck(t *testing.T, st update.Status, err error) {
	t.Helper()
	prev := updateCheck
	updateCheck = func(context.Context, string) (update.Status, error) { return st, err }
	t.Cleanup(func() { updateCheck = prev })
}

// The whole point of the automatic check: the user finds out without having to
// go looking. One event, carrying the answer.
func TestAnnounceUpdateTellsTheFrontendWhenThereIsANewerBuild(t *testing.T) {
	a := &App{}
	events := captureEmit(t, a)
	stubCheck(t, update.Status{Current: "0.9.6", Latest: "0.9.7", Available: true, CanAuto: true}, nil)

	a.announceUpdate()

	if len(*events) != 1 || (*events)[0] != "update:available" {
		t.Errorf("events = %v, want exactly [update:available]", *events)
	}
}

// The status has to arrive intact. The notice offers a different action per
// channel — one click for portable, the Scoop command for Scoop, the release
// page for the rest — and that decision is internal/update's, already made and
// already tested. A frontend that had to re-derive it would be a second place
// answering the same question, free to drift.
func TestAnnounceUpdateCarriesTheWholeStatus(t *testing.T) {
	a := &App{}
	var got []any
	a.emit = func(_ string, data ...any) { got = data }
	want := update.Status{
		Current: "0.9.6", Latest: "0.9.7", Available: true,
		Channel: "scoop", Hint: "scoop update aetox", CanAuto: false,
		URL: "https://example.invalid/releases/v0.9.7",
	}
	stubCheck(t, want, nil)

	a.announceUpdate()

	if len(got) != 1 {
		t.Fatalf("payload = %v, want one value", got)
	}
	st, ok := got[0].(update.Status)
	if !ok {
		t.Fatalf("payload is %T, want update.Status — the UI decodes this shape", got[0])
	}
	if st != want {
		t.Errorf("payload = %+v, want %+v", st, want)
	}
}

// Nobody asked this question, so the answers nobody wants stay quiet. A red
// banner about GitHub being unreachable, on an app the user is in the middle of
// using, is an answer to a question they never posed — and it makes a working
// app look broken. The explicit button in Settings still reports its failures.
func TestAnnounceUpdateSaysNothingWhenThereIsNothingToSay(t *testing.T) {
	cases := []struct {
		name string
		st   update.Status
		err  error
	}{
		{"already current", update.Status{Current: "0.9.6", Latest: "0.9.6"}, nil},
		{"offline", update.Status{}, errors.New("dial tcp: no such host")},
		{"rate limited", update.Status{}, errors.New("github returned 403 Forbidden")},
		{"switched off", update.Status{Disabled: true}, update.ErrDisabled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &App{}
			events := captureEmit(t, a)
			stubCheck(t, c.st, c.err)

			a.announceUpdate()

			if len(*events) != 0 {
				t.Errorf("events = %v, want none", *events)
			}
		})
	}
}

// The check runs before anything has touched a.ctx in a fresh process, and
// http.NewRequestWithContext panics on a nil one. Falls back rather than
// crashing the app on the way to an optional convenience.
func TestAnnounceUpdateSurvivesANilContext(t *testing.T) {
	a := &App{}
	if a.ctx != nil {
		t.Fatal("this test is only meaningful with no Wails context")
	}
	var gotCtx context.Context
	prev := updateCheck
	updateCheck = func(ctx context.Context, _ string) (update.Status, error) {
		gotCtx = ctx
		return update.Status{}, nil
	}
	t.Cleanup(func() { updateCheck = prev })
	a.emit = func(string, ...any) {}

	a.announceUpdate()

	if gotCtx == nil {
		t.Error("the check was handed a nil context")
	}
}

// The env var is the whole off switch: set it and the app never talks to
// github.com, which is the point of having it. watchForUpdates has to honour it
// before the first sleep, not just rely on Check refusing later — an opted-out
// user should not have a goroutine ticking on their behalf at all.
func TestWatchForUpdatesReturnsImmediatelyWhenSwitchedOff(t *testing.T) {
	t.Setenv(update.DisableEnv, "1")
	a := &App{}
	events := captureEmit(t, a)
	stubCheck(t, update.Status{Available: true}, nil)

	done := make(chan struct{})
	go func() {
		a.watchForUpdates()
		close(done)
	}()

	// Returned, or asleep for updateFirstCheckDelay. A second is generous for
	// the first and far short of the second, so this tells them apart without
	// waiting out the delay.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchForUpdates is still running with the check switched off")
	}
	if len(*events) != 0 {
		t.Errorf("events = %v, want none", *events)
	}
}
