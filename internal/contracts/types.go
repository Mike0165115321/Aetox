package contracts

import (
	"fmt"
	"time"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type StepStatus string

const (
	StatusSuccess StepStatus = "success"
	StatusFailure StepStatus = "failure"
	StatusPartial StepStatus = "partial"
)

type CriticVerdictType string

const (
	CriticPass     CriticVerdictType = "pass"
	CriticRetry    CriticVerdictType = "retry"
	CriticEscalate CriticVerdictType = "escalate"
)

type CriticVerdict struct {
	Verdict    CriticVerdictType `json:"verdict"`
	Score      float64           `json:"score"`
	Issues     []string          `json:"issues"`
	Suggestion string            `json:"suggestion"`
}

type TaskPlan struct {
	ID                 string     `json:"id"`
	Goal               string     `json:"goal"`
	Steps              []TaskStep `json:"steps"`
	RequiresPermission bool       `json:"requires_permission"`
	RiskLevel          RiskLevel  `json:"risk_level"`
}

type TaskStep struct {
	ID              int               `json:"id"`
	Description     string            `json:"description"`
	Tool            string            `json:"tool"`
	Action          string            `json:"action"`
	Params          map[string]any    `json:"params"`
	DependsOn       []int             `json:"depends_on"`
	SuccessCriteria string            `json:"success_criteria"`
	RiskLevel       RiskLevel         `json:"risk_level"`
}

type StepResult struct {
	StepID        int               `json:"step_id"`
	Status        StepStatus        `json:"status"`
	Output        string            `json:"output"`
	Artifacts     map[string]string `json:"artifacts"`
	Error         string            `json:"error"`
	Confidence    float64           `json:"confidence"`
	MemoryUpdates map[string]any    `json:"memory_updates"`
}

func NewTaskPlanID() string {
	return fmt.Sprintf("plan-%d", time.Now().UnixNano())
}

