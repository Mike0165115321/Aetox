package memory

import (
	"strconv"

	"aetox-cli/internal/contracts"
)

type SessionContext struct {
	TaskID      string
	TaskGoal    string
	CurrentStep int
	StepResults []contracts.StepResult
	Artifacts   map[string]string
	Context     map[string]any
}

func NewSessionContext(taskID, goal string) *SessionContext {
	return &SessionContext{
		TaskID:      taskID,
		TaskGoal:    goal,
		Artifacts:   map[string]string{},
		Context:     map[string]any{},
		StepResults: []contracts.StepResult{},
	}
}

func (s *SessionContext) AddResult(result contracts.StepResult) {
	s.CurrentStep++
	s.StepResults = append(s.StepResults, result)
}

func (s *SessionContext) CompactSummary() string {
	return "steps=" + strconv.Itoa(len(s.StepResults))
}
