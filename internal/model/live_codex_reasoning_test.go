package model

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
)

// Does the ChatGPT backend actually stream its thinking, and under which event
// names?
//
// The thinking panel went quiet on Codex at some point and nothing on the
// machine recorded why: `buildResponsesRequest` asks for a reasoning summary on
// every reasoning turn, the stream switch had no default, and an event shape
// the backend renamed — or stopped sending — was indistinguishable from one
// that never occurred. 1,266 turns ran with thinking on and an empty panel.
//
// No fixture can answer this. The question is what a specific live backend
// emits, so the only honest test is one that asks it. Live-gated like its
// neighbours, and it fails rather than skips when the summary never arrives:
// a silent thinking panel is the bug, and a test that shrugs at it would have
// let the last two weeks happen again.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveCodexReasoning -v -count=1
//
// AETOX_LIVE_MODEL overrides the model, because which ids a ChatGPT plan serves
// changes without notice and a hardcoded name that stopped existing would fail
// this for the wrong reason.
func TestLiveCodexReasoningReachesTheThinkingPanel(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against the real ChatGPT backend")
	}
	// Point CODEX_HOME at an empty directory so signInCodexLive takes its other
	// branch and uses the app's own credential store.
	//
	// Its default preference is the Codex CLI's auth.json, which is right for
	// the test next door — that one only needs *a* working sign-in and would
	// rather not touch the developer's store. This one is different: the whole
	// question is what the backend sends to Aetox on the credential Aetox
	// actually runs on, and a stale CLI file answers it with a 401. On this
	// machine that file was fifteen days old and its refresh token long spent,
	// which is a failure about the fixture rather than about reasoning.
	t.Setenv("CODEX_HOME", t.TempDir())
	signInCodexLive(t)

	modelID := strings.TrimSpace(os.Getenv("AETOX_LIVE_MODEL"))
	if modelID == "" {
		modelID = "gpt-5.6-luna"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	p, err := NewProvider(ProviderOptions{
		Provider: "codex",
		Model:    modelID,
		Timeout:  120 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewProvider(%s): %v", modelID, err)
	}
	streamer, ok := p.(StreamingProvider)
	if !ok {
		t.Fatalf("%s does not stream, so there is no panel to fill", modelID)
	}

	// The ring is the seam. `debuglog.Msg` records whether or not a log file is
	// open, so the default case added to the stream switch reports here without
	// this test having to reach inside the parser.
	before := len(debuglog.Recent(0))

	var thinking, answer strings.Builder
	resp, err := streamer.StreamComplete(ctx, Request{
		Model: modelID,
		// A question with a wrong-looking obvious answer, so the model has
		// something to actually think about. A greeting is answered without
		// reasoning and would prove nothing either way.
		Messages: []Message{{
			Role: RoleUser,
			Content: "A bat and a ball cost 1.10 together. The bat costs 1.00 more than the ball. " +
				"How much is the ball? Answer with the number alone.",
		}},
		Reasoning: &ReasoningConfig{Effort: "medium"},
	}, func(chunk string) error {
		answer.WriteString(chunk)
		return nil
	}, func(chunk string) error {
		thinking.WriteString(chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}

	// Everything the switch did not recognise, in the order it arrived. Printed
	// whether the test passes or fails: on a pass it is the record of which
	// names the backend is using this month, which is the thing that will
	// change again.
	var unhandled []string
	for _, line := range debuglog.Recent(0)[before:] {
		if i := strings.Index(line, "unhandled event "); i >= 0 {
			unhandled = append(unhandled, strings.TrimSpace(line[i+len("unhandled event "):]))
		}
	}
	t.Logf("model:      %s", modelID)
	t.Logf("answer:     %q", strings.TrimSpace(answer.String()))
	t.Logf("thinking:   %d chars streamed", len(strings.TrimSpace(thinking.String())))
	t.Logf("collected:  %d chars on Response.ReasoningContent", len(strings.TrimSpace(resp.ReasoningContent)))
	if len(unhandled) == 0 {
		t.Log("unhandled:  none — every event this backend sent has a case")
	} else {
		t.Logf("unhandled:  %s", strings.Join(unhandled, ", "))
	}

	if strings.TrimSpace(answer.String()) == "" {
		t.Error("no answer text streamed at all — this is not a reasoning problem")
	}

	// The two halves are asserted apart because they fail apart. The handler is
	// what draws the panel live; ReasoningContent is what the turn keeps. A
	// backend that sends its summary in one final burst fills the second and
	// not the first, and that difference is exactly what the 2026-08-08 change
	// from summary:"auto" to summary:"detailed" was trying to buy.
	if strings.TrimSpace(thinking.String()) == "" {
		t.Errorf("nothing reached onReasoningChunk — the thinking panel stays empty on %s. "+
			"Unhandled events this turn: %v", modelID, unhandled)
	}
	if strings.TrimSpace(resp.ReasoningContent) == "" {
		t.Errorf("Response.ReasoningContent is empty on %s, so the turn kept no thinking either", modelID)
	}
}
