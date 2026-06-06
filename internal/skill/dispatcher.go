package skill

import (
	"context"
	"fmt"
	"strings"
)

type Dispatcher struct {
	registry *Registry
}

func NewDispatcher(registry *Registry) *Dispatcher {
	return &Dispatcher{registry: registry}
}

func (d *Dispatcher) Execute(ctx context.Context, input string) (Output, bool, error) {
	if d == nil || d.registry == nil {
		return Output{}, false, nil
	}

	name, args := ParseCommand(input)
	if name == "" {
		return Output{}, false, nil
	}

	skill, ok := d.registry.Get(name)
	if !ok {
		return Output{}, false, nil
	}

	execInput := Input{
		"raw":  input,
		"args": args,
	}
	output, err := skill.Execute(ctx, execInput)
	if err != nil {
		return Output{}, true, fmt.Errorf("skill %q failed: %w", name, err)
	}
	return output, true, nil
}

func (d *Dispatcher) Names() []string {
	if d == nil || d.registry == nil {
		return nil
	}
	return d.registry.Names()
}

func splitCommand(input string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return "", nil
	}
	return strings.ToLower(fields[0]), fields[1:]
}

func ParseCommand(input string) (string, []string) {
	return splitCommand(input)
}
