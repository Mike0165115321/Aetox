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
// the row.
func TestToolProgressTrackerPaces(t *testing.T) {
	var seen []progressUpdate
	tr := newToolProgressTracker(recordProgress(&seen))
	args := `{"path": "x.html", "content": "`
	for i := 0; i < 500; i++ {
		args += `line\n`
		tr.report(0, "call_1", "write", args)
	}
	if len(seen) != 1 {
		t.Errorf("500 fragments inside one interval produced %d updates, want 1", len(seen))
	}
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
