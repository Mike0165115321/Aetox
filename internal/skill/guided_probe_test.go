package skill

import (
	"context"
	"errors"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// guidedProbe is a tool that exists only to be taught about.
type guidedProbe struct {
	fail   bool
	silent bool
}

func (*guidedProbe) Name() string        { return "probe" }
func (*guidedProbe) Description() string { return "a tool for testing guidance delivery" }

func (p *guidedProbe) Guidance(map[string]any) string {
	if p.silent {
		return ""
	}
	return "when to reach for probe: never, it is a test"
}

func (*guidedProbe) ToolDefinition() model.ToolDefinition {
	return model.ToolDefinition{
		Type:     "function",
		Function: model.ToolFunction{Name: "probe", Description: "probe", Parameters: []byte(`{"type":"object"}`)},
	}
}

func (p *guidedProbe) ExecuteTool(context.Context, map[string]any) (Output, error) {
	out := Output{Name: "probe", Content: "did the thing", RawOutput: "did the thing", Success: !p.fail}
	if p.fail {
		return out, errors.New("probe failed on purpose")
	}
	return out, nil
}

func (p *guidedProbe) Execute(ctx context.Context, _ Input) (Output, error) {
	return p.ExecuteTool(ctx, nil)
}
