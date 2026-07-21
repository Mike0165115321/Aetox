package skill

import (
	"context"
	"testing"
)

func TestEchoSkillExecute(t *testing.T) {
	s := &echoSkill{}
	out, err := s.Execute(context.Background(), Input{"args": []string{"hello", "world"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Content != "hello world" {
		t.Errorf("Content = %q, want %q", out.Content, "hello world")
	}
}

func TestEchoSkillEmptyArgs(t *testing.T) {
	s := &echoSkill{}
	out, err := s.Execute(context.Background(), Input{"args": nil})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Content != "(no output)" {
		t.Errorf("Content = %q, want %q (newToolOutput fills empty content)", out.Content, "(no output)")
	}
}
