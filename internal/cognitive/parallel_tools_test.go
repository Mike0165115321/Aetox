package cognitive

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/think"
	"github.com/Mikedev115/Aetox/internal/turn"
)

func toolCall(id, name, args string) model.ToolCall {
	return model.ToolCall{ID: id, Type: "function", Function: model.FunctionCall{Name: name, Arguments: args}}
}

func TestParallelGroup(t *testing.T) {
	cases := []struct {
		name  string
		calls []model.ToolCall
		want  int
	}{
		{"one read alone", []model.ToolCall{toolCall("1", "read", "{}")}, 1},
		{"three reads", []model.ToolCall{
			toolCall("1", "read", "{}"), toolCall("2", "grep", "{}"), toolCall("3", "glob", "{}"),
		}, 3},
		// The cap is a real ceiling, not a suggestion: seven reads run five then two.
		{"capped at five", []model.ToolCall{
			toolCall("1", "read", "{}"), toolCall("2", "read", "{}"), toolCall("3", "read", "{}"),
			toolCall("4", "read", "{}"), toolCall("5", "read", "{}"), toolCall("6", "read", "{}"),
			toolCall("7", "read", "{}"),
		}, 5},
		// The write ends the group. Reading again after a write is a different
		// question from reading before it, and the answer depends on the order.
		{"stops at a write", []model.ToolCall{
			toolCall("1", "read", "{}"), toolCall("2", "write", "{}"), toolCall("3", "read", "{}"),
		}, 1},
		{"a mutating call leads alone", []model.ToolCall{
			toolCall("1", "shell", "{}"), toolCall("2", "read", "{}"),
		}, 1},
		// An unknown name is the case the allow-list exists for: a tool nobody
		// here has classified (every MCP tool is one) runs by itself.
		{"unknown tool is never grouped", []model.ToolCall{
			toolCall("1", "jira_create_issue", "{}"), toolCall("2", "jira_create_issue", "{}"),
		}, 1},
		{"case and spacing do not smuggle a name in", []model.ToolCall{
			toolCall("1", " Read ", "{}"), toolCall("2", "GREP", "{}"),
		}, 2},
	}
	for _, tc := range cases {
		if got := parallelGroup(tc.calls); got != tc.want {
			t.Errorf("%s: parallelGroup = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// Three reads asked for in one round must actually overlap. Proven by making
// each call wait for the other two to arrive: under the old sequential loop the
// first one waits forever and this fails on the deadline instead of passing
// slowly.
func TestReadsInOneRoundRunTogether(t *testing.T) {
	provider := &toolLoopProvider{responses: []model.Response{
		{ToolCalls: []model.ToolCall{
			toolCall("call_1", "read", `{"path":"a.txt"}`),
			toolCall("call_2", "read", `{"path":"b.txt"}`),
			toolCall("call_3", "read", `{"path":"c.txt"}`),
		}},
		{Text: "อ่านครบแล้วครับ"},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	var mu sync.Mutex
	arrived := 0
	all := make(chan struct{})
	exec := func(ctx context.Context, call model.ToolCall) (string, []model.Image, error) {
		mu.Lock()
		arrived++
		if arrived == 3 {
			close(all)
		}
		mu.Unlock()
		select {
		case <-all:
		case <-time.After(5 * time.Second):
			return "", nil, context.DeadlineExceeded
		}
		return "content of " + call.Function.Arguments, nil, nil
	}

	reply, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}}},
		"อ่านสามไฟล์นี้ให้ที",
		exec, nil,
		turn.TurnOptions{ThinkLevel: think.LevelMedium},
	)
	if err != nil {
		t.Fatalf("RespondWithTools: %v", err)
	}
	if reply != "อ่านครบแล้วครับ" {
		t.Fatalf("reply = %q", reply)
	}

	// However they finished, the history reads in the order the model wrote the
	// calls — a provider handed its tool_calls back out of order is a provider
	// that will one day reject the request.
	var results []string
	for _, m := range agent.context.Messages() {
		if m.Role == model.RoleTool {
			results = append(results, m.ToolCallID)
		}
	}
	want := []string{"call_1", "call_2", "call_3"}
	if len(results) != len(want) {
		t.Fatalf("tool results = %v, want %v", results, want)
	}
	for i := range want {
		if results[i] != want[i] {
			t.Fatalf("tool results = %v, want %v", results, want)
		}
	}
}

// A write between two reads keeps all three in the order the model asked for.
// This is the guarantee the grouping must not trade away for speed.
func TestAWriteBetweenReadsStaysOrdered(t *testing.T) {
	provider := &toolLoopProvider{responses: []model.Response{
		{ToolCalls: []model.ToolCall{
			toolCall("call_1", "read", `{"path":"a.txt"}`),
			toolCall("call_2", "write", `{"path":"a.txt"}`),
			toolCall("call_3", "read", `{"path":"a.txt"}`),
		}},
		{Text: "เสร็จแล้วครับ"},
	}}
	agent := NewAgent(AgentConfig{Provider: provider, Model: "test-model"})

	var mu sync.Mutex
	var order []string
	running := 0
	exec := func(_ context.Context, call model.ToolCall) (string, []model.Image, error) {
		mu.Lock()
		running++
		if running > 1 {
			mu.Unlock()
			return "", nil, context.Canceled // two ran at once: the failure this test is for
		}
		order = append(order, call.ID)
		mu.Unlock()
		time.Sleep(2 * time.Millisecond)
		mu.Lock()
		running--
		mu.Unlock()
		return "ok", nil, nil
	}

	if _, _, err := agent.RespondWithTools(
		context.Background(),
		[]model.ToolDefinition{
			{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}},
			{Type: "function", Function: model.ToolFunction{Name: "write", Parameters: []byte(`{"type":"object"}`)}},
		},
		"อ่าน แก้ แล้วอ่านซ้ำ",
		exec, nil,
		turn.TurnOptions{ThinkLevel: think.LevelMedium},
	); err != nil {
		t.Fatalf("RespondWithTools: %v", err)
	}

	want := []string{"call_1", "call_2", "call_3"}
	if len(order) != len(want) {
		t.Fatalf("ran %v, want %v one at a time", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("ran %v, want %v", order, want)
		}
	}
}
