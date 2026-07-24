package skill

import (
	"context"
	"strings"
	"testing"
)

func TestComputerSkillUsageErrors(t *testing.T) {
	s := &computerSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{}}); err == nil {
		t.Fatal("expected usage error for missing action, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected error for missing action arg, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"action": "dance"}); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("expected unknown-action error, got %v", err)
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"action": "mouse_move"}); err == nil {
		t.Fatal("expected error for mouse_move without coordinates, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"action": "click", "button": "middle"}); err == nil || !strings.Contains(err.Error(), "unknown button") {
		t.Fatalf("expected unknown-button error, got %v", err)
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"action": "type"}); err == nil {
		t.Fatal("expected error for type without text, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"action": "key"}); err == nil {
		t.Fatal("expected error for key without combo, got nil")
	}
}
