package skill

import (
	"aetox-cli/internal/command"
	"context"
	"fmt"
)

type Dispatcher struct {
	registry   *Registry
	commandSet map[string]struct{}
}

func NewDispatcher(registry *Registry) *Dispatcher {
	var commandSet map[string]struct{}
	if registry != nil {
		commandSet = command.BuildCommandSet(registry.Names())
	}
	return &Dispatcher{registry: registry, commandSet: commandSet}
}

func (d *Dispatcher) Execute(ctx context.Context, input string) (Output, bool, error) {
	if d == nil || d.registry == nil {
		return Output{}, false, nil
	}

	intent := command.Parse(input, command.ParseTokens, d.commandSet)
	if intent.Kind != command.KindSkill {
		return Output{}, false, nil
	}
	name := intent.Command
	args := intent.Args

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
		return output, true, fmt.Errorf("skill %q failed: %w", name, err)
	}
	return output, true, nil
}

func (d *Dispatcher) Names() []string {
	if d == nil || d.registry == nil {
		return nil
	}
	return d.registry.Names()
}

func (d *Dispatcher) Snapshot() map[string]Skill {
	if d == nil || d.registry == nil {
		return nil
	}
	return d.registry.Snapshot()
}
