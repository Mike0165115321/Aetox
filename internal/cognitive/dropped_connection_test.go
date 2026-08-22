package cognitive

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// The exact shape Windows hands back when a provider cuts the socket while it
// is answering: a *net.OpError wrapping a wsarecv syscall error. Built rather
// than string-matched, because the classifier under test reads the type and a
// test that only matched the words would pass against a change that broke it.
//
// The inner error is a plain one so the message reads the same on every
// platform; errors.As looks at the *net.OpError, not at what it wraps.
func droppedMidAnswer() error {
	return &net.OpError{
		Op: "read", Net: "tcp",
		Source: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 40), Port: 47800},
		Addr:   &net.TCPAddr{IP: net.IPv4(3, 173, 21, 63), Port: 443},
		Err:    os.NewSyscallError("wsarecv", errors.New("An existing connection was forcibly closed by the remote host.")),
	}
}

// The turn the owner lost at 10:54 on 22 ส.ค.
//
// DeepSeek dropped the connection three seconds in, and the whole turn died
// with the raw Go error on screen. The two retries that were built for exactly
// this (model.retryTransport) never ran: they wrap RoundTrip, which had already
// returned successfully with the headers, and the socket died while the body
// was being read. So the one failure mode the retry existed for was the one it
// could not see.
func TestADroppedConnectionMidAnswerIsAskedAgain(t *testing.T) {
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) { return model.Response{}, droppedMidAnswer() },
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "คำตอบจริง"}, nil
		},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash", SystemPrompt: "sys", MaxChars: 1_000_000})

	reply, err := oneTurn(agent, "อะไรก็ได้เทส")
	if err != nil {
		t.Fatalf("the turn died on a blip that was worth one more try: %v", err)
	}
	if reply != "คำตอบจริง" {
		t.Errorf("reply = %q, want the answer the second attempt produced", reply)
	}
	if len(provider.seen) != 2 {
		t.Fatalf("provider was called %d times, want 2 (the failure and the replay)", len(provider.seen))
	}
	// The failed round must leave no residue: a replay that carried half a dead
	// exchange forward would be asking a different question than the one that
	// failed.
	first, second := provider.seen[0].Messages, provider.seen[1].Messages
	if len(first) != len(second) {
		t.Fatalf("the replay sent %d messages, the failed round sent %d — the dead round left something behind",
			len(second), len(first))
	}
	for i := range first {
		if first[i].Content != second[i].Content {
			t.Errorf("message %d differs between the failed round and its replay:\n first: %q\nsecond: %q",
				i, first[i].Content, second[i].Content)
		}
	}
}

// Bounded, and the bound is the point. A network that is down stays down, and
// an app that keeps asking is an app that looks hung — which is the complaint
// this whole change came from.
func TestAConnectionThatKeepsDroppingGivesUpAndSaysSo(t *testing.T) {
	alwaysDrops := func(model.Request) (model.Response, error) { return model.Response{}, droppedMidAnswer() }
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		alwaysDrops, alwaysDrops, alwaysDrops, alwaysDrops,
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash", SystemPrompt: "sys", MaxChars: 1_000_000})

	_, err := oneTurn(agent, "อะไรก็ได้เทส")
	if err == nil {
		t.Fatal("a connection that never came back reported success")
	}
	// Labelled, so the window can say "the connection dropped" in the user's own
	// language rather than showing them "wsarecv". Windows, Linux and macOS all
	// word this failure differently and Go is the only layer that knows it by
	// type, so the label is the contract between the two.
	if !strings.Contains(err.Error(), model.DroppedConnectionMarker) {
		t.Errorf("error = %v, want the %q label the window keys on", err, model.DroppedConnectionMarker)
	}
	// The original stays underneath. It is what the debug log needs, and a
	// label that replaced the evidence would make the next one of these
	// unfindable.
	if !strings.Contains(err.Error(), "forcibly closed") {
		t.Errorf("error = %v, want the transport's own words kept under the label", err)
	}
	if len(provider.seen) != 1+maxDroppedConnectionRetries {
		t.Errorf("provider was called %d times, want %d (the first try plus its retries)",
			len(provider.seen), 1+maxDroppedConnectionRetries)
	}
}

// The other half of the bound, and the older bug. A provider that ANSWERED and
// said no is not a dropped connection: asking again spends the same money on
// the same refusal and hides what the user was told. §Aetox learned this once
// already with the ask-again-without-tools path, which used to fire on exactly
// this class of failure.
func TestAProviderThatAnsweredAndRefusedIsNotRetried(t *testing.T) {
	refused := errors.New(`deepseek request failed with status 401: {"error":{"message":"Authentication Fails, Your api key is invalid"}}`)
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) { return model.Response{}, refused },
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "ไม่ควรมาถึงตรงนี้"}, nil
		},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash", SystemPrompt: "sys", MaxChars: 1_000_000})

	if _, err := oneTurn(agent, "อะไรก็ได้เทส"); err == nil {
		t.Fatal("a rejected key was reported as a working turn")
	}
	if len(provider.seen) != 1 {
		t.Errorf("provider was called %d times, want 1: a 401 is an answer, not a blip", len(provider.seen))
	}
}

// The same rule on the route that has no tools, which is not a side road: it is
// what a provider without tool calling uses, what the compaction call uses, and
// what the tool loop itself falls back to. It lost a whole turn to a blip just
// as readily, and it is a different function, so it gets its own guard rather
// than an assumption that the shared helper reached it.
func TestTheToolLessRouteReconnectsToo(t *testing.T) {
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) { return model.Response{}, droppedMidAnswer() },
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "คำตอบจากรอบสอง"}, nil
		},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash", SystemPrompt: "sys", MaxChars: 1_000_000})

	// No tools and no executor, which is what sends RespondWithTools down the
	// plain Respond path.
	reply, _, err := agent.RespondWithTools(context.Background(), nil, "อะไรก็ได้เทส", nil, nil, turn.TurnOptions{})
	if err != nil {
		t.Fatalf("the tool-less route still dies on a blip: %v", err)
	}
	if reply != "คำตอบจากรอบสอง" {
		t.Errorf("reply = %q, want the second attempt's answer", reply)
	}
	if len(provider.seen) != 2 {
		t.Errorf("provider was called %d times, want 2", len(provider.seen))
	}
}

// The call that decides whether a turn fits at all, and the least obvious one
// to have been exposed.
//
// A blip on the compaction request used to make compact() return false, which
// the overflow path reads as "nothing left to summarize" and reports as *this
// conversation no longer fits the model's context window*. So a dropped socket
// came back to the user as their history being too long, with the fix on offer
// being to start a new chat or buy a bigger model. Same shape as the failure
// §166 was written about, one layer further in.
func TestTheCompactionCallReconnectsToo(t *testing.T) {
	tooLong := errors.New(`request failed with status 400: {"error":{"message":"Your input exceeds the context window of this model.","code":"context_length_exceeded"}}`)
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) { return model.Response{}, tooLong },
		// The compaction call the rejection triggers, and the socket dies under
		// it rather than under the turn.
		func(model.Request) (model.Response, error) { return model.Response{}, droppedMidAnswer() },
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "COMPACT-SUMMARY: they were chasing a dropped connection"}, nil
		},
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "คำตอบหลังย่อประวัติ"}, nil
		},
	}}
	// Far above this history, so only the provider's refusal can be what
	// compacts — the same setup TestAProviderSayingTheHistoryIsTooLong uses.
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash", SystemPrompt: "sys", MaxChars: 10_000_000})
	agent.RestoreHistory(longHistory(10))

	reply, err := oneTurn(agent, "สรุปให้หน่อย")
	if err != nil {
		t.Fatalf("a blip on the compaction call still kills the turn: %v", err)
	}
	if reply != "คำตอบหลังย่อประวัติ" {
		t.Errorf("reply = %q, want the answer that came after the summary", reply)
	}
	if len(provider.seen) != 4 {
		t.Errorf("provider was called %d times, want 4: the rejection, the dropped summary, its replay, the answer",
			len(provider.seen))
	}
}

// Stop has to land during the pause, not only between attempts. The wait is
// short, but a user who pressed Stop and then watched the app sit there for
// another second has been told the button is advisory.
//
// Cancelled from a goroutine rather than inline, and that is what makes this
// test about the pause: cancelling before the error is returned would be
// answered a step earlier, by IsDroppedConnection refusing to call a cancelled
// context a network blip. Both endings are correct; only one of them is the one
// this guards.
func TestStopDuringTheRetryPauseEndsTheTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &scriptedProvider{steps: []func(model.Request) (model.Response, error){
		func(model.Request) (model.Response, error) {
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()
			return model.Response{}, droppedMidAnswer()
		},
		func(model.Request) (model.Response, error) {
			return model.Response{Text: "ไม่ควรมาถึงตรงนี้"}, nil
		},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "deepseek-v4-flash", SystemPrompt: "sys", MaxChars: 1_000_000})

	_, _, err := agent.RespondWithTools(ctx, nil, "อะไรก็ได้เทส",
		func(_ context.Context, _ model.ToolCall) (string, []model.Image, error) { return "", nil, nil },
		nil, turn.TurnOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled — Stop was pressed during the pause", err)
	}
	if len(provider.seen) != 1 {
		t.Errorf("provider was called %d times, want 1: the retry must not outlive Stop", len(provider.seen))
	}
}
