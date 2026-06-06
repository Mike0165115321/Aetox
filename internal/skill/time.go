package skill

import (
	"context"
	"time"
)

type timeSkill struct{}

func (*timeSkill) Name() string { return "time" }

func (*timeSkill) Description() string {
	return "show current local time"
}

func (*timeSkill) Execute(_ context.Context, input Input) (Output, error) {
	_ = input
	return Output{
		Name:    "time",
		Content: time.Now().Format("2006-01-02 15:04:05 MST"),
	}, nil
}

