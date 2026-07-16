package turn

import (
	"time"

	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// StepKind ระบุว่า turn นี้ใช้เส้นทางไหนในการ execute
type StepKind string

const (
	StepKindConversation  StepKind = "conversation"
	StepKindExplicitSkill StepKind = "explicit_skill"
	StepKindInferredSkill StepKind = "inferred_skill"
	StepKindAgentTool     StepKind = "agent_tool"
	StepKindFallback      StepKind = "fallback"
)

// Record คือ immutable execution record ของหนึ่ง turn
// ใช้สำหรับ audit, debugging, testing, และต่อยอดไป integration tests
type Record struct {
	// Input
	RawInput string
	Intent   command.Intent

	// Plan (inferred candidates จาก planner)
	Step       StepKind
	Candidates []InferredToolCandidate

	// Safety
	SafetyAssessment *safety.Assessment
	ApprovalGranted  *bool // nil = no approval needed
	ApprovalReason   string

	// Execution — explicit skill path
	SkillOutput  *skill.Output
	SkillHandled bool
	SkillError   string

	// Execution — agent / tool loop path
	AgentToolCalled bool
	ToolCalls       []ToolCallRecord
	AgentReply      string
	AgentError      string

	// Unified outcome
	Reply  string
	Status TurnStatus
	Error  string

	// Timing (wall-clock)
	StartedAt  time.Time
	FinishedAt time.Time
	DurationMs int64

	// Metadata
	TokensUsed *model.Usage // optional
}

// ToolCallRecord บันทึกการเรียก tool แต่ละครั้งใน agent tool loop
type ToolCallRecord struct {
	Name      string
	Arguments map[string]any
	Output    string
	Success   bool
	Error     string
}

// Snapshot คืนค่า record โดยไม่ expose internal state
// ใช้สำหรับ logging, testing, และ external consumers
func (r Record) Snapshot() Record {
	// Record is a value type — copying is safe and immutable-by-default
	return r
}

// IsError บอกว่า turn นี้จบด้วย error หรือไม่
func (r Record) IsError() bool {
	return r.Status == TurnStatusError || r.Error != ""
}

// IsBlocked บอกว่า turn นี้ถูกบล็อกโดย safety approval หรือไม่
func (r Record) IsBlocked() bool {
	return r.Status == TurnStatusBlocked
}

// IsConversation บอกว่า turn นี้เป็นบทสนทนาทั่วไป (ไม่ใช้ tool)
func (r Record) IsConversation() bool {
	return r.Step == StepKindConversation
}

// ToolNames คืนชื่อ tools ทั้งหมดที่ถูกเรียกใน turn นี้
func (r Record) ToolNames() []string {
	names := make([]string, 0, len(r.ToolCalls))
	for _, tc := range r.ToolCalls {
		if tc.Name != "" {
			names = append(names, tc.Name)
		}
	}
	return names
}

// SafetyApproved บอกว่า safety check ผ่านหรือไม่
// nil = ไม่ต้อง approve, true = approved, false = denied
func (r Record) SafetyApproved() *bool {
	return r.ApprovalGranted
}
