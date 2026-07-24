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
