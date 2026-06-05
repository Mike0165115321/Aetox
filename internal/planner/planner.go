package planner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"aetox-cli/internal/config"
	"aetox-cli/internal/contracts"
)

type Planner struct{}

func New() *Planner {
	return &Planner{}
}

func (p *Planner) BuildPlan(_ context.Context, goal string, cfg config.Config) (contracts.TaskPlan, error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return contracts.TaskPlan{}, errors.New("empty goal")
	}

	clauses := splitGoalClauses(goal)
	if len(clauses) == 0 {
		return contracts.TaskPlan{}, fmt.Errorf("no supported planner path for goal: %s", goal)
	}

	steps := make([]contracts.TaskStep, 0, len(clauses))
	requiresPermission := false
	planRisk := contracts.RiskLow

	for i, clause := range clauses {
		step, err := inferTaskStep(clause, cfg.SandboxRoot)
		if err != nil {
			return contracts.TaskPlan{}, fmt.Errorf("cannot infer step %d: %w", i+1, err)
		}
		step.ID = i + 1
		steps = append(steps, step)
		if step.RiskLevel != contracts.RiskLow {
			requiresPermission = true
		}
		planRisk = maxRisk(planRisk, step.RiskLevel)
	}

	return contracts.TaskPlan{
		ID:                 contracts.NewTaskPlanID(),
		Goal:               goal,
		Steps:              steps,
		RequiresPermission: requiresPermission,
		RiskLevel:          planRisk,
	}, nil
}

func (p *Planner) BuildPlanFromHint(
	ctx context.Context,
	goal string,
	cfg config.Config,
	hint string,
	verdict contracts.CriticVerdict,
	lastResult contracts.StepResult,
) (contracts.TaskPlan, error) {
	composed := strings.TrimSpace(goal)
	if strings.TrimSpace(hint) != "" {
		composed += ". Hint: " + strings.TrimSpace(hint)
	}
	if strings.TrimSpace(verdict.Suggestion) != "" && verdict.Suggestion != hint {
		composed += ". Critic suggestion: " + strings.TrimSpace(verdict.Suggestion)
	}
	if out := strings.TrimSpace(lastResult.Output); out != "" {
		if len(out) > 260 {
			out = out[:260] + "..."
		}
		composed += ". Last output: " + out
	}
	if lastResult.Error != "" {
		composed += ". Last error: " + strings.TrimSpace(lastResult.Error)
	}
	return p.BuildPlan(ctx, composed, cfg)
}

func inferTaskStep(goal string, fallback string) (contracts.TaskStep, error) {
	normalized := strings.ToLower(strings.TrimSpace(goal))
	if normalized == "" {
		return contracts.TaskStep{}, errors.New("empty clause")
	}

	isMarkdown := strings.Contains(normalized, "markdown") || strings.Contains(normalized, ".md")
	isList := strings.Contains(normalized, "list")
	isRead := strings.Contains(normalized, "read file") || strings.HasPrefix(normalized, "read ")
	isWrite := strings.Contains(normalized, "write file") || strings.Contains(normalized, "write ")
	isMove := strings.Contains(normalized, "move file") || strings.Contains(normalized, "move ")
	isDelete := strings.Contains(normalized, "delete file") || strings.Contains(normalized, "delete ")

	if isList && isMarkdown {
		targetPath := inferPathFromGoal(goal, fallback)
		return contracts.TaskStep{
			Description:     "List markdown files in target directory",
			Tool:            "files",
			Action:          "list",
			Params:          map[string]any{"path": targetPath, "pattern": "*.md", "recursive": false},
			DependsOn:       nil,
			SuccessCriteria: "Return matched file list",
			RiskLevel:       contracts.RiskLow,
		}, nil
	}

	if strings.Contains(normalized, "list") || strings.Contains(normalized, "show") {
		targetPath := inferPathFromGoal(goal, fallback)
		return contracts.TaskStep{
			Description:     "List files in target directory",
			Tool:            "files",
			Action:          "list",
			Params:          map[string]any{"path": targetPath, "pattern": "*", "recursive": false},
			DependsOn:       nil,
			SuccessCriteria: "Return directory entries",
			RiskLevel:       contracts.RiskLow,
		}, nil
	}

	isWeb := strings.Contains(normalized, "web fetch") ||
		strings.HasPrefix(normalized, "fetch ") ||
		strings.Contains(normalized, " http://") ||
		strings.Contains(normalized, " https://")
	if isWeb {
		url := inferWebParams(goal)
		if url == "" {
			return contracts.TaskStep{}, errors.New("unable to infer URL")
		}
		return contracts.TaskStep{
			Description:     "Fetch URL content",
			Tool:            "web",
			Action:          "fetch",
			Params:          map[string]any{"url": url},
			DependsOn:       nil,
			SuccessCriteria: "Return response body",
			RiskLevel:       contracts.RiskHigh,
		}, nil
	}

	isShell := strings.HasPrefix(normalized, "run ") ||
		strings.HasPrefix(normalized, "execute ") ||
		strings.Contains(normalized, " shell ") ||
		strings.HasPrefix(normalized, "run command ") ||
		strings.HasPrefix(normalized, "shell run ")
	if isShell {
		command := inferShellParams(goal)
		if command == "" {
			return contracts.TaskStep{}, errors.New("unable to infer shell command")
		}
		return contracts.TaskStep{
			Description:     "Run shell command",
			Tool:            "shell",
			Action:          "run",
			Params:          map[string]any{"command": command, "cwd": fallback},
			DependsOn:       nil,
			SuccessCriteria: "Execute command and return output",
			RiskLevel:       contracts.RiskHigh,
		}, nil
	}

	if isRead {
		path := inferReadParams(goal, fallback)
		return contracts.TaskStep{
			Description:     "Read file",
			Tool:            "files",
			Action:          "read",
			Params:          map[string]any{"path": path},
			DependsOn:       nil,
			SuccessCriteria: "Return file content",
			RiskLevel:       contracts.RiskLow,
		}, nil
	}

	if isWrite {
		path, content := inferWriteParams(goal, fallback)
		if content == "" {
			return contracts.TaskStep{}, errors.New("unable to infer write content")
		}
		return contracts.TaskStep{
			Description:     "Write file",
			Tool:            "files",
			Action:          "write",
			Params:          map[string]any{"path": path, "content": content},
			DependsOn:       nil,
			SuccessCriteria: "Write content to file",
			RiskLevel:       contracts.RiskMedium,
		}, nil
	}

	if isMove {
		src, dst := inferMoveParams(goal, fallback)
		if src == "" || dst == "" {
			return contracts.TaskStep{}, errors.New("unable to infer move source or destination")
		}
		return contracts.TaskStep{
			Description:     "Move file",
			Tool:            "files",
			Action:          "move",
			Params:          map[string]any{"source": src, "target": dst},
			DependsOn:       nil,
			SuccessCriteria: "Move source file to target path",
			RiskLevel:       contracts.RiskHigh,
		}, nil
	}

	if isDelete {
		path := inferDeleteParams(goal, fallback)
		if path == "" {
			return contracts.TaskStep{}, errors.New("unable to infer delete path")
		}
		return contracts.TaskStep{
			Description:     "Delete file",
			Tool:            "files",
			Action:          "delete",
			Params:          map[string]any{"path": path},
			DependsOn:       nil,
			SuccessCriteria: "Delete target file",
			RiskLevel:       contracts.RiskHigh,
		}, nil
	}

	return contracts.TaskStep{}, fmt.Errorf("no supported action in clause: %s", goal)
}

func splitGoalClauses(goal string) []string {
	segments := []string{goal}
	for _, sep := range []string{"&&", ";"} {
		var next []string
		for _, seg := range segments {
			next = append(next, strings.Split(seg, sep)...)
		}
		segments = next
	}

	thenRe := regexp.MustCompile(`(?i)\s+then\s+`)
	var clauses []string
	for _, seg := range segments {
		for _, clause := range thenRe.Split(seg, -1) {
			clause = strings.TrimSpace(clause)
			if clause == "" {
				continue
			}
			clauses = append(clauses, clause)
		}
	}
	return clauses
}

func maxRisk(lhs, rhs contracts.RiskLevel) contracts.RiskLevel {
	weight := map[contracts.RiskLevel]int{
		contracts.RiskLow:    1,
		contracts.RiskMedium: 2,
		contracts.RiskHigh:   3,
	}
	if weight[rhs] > weight[lhs] {
		return rhs
	}
	return lhs
}

func inferWriteParams(goal string, fallback string) (string, string) {
	quoted := extractQuotedValues(goal)
	if len(quoted) >= 2 {
		return quoted[0], quoted[1]
	}

	raw := strings.TrimSpace(goal)
	if strings.HasPrefix(strings.ToLower(raw), "write") {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "write"))
	}
	if len(quoted) == 1 {
		head := strings.TrimSpace(raw)
		head = strings.TrimPrefix(head, "file")
		head = strings.TrimSpace(head)
		if strings.HasPrefix(head, quoted[0]) {
			head = strings.TrimSpace(strings.TrimPrefix(head, quoted[0]))
		}
		return quoted[0], strings.TrimSpace(trimQuotes(head))
	}

	if colon := strings.Index(raw, ":"); colon >= 0 {
		tail := strings.TrimSpace(raw[colon+1:])
		if quote := extractQuotedValues(tail); len(quote) >= 1 {
			return strings.TrimSpace(raw[:colon]), trimQuotes(quote[0])
		}
		return strings.TrimSpace(raw[:colon]), trimQuotes(tail)
	}
	return fallback, ""
}

func inferMoveParams(goal string, fallback string) (string, string) {
	quoted := extractQuotedValues(goal)
	if len(quoted) >= 2 {
		return quoted[0], quoted[1]
	}

	lower := strings.ToLower(goal)
	idx := strings.Index(lower, " to ")
	if idx >= 0 {
		left := strings.TrimSpace(goal[:idx])
		right := strings.TrimSpace(goal[idx+4:])
		left = trimLeadingCommand(left, "move")
		left = trimLeadingCommand(left, "file")
		right = trimLeadingCommand(right, "to")
		return strings.TrimSpace(left), strings.TrimSpace(right)
	}

	path := inferPathFromGoal(goal, fallback)
	return path, ""
}

func inferDeleteParams(goal string, fallback string) string {
	quoted := extractQuotedValues(goal)
	if len(quoted) >= 1 {
		return quoted[0]
	}
	return inferPathFromGoal(strings.TrimPrefix(goal, "delete"), fallback)
}

func inferWebParams(goal string) string {
	quoted := extractQuotedValues(goal)
	if len(quoted) >= 1 {
		return quoted[0]
	}

	for _, token := range strings.Fields(goal) {
		lower := strings.ToLower(token)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return trimQuotes(token)
		}
	}
	return trimLeadingCommand(trimLeadingCommand(trimLeadingCommand(goal, "fetch"), "web"), "get")
}

func inferShellParams(goal string) string {
	quoted := extractQuotedValues(goal)
	if len(quoted) >= 1 {
		return quoted[0]
	}

	trimmed := strings.TrimSpace(goal)
	lower := strings.ToLower(trimmed)

	if strings.HasPrefix(lower, "run ") {
		if strings.HasPrefix(lower, "run command ") {
			return trimLeadingCommand(trimmed, "run command")
		}
		return trimLeadingCommand(trimmed, "run")
	}
	if strings.HasPrefix(lower, "execute command ") {
		return trimLeadingCommand(trimLeadingCommand(trimmed, "execute"), "command")
	}
	if strings.HasPrefix(lower, "execute ") {
		return trimLeadingCommand(trimmed, "execute")
	}
	if strings.HasPrefix(lower, "shell run ") {
		return trimLeadingCommand(trimLeadingCommand(trimmed, "shell"), "run")
	}
	if strings.HasPrefix(lower, "shell ") {
		return trimLeadingCommand(trimmed, "shell")
	}
	return ""
}

func inferReadParams(goal string, fallback string) string {
	quoted := extractQuotedValues(goal)
	if len(quoted) >= 1 {
		return quoted[0]
	}

	if idx := strings.Index(strings.ToLower(goal), " from "); idx >= 0 {
		return strings.TrimSpace(goal[idx+6:])
	}
	if idx := strings.Index(strings.ToLower(goal), " file "); idx >= 0 {
		path := strings.TrimSpace(goal[idx+6:])
		return strings.Trim(strings.TrimSpace(path), "\"'")
	}
	return inferPathFromGoal(goal, fallback)
}

func inferPathFromGoal(goal string, fallback string) string {
	key := " in "
	idx := strings.Index(strings.ToLower(goal), key)
	if idx < 0 {
		return fallback
	}

	raw := strings.TrimSpace(goal[idx+len(key):])
	if raw == "" || strings.Contains(strings.ToLower(raw), "this folder") || strings.Contains(strings.ToLower(raw), "here") {
		return fallback
	}

	raw = strings.Trim(raw, "\"'")
	return filepath.Clean(raw)
}

func trimLeadingCommand(text, command string) string {
	text = strings.TrimSpace(text)
	prefix := strings.TrimSpace(strings.ToLower(command))
	lower := strings.ToLower(text)
	if lower == prefix {
		return ""
	}
	if strings.HasPrefix(lower, prefix+" ") {
		return strings.TrimSpace(text[len(prefix)+1:])
	}
	return text
}

func trimQuotes(value string) string {
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func extractQuotedValues(goal string) []string {
	var values []string
	re := regexp.MustCompile(`'([^']*)'|"([^"]*)"`)
	matches := re.FindAllStringSubmatch(goal, -1)
	for _, match := range matches {
		if match[1] != "" {
			values = append(values, match[1])
		} else if match[2] != "" {
			values = append(values, match[2])
		}
	}
	return values
}

