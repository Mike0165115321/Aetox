package model

import (
	"context"
	"strings"
	"testing"
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

	// image model: any prompt returns the gallery showcase...
	if got := ask("aetox-image:test", "สวัสดี").Text; strings.Count(got, "picsum.photos") != 3 {
		t.Errorf("image model must reply with the 3-image showcase, got:\n%s", got)
	}
	// ...and the img* keywords still pick specific cases
	if got := ask("aetox-image:test", "img5").Text; strings.Count(got, "picsum.photos") != 5 {
		t.Errorf("image model img5 must return 5 images, got:\n%s", got)
	}

	think := ask("aetox-think:test", "ทำไมฟ้าสีฟ้า")
	if think.ReasoningContent == "" || !strings.Contains(think.Text, "[think-test]") {
		t.Errorf("think model must fill ReasoningContent + short answer, got: %+v", think)
	}

	md := ask("aetox-markdown:test", "อะไรก็ได้").Text
	for _, want := range []string{"```go", "| คอลัมน์ |", "> คำพูดยกมา"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown model missing %q in:\n%s", want, md)
		}
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
