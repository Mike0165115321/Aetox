package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type progressUpdate struct {
	id, name, subject string
	lines             int
}

func recordProgress(into *[]progressUpdate) func(id, name, subject string, lines int) {
	return func(id, name, subject string, lines int) {
		*into = append(*into, progressUpdate{id, name, subject, lines})
	}
}

// The tracker is the one place that decides what a UI hears while a tool call
// is still being written. Every provider funnels through it, so its rules are
// worth stating once, here, rather than re-deriving per wire format.
func TestToolProgressTrackerRules(t *testing.T) {
	defer func(prev time.Duration) { toolProgressInterval = prev }(toolProgressInterval)
	toolProgressInterval = 0

	t.Run("opens the row before any argument is nameable", func(t *testing.T) {
		var seen []progressUpdate
		tr := newToolProgressTracker(recordProgress(&seen))
		tr.report(0, "call_1", "write", "")
		if len(seen) != 1 {
			t.Fatalf("want the row to open on the name alone, got %+v", seen)
		}
		if seen[0] != (progressUpdate{"call_1", "write", "", 0}) {
			t.Errorf("first update = %+v", seen[0])
		}
	})

	t.Run("relabels when the subject finally arrives", func(t *testing.T) {
		var seen []progressUpdate
		tr := newToolProgressTracker(recordProgress(&seen))
		tr.report(0, "call_1", "write", `{"content": "a\nb\n`)
		tr.report(0, "call_1", "write", `{"content": "a\nb\nc\n", "path": "x.html"}`)
		if len(seen) != 2 {
			t.Fatalf("want 2 updates, got %+v", seen)
		}
		if seen[0].subject != "" {
			t.Errorf("named the row before the path existed: %+v", seen[0])
		}
		if seen[1].subject != "x.html" {
			t.Errorf("never relabelled: %+v", seen[1])
		}
	})

	t.Run("drops updates that say nothing new", func(t *testing.T) {
		var seen []progressUpdate
		tr := newToolProgressTracker(recordProgress(&seen))
		args := `{"path": "x.html", "content": "a\n`
		tr.report(0, "call_1", "write", args)
		tr.report(0, "call_1", "write", args)
		tr.report(0, "call_1", "write", args)
		if len(seen) != 1 {
			t.Errorf("identical state produced %d updates, want 1: %+v", len(seen), seen)
		}
	})

	t.Run("keeps concurrent calls apart", func(t *testing.T) {
		var seen []progressUpdate
		tr := newToolProgressTracker(recordProgress(&seen))
		tr.report(0, "call_a", "write", `{"path": "a.html", "content": "1\n`)
		tr.report(1, "call_b", "write", `{"path": "b.html", "content": "1\n`)
		if len(seen) != 2 || seen[0].id != "call_a" || seen[1].id != "call_b" {
			t.Errorf("two calls collapsed into one row: %+v", seen)
		}
	})

	t.Run("a nameless call is not announced", func(t *testing.T) {
		var seen []progressUpdate
		tr := newToolProgressTracker(recordProgress(&seen))
		tr.report(0, "call_1", "", `{"path": "x.html"`)
		if len(seen) != 0 {
			t.Errorf("announced a call with no tool name: %+v", seen)
		}
	})

	t.Run("no hook and nil tracker are both inert", func(t *testing.T) {
		newToolProgressTracker(nil).report(0, "call_1", "write", `{"path": "x"}`)
		var nilTracker *toolProgressTracker
		nilTracker.report(0, "call_1", "write", `{"path": "x"}`)
	})
}

// Pacing is what keeps an 800-line file from firing a thousand IPC messages.
// Left at its real interval, a burst collapses to the one update that opened
// the row — and one more once the interval has actually passed.
//
// The clock is held still rather than raced against: this used to feed 500
// fragments and hope they finished inside a real 200ms window, which is a test
// of how loaded the machine is. It failed on a busy one.
func TestToolProgressTrackerPaces(t *testing.T) {
	clock := freezeClock(t)

	var seen []progressUpdate
	tr := newToolProgressTracker(recordProgress(&seen))
	args := `{"path": "x.html", "content": "`
	for i := 0; i < 500; i++ {
		args += `line\n`
		tr.report(0, "call_1", "write", args)
	}
	if len(seen) != 1 {
		t.Fatalf("500 fragments inside one interval produced %d updates, want 1", len(seen))
	}

	// A tick short of the interval is still the same window.
	clock.advance(toolProgressInterval - time.Millisecond)
	args += `line\n`
	tr.report(0, "call_1", "write", args)
	if len(seen) != 1 {
		t.Errorf("an update escaped before the interval elapsed: %+v", seen)
	}

	// Past it, the counter is allowed to move again.
	clock.advance(2 * time.Millisecond)
	args += `line\n`
	tr.report(0, "call_1", "write", args)
	if len(seen) != 2 {
		t.Fatalf("no update after the interval elapsed: %+v", seen)
	}
	if seen[1].lines <= seen[0].lines {
		t.Errorf("the line count did not climb: %+v", seen)
	}
}

// The naming argument arriving last is the model's choice, not an edge case:
// `{"content": "...800 lines...", "path": "index.html"}` is a shape DeepSeek
// really produces, and the row then learns its name in the final fragment —
// inside the pacing window opened by the update just before it. Without a flush
// at the end, that row stays unnamed for the whole turn.
func TestToolProgressTrackerNamesTheRowWhenThePathArrivesLast(t *testing.T) {
	clock := freezeClock(t)

	var seen []progressUpdate
	tr := newToolProgressTracker(recordProgress(&seen))
	args := `{"content": "`
	for i := 0; i < 40; i++ {
		args += `line\n`
		tr.report(0, "call_1", "write", args)
		clock.advance(time.Millisecond) // 40ms all told: well inside one 200ms window
	}
	if last := seen[len(seen)-1]; last.subject != "" {
		t.Fatalf("nothing should have named the row yet: %+v", last)
	}

	// The tail arrives — the path, right at the end, inside the same window.
	args += `", "path": "index.html"}`
	tr.report(0, "call_1", "write", args)
	if last := seen[len(seen)-1]; last.subject != "" {
		t.Errorf("pacing was supposed to swallow this one: %+v", last)
	}

	tr.flush(0, "call_1", "write", args)
	last := seen[len(seen)-1]
	if last.subject != "index.html" {
		t.Errorf("the row never learned the path: %+v", last)
	}

	// Flushing again says nothing: the row is already named and the count stood
	// still, so a finished call cannot spam the UI on the way out.
	before := len(seen)
	tr.flush(0, "call_1", "write", args)
	if len(seen) != before {
		t.Errorf("a second flush sent %d extra update(s)", len(seen)-before)
	}
}

// testClock hands a test the clock the tracker reads, so pacing is checked
// exactly instead of being raced against wall time.
type testClock struct{ at time.Time }

func (c *testClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func freezeClock(t *testing.T) *testClock {
	t.Helper()
	c := &testClock{at: time.Now()}
	prev := nowFunc
	nowFunc = func() time.Time { return c.at }
	t.Cleanup(func() { nowFunc = prev })
	return c
}

// Ollama hands over a whole tool call at once rather than streaming its
// arguments, so there is no counter to climb — but the row must still appear,
// and with the id the finished call will carry, or the timeline draws the same
// call twice.
func TestOllamaStreamReportsToolCallProgress(t *testing.T) {
	defer func(prev time.Duration) { toolProgressInterval = prev }(toolProgressInterval)
	toolProgressInterval = 0

	lines := []string{
		`{"model":"tiny","done":false,"message":{"role":"assistant","content":"ok"}}`,
		`{"model":"tiny","done":false,"message":{"role":"assistant","tool_calls":[{"function":{"name":"write","arguments":{"path":"landing.html","content":"<h1>a</h1>"}}}]}}`,
		`{"model":"tiny","done":true,"message":{"role":"assistant"}}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, l := range lines {
			_, _ = w.Write([]byte(l + "\n"))
		}
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(OllamaConfig{Model: "tiny", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	var seen []progressUpdate
	resp, err := provider.StreamComplete(context.Background(), Request{
		Messages:           []Message{{Role: RoleUser, Content: "hi"}},
		OnToolCallProgress: recordProgress(&seen),
	}, nil, nil)
	if err != nil {
		t.Fatalf("stream complete: %v", err)
	}

	if len(seen) != 1 {
		t.Fatalf("want one announcement for one call, got %+v", seen)
	}
	if seen[0].name != "write" || seen[0].subject != "landing.html" {
		t.Errorf("update = %+v, want it named", seen[0])
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
	// The whole point of the shared id helper: these must not drift.
	if seen[0].id != resp.ToolCalls[0].ID {
		t.Errorf("streamed id %q != finished id %q — the UI would draw two rows",
			seen[0].id, resp.ToolCalls[0].ID)
	}
}
