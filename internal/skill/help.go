package skill

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type helpSkill struct {
	registry *Registry
}

func (*helpSkill) Name() string { return "help" }

func (*helpSkill) Description() string {
	return "show available commands"
}

func (s *helpSkill) Execute(_ context.Context, input Input) (Output, error) {
	_ = input
	if s == nil || s.registry == nil {
		return Output{
			Name:    "help",
			Content: "No commands are available.",
		}, nil
	}

	snapshot := s.registry.Snapshot()
	names := make([]string, 0, len(snapshot))
	for name := range snapshot {
		names = append(names, name)
	}
	sort.Strings(names)

	var lines []string
	for _, name := range names {
		item := snapshot[name]
		lines = append(lines, fmt.Sprintf("%-8s - %s", name, item.Description()))
	}

	if len(lines) == 0 {
		return Output{Name: "help", Content: "No commands are available."}, nil
	}

	return Output{
		Name:    "help",
		Content: "Available commands:\n" + strings.Join(lines, "\n"),
	}, nil
}
