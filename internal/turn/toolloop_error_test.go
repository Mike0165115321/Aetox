package turn

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// executeAgentToolLoop reported handled=false on error, which is the same
// answer its guard clauses give for "the tool loop does not apply here". execute
// reacts to that by falling through to RespondStream — so a turn that FAILED was
// silently re-sent as a second, non-tool completion. The user's message went
// into the model's context twice, the real error was replaced by whatever the
// retry produced, and one DNS failure cost three provider calls.

// countingAgent records how the executor drove it.
type countingAgent struct {
	toolLoopCalls int
	streamCalls   int
	toolLoopErr   error
	streamReply   string
}

func (a *countingAgent) SupportsToolCalling() bool { return true }

func (a *countingAgent) RespondWithTools(
	_ context.Context, _ []model.ToolDefinition, _ string,
	_ func(context.Context, model.ToolCall) (string, []model.Image, error),
	_ func(string) error, _ TurnOptions,
) (string, bool, error) {
	a.toolLoopCalls++
	return "", false, a.toolLoopErr
}

func (a *countingAgent) RespondStream(
	_ context.Context, _ string, _ func(string) error, _ func(string) error, _ TurnOptions,
) (string, bool, error) {
	a.streamCalls++
	return a.streamReply, true, nil
}

func (a *countingAgent) Respond(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "", nil
}

func (a *countingAgent) RespondEphemeral(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "", nil
}

// oneToolDispatcher is the minimum that opens the tool-loop branch.
type oneToolDispatcher struct{}

func (oneToolDispatcher) Execute(context.Context, string) (skill.Output, bool, error) {
	return skill.Output{}, false, nil
}

func (oneToolDispatcher) ExecuteTool(context.Context, string, map[string]any) (skill.Output, bool, error) {
	return skill.Output{}, false, nil
}

func (oneToolDispatcher) ToolDefinitions() []model.ToolDefinition {
	return []model.ToolDefinition{{
		Type:     "function",
		Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)},
	}}
}

func TestAFailedToolLoopIsNotSilentlyRetriedAsAPlainTurn(t *testing.T) {
	dnsFailure := errors.New("dial tcp: lookup api.deepseek.com: no such host")
	agent := &countingAgent{toolLoopErr: dnsFailure, streamReply: "an answer the user never asked twice for"}
	exec := NewExecutor(ExecutorOptions{Agent: agent, Dispatcher: oneToolDispatcher{}})

	result, err := exec.Execute(
		context.Background(), "แบตผม 17",
		command.Intent{Raw: "แบตผม 17", Kind: command.KindConversation},
		nil, nil, nil,
	)

	if err == nil {
		t.Fatal("the turn reported success after the tool loop failed")
	}
	if !strings.Contains(err.Error(), "no such host") {
		t.Errorf("err = %v; want the tool loop's own failure, not a later one", err)
	}
	if agent.toolLoopCalls != 1 {
		t.Errorf("tool loop ran %d times; want once", agent.toolLoopCalls)
	}
	if agent.streamCalls != 0 {
		t.Errorf("the failed turn was re-sent %d more time(s) as a plain completion — "+
			"that is what put the user's message in the model's context twice", agent.streamCalls)
	}
	if result.Reply != "" {
		t.Errorf("reply = %q; want nothing surfaced from a failed turn", result.Reply)
	}
}

// The guard clauses still mean what they always did: when the tool loop does
// not apply, the conversation path takes over.
func TestTheConversationPathStillTakesOverWhenToolsDoNotApply(t *testing.T) {
	agent := &countingAgent{streamReply: "answered without tools"}
	// No dispatcher at all — one of executeAgentToolLoop's genuine "not
	// applicable" exits.
	exec := NewExecutor(ExecutorOptions{Agent: agent})

	result, err := exec.Execute(
		context.Background(), "hello",
		command.Intent{Raw: "hello", Kind: command.KindConversation},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if agent.streamCalls != 1 {
		t.Errorf("conversation path ran %d times; want once", agent.streamCalls)
	}
	if result.Reply != "answered without tools" {
		t.Errorf("reply = %q", result.Reply)
	}
}
