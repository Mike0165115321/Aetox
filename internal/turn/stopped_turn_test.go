package turn

import (
	"context"
	"errors"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// Stop used to delete the turn it stopped.
//
// executeAgentToolLoop returned `Result{}` on every error, so the sequence it
// had spent the whole turn assembling — each tool call, each line of narration
// between them — went out with the error. desktop's appendFailedTurn stored the
// empty result, and the conversation reopened as a question with nothing under
// it: twenty minutes of work, and the record of it deleted by the act of
// stopping.
//
// The wall is the same wall for every ending: a Stop, a dropped connection, a
// spent quota. A turn that hit one did not do less work than a turn that
// finished.
type stoppedMidWorkAgent struct{}

func (stoppedMidWorkAgent) SupportsToolCalling() bool { return true }

func (stoppedMidWorkAgent) RespondWithTools(
	ctx context.Context, _ []model.ToolDefinition, _ string,
	execTool func(context.Context, model.ToolCall) (string, []model.Image, error),
	_ func(string) error, opts TurnOptions,
) (string, bool, error) {
	read := func(id, path string) {
		_, _, _ = execTool(ctx, model.ToolCall{
			ID:       id,
			Function: model.FunctionCall{Name: "read", Arguments: `{"path":"` + path + `"}`},
		})
	}
	opts.OnRound(RoundEvent{Text: "กำลังไล่ดูพอร์ตให้ครับ"})
	read("call_1", "a.txt")
	opts.OnRound(RoundEvent{Text: "ยังไม่เจอ ขอดูอีกไฟล์"})
	read("call_2", "b.txt")
	// What the loop really hands back when a tool is killed mid-run: the
	// cancelled call's own output, which is the error string.
	return "context canceled", true, context.Canceled
}

func (stoppedMidWorkAgent) RespondStream(
	_ context.Context, _ string, _ func(string) error, _ func(string) error, _ TurnOptions,
) (string, bool, error) {
	return "", false, errors.New("the conversation path must not run for a stopped tool turn")
}

func (stoppedMidWorkAgent) Respond(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "", nil
}

func (stoppedMidWorkAgent) RespondEphemeral(_ context.Context, _ string, _ TurnOptions) (string, error) {
	return "", nil
}

func TestAStoppedTurnKeepsTheWorkItDid(t *testing.T) {
	exec := NewExecutor(ExecutorOptions{Agent: stoppedMidWorkAgent{}, Dispatcher: oneToolDispatcher{}})

	result, err := exec.Execute(
		context.Background(), "เปิดโปรเจกต์ให้ที",
		command.Intent{Raw: "เปิดโปรเจกต์ให้ที", Kind: command.KindConversation},
		nil, nil, nil,
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want the cancellation to reach the caller", err)
	}
	var kinds []PartKind
	for _, p := range result.Parts {
		kinds = append(kinds, p.Kind)
	}
	want := []PartKind{PartText, PartTool, PartText, PartTool}
	if len(kinds) != len(want) {
		t.Fatalf("parts = %v, want %v — the stopped turn came back without its work", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("parts = %v, want %v", kinds, want)
		}
	}
	if result.Parts[0].Text != "กำลังไล่ดูพอร์ตให้ครับ" {
		t.Errorf("first narration = %q", result.Parts[0].Text)
	}
	if result.Parts[1].Tool == nil || result.Parts[1].Tool.Ref != "call_1" {
		t.Errorf("first tool part = %+v; want the call that ran", result.Parts[1].Tool)
	}
	// The last thing the model said, never the cancelled tool's error string:
	// a bubble reading "context canceled" is the app blaming itself for obeying.
	if result.Reply != "ยังไม่เจอ ขอดูอีกไฟล์" {
		t.Errorf("reply = %q; want the last sentence the model wrote", result.Reply)
	}
	if result.Status != TurnStatusError {
		t.Errorf("status = %q; want %q", result.Status, TurnStatusError)
	}
}
