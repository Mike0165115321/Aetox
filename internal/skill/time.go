package skill

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"aetox-cli/internal/model"
)

type timeSkill struct{}

func (*timeSkill) Name() string { return "time" }

func (*timeSkill) Description() string {
	return "แสดงเวลา/เวลาในระบบปัจจุบัน"
}

func (*timeSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "time",
			Description: "Return current local timestamp.",
			Parameters:  payload,
		},
	}
}

func (*timeSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	if len(args) > 0 {
		return newToolOutput("time", "time", "time accepts no arguments", time.Now(), false, errors.New("time takes no arguments")), errors.New("time takes no arguments")
	}
	return (&timeSkill{}).Execute(ctx, Input{})
}

func (*timeSkill) Execute(_ context.Context, input Input) (Output, error) {
	_ = input
	start := time.Now()
	content := time.Now().Format("2006-01-02 15:04:05 MST")
	return newToolOutput("time", "time", content, start, false, nil), nil
}
