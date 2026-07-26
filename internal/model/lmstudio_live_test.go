package model

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// Live checks against a real LM Studio server, skipped unless AETOX_LIVE=1 and
// something is actually listening:
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveLMStudio -v -count=1
//
// LM Studio shares the OpenAI-compatible adapter with several hosted
// providers, so what is unproven here is not the wire format but the local
// specifics: which model a keyless local server says it is serving, and
// whether streaming and tool calls survive that path.
func liveLMStudio(t *testing.T) (Provider, string) {
	t.Helper()
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run live provider tests")
	}
	conn, err := net.DialTimeout("tcp", "localhost:1234", 2*time.Second)
	if err != nil {
		t.Skipf("no LM Studio server on :1234: %v", err)
	}
	_ = conn.Close()

	base := DefaultBaseURL("lmstudio")
	modelName := ResolveDefaultModel("lmstudio", base, "")
	if modelName == "" {
		t.Skip("LM Studio is running but serving no model")
	}
	p, err := NewProvider(ProviderOptions{Provider: "lmstudio", Model: modelName, Timeout: 5 * time.Minute})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return p, modelName
}

// The picker lists everything downloaded; the default must be the loaded chat
// model. LM Studio serves embedding models from the same list and they cannot
// answer a chat request, so picking off a sorted list is a coin flip that
// depends on how the embedding model happens to be named.
func TestLiveLMStudioResolvesTheLoadedModel(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run live provider tests")
	}
	if conn, err := net.DialTimeout("tcp", "localhost:1234", 2*time.Second); err != nil {
		t.Skipf("no LM Studio server on :1234: %v", err)
	} else {
		_ = conn.Close()
	}
	base := DefaultBaseURL("lmstudio")

	choices, err := ModelChoicesWithEndpointAndAPIKey("lmstudio", base, "")
	if err != nil {
		t.Fatalf("model discovery: %v", err)
	}
	active := activeLocalModel("lmstudio", base, "")
	resolved := ResolveDefaultModel("lmstudio", base, "")
	t.Logf("picker list : %v", choices)
	t.Logf("loaded now  : %q", active)
	t.Logf("default pick: %q", resolved)

	if active == "" {
		t.Fatal("no loaded model reported — /api/v0/models did not answer")
	}
	if resolved != active {
		t.Errorf("default = %q but the server has %q loaded", resolved, active)
	}
	if strings.Contains(strings.ToLower(resolved), "embed") {
		t.Errorf("default %q is an embedding model — it cannot answer a chat request", resolved)
	}
}

func TestLiveLMStudioStreams(t *testing.T) {
	p, modelName := liveLMStudio(t)
	streamer, ok := p.(StreamingProvider)
	if !ok {
		t.Fatal("provider does not stream")
	}

	var streamed, reasoned strings.Builder
	var chunks, reasonChunks int
	// firstAt is the first token of ANY kind. A thinking model legitimately
	// cannot start its answer until it stops thinking, so timing content alone
	// calls a perfectly live stream dead — the lesson from qwen3 on Ollama.
	var firstAt, firstContentAt time.Duration
	start := time.Now()
	mark := func(d *time.Duration) {
		if *d == 0 {
			*d = time.Since(start)
		}
	}
	resp, err := streamer.StreamComplete(context.Background(), Request{
		Model: modelName, MaxTokens: 2000, Temperature: 0.2,
		Messages: []Message{
			{Role: RoleUser, Content: "Name three fruits, one per line, nothing else."},
		},
	}, func(chunk string) error {
		mark(&firstAt)
		mark(&firstContentAt)
		chunks++
		streamed.WriteString(chunk)
		return nil
	}, func(chunk string) error {
		mark(&firstAt)
		reasonChunks++
		reasoned.WriteString(chunk)
		return nil
	})
	total := time.Since(start)
	if err != nil {
		t.Fatalf("stream failed after %v: %v", total, err)
	}
	t.Logf("model=%s total=%v", modelName, total.Round(time.Millisecond))
	t.Logf("content : %d chunks, first at %v", chunks, firstContentAt.Round(time.Millisecond))
	t.Logf("thinking: %d chunks, %d chars", reasonChunks, reasoned.Len())
	t.Logf("firstAt (any kind) = %v", firstAt.Round(time.Millisecond))
	t.Logf("text=%q", resp.Text)
	if resp.Usage != nil {
		t.Logf("usage=%+v", *resp.Usage)
	}

	if chunks < 2 {
		t.Fatalf("got %d content chunk(s) — buffered, not streamed", chunks)
	}
	if streamed.String() != resp.Text {
		t.Errorf("live text != final text:\n live: %q\nfinal: %q", streamed.String(), resp.Text)
	}
	if firstAt > total/2 {
		t.Errorf("first token of any kind at %v of a %v stream — nothing was live", firstAt, total)
	}
	// Inter-token whitespace has to survive: a trim per chunk would glue the
	// words together, which is exactly what was wrong on the Ollama path.
	if !strings.Contains(strings.TrimSpace(resp.Text), " ") && !strings.Contains(resp.Text, "\n") {
		t.Errorf("no whitespace anywhere in a multi-word reply — chunks were trimmed: %q", resp.Text)
	}
	// A local runtime has no prompt cache; the stats page must show an em dash
	// rather than a 0% hit rate it never claimed.
	if resp.Usage != nil && resp.Usage.CacheReported {
		t.Error("CacheReported = true for LM Studio, which reports no cache accounting")
	}
}

func TestLiveLMStudioCallsTools(t *testing.T) {
	p, modelName := liveLMStudio(t)
	streamer, ok := p.(StreamingProvider)
	if !ok {
		t.Fatal("provider does not stream")
	}

	var seen []progressUpdate
	start := time.Now()
	resp, err := streamer.StreamComplete(context.Background(), Request{
		Model: modelName, MaxTokens: 4000, Temperature: 0.2,
		Tools: []ToolDefinition{liveWriteTool()},
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
	t.Logf("total=%v tool=%s id=%q args=%s progress=%d",
		total.Round(time.Millisecond), call.Function.Name, call.ID,
		truncOllama(call.Function.Arguments, 200), len(seen))

	if !strings.HasPrefix(strings.TrimSpace(call.Function.Arguments), "{") {
		t.Errorf("tool arguments are not JSON: %q", call.Function.Arguments)
	}
	if len(seen) > 0 && seen[0].id != call.ID {
		t.Errorf("streamed id %q != finished id %q — the timeline would draw two rows", seen[0].id, call.ID)
	}
}
