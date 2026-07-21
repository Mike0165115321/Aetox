package skill

import (
	"context"
	"testing"
	"time"
)

func TestTimeSkillExecute(t *testing.T) {
	s := &timeSkill{}
	out, err := s.Execute(context.Background(), Input{})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if _, parseErr := time.Parse("2006-01-02 15:04:05 MST", out.Content); parseErr != nil {
		t.Errorf("Content = %q, not parseable as expected timestamp format: %v", out.Content, parseErr)
	}
}

func TestTimeSkillExecuteToolRejectsArgs(t *testing.T) {
	s := &timeSkill{}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"unexpected": "x"}); err == nil {
		t.Fatal("expected error for time with arguments, got nil")
	}
}

func TestTimeSkillExecuteToolNoArgs(t *testing.T) {
	s := &timeSkill{}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
}
