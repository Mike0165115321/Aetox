package turn

import (
	"context"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// A turn is a sequence. These pin down that the executor keeps it whole rather
// than reducing it to its last sentence — which is what made narration land in
// a separate panel, made the tool timeline unpersistable, and made fifteen
// places in the loop have to decide which text was "the answer".

// scriptedAgent replays a scripted tool loop through the executor's own
// callbacks, so the assembled sequence is exercised end to end.
type scriptedAgent struct {
	// rounds is what the model "writes" each round; a round with tools calls
	// them before the next round runs.
	rounds []scriptedRound
}

type scriptedRound struct {
	text  string
	tools []string
	final bool
	// demoted is a round the model wrote as its whole answer, which an
	// interjection then kept the turn going past.
	demoted bool
}

func (a *scriptedAgent) SupportsToolCalling() bool { return true }

func (a *scriptedAgent) Respond(context.Context, string, TurnOptions) (string, error) {
	return "", nil
}

func (a *scriptedAgent) RespondEphemeral(context.Context, string, TurnOptions) (string, error) {
	return "", nil
}

func (a *scriptedAgent) RespondStream(context.Context, string, func(string) error, func(string) error, TurnOptions) (string, bool, error) {
	return "", false, nil
}

func (a *scriptedAgent) RespondWithTools(
	ctx context.Context, _ []model.ToolDefinition, _ string,
	execTool func(context.Context, model.ToolCall) (string, []model.Image, error),
	_ func(string) error, opts TurnOptions,
) (string, bool, error) {
	usedTools := false
	reply := ""
	for _, r := range a.rounds {
		if opts.OnRound != nil {
			opts.OnRound(RoundEvent{Text: r.text, Final: r.final, Demoted: r.demoted})
		}
		if r.final {
			reply = r.text
			break
		}
		for i, name := range r.tools {
			usedTools = true
			_, _, _ = execTool(ctx, model.ToolCall{
				ID:       name + string(rune('0'+i)),
				Type:     "function",
				Function: model.FunctionCall{Name: name, Arguments: `{"path":"note.txt"}`},
			})
		}
	}
	return reply, usedTools, nil
}

type twoToolDispatcher struct{}

func (twoToolDispatcher) Execute(context.Context, string) (skill.Output, bool, error) {
	return skill.Output{}, false, nil
}

func (twoToolDispatcher) ExecuteTool(context.Context, string, map[string]any) (skill.Output, bool, error) {
	return skill.Output{Name: "read", Content: "alpha", Success: true}, true, nil
}

func (twoToolDispatcher) ToolDefinitions() []model.ToolDefinition {
	return []model.ToolDefinition{
		{Type: "function", Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)}},
		{Type: "function", Function: model.ToolFunction{Name: "grep", Parameters: []byte(`{"type":"object"}`)}},
	}
}

func runScripted(t *testing.T, rounds ...scriptedRound) Result {
	t.Helper()
	exec := NewExecutor(ExecutorOptions{
		Agent:        &scriptedAgent{rounds: rounds},
		Dispatcher:   twoToolDispatcher{},
		ApprovalMode: "full-access",
	})
	result, err := exec.Execute(
		context.Background(), "อ่านไฟล์ให้หน่อย",
		command.Intent{Raw: "อ่านไฟล์ให้หน่อย", Kind: command.KindConversation},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return result
}

// shape renders a sequence compactly, so a test asserts the ORDER rather than
// twenty field comparisons.
func shape(parts []TurnPart) string {
	var out []string
	for _, p := range parts {
		switch p.Kind {
		case PartText:
			if p.Demoted {
				out = append(out, "answer("+p.Text+")")
				continue
			}
			out = append(out, "text("+p.Text+")")
		case PartThinking:
			out = append(out, "thinking")
		case PartTool:
			out = append(out, "tool("+p.Tool.Name+")")
		}
	}
	return strings.Join(out, " → ")
}

func TestTheTurnKeepsItsNarrationToolsAndAnswerInOrder(t *testing.T) {
	result := runScripted(t,
		scriptedRound{text: "กำลังอ่านไฟล์ให้ครับ", tools: []string{"read"}},
		scriptedRound{text: "ขอ grep ต่ออีกนิด", tools: []string{"grep"}},
		scriptedRound{text: "เจอแล้วครับ อยู่บรรทัดที่ 12", final: true},
	)

	want := "text(กำลังอ่านไฟล์ให้ครับ) → tool(read) → text(ขอ grep ต่ออีกนิด) → tool(grep) → text(เจอแล้วครับ อยู่บรรทัดที่ 12)"
	if got := shape(result.Parts); got != want {
		t.Errorf("sequence =\n  %s\nwant\n  %s", got, want)
	}
}

// Reply is the concatenation of the text parts — not a separate truth. Anything
// that still reads Reply (the model's own context, the store, a sub-agent's
// return value) has to keep seeing a complete answer.
func TestReplyStaysTheAnswerWhileTheSequenceHoldsEverything(t *testing.T) {
	result := runScripted(t,
		scriptedRound{text: "กำลังหาให้ครับ", tools: []string{"read"}},
		scriptedRound{text: "คำตอบสุดท้าย", final: true},
	)

	if result.Reply != "คำตอบสุดท้าย" {
		t.Errorf("Reply = %q; want the closing answer alone", result.Reply)
	}
	// TextOf sees the whole turn, which is deliberately more than Reply: the
	// narration is text the model wrote, and the sequence remembers it.
	if got := TextOf(result.Parts); !strings.Contains(got, "กำลังหาให้ครับ") || !strings.Contains(got, "คำตอบสุดท้าย") {
		t.Errorf("TextOf = %q; want every text part", got)
	}
}

// An answer the user typed over is still an answer.
//
// Owner, 16 ส.ค., on a screenshot of a reply whose `##` and `**` were showing as
// source in grey 11px: "ตอนทักกลางคันมันเหมือนที่ เอไอ ตอบมาก่อนหน้ามันพังหมดเลย".
// It had gone out as a "note" — the channel for a one-line "reading the config
// first" — which the chat draws as raw text at --fs-xs in --text-muted, and then
// files behind the "used N tools" toggle because a text part that is not the
// last one is not the bubble's body. The reply was intact in the record and
// wrecked everywhere it was read.
func TestAnAnswerAnInterjectionRePlacedIsStillDrawnAsAnAnswer(t *testing.T) {
	var events []ToolEvent
	exec := NewExecutor(ExecutorOptions{
		Agent: &scriptedAgent{rounds: []scriptedRound{
			{text: "## สรุป\n\n- ข้อหนึ่ง", demoted: true},
			{text: "กำลังเช็คเพิ่มให้ครับ", tools: []string{"grep"}},
			{text: "เช็คแล้วครับ", final: true},
		}},
		Dispatcher:   twoToolDispatcher{},
		ApprovalMode: "full-access",
		OnToolAction: func(ev ToolEvent) { events = append(events, ev) },
	})
	result, err := exec.Execute(
		context.Background(), "เช็คหน่อย",
		command.Intent{Raw: "เช็คหน่อย", Kind: command.KindConversation},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	want := "answer(## สรุป\n\n- ข้อหนึ่ง) → text(กำลังเช็คเพิ่มให้ครับ) → tool(grep) → text(เช็คแล้วครับ)"
	if got := shape(result.Parts); got != want {
		t.Errorf("sequence =\n  %s\nwant\n  %s", got, want)
	}
	// The narration that follows must not be swallowed into the answer: they are
	// two things the model said, either side of the user cutting in.
	for _, ev := range events {
		if ev.Action == "note" && strings.Contains(ev.Text, "## สรุป") {
			t.Errorf("the answer went out as narration: %+v", ev)
		}
	}
	var said []string
	for _, ev := range events {
		if ev.Action == "said" {
			said = append(said, ev.Text)
		}
	}
	if len(said) != 1 || said[0] != "## สรุป\n\n- ข้อหนึ่ง" {
		t.Errorf("said events = %q; want the demoted answer, whole and once", said)
	}
}

// A turn with no tools is still a sequence — of one part.
func TestAPlainAnswerIsASequenceOfOne(t *testing.T) {
	result := runScripted(t, scriptedRound{text: "ชาร์จไปเรื่อยๆ ครับ", final: true})

	if got := shape(result.Parts); got != "text(ชาร์จไปเรื่อยๆ ครับ)" {
		t.Errorf("sequence = %s", got)
	}
	if result.Reply != "ชาร์จไปเรื่อยๆ ครับ" {
		t.Errorf("Reply = %q", result.Reply)
	}
}

// A tool call carries its own outcome into the transcript, so a reopened
// session can show the work instead of only its conclusion.
func TestAToolPartRemembersWhatTheCallDid(t *testing.T) {
	result := runScripted(t,
		scriptedRound{text: "อ่านให้ครับ", tools: []string{"read"}},
		scriptedRound{text: "เสร็จแล้ว", final: true},
	)

	var tool *ToolPart
	for _, p := range result.Parts {
		if p.Kind == PartTool {
			tool = p.Tool
		}
	}
	if tool == nil {
		t.Fatal("the tool call left no part behind")
	}
	if tool.Name != "read" {
		t.Errorf("name = %q; want read", tool.Name)
	}
	if !tool.OK {
		t.Errorf("a successful call was recorded as failed: %+v", tool)
	}
	if tool.Ref == "" {
		t.Error("the part has no call id — a sub-agent's steps could never be joined back to it")
	}
}

// The loop can end with a reply it wrote itself — the doom-loop stop, loop
// exhaustion, a cancelled tool's output. None of those go through OnRound, so
// without a catch the sequence would omit the only thing the user is told.
func TestALoopWrittenReplyStillReachesTheSequence(t *testing.T) {
	exec := NewExecutor(ExecutorOptions{
		Agent:        &selfWrittenReplyAgent{reply: "หยุดเพราะเรียกเครื่องมือซ้ำ"},
		Dispatcher:   twoToolDispatcher{},
		ApprovalMode: "full-access",
	})
	result, err := exec.Execute(
		context.Background(), "q",
		command.Intent{Raw: "q", Kind: command.KindConversation},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := shape(result.Parts); got != "text(หยุดเพราะเรียกเครื่องมือซ้ำ)" {
		t.Errorf("sequence = %s; want the loop's own reply recorded", got)
	}
}

// selfWrittenReplyAgent returns a reply without ever reporting a round.
type selfWrittenReplyAgent struct{ reply string }

func (a *selfWrittenReplyAgent) SupportsToolCalling() bool { return true }
func (a *selfWrittenReplyAgent) Respond(context.Context, string, TurnOptions) (string, error) {
	return "", nil
}
func (a *selfWrittenReplyAgent) RespondEphemeral(context.Context, string, TurnOptions) (string, error) {
	return "", nil
}
func (a *selfWrittenReplyAgent) RespondStream(context.Context, string, func(string) error, func(string) error, TurnOptions) (string, bool, error) {
	return "", false, nil
}
func (a *selfWrittenReplyAgent) RespondWithTools(
	context.Context, []model.ToolDefinition, string,
	func(context.Context, model.ToolCall) (string, []model.Image, error),
	func(string) error, TurnOptions,
) (string, bool, error) {
	return a.reply, true, nil
}

func TestTextOfJoinsOnlyProse(t *testing.T) {
	parts := []TurnPart{
		{Kind: PartText, Text: "หนึ่ง"},
		{Kind: PartThinking, Secs: 4},
		{Kind: PartTool, Tool: &ToolPart{Name: "read"}},
		{Kind: PartText, Text: "สอง"},
	}
	if got := TextOf(parts); got != "หนึ่ง\n\nสอง" {
		t.Errorf("TextOf = %q; want the prose joined and nothing else", got)
	}
	if got := TextOf(nil); got != "" {
		t.Errorf("TextOf(nil) = %q; want empty", got)
	}
}

// Two text parts in a row mean nothing happened between them — an arbitrary
// seam in the middle of a paragraph would be a rendering artifact, not a fact.
func TestConsecutiveTextMergesIntoOnePart(t *testing.T) {
	list := &partList{}
	list.addText("first")
	list.addText("second")
	list.addTool(ToolPart{Name: "read"})
	list.addText("third")

	if got := shape(list.all()); got != "text(first\n\nsecond) → tool(read) → text(third)" {
		t.Errorf("sequence = %q", got)
	}
}
