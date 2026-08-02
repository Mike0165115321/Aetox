package cognitive

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// The owner's report: DeepSeek's thinking streamed live but its answer landed
// in one silent jump, because completeToolLoop passed a hardcoded nil content
// handler. These tests pin down the two halves of the fix — that the answer now
// streams, and that anything which is NOT the answer is taken back off screen.

// previewRecorder stands in for the desktop's live answer bubble.
type previewRecorder struct {
	shown  strings.Builder
	resets int
	// rounds is what the agent reported as the turn's sequence. Narration and
	// the closing answer both arrive here; the executor is what turns them into
	// parts, so at this level they are simply reported, never retracted.
	rounds []turn.RoundEvent
	// log records the order of events, so a test can assert that an erase
	// happened between two writes rather than merely that both occurred.
	log []string
}

func (p *previewRecorder) opts() turn.TurnOptions {
	return turn.TurnOptions{
		OnContent: func(chunk string) {
			p.shown.WriteString(chunk)
			p.log = append(p.log, "write:"+chunk)
		},
		OnContentReset: func() {
			p.shown.Reset()
			p.resets++
			p.log = append(p.log, "reset")
		},
		OnRound: func(r turn.RoundEvent) {
			p.rounds = append(p.rounds, r)
			p.log = append(p.log, "round:"+r.Text)
		},
	}
}

// nonFinalRounds is what the turn said along the way — everything but the reply.
func (p *previewRecorder) nonFinalRounds() []string {
	var out []string
	for _, r := range p.rounds {
		if !r.Final {
			out = append(out, r.Text)
		}
	}
	return out
}

// contentStreamProvider emits each response's text as deltas before returning
// it, the way a real streaming provider does.
type contentStreamProvider struct {
	responses  []model.Response
	deltaRunes int // 0 = deliver each response's text in one delta
	sawNilBoth bool
	// duringStream fires while a call is in flight, so a test can do what the
	// user does: type something while the model is still writing.
	duringStream func()
}

func (p *contentStreamProvider) Name() string              { return "deepseek" }
func (p *contentStreamProvider) SupportsToolCalling() bool { return true }
func (p *contentStreamProvider) SupportsReasoning() bool   { return true }

func (p *contentStreamProvider) next() model.Response {
	if len(p.responses) == 0 {
		return model.Response{Text: "done"}
	}
	r := p.responses[0]
	p.responses = p.responses[1:]
	return r
}

func (p *contentStreamProvider) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	return p.next(), nil
}

func (p *contentStreamProvider) StreamComplete(_ context.Context, _ model.Request, onChunk model.StreamChunkHandler, onReasoningChunk model.StreamChunkHandler) (model.Response, error) {
	resp := p.next()
	if onChunk == nil {
		p.sawNilBoth = true
	}
	if onReasoningChunk != nil && resp.ReasoningContent != "" {
		if err := onReasoningChunk(resp.ReasoningContent); err != nil {
			return model.Response{}, err
		}
	}
	if onChunk != nil && resp.Text != "" {
		for _, delta := range splitRunes(resp.Text, p.deltaRunes) {
			if err := onChunk(delta); err != nil {
				return model.Response{}, err
			}
		}
	}
	if p.duringStream != nil {
		p.duringStream()
		p.duringStream = nil // first round only
	}
	return resp, nil
}

func splitRunes(s string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	var out []string
	runes := []rune(s)
	for i := 0; i < len(runes); i += n {
		end := i + n
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[i:end]))
	}
	return out
}

func newStreamAgent(p model.Provider) *Agent {
	return NewAgent(AgentConfig{Provider: p, Model: "test-model", MaxToolCalls: 6})
}

var readTool = []model.ToolDefinition{{
	Type:     "function",
	Function: model.ToolFunction{Name: "read", Parameters: []byte(`{"type":"object"}`)},
}}

func noTools(_ context.Context, _ model.ToolCall) (string, []model.Image, error) {
	return `{"tool":"read","status":"done","output":"alpha"}`, nil, nil
}

// The headline fix: a plain answer now arrives as it is written.
func TestToolLoopStreamsTheAnswer(t *testing.T) {
	provider := &contentStreamProvider{
		responses:  []model.Response{{Text: "ชาร์จไปเรื่อยๆ ครับ"}},
		deltaRunes: 3,
	}
	preview := &previewRecorder{}
	agent := newStreamAgent(provider)

	reply, _, err := agent.RespondWithTools(
		context.Background(), readTool, "ทำไมแบตขึ้นช้า", noTools,
		func(string) error { return nil }, preview.opts(),
	)
	if err != nil {
		t.Fatalf("RespondWithTools: %v", err)
	}
	if reply != "ชาร์จไปเรื่อยๆ ครับ" {
		t.Fatalf("reply = %q", reply)
	}
	if provider.sawNilBoth {
		t.Error("the provider was still given a nil content handler — nothing would stream")
	}
	// More than one write: the answer arrived progressively, not in one jump.
	writes := 0
	for _, e := range preview.log {
		if strings.HasPrefix(e, "write:") {
			writes++
		}
	}
	if writes < 2 {
		t.Errorf("preview took %d writes; want the answer to arrive progressively", writes)
	}
	if got := preview.shown.String(); got != reply {
		t.Errorf("preview showed %q; want it to match the reply %q", got, reply)
	}
}

// Without a preview wired (the CLI), nothing changes: the provider still gets a
// nil content handler and no chunk is produced.
func TestToolLoopKeepsContentSilentWhenNoPreviewIsWanted(t *testing.T) {
	provider := &contentStreamProvider{responses: []model.Response{{Text: "answer"}}}
	agent := newStreamAgent(provider)

	if _, _, err := agent.RespondWithTools(
		context.Background(), readTool, "q", noTools,
		func(string) error { return nil }, turn.TurnOptions{},
	); err != nil {
		t.Fatalf("RespondWithTools: %v", err)
	}
	if !provider.sawNilBoth {
		t.Error("a caller that asked for no preview was still streamed content")
	}
}

// Narration in front of a tool call is reported as a non-final round, so the
// executor can put it in the sequence where it was said. It is NOT retracted:
// the model really did say it, and taking it back was only ever necessary
// because a turn could hold one string.
func TestNarrationIsReportedAsPartOfTheTurn(t *testing.T) {
	provider := &contentStreamProvider{responses: []model.Response{
		{
			Text: "กำลังอ่านไฟล์ให้ครับ",
			ToolCalls: []model.ToolCall{{
				ID: "1", Type: "function",
				Function: model.FunctionCall{Name: "read", Arguments: `{}`},
			}},
		},
		{Text: "ไฟล์มีข้อความว่า alpha"},
	}}
	preview := &previewRecorder{}
	agent := newStreamAgent(provider)

	reply, usedTools, err := agent.RespondWithTools(
		context.Background(), readTool, "อ่านไฟล์ให้หน่อย", noTools,
		func(string) error { return nil }, preview.opts(),
	)
	if err != nil || !usedTools {
		t.Fatalf("RespondWithTools: reply=%q usedTools=%v err=%v", reply, usedTools, err)
	}
	narration := preview.nonFinalRounds()
	if len(narration) != 1 || narration[0] != "กำลังอ่านไฟล์ให้ครับ" {
		t.Fatalf("non-final rounds = %q; want the narration reported once", narration)
	}
	last := preview.rounds[len(preview.rounds)-1]
	if !last.Final || last.Text != "ไฟล์มีข้อความว่า alpha" {
		t.Errorf("last round = %+v; want the answer marked final", last)
	}
	// Both are in the sequence, in the order they happened — the tool call
	// belongs between them, which is what the executor assembles.
	if preview.log[0] != "write:กำลังอ่านไฟล์ให้ครับ" {
		t.Errorf("first event was %q; want the narration to have streamed first", preview.log[0])
	}
}

// The sharpest case in the loop: the model wrote a complete answer, the user
// typed under it, and the turn keeps running. Under the old single-string model
// that answer had to be taken off screen. As a part it simply becomes a
// non-final round — said, kept, and followed by the real reply.
func TestAnInterjectedOverAnswerStaysInTheTurn(t *testing.T) {
	provider := &contentStreamProvider{responses: []model.Response{
		{Text: "คำตอบแรกที่ถูกลดชั้น"},
		{Text: "คำตอบจริงหลังผู้ใช้พิมพ์แทรก"},
	}}
	preview := &previewRecorder{}
	agent := newStreamAgent(provider)
	// Typed WHILE the first answer is being written — the loop drains its
	// buffer at the top of each round, so anything queued earlier would simply
	// be folded into that round's request instead of demoting its answer.
	provider.duringStream = func() { agent.Interject("เดี๋ยว เปลี่ยนคำถาม") }

	reply, _, err := agent.RespondWithTools(
		context.Background(), readTool, "คำถามแรก", noTools,
		func(string) error { return nil }, preview.opts(),
	)
	if err != nil {
		t.Fatalf("RespondWithTools: %v", err)
	}
	if reply != "คำตอบจริงหลังผู้ใช้พิมพ์แทรก" {
		t.Fatalf("reply = %q; want the answer from after the interjection", reply)
	}
	demoted := preview.nonFinalRounds()
	if len(demoted) != 1 || demoted[0] != "คำตอบแรกที่ถูกลดชั้น" {
		t.Fatalf("non-final rounds = %q; want the demoted answer kept in the sequence", demoted)
	}
	last := preview.rounds[len(preview.rounds)-1]
	if !last.Final || last.Text != "คำตอบจริงหลังผู้ใช้พิมพ์แทรก" {
		t.Errorf("last round = %+v; want the post-interjection answer marked final", last)
	}
}

// A round the loop nudges away for leaking markup produced prose too — and that
// prose is not the answer either.
func TestPreviewIsErasedWhenARoundLeaksMarkup(t *testing.T) {
	provider := &contentStreamProvider{responses: []model.Response{
		{Text: "กำลังสร้างไฟล์ให้ครับ\n<｜DSML｜invoke name=\"write\">"},
		{Text: "เขียนไฟล์เรียบร้อยครับ"},
	}}
	preview := &previewRecorder{}
	agent := newStreamAgent(provider)

	reply, _, err := agent.RespondWithTools(
		context.Background(), readTool, "สร้างไฟล์ให้หน่อย", noTools,
		func(string) error { return nil }, preview.opts(),
	)
	if err != nil {
		t.Fatalf("RespondWithTools: %v", err)
	}
	if reply != "เขียนไฟล์เรียบร้อยครับ" {
		t.Fatalf("reply = %q; want the answer from after the nudge", reply)
	}
	// The gate kept the markup itself out, and the reset took the doomed prose
	// with it.
	if strings.Contains(preview.shown.String(), "DSML") {
		t.Fatal("leaked markup reached the preview — the gate failed")
	}
	if strings.Contains(preview.shown.String(), "กำลังสร้างไฟล์") {
		t.Error("prose from the nudged-away round survived into the answer")
	}
}

// A half-written answer must not outlive the failure that interrupted it.
func TestPreviewIsErasedWhenTheRoundFails(t *testing.T) {
	provider := &failAfterStreamingProvider{}
	preview := &previewRecorder{}
	agent := newStreamAgent(provider)

	_, _, _ = agent.RespondWithTools(
		context.Background(), readTool, "q", noTools,
		func(string) error { return nil }, preview.opts(),
	)
	if preview.resets == 0 {
		t.Error("a failed round left its half-written answer on screen")
	}
}

// failAfterStreamingProvider writes part of an answer, then dies — a dropped
// connection mid-generation.
type failAfterStreamingProvider struct{ calls int }

func (p *failAfterStreamingProvider) Name() string              { return "deepseek" }
func (p *failAfterStreamingProvider) SupportsToolCalling() bool { return true }
func (p *failAfterStreamingProvider) SupportsReasoning() bool   { return true }

func (p *failAfterStreamingProvider) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	return model.Response{}, errors.New("dial tcp: lookup api.deepseek.com: no such host")
}

func (p *failAfterStreamingProvider) StreamComplete(_ context.Context, _ model.Request, onChunk model.StreamChunkHandler, _ model.StreamChunkHandler) (model.Response, error) {
	p.calls++
	if onChunk != nil {
		_ = onChunk("คำตอบที่เขียนได้ครึ่งเดียว")
	}
	return model.Response{}, errors.New("dial tcp: lookup api.deepseek.com: no such host")
}
