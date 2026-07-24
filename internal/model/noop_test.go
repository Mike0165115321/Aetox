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
	if resp.Provider != "noop" {
		t.Fatalf("expected provider noop, got %s", resp.Provider)
	}
	if resp.Model != "test-model" {
		t.Fatalf("expected model test-model, got %s", resp.Model)
	}
	if resp.Text != "[noop:test-model] hello" {
		t.Fatalf("unexpected text: %s", resp.Text)
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
	if resp.Text != "[noop:model-x] (empty prompt)" {
		t.Fatalf("unexpected text: %s", resp.Text)
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

	// default model stays a plain echo
	if got := ask("Aetox0.0.1:0b", "สวัสดี").Text; got != "[noop:Aetox0.0.1:0b] สวัสดี" {
		t.Errorf("default model must stay echo, got %q", got)
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
	// scenario keys trigger only as the first word — normal chat stays echo
	if got := ask("ผมชอบ img5 นะ"); !strings.HasPrefix(got, "[noop:test-model]") {
		t.Errorf("mid-sentence keyword must not trigger a scenario, got:\n%s", got)
	}
}
