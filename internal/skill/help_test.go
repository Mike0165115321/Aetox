package skill

import (
	"context"
	"strings"
	"testing"
)

func TestHelpSkillListsRegisteredSkills(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubSkill{name: "alpha"}, SourceBuiltin); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := r.Register(&stubSkill{name: "beta"}, SourceBuiltin); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &helpSkill{registry: r}

	out, err := s.Execute(context.Background(), Input{})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "alpha") || !strings.Contains(out.Content, "beta") {
		t.Errorf("Content = %q, want to contain both registered skill names", out.Content)
	}
}

func TestHelpSkillEmptyRegistry(t *testing.T) {
	s := &helpSkill{registry: NewRegistry()}
	out, err := s.Execute(context.Background(), Input{})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "ไม่มีคำสั่งให้แสดง") {
		t.Errorf("Content = %q, want the empty-registry message", out.Content)
	}
}

func TestHelpSkillNilRegistry(t *testing.T) {
	s := &helpSkill{registry: nil}
	if _, err := s.Execute(context.Background(), Input{}); err != nil {
		t.Fatalf("Execute with nil registry: unexpected error: %v", err)
	}
}
