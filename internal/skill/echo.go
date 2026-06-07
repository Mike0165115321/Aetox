package skill

import (
	"context"
	"strings"
	"time"
)

type echoSkill struct{}

func (*echoSkill) Name() string { return "echo" }

func (*echoSkill) Description() string {
	return "echo arguments back as plain text"
}

func (*echoSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	content := strings.Join(args, " ")

	return newToolOutput("echo", "echo "+content, content, start, false, nil), nil
}
