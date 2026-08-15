package model

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNoopProviderComplete(t *testing.T) {
	provider := NewNoopProvider("test-model")
	resp, err := provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Provider != "aetox" {
		t.Fatalf("expected provider aetox, got %s", resp.Provider)
	}
	if resp.Model != "test-model" {
		t.Fatalf("expected model test-model, got %s", resp.Model)
	}
	if resp.Text != noopOnboardingReply {
		t.Fatalf("unconfigured install must get the onboarding reply, got: %s", resp.Text)
	}
}

func TestNoopProviderEmptyPrompt(t *testing.T) {
	provider := NewNoopProvider("model-x")
	resp, err := provider.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "   "},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Text != noopOnboardingReply {
		t.Fatalf("empty prompt on an unconfigured install must still get the onboarding reply, got: %s", resp.Text)
	}
}

func TestNoopProviderNoMessages(t *testing.T) {
	provider := NewNoopProvider("model-y")
	_, err := provider.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected ErrNoMessages")
	}
}

func TestNoopProviderTestModels(t *testing.T) {
	ask := func(modelName, text string) Response {
		provider := NewNoopProvider(modelName)
		resp, err := provider.Complete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: text}},
		})
		if err != nil {
			t.Fatalf("complete(%s, %q) failed: %v", modelName, text, err)
		}
		return resp
	}

	// The render model is one bench for one renderer: markdown structure and an
	// image gallery in the same answer. Two models proved the same thing twice.
	render := ask("aetox-render:test", "อะไรก็ได้").Text
	for _, want := range []string{"```go", "| คอลัมน์ |", "> คำพูดยกมา"} {
		if !strings.Contains(render, want) {
			t.Errorf("render model missing %q in:\n%s", want, render)
		}
	}
	if strings.Count(render, "picsum.photos") != 3 {
		t.Errorf("render model must also carry the gallery, got:\n%s", render)
	}
	// The img* keywords still pick one case out of it.
	if got := ask("aetox-render:test", "img5").Text; strings.Count(got, "picsum.photos") != 5 {
		t.Errorf("render model img5 must return 5 images, got:\n%s", got)
	}

	// The names it had when it was two still resolve. A preference file saved
	// before the merge names one of them, and a model id that stops resolving is
	// a chat that stops working.
	for _, retired := range []string{"aetox-image:test", "aetox-markdown:test"} {
		if got := ask(retired, "อะไรก็ได้").Text; !strings.Contains(got, "```go") {
			t.Errorf("%s no longer resolves to the render bench:\n%s", retired, got)
		}
	}

	think := ask("aetox-think:test", "ทำไมฟ้าสีฟ้า")
	if think.ReasoningContent == "" || !strings.Contains(think.Text, "[think-test]") {
		t.Errorf("think model must fill ReasoningContent + short answer, got: %+v", think)
	}

	// the catalog's default noop model is what a genuinely unconfigured
	// install lands on — it must guide the user to Settings, not echo debug text
	if got := ask("aetox-grid", "สวัสดี").Text; got != noopOnboardingReply {
		t.Errorf("default model must return the onboarding reply, got %q", got)
	}
}

func TestNoopStreamDeliversReasoningSeparately(t *testing.T) {
	provider := NewNoopProvider("aetox-think:test")
	var reasoning, text strings.Builder
	resp, err := provider.StreamComplete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "คิดหน่อย"}},
	}, func(chunk string) error {
		text.WriteString(chunk)
		return nil
	}, func(chunk string) error {
		reasoning.WriteString(chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if reasoning.Len() == 0 {
		t.Fatal("reasoning chunks must stream via onReasoningChunk")
	}
	if strings.Contains(text.String(), "reasoning panel") {
		t.Fatal("reasoning text must never leak into the visible answer stream")
	}
	if resp.ReasoningContent == "" || resp.Text == "" {
		t.Fatalf("final response must carry both parts, got %+v", resp)
	}
}

// What is streamed must be what was written. Chunks used to come from
// strings.Fields, which drops the whitespace it splits on, so a streamed
// answer reached the screen with every newline missing — headings ran into
// their paragraphs and a markdown table arrived as one row of pipes, until the
// finished text replaced the stream and the bubble snapped into shape.
func TestNoopStreamRebuildsTheAnswerExactly(t *testing.T) {
	for _, tc := range []struct{ model, prompt string }{
		{"aetox-render:test", "imgmix"},  // markdown: heading, table, list, images
		{"aetox-grid", "สวัสดี"},          // the onboarding reply, Thai, no spaces
		{"aetox-think:test", "คิดหน่อย"}, // both streams at once
	} {
		provider := NewNoopProvider(tc.model)
		var text, reasoning strings.Builder
		start := time.Now()
		resp, err := provider.StreamComplete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: tc.prompt}},
		}, func(chunk string) error { text.WriteString(chunk); return nil },
			func(chunk string) error { reasoning.WriteString(chunk); return nil })
		if err != nil {
			t.Fatalf("%s: stream failed: %v", tc.model, err)
		}
		if text.String() != resp.Text {
			t.Errorf("%s: streamed text != final text\n streamed %d newlines, final has %d",
				tc.model, strings.Count(text.String(), "\n"), strings.Count(resp.Text, "\n"))
		}
		if reasoning.String() != resp.ReasoningContent {
			t.Errorf("%s: streamed reasoning != final reasoning", tc.model)
		}
		// Loose on purpose — sleeps overrun on a loaded machine. It is here to
		// catch a pause that grows with the answer, which is what 40ms a word
		// was: the span is declared, so hold it to roughly the span.
		if took := time.Since(start); took > 4*noopStreamSpan {
			t.Errorf("%s: streaming took %v; the span for two parts is %v", tc.model, took, 2*noopStreamSpan)
		}
	}
}

func TestNoopProviderImageScenarios(t *testing.T) {
	provider := NewNoopProvider("test-model")
	ask := func(text string) string {
		resp, err := provider.Complete(context.Background(), Request{
			Messages: []Message{{Role: RoleUser, Content: text}},
		})
		if err != nil {
			t.Fatalf("complete(%q) failed: %v", text, err)
		}
		return resp.Text
	}

	if got := ask("img5"); strings.Count(got, "https://picsum.photos/") != 5 {
		t.Errorf("img5 must embed 5 images, got:\n%s", got)
	}
	if got := ask("imgbroken"); !strings.Contains(got, "https://aetox.invalid/broken.jpg") {
		t.Errorf("imgbroken must include a dead URL, got:\n%s", got)
	}
	if got := ask("imgmix"); !strings.Contains(got, "|") || strings.Count(got, "picsum.photos") != 3 {
		t.Errorf("imgmix must include a table and 3 images, got:\n%s", got)
	}
	// scenario keys trigger only as the first word — normal chat falls
	// through to the onboarding reply, same as any other unscripted prompt
	if got := ask("ผมชอบ img5 นะ"); got != noopOnboardingReply {
		t.Errorf("mid-sentence keyword must not trigger a scenario, got:\n%s", got)
	}
}

// aetox-tools:test walks a fixed script: todo_write → ask_user → todo_write
// (all done) → final text. Each round is derived from the tool results already
// in the transcript, so the sequence is stateless and deterministic.
func TestNoopToolsModelScriptsToolLoop(t *testing.T) {
	p := NewNoopProvider("aetox-tools:test")
	if !p.SupportsToolCalling() {
		t.Fatal("aetox-tools:test must opt into tool calling")
	}
	if NewNoopProvider("aetox-grid").SupportsToolCalling() {
		t.Fatal("plain aetox models must stay tool-less")
	}

	msgs := []Message{{Role: RoleUser, Content: "เริ่มทดสอบ"}}
	step := func() Response {
		resp, err := p.Complete(context.Background(), Request{Model: "aetox-tools:test", Messages: msgs})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return resp
	}
	feed := func(resp Response, result string) {
		msgs = append(msgs, Message{Role: RoleAssistant, ToolCalls: resp.ToolCalls})
		msgs = append(msgs, Message{
			Role: RoleTool, Name: resp.ToolCalls[0].Function.Name,
			ToolCallID: resp.ToolCalls[0].ID, Content: result,
		})
	}

	r1 := step()
	if len(r1.ToolCalls) != 1 || r1.ToolCalls[0].Function.Name != "todo_write" {
		t.Fatalf("round 1 must call todo_write, got %+v", r1.ToolCalls)
	}
	feed(r1, "todo list updated: 4 items, 1 completed")

	r2 := step()
	if len(r2.ToolCalls) != 1 || r2.ToolCalls[0].Function.Name != "ask_user" {
		t.Fatalf("round 2 must call ask_user, got %+v", r2.ToolCalls)
	}
	feed(r2, "user chose: ขำๆ มีอีโมจิ")

	r3 := step()
	if len(r3.ToolCalls) != 1 || r3.ToolCalls[0].Function.Name != "todo_write" {
		t.Fatalf("round 3 must complete the todos, got %+v", r3.ToolCalls)
	}
	feed(r3, "todo list updated: 4 items, 4 completed")

	r4 := step()
	if len(r4.ToolCalls) != 0 {
		t.Fatalf("round 4 must be the final text, got tool calls %+v", r4.ToolCalls)
	}
	if !strings.Contains(r4.Text, "ขำๆ มีอีโมจิ") {
		t.Fatalf("final text must echo the user's choice, got %q", r4.Text)
	}
}

// A caller handed a trimmed tool set is a sub-agent (internal/subagent
// force-denies todo_write/ask_user to every delegate). The script has to work
// there too, or the built-in model cannot exercise the delegation path at all —
// and system tests are supposed to run on this provider, not on a fake one.
func TestNoopToolsModelScriptsADelegateRound(t *testing.T) {
	p := NewNoopProvider("aetox-tools:test")
	readOnly := []ToolDefinition{
		{Type: "function", Function: ToolFunction{Name: "read"}},
		{Type: "function", Function: ToolFunction{Name: "grep"}},
		{Type: "function", Function: ToolFunction{Name: "list"}},
	}

	r1, err := p.Complete(context.Background(), Request{
		Model: "aetox-tools:test", Tools: readOnly,
		Messages: []Message{{Role: RoleUser, Content: "สำรวจโฟลเดอร์นี้"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r1.ToolCalls) != 1 || r1.ToolCalls[0].Function.Name != "list" {
		t.Fatalf("a delegate round must call a read-only tool it was given, got %+v", r1.ToolCalls)
	}

	r2, err := p.Complete(context.Background(), Request{
		Model: "aetox-tools:test", Tools: readOnly,
		Messages: []Message{
			{Role: RoleUser, Content: "สำรวจโฟลเดอร์นี้"},
			{Role: RoleAssistant, ToolCalls: r1.ToolCalls},
			{Role: RoleTool, Name: "list", ToolCallID: r1.ToolCalls[0].ID, Content: "hay.txt\nnotes.md"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r2.ToolCalls) != 0 {
		t.Fatalf("round 2 must report and stop, got %+v", r2.ToolCalls)
	}
	if !strings.Contains(r2.Text, "hay.txt") {
		t.Fatalf("the delegate must report what the tool returned, got %q", r2.Text)
	}

	// Handed nothing readable, it says so instead of calling something it lacks.
	r3, err := p.Complete(context.Background(), Request{
		Model:    "aetox-tools:test",
		Tools:    []ToolDefinition{{Type: "function", Function: ToolFunction{Name: "web_search"}}},
		Messages: []Message{{Role: RoleUser, Content: "ไปดูให้หน่อย"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r3.ToolCalls) != 0 || r3.Text == "" {
		t.Fatalf("with no read-only tool it must answer in text, got %+v / %q", r3.ToolCalls, r3.Text)
	}
}

// The other side of the same coin: a caller holding `task` is a parent, and the
// built-in model has to be able to drive delegation too — otherwise the CLI, which
// has no todo_write/ask_user, can never exercise §44 without an API key.
func TestNoopToolsModelDelegatesAndCollects(t *testing.T) {
	p := NewNoopProvider("aetox-tools:test")
	parentTools := []ToolDefinition{
		{Type: "function", Function: ToolFunction{Name: "read"}},
		{Type: "function", Function: ToolFunction{Name: "task"}},
		{Type: "function", Function: ToolFunction{Name: "task_result"}},
	}
	const brief = "ส่งงานให้ subagent general ไปดูหน่อย"
	msgs := []Message{{Role: RoleUser, Content: brief}}
	step := func() Response {
		resp, err := p.Complete(context.Background(), Request{
			Model: "aetox-tools:test", Tools: parentTools, Messages: msgs,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return resp
	}
	feed := func(resp Response, result string) {
		msgs = append(msgs, Message{Role: RoleAssistant, ToolCalls: resp.ToolCalls})
		msgs = append(msgs, Message{
			Role: RoleTool, Name: resp.ToolCalls[0].Function.Name,
			ToolCallID: resp.ToolCalls[0].ID, Content: result,
		})
	}

	r1 := step()
	if len(r1.ToolCalls) != 1 || r1.ToolCalls[0].Function.Name != "task" {
		t.Fatalf("round 1 must delegate, got %+v", r1.ToolCalls)
	}
	if !strings.Contains(r1.ToolCalls[0].Function.Arguments, `"general"`) {
		t.Errorf("the brief named general, so the script must pick it: %s", r1.ToolCalls[0].Function.Arguments)
	}
	// Word for word what task.go hands back, so the id is parsed the way it will be.
	feed(r1, `started sub-agent general as task_2 — it is running now. Do other work, then call task_result with task_id "task_2" to collect it.`)

	r2 := step()
	if len(r2.ToolCalls) != 1 || r2.ToolCalls[0].Function.Name != "task_result" {
		t.Fatalf("round 2 must collect, got %+v", r2.ToolCalls)
	}
	// The id has to be read out of the handle, not assumed: "task_1" would be wrong
	// here, and would collect somebody else's delegate in a two-task turn.
	if !strings.Contains(r2.ToolCalls[0].Function.Arguments, `"task_2"`) {
		t.Fatalf("round 2 collected the wrong id: %s", r2.ToolCalls[0].Function.Arguments)
	}
	feed(r2, "hay.txt\nnotes.md\n[task general: 1 tool calls, 0.2s]")

	r3 := step()
	if len(r3.ToolCalls) != 0 {
		t.Fatalf("round 3 must report and stop, got %+v", r3.ToolCalls)
	}
	if !strings.Contains(r3.Text, "hay.txt") {
		t.Fatalf("the parent must report what the delegate returned, got %q", r3.Text)
	}

	// Without the keyword it stays a one-round delegate script — delegating on every
	// prompt would make every other tools-test a three-round affair.
	plain, err := p.Complete(context.Background(), Request{
		Model: "aetox-tools:test", Tools: parentTools,
		Messages: []Message{{Role: RoleUser, Content: "สวัสดี"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plain.ToolCalls) == 1 && plain.ToolCalls[0].Function.Name == "task" {
		t.Error("delegation must be opt-in by keyword, not the default round")
	}
}

// aetox-think:test must produce a LONG multi-section reasoning stream — it is
// the workout for the reasoning panel's unbounded height and auto-scroll.
func TestNoopThinkModelProducesLongSectionedReasoning(t *testing.T) {
	p := NewNoopProvider("aetox-think:test")
	resp, err := p.Complete(context.Background(), Request{
		Model:    "aetox-think:test",
		Messages: []Message{{Role: RoleUser, Content: "ทดสอบ"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ReasoningContent) < 1000 {
		t.Fatalf("reasoning too short to exercise the panel: %d chars", len(resp.ReasoningContent))
	}
	if !strings.Contains(resp.ReasoningContent, "[1/6]") || !strings.Contains(resp.ReasoningContent, "[6/6]") {
		t.Fatal("reasoning must keep its numbered sections")
	}
}

// The learning floor (§82) had no keyless bench at all: `memory` is the only
// way anything reaches the approval queue, and nothing could call it without an
// API key. Opt-in per brief rather than a seventh model in the picker — a list
// nobody reads to the bottom is not a bench.
func TestNoopToolsModelProposesSomethingToRemember(t *testing.T) {
	provider := NewNoopProvider("aetox-tools:test")
	tools := []ToolDefinition{
		{Type: "function", Function: ToolFunction{Name: "memory"}},
		{Type: "function", Function: ToolFunction{Name: "todo_write"}},
		{Type: "function", Function: ToolFunction{Name: "ask_user"}},
	}

	first, err := provider.Complete(context.Background(), Request{
		Model:    "aetox-tools:test",
		Tools:    tools,
		Messages: []Message{{Role: RoleUser, Content: "memory: ลองระบบจำ"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].Function.Name != "memory" {
		t.Fatalf("a memory brief must open with a memory call, got %+v", first.ToolCalls)
	}
	if !strings.Contains(first.ToolCalls[0].Function.Arguments, `"why"`) {
		t.Errorf("the proposal must carry its reasoning — the review page shows it: %s",
			first.ToolCalls[0].Function.Arguments)
	}

	// The reported answer is the point of the second round: `memory` says
	// "queued for approval", never "saved", and a bench that hid that would let
	// the tool start claiming the write with nobody noticing.
	second, err := provider.Complete(context.Background(), Request{
		Model: "aetox-tools:test",
		Tools: tools,
		Messages: []Message{
			{Role: RoleUser, Content: "memory: ลองระบบจำ"},
			{Role: RoleTool, Name: "memory", Content: "Queued for the user to approve."},
		},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(second.ToolCalls) != 0 {
		t.Fatalf("the memory script is one round, got another call: %+v", second.ToolCalls)
	}
	if !strings.Contains(second.Text, "approve") {
		t.Errorf("the report must repeat what the tool actually answered:\n%s", second.Text)
	}
}

// Without the brief it stays the tool-UI script it always was — one keyword
// must not cost every other bench a round.
func TestAMemorylessBriefKeepsTheOldToolsScript(t *testing.T) {
	provider := NewNoopProvider("aetox-tools:test")
	resp, err := provider.Complete(context.Background(), Request{
		Model: "aetox-tools:test",
		Tools: []ToolDefinition{
			{Type: "function", Function: ToolFunction{Name: "memory"}},
			{Type: "function", Function: ToolFunction{Name: "todo_write"}},
			{Type: "function", Function: ToolFunction{Name: "ask_user"}},
		},
		Messages: []Message{{Role: RoleUser, Content: "ทดสอบ UI"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Function.Name != "todo_write" {
		t.Fatalf("want the usual todo_write opener, got %+v", resp.ToolCalls)
	}
}

// A tool it was never handed is a call that dies at dispatch — the script has
// to read the tool list the way a real model does, memory included.
func TestTheMemoryScriptIsSkippedWhenTheToolIsNotOffered(t *testing.T) {
	provider := NewNoopProvider("aetox-tools:test")
	resp, err := provider.Complete(context.Background(), Request{
		Model: "aetox-tools:test",
		Tools: []ToolDefinition{
			{Type: "function", Function: ToolFunction{Name: "todo_write"}},
			{Type: "function", Function: ToolFunction{Name: "ask_user"}},
		},
		Messages: []Message{{Role: RoleUser, Content: "memory: ลองระบบจำ"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	for _, c := range resp.ToolCalls {
		if c.Function.Name == "memory" {
			t.Fatal("called memory without being handed it")
		}
	}
}

// The delegate half of the same bench. Which file a proposal lands in is the
// claim the whole flat-context design rests on, and a delegate writing into the
// main agent's memory would look identical from the outside.
func TestADelegateBriefedWithMemoryProposesToo(t *testing.T) {
	provider := NewNoopProvider("aetox-tools:test")
	resp, err := provider.Complete(context.Background(), Request{
		Model: "aetox-tools:test",
		// No todo_write/ask_user: that is what marks the caller as a delegate.
		Tools: []ToolDefinition{
			{Type: "function", Function: ToolFunction{Name: "memory"}},
			{Type: "function", Function: ToolFunction{Name: "list"}},
		},
		Messages: []Message{{Role: RoleUser, Content: "memory: จำเรื่องนี้ไว้"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Function.Name != "memory" {
		t.Fatalf("a delegate briefed with memory must propose, got %+v", resp.ToolCalls)
	}
}
