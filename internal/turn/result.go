package turn

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"aetox-cli/internal/skill"
)

func shouldUseDeterministicToolSummary(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write", "read", "delete", "github_repo_summary", "plugin_install":
		return true
	default:
		return false
	}
}
func (e *Executor) summarizeToolExecution(
	ctx context.Context,
	originalInput string,
	result skill.Output,
	status TurnStatus,
	execErr error,
) (string, error) {
	output := strings.TrimSpace(result.RawOutput)
	if output == "" {
		output = strings.TrimSpace(result.Content)
	}
	output = e.sanitizeAndTrimOutput(output)
	if output == "(no output)" || output == "" {
		output = e.inferredToolFallbackOutput(result.Name, result.Command, status, execErr, result.Stderr)
	}

	commandLine := strings.TrimSpace(result.Command)
	if commandLine == "" {
		commandLine = result.Name
	}

	errorLine := ""
	if strings.TrimSpace(result.Stderr) != "" {
		errorLine = fmt.Sprintf("\nTool error: %s", result.Stderr)
	} else if execErr != nil {
		errorLine = fmt.Sprintf("\nTool error: %s", execErr.Error())
	}

	summaryPrompt := fmt.Sprintf(
		"Original user request: %q\n"+
			"Tool: %s\n"+
			"Command: %s\n"+
			"Execution status: %s\n"+
			"DurationMs: %d\n"+
			"Output:\n%s\n%s\n\n"+
			"Respond in the same language as the user and be concise.\n"+
			"Start with an explicit status phrase for executed (%s), then summarize key result and mention completion.",
		originalInput,
		result.Name,
		commandLine,
		status,
		result.DurationMs,
		output,
		errorLine,
		status,
	)

	summaryCtx, cancel := context.WithTimeout(ctx, e.summaryTimeout)
	defer cancel()
	summary, err := e.agent.Respond(summaryCtx, summaryPrompt, e.turnOptions)
	if err != nil {
		return "", err
	}

	summary = strings.TrimSpace(summary)
	if strings.Contains(summary, "Start with an explicit status phrase") ||
		strings.Contains(summary, "Original user request:") {
		return "", errors.New("provider did not generate a concise summary")
	}
	if summary == "" {
		return "", errors.New("empty summary response")
	}
	if len(summary) > e.summaryLimit {
		summary = summary[:e.summaryLimit] + "\n...(output truncated)"
	}
	return summary, nil
}

func (e *Executor) fallbackToolSummary(result skill.Output, status TurnStatus, execErr error) string {
	output := strings.TrimSpace(result.RawOutput)
	if output == "" {
		output = strings.TrimSpace(result.Content)
	}
	if output == "" {
		output = e.inferredToolFallbackOutput(result.Name, result.Command, status, execErr, result.Stderr)
	}
	output = e.sanitizeAndTrimOutput(output)
	stderr := strings.TrimSpace(result.Stderr)
	if status == TurnStatusError && stderr != "" && !strings.Contains(output, stderr) {
		output = fmt.Sprintf("%s\nError: %s", output, stderr)
	} else if execErr != nil && stderr == "" {
		output = fmt.Sprintf("%s\nError: %s", output, execErr.Error())
	}

	prefix := "executed (done)"
	switch status {
	case TurnStatusError:
		prefix = "executed (error)"
	case TurnStatusBlocked:
		prefix = "executed (blocked)"
	}
	commandText := strings.TrimSpace(result.Command)
	if commandText != "" {
		commandText = fmt.Sprintf("command: %s. ", commandText)
	}
	return fmt.Sprintf("%s. %s%s", prefix, commandText, output)
}

func (e *Executor) inferredToolFallbackOutput(
	commandName string,
	command string,
	status TurnStatus,
	execErr error,
	stderr string,
) string {
	name := strings.ToLower(strings.TrimSpace(commandName))
	command = strings.TrimSpace(command)
	trimmedErr := strings.TrimSpace(stderr)
	switch name {
	case "list":
		if status == TurnStatusError {
			if trimmedErr != "" {
				return "list failed: " + trimmedErr
			}
			if execErr != nil {
				return "list failed: " + strings.TrimSpace(execErr.Error())
			}
			return "list failed for " + command + " (no output)"
		}
		if command == "" {
			return "list completed with no output"
		}
		return "list completed with no output for " + command
	default:
		if status == TurnStatusError {
			if trimmedErr != "" {
				return trimmedErr
			}
			if execErr != nil {
				return strings.TrimSpace(execErr.Error())
			}
			return "command completed with no output"
		}
		return "command completed with no output"
	}
}

func (e *Executor) sanitizeAndTrimOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "(no output)"
	}

	redactionRules := map[string]*regexp.Regexp{
		"api key":  regexp.MustCompile("(?i)(api key\\s*[:=]\\s*)[^\\s]+"),
		"token":    regexp.MustCompile("(?i)(token\\s*[:=]\\s*)[^\\s]+"),
		"password": regexp.MustCompile("(?i)(password\\s*[:=]\\s*)[^\\s]+"),
	}
	for _, re := range redactionRules {
		output = re.ReplaceAllString(output, "$1[REDACTED]")
	}

	if len(output) > e.summaryLimit {
		output = output[:e.summaryLimit] + "\n...(output truncated)"
	}
	return output
}

func (e *Executor) normalizeToolResult(result skill.Output) skill.Output {
	output := strings.TrimSpace(result.RawOutput)
	if output == "" {
		output = strings.TrimSpace(result.Content)
	}
	output = e.sanitizeAndTrimOutput(output)
	result.Content = output
	result.RawOutput = output
	return result
}

func (e *Executor) newToolResultForTurn(name, command, detail string) skill.Output {
	if strings.TrimSpace(name) == "" {
		name = "tool"
	}
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = "(no output)"
	}
	return skill.Output{
		Name:       name,
		Command:    command,
		Content:    detail,
		RawOutput:  detail,
		Success:    false,
		Stderr:     detail,
		DurationMs: 0,
	}
}

