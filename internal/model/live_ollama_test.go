package model

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// Live tests against a real local Ollama, skipped unless AETOX_LIVE=1:
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveOllama -v -count=1
//
// A fake stream cannot answer what actually broke here: Ollama puts the spaces
// and newlines *inside* each token, and puts a thinking model's tokens in a
// field ("thinking") that no fake in this package was sending. Both bugs only
// show up against the real server.
const liveOllamaModel = "qwen3:8b"

func liveOllama(t *testing.T) *OllamaProvider {
	t.Helper()
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run live provider tests")
	}
	conn, err := net.DialTimeout("tcp", "localhost:11434", 2*time.Second)
	if err != nil {
		t.Skipf("no local Ollama on :11434: %v", err)
	}
	_ = conn.Close()

	p, err := NewOllamaProvider(OllamaConfig{Model: liveOllamaModel, Timeout: 5 * time.Minute})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return p
}

// The whole point of streaming is that text arrives while the model is still
// writing, with its whitespace intact. Both halves are asserted here because
// chunk-level TrimSpace passed "it streams" while producing "Helloworld".
func TestLiveOllamaStream_TextArrivesIncrementallyIntact(t *testing.T) {
	p := liveOllama(t)

	var streamed strings.Builder
	var chunks int
	// firstAt is the first token of *any* kind. On a thinking model the answer
	// legitimately cannot start until thinking ends — measured here, qwen3:8b
	// thinks for 23s of a 24s turn — so timing content alone would call a
	// perfectly live stream dead. What must be early is the first thing the UI
	// can show at all.
	var firstAt time.Duration
	start := time.Now()
	markFirst := func() {
		if firstAt == 0 {
			firstAt = time.Since(start)
		}
	}

	resp, err := p.StreamComplete(context.Background(), Request{
		MaxTokens:   2000,
		Temperature: 0.2,
		Messages: []Message{
			{Role: RoleUser, Content: `Reply with exactly this and nothing else:` + "\n" + `Hello world` + "\n\n" + `- Apple` + "\n" + `- Banana`},
		},
	}, func(chunk string) error {
		markFirst()
		chunks++
		streamed.WriteString(chunk)
		return nil
	}, func(string) error {
		markFirst()
		return nil
	})
	total := time.Since(start)
	if err != nil {
		t.Fatalf("stream failed after %v: %v", total, err)
	}
	t.Logf("total=%v chunks=%d firstAt=%v", total.Round(time.Millisecond), chunks, firstAt.Round(time.Millisecond))
	t.Logf("text=%q", resp.Text)

	if chunks < 2 {
		t.Fatalf("got %d content chunk(s) — that is a buffered response, not a stream", chunks)
	}
	if firstAt > total/2 {
		t.Errorf("first token at %v of a %v stream — nothing was live", firstAt, total)
	}
	// The handler must see the same bytes the final Response carries, or the UI
	// renders one thing live and a different thing when the turn settles.
	if streamed.String() != resp.Text {
		t.Errorf("streamed text != final text:\n live: %q\nfinal: %q", streamed.String(), resp.Text)
	}
	// Whitespace inside the tokens is what a trim destroys: no spaces between
	// words, no line breaks, so every list and code block collapses.
	if !strings.Contains(resp.Text, "Hello world") {
		t.Errorf("words were glued together — expected a space in %q", resp.Text)
	}
	if !strings.Contains(resp.Text, "\n") {
		t.Errorf("every line break was lost: %q", resp.Text)
	}
}

// qwen3 is a thinking model: Ollama sends its reasoning in message.thinking.
// Reading only reasoning_content dropped all of it, so the desktop showed a
// silent spinner for the entire thinking phase.
func TestLiveOllamaStream_DeliversThinkingTokens(t *testing.T) {
	p := liveOllama(t)

	var reasoning strings.Builder
	var reasonChunks int
	var firstReasonAt, firstContentAt time.Duration
	start := time.Now()

	resp, err := p.StreamComplete(context.Background(), Request{
		MaxTokens:   4000,
		Temperature: 0.2,
		Messages: []Message{
			{Role: RoleUser, Content: "A farmer has 17 sheep. All but 9 run away. How many are left? Think it through."},
		},
	}, func(string) error {
		if firstContentAt == 0 {
			firstContentAt = time.Since(start)
		}
		return nil
	}, func(chunk string) error {
		if reasonChunks == 0 {
			firstReasonAt = time.Since(start)
		}
		reasonChunks++
		reasoning.WriteString(chunk)
		return nil
	})
	total := time.Since(start)
	if err != nil {
		t.Fatalf("stream failed after %v: %v", total, err)
	}
	t.Logf("total=%v reasonChunks=%d reasonChars=%d firstReasonAt=%v firstContentAt=%v",
		total.Round(time.Millisecond), reasonChunks, reasoning.Len(),
		firstReasonAt.Round(time.Millisecond), firstContentAt.Round(time.Millisecond))
	t.Logf("thinking[:200]=%q", truncOllama(reasoning.String(), 200))

	if reasonChunks == 0 {
		t.Fatalf("no thinking tokens from %s — the reasoning field is being dropped again", liveOllamaModel)
	}
	if firstReasonAt > firstContentAt && firstContentAt != 0 {
		t.Errorf("thinking arrived after the answer (%v vs %v)", firstReasonAt, firstContentAt)
	}
	if got := strings.TrimSpace(resp.ReasoningContent); got == "" {
		t.Error("thinking streamed live but never reached the final Response")
	}
}

// Tools have to survive the streaming path too — that is the path the desktop
// takes, and the one where a rejection used to fail the whole turn.
func TestLiveOllamaStream_CallsToolsAndReportsProgress(t *testing.T) {
	p := liveOllama(t)

	var seen []progressUpdate
	start := time.Now()
	resp, err := p.StreamComplete(context.Background(), Request{
		MaxTokens:   4000,
		Temperature: 0.2,
		Tools:       []ToolDefinition{liveWriteTool()},
		Messages: []Message{
			{Role: RoleSystem, Content: "You are a coding agent. Use the write tool. Do not explain."},
			{Role: RoleUser, Content: "Write a file notes.txt containing three lines of plain text. Call the write tool once."},
		},
		OnToolCallProgress: func(id, name, subject string, lines int) {
			seen = append(seen, progressUpdate{id, name, subject, lines})
		},
	}, nil, nil)
	total := time.Since(start)
	if err != nil {
		t.Fatalf("stream failed after %v: %v", total, err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Fatalf("no tool call in %v; text was %q", total, resp.Text)
	}
	call := resp.ToolCalls[0]
	t.Logf("total=%v tool=%s id=%q args=%s", total.Round(time.Millisecond), call.Function.Name, call.ID, truncOllama(call.Function.Arguments, 200))
	t.Logf("progress updates: %d", len(seen))

	if len(seen) == 0 {
		t.Fatal("no progress update — the UI would show nothing until the call finished")
	}
	// Streamed id and finished id must agree or the timeline draws two rows.
	if seen[0].id != call.ID {
		t.Errorf("streamed id %q != finished id %q", seen[0].id, call.ID)
	}
	if !strings.HasPrefix(strings.TrimSpace(call.Function.Arguments), "{") {
		t.Errorf("tool arguments are not JSON: %q", call.Function.Arguments)
	}
}
