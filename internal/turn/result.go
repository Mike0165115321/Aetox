package turn

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Mikedev115/Aetox/internal/skill"
)

func shouldUseDeterministicToolSummary(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	// calc belongs here for a reason the others do not have: its output is the
	// number itself. Handing it to a model to be phrased puts the answer back
	// through the one step the tool exists to take it out of.
	case "write", "sheet_write", "doc_write", "read", "delete", "github_repo_summary", "plugin_install", "calc":
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
	// Ephemeral: the summary prompt (with kilobytes of tool output) must never
	// land in conversation history as a fake user message.
	summary, err := e.agent.RespondEphemeral(summaryCtx, summaryPrompt, e.turnOptions)
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
	return trimToBackstop(summary, e.summaryLimit), nil
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

	return trimToBackstop(output, e.summaryLimit)
}

// trimToBackstop cuts an over-long tool result and says what it did.
//
// Two things were wrong with the line this replaces, and the first is the one
// that cost real turns. "...(output truncated)" states that something was
// removed and nothing about how much, so a reader cannot tell a result that
// lost its last sentence from one that lost 85% of itself. Worse, the tools
// underneath here publish continuation contracts that only work if you know:
// `read` ends with "continue with offset=N", and this cut ate that line — so
// the model was told to use an offset it could no longer see.
//
// The second is that the cut was made in bytes on a UTF-8 string, so it could
// land inside a Thai character and hand the model a broken rune. The same
// walk-back already exists in skill.readSkillFile; this is that rule applied at
// the other end of the same pipe.
func trimToBackstop(output string, limit int) string {
	if limit <= 0 || len(output) <= limit {
		return output
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(output[cut]) {
		cut--
	}
	return output[:cut] + fmt.Sprintf(
		"\n...(output truncated — showed the first %d of %d characters; anything the tool said about how to ask for the rest was cut off with them)",
		cut, len(output))
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
