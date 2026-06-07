package skill

import (
	"context"
	"time"
)

type timeSkill struct{}

func (*timeSkill) Name() string { return "time" }

func (*timeSkill) Description() string {
	return "แสดงเวลาท้องถิ่นปัจจุบัน"
}

func (*timeSkill) Execute(_ context.Context, input Input) (Output, error) {
	_ = input
	start := time.Now()
	content := time.Now().Format("2006-01-02 15:04:05 MST")
	return newToolOutput("time", "time", content, start, false, nil), nil
}
