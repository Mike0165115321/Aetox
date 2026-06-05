package planner

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"path/filepath"
	"strings"

	"aetox-cli/internal/config"
	"aetox-cli/internal/contracts"
)

type Planner struct{}

func New() *Planner {
	return &Planner{}
}

func (p *Planner) BuildPlan(_ context.Context, goal string, cfg config.Config) (contracts.TaskPlan, error) {
	normalized := strings.ToLower(strings.TrimSpace(goal))
	if normalized == "" {
		return contracts.TaskPlan{}, errors.New("empty goal")
	}

	isMarkdown := strings.Contains(normalized, "markdown") || strings.Contains(normalized, ".md")
	isList := strings.Contains(normalized, "list")
	isRead := strings.Contains(normalized, "read file") || strings.HasPrefix(normalized, "read ")
	isWrite := strings.Contains(normalized, "write file") || strings.Contains(normalized, "write ")
	isMove := strings.Contains(normalized, "move file") || strings.Contains(normalized, "move ")
	isDelete := strings.Contains(normalized, "delete file") || strings.Contains(normalized, "delete ")

	if isList && isMarkdown {
		targetPath := inferPathFromGoal(normalized, cfg.SandboxRoot)
		step := contracts.TaskStep{
			ID:              1,
			Description:     "List markdown files in target directory",
			Tool:            "files",
			Action:          "list",
			Params:          map[string]any{"path": targetPath, "pattern": "*.md", "recursive": false},
			DependsOn:       nil,
			SuccessCriteria: "Return matched file list",
			RiskLevel:       contracts.RiskLow,
		}
		return contracts.TaskPlan{
			ID:                 contracts.NewTaskPlanID(),
			Goal:               goal,
			Steps:              []contracts.TaskStep{step},
			RequiresPermission: false,
			RiskLevel:          contracts.RiskLow,
		}, nil
	}

	if strings.Contains(normalized, "list") || strings.Contains(normalized, "show") {
		targetPath := inferPathFromGoal(normalized, cfg.SandboxRoot)
		step := contracts.TaskStep{
			ID:              1,
			Description:     "List files in target directory",
			Tool:            "files",
			Action:          "list",
			Params:          map[string]any{"path": targetPath, "pattern": "*", "recursive": false},
			DependsOn:       nil,
			SuccessCriteria: "Return directory entries",
			RiskLevel:       contracts.RiskLow,
		}
		return contracts.TaskPlan{
			ID:                 contracts.NewTaskPlanID(),
			Goal:               goal,
			Steps:              []contracts.TaskStep{step},
			RequiresPermission: false,
			RiskLevel:          contracts.RiskLow,
		}, nil
	}

	isWeb := strings.Contains(normalized, "web fetch") ||
		strings.HasPrefix(normalized, "fetch ") ||
		strings.Contains(normalized, " http://") ||
		strings.Contains(normalized, " https://")
	if isWeb {
		url := inferWebParams(goal)
		if url == "" {
			return contracts.TaskPlan{}, fmt.Errorf("unable to infer URL from goal")
		}
		step := contracts.TaskStep{
			ID:              1,
			Description:     "Fetch URL content",
			Tool:            "web",
			Action:          "fetch",
			Params:          map[string]any{"url": url},
			DependsOn:       nil,
			SuccessCriteria: "Return response body",
			RiskLevel:       contracts.RiskHigh,
		}
		return contracts.TaskPlan{
			ID:                 contracts.NewTaskPlanID(),
			Goal:               goal,
			Steps:              []contracts.TaskStep{step},
			RequiresPermission: true,
			RiskLevel:          contracts.RiskHigh,
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
			return contracts.TaskPlan{}, fmt.Errorf("unable to infer shell command from goal")
		}
		step := contracts.TaskStep{
			ID:              1,
			Description:     "Run shell command",
			Tool:            "shell",
			Action:          "run",
			Params:          map[string]any{"command": command, "cwd": cfg.SandboxRoot},
			DependsOn:       nil,
			SuccessCriteria: "Execute command and return output",
			RiskLevel:       contracts.RiskHigh,
		}
		return contracts.TaskPlan{
			ID:                 contracts.NewTaskPlanID(),
			Goal:               goal,
			Steps:              []contracts.TaskStep{step},
			RequiresPermission: true,
			RiskLevel:          contracts.RiskHigh,
		}, nil
	}

	if isRead {
		path := inferReadParams(goal, cfg.SandboxRoot)
		step := contracts.TaskStep{
			ID:              1,
			Description:     "Read file",
			Tool:            "files",
			Action:          "read",
			Params:          map[string]any{"path": path},
			DependsOn:       nil,
			SuccessCriteria: "Return file content",
			RiskLevel:       contracts.RiskLow,
		}
		return contracts.TaskPlan{
			ID:                 contracts.NewTaskPlanID(),
			Goal:               goal,
			Steps:              []contracts.TaskStep{step},
			RequiresPermission: false,
			RiskLevel:          contracts.RiskLow,
		}, nil
	}

	if isWrite {
		path, content := inferWriteParams(normalized, cfg.SandboxRoot)
		if content == "" {
			return contracts.TaskPlan{}, fmt.Errorf("unable to infer write content from goal")
		}
		step := contracts.TaskStep{
			ID:              1,
			Description:     "Write file",
			Tool:            "files",
			Action:          "write",
			Params:          map[string]any{"path": path, "content": content},
			DependsOn:       nil,
			SuccessCriteria: "Write content to file",
			RiskLevel:       contracts.RiskMedium,
		}
		return contracts.TaskPlan{
			ID:                 contracts.NewTaskPlanID(),
			Goal:               goal,
			Steps:              []contracts.TaskStep{step},
			RequiresPermission: true,
			RiskLevel:          contracts.RiskMedium,
		}, nil
	}

	if isMove {
		src, dst := inferMoveParams(normalized, cfg.SandboxRoot)
		if src == "" || dst == "" {
			return contracts.TaskPlan{}, fmt.Errorf("unable to infer move source or destination from goal")
		}
		step := contracts.TaskStep{
			ID:              1,
			Description:     "Move file",
			Tool:            "files",
			Action:          "move",
			Params:          map[string]any{"source": src, "target": dst},
			DependsOn:       nil,
			SuccessCriteria: "Move source file to target path",
			RiskLevel:       contracts.RiskHigh,
		}
		return contracts.TaskPlan{
			ID:                 contracts.NewTaskPlanID(),
			Goal:               goal,
			Steps:              []contracts.TaskStep{step},
			RequiresPermission: true,
			RiskLevel:          contracts.RiskHigh,
		}, nil
	}

	if isDelete {
		path := inferDeleteParams(normalized, cfg.SandboxRoot)
		if path == "" {
			return contracts.TaskPlan{}, fmt.Errorf("unable to infer delete path from goal")
		}
		step := contracts.TaskStep{
			ID:              1,
			Description:     "Delete file",
			Tool:            "files",
			Action:          "delete",
			Params:          map[string]any{"path": path},
			DependsOn:       nil,
			SuccessCriteria: "Delete target file",
			RiskLevel:       contracts.RiskHigh,
		}
		return contracts.TaskPlan{
			ID:                 contracts.NewTaskPlanID(),
			Goal:               goal,
			Steps:              []contracts.TaskStep{step},
			RequiresPermission: true,
			RiskLevel:          contracts.RiskHigh,
		}, nil
	}

	return contracts.TaskPlan{}, fmt.Errorf("no supported planner path for goal: %s", goal)
}

func inferWriteParams(goal string, fallback string) (string, string) {
	quoted := extractQuotedValues(goal)
	if len(quoted) >= 2 {
		return quoted[0], quoted[1]
	}

	raw := strings.TrimSpace(goal)
	if strings.HasPrefix(raw, "write") {
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
		head := strings.TrimSpace(raw[:colon])
		tail := strings.TrimSpace(raw[colon+1:])
		if quote := extractQuotedValues(tail); len(quote) >= 1 {
			return head, trimQuotes(quote[0])
		}
		return head, trimQuotes(tail)
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

func inferPathFromGoal(normalized string, fallback string) string {
	key := " in "
	idx := strings.Index(normalized, key)
	if idx < 0 {
		return fallback
	}

	raw := strings.TrimSpace(normalized[idx+len(key):])
	if raw == "" || strings.Contains(raw, "this folder") || strings.Contains(raw, "here") {
		return fallback
	}

	if strings.HasPrefix(raw, "\"") || strings.HasPrefix(raw, "'") {
		raw = strings.Trim(raw, "\"'")
	}

	return filepath.Clean(raw)
}

func trimLeadingCommand(text, command string) string {
	text = strings.TrimSpace(text)
	cmd := strings.TrimSpace(strings.ToLower(command))
	lower := strings.ToLower(text)
	prefix := strings.TrimSpace(cmd)
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
