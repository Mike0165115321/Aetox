package skill

import (
	"context"
	"strings"
)

type echoSkill struct{}

func (*echoSkill) Name() string { return "echo" }

func (*echoSkill) Description() string {
	return "echo arguments back as plain text"
}

func (*echoSkill) Execute(_ context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	return Output{
		Name:    "echo",
		Content: strings.Join(args, " "),
	}, nil
}

