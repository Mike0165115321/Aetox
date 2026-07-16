package model

import (
	"context"
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
