package skill

import (
	"context"
	"testing"
)

type stubSkill struct{ name string }

func (s *stubSkill) Name() string        { return s.name }
func (s *stubSkill) Description() string { return "stub" }
func (s *stubSkill) Execute(ctx context.Context, input Input) (Output, error) {
	return Output{Name: s.name}, nil
}

func TestRegisterTracksSource(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubSkill{name: "read"}, SourceBuiltin); err != nil {
		t.Fatalf("Register(builtin): unexpected error: %v", err)
	}
	src, ok := r.SourceOf("read")
	if !ok || src != SourceBuiltin {
		t.Fatalf("SourceOf(read) = %v, %v; want %v, true", src, ok, SourceBuiltin)
	}
}

func TestRegisterRefusesCollision(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubSkill{name: "read"}, SourceBuiltin); err != nil {
		t.Fatalf("first Register: unexpected error: %v", err)
	}
	err := r.Register(&stubSkill{name: "read"}, SourceExternal)
	if err == nil {
		t.Fatal("second Register with same name: expected error, got nil")
	}
	// original registration must survive untouched
	if src, _ := r.SourceOf("read"); src != SourceBuiltin {
		t.Fatalf("SourceOf(read) after refused overwrite = %v, want %v (unchanged)", src, SourceBuiltin)
	}
}
