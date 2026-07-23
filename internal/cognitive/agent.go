package cognitive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/memory"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

const (
	// toolLoopOutputTokenMax mirrors OpenCode's OUTPUT_TOKEN_MAX: the global
	// ceiling for tool-loop output, clamped down per provider in
	// toolLoopMaxTokens where an API rejects values this large.
	toolLoopOutputTokenMax = 32000

	// Doom-loop guard thresholds, same as OpenCode's (warn at 3 identical
	// consecutive calls, hard-stop at 5).
	doomLoopWarn = 3
	doomLoopStop = 5
)

// toolLoopMaxTokens returns the max_tokens sent on every tool-loop request —
// always explicit (Anthropic requires the field; the rest get a value their
// API accepts instead of a provider default that may be as low as 4096).
// ponytail: per-provider floors, not per-model — swap for a models.dev-style
// catalog when one provider's newer models need more than its floor.
func (a *Agent) toolLoopMaxTokens() int {
	name := ""
	if a.provider != nil {
		name = model.NormalizeProvider(a.provider.Name())
	}
	switch name {
	case "deepseek":
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.model)), "deepseek-v4") {
			return toolLoopOutputTokenMax // V4 series allows up to 384K output
		}
		return 8192 // V3-era API max; larger values are rejected with 400
	case "openai":
		return 16384 // gpt-4o floor; newer models allow more
	case "anthropic", "gemini", "zai":
		return toolLoopOutputTokenMax
	default:
		return 8192 // openrouter/groq/unknown route mixed models — stay safe
	}
}

type Agent struct {
	provider     model.Provider
	model        string
	context      *memory.Context
	lastUsage    model.Usage
	maxToolCalls int
}

type AgentConfig struct {
	Provider     model.Provider
	Model        string
	SystemPrompt string
	MaxChars     int
	MaxToolCalls int
}

func NewAgent(cfg AgentConfig) *Agent {
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = "You are Aetox, a concise and helpful terminal assistant."
	}
	return &Agent{
		provider:     cfg.Provider,
		model:        cfg.Model,
		lastUsage:    model.Usage{},
		maxToolCalls: cfg.MaxToolCalls,
		context:      memory.NewContext(systemPrompt, 0, cfg.MaxChars),
	}
}

func (a *Agent) RespondWithTools(
	ctx context.Context,
	modelTools []model.ToolDefinition,
	userMessage string,
	execTool func(context.Context, model.ToolCall) (string, error),
	opts turn.TurnOptions,
) (string, bool, error) {
	defer debuglog.Block(fmt.Sprintf("Agent.RespondWithTools (tools=%d)", len(modelTools)))()

	if len(modelTools) == 0 || execTool == nil || !a.supportsToolCalling() {
		debuglog.Msg("fallback to Respond (tools=%d supportsToolCalling=%v)", len(modelTools), a.supportsToolCalling())
		reply, err := a.Respond(ctx, userMessage, opts)
		return reply, false, err
	}
	if a.provider == nil {
		return "", false, errors.New("agent provider is not initialized")
	}
	a.lastUsage = model.Usage{}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", false, errors.New("input is empty")
	}
	a.context.Add(model.RoleUser, msg)

	// OpenCode-style loop: run until the model stops calling tools. The brakes
	// are the permission/approval layer, ctx cancellation (Ctrl+C in the CLI,
	// the Stop button in the desktop app), and the doom-loop guard below — not
	// an arbitrary round cap. MaxToolCalls > 0 opts back into a hard cap.
	maxToolCalls := a.maxToolCalls
	debuglog.Info("maxToolCalls", fmt.Sprintf("%d (<=0 means unlimited)", maxToolCalls))
	anyToolUsed := false
	var lastCallKey string
	repeatedCalls := 0
	loopMaxTokens := a.toolLoopMaxTokens()
	for i := 0; maxToolCalls <= 0 || i < maxToolCalls; i++ {
		debuglog.Msg("tool loop iteration %d (max=%d)", i+1, maxToolCalls)
		if ctx.Err() != nil {
			return "", anyToolUsed, ctx.Err()
		}
		response, err := a.provider.Complete(ctx, a.buildRequest(a.context.Messages(), loopMaxTokens, 0.2, modelTools, "auto", opts))
		if err != nil {
			debuglog.Msg("Complete() error: %v", err)
			if i == 0 {
				reply, err := a.Respond(ctx, msg, opts)
				return reply, false, err
			}
			return "", false, err
		}
		if response.Usage != nil {
			a.lastUsage = *response.Usage
		}

		content := strings.TrimSpace(response.Text)
		debuglog.Info("response.text", truncateStr(content, 100))
		debuglog.Info("response.toolCalls", fmt.Sprintf("%d", len(response.ToolCalls)))
		if len(response.ToolCalls) == 0 {
			if content == "" {
				content = "(empty response)"
			}
			a.context.AddMessage(model.Message{
				Role:             model.RoleAssistant,
				Content:          content,
				ReasoningContent: strings.TrimSpace(response.ReasoningContent),
			})
			return content, anyToolUsed, nil
		}
		anyToolUsed = true

		a.context.AddMessage(model.Message{
			Role:             model.RoleAssistant,
			Content:          content,
			ReasoningContent: strings.TrimSpace(response.ReasoningContent),
			ToolCalls:        response.ToolCalls,
		})

		// Truncation guard (same failure OpenCode hit, sst/opencode#18108):
		// finish_reason "length" means the tool-call JSON was cut off at
		// MaxTokens. Executing it would fail with a misleading parse/path
		// error the model then "fixes" forever. Tell it the truth instead.
		if response.FinishReason == model.FinishReasonLength {
			debuglog.Msg("tool call truncated at max_tokens, telling the model")
			for _, toolCall := range response.ToolCalls {
				a.context.AddMessage(model.Message{
					Role:       model.RoleTool,
					Name:       toolCall.Function.Name,
					ToolCallID: toolCall.ID,
					Content: fmt.Sprintf(
						"tool call NOT executed: your %s arguments were truncated at the %d-token output limit. Produce a shorter version, or split the work into several smaller tool calls (e.g. write a skeleton file first, then extend it with edit).",
						toolCall.Function.Name, loopMaxTokens),
				})
			}
			continue
		}

		for _, toolCall := range response.ToolCalls {
			// Doom-loop guard (mirrors OpenCode session/prompt.ts): identical
			// (name, args) calls back to back — nudge at 3, stop at 5.
			callKey := toolCall.Function.Name + "\x00" + toolCall.Function.Arguments
			if callKey == lastCallKey {
				repeatedCalls++
			} else {
				lastCallKey, repeatedCalls = callKey, 1
			}
			if repeatedCalls >= doomLoopStop {
				debuglog.Msg("doom loop: %s repeated %d times, stopping", toolCall.Function.Name, repeatedCalls)
				stopMsg := fmt.Sprintf("หยุดการทำงาน: เรียกเครื่องมือ %s ด้วยค่าเดิมซ้ำ %d ครั้งติดกันโดยไม่คืบหน้า — ลองสั่งใหม่หรือปรับคำสั่งดูครับ", toolCall.Function.Name, repeatedCalls)
				a.context.AddMessage(model.Message{
					Role:       model.RoleTool,
					Name:       toolCall.Function.Name,
					ToolCallID: toolCall.ID,
					Content:    "aborted: identical tool call repeated " + fmt.Sprint(repeatedCalls) + " times",
				})
				return stopMsg, true, nil
			}

			debuglog.Msg("tool call: %s(%s)", toolCall.Function.Name, truncateStr(toolCall.Function.Arguments, 80))
			callOutput, toolErr := a.executeToolCall(ctx, toolCall, execTool)
			callOutput = strings.TrimSpace(callOutput)
			if callOutput == "" {
				if toolErr != nil {
					callOutput = toolErr.Error()
				} else {
					callOutput = "(no output)"
				}
			}
			if repeatedCalls == doomLoopWarn {
				callOutput += "\n[loop warning] You have now made this exact tool call " + fmt.Sprint(repeatedCalls) + " times in a row with the same result. Try a different approach instead of repeating it."
			}
			debuglog.Msg("tool result: %s (err=%v)", truncateStr(callOutput, 120), toolErr)
			a.context.AddMessage(model.Message{
				Role:       model.RoleTool,
				Name:       toolCall.Function.Name,
				ToolCallID: toolCall.ID,
				Content:    callOutput,
			})
			if toolErr != nil && ctx.Err() != nil {
				return callOutput, true, ctx.Err()
			}
		}
	}

	return "agent tool loop reached maximum iterations", anyToolUsed, nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func (a *Agent) executeToolCall(ctx context.Context, toolCall model.ToolCall, execTool func(context.Context, model.ToolCall) (string, error)) (string, error) {
	if strings.TrimSpace(toolCall.Function.Name) == "" {
		return "tool-call-missing-name", errors.New("tool call missing function name")
	}

	output, err := execTool(ctx, toolCall)
	if err != nil {
		return output, err
	}
	return output, nil
}

func (a *Agent) Respond(ctx context.Context, userMessage string, opts turn.TurnOptions) (string, error) {
	if a.provider == nil {
		return "", errors.New("agent provider is not initialized")
	}
	a.lastUsage = model.Usage{}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", errors.New("input is empty")
	}

	a.context.Add(model.RoleUser, msg)

	response, err := a.provider.Complete(ctx, a.buildRequest(a.context.Messages(), 768, 0.2, nil, "", opts))
	if err != nil {
		return "", err
	}

	reply := strings.TrimSpace(response.Text)
	if reply == "" {
		return "(empty response)", nil
	}
	a.lastUsage = model.Usage{}
	if response.Usage != nil {
		a.lastUsage = *response.Usage
	}

	a.context.AddMessage(model.Message{
		Role:             model.RoleAssistant,
		Content:          reply,
		ReasoningContent: strings.TrimSpace(response.ReasoningContent),
	})
	return reply, nil
}

func (a *Agent) RespondStream(ctx context.Context, userMessage string, onChunk func(string) error, onReasoningChunk func(string) error, opts turn.TurnOptions) (string, bool, error) {
	if a.provider == nil {
		return "", false, errors.New("agent provider is not initialized")
	}
	a.lastUsage = model.Usage{}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", false, errors.New("input is empty")
	}

	a.context.Add(model.RoleUser, msg)

	req := a.buildRequest(a.context.Messages(), 768, 0.2, nil, "", opts)

	if streamer, ok := a.provider.(model.StreamingProvider); ok {
		response, err := streamer.StreamComplete(ctx, req, onChunk, onReasoningChunk)
		if err == nil {
			reply := strings.TrimSpace(response.Text)
			if reply == "" {
				reply = "(empty response)"
			}
			a.lastUsage = model.Usage{}
			if response.Usage != nil {
				a.lastUsage = *response.Usage
			}
			a.context.AddMessage(model.Message{
				Role:             model.RoleAssistant,
				Content:          reply,
				ReasoningContent: strings.TrimSpace(response.ReasoningContent),
			})
			return reply, true, nil
		}
		// fallback to non-streaming when streaming path fails
	}

	response, err := a.provider.Complete(ctx, req)
	if err != nil {
		return "", false, err
	}

	reply := strings.TrimSpace(response.Text)
	if reply == "" {
		return "(empty response)", false, nil
	}
	a.lastUsage = model.Usage{}
	if response.Usage != nil {
		a.lastUsage = *response.Usage
	}
	a.context.AddMessage(model.Message{
		Role:             model.RoleAssistant,
		Content:          reply,
		ReasoningContent: strings.TrimSpace(response.ReasoningContent),
	})
	return reply, false, nil
}

func (a *Agent) supportsToolCalling() bool {
	provider, ok := a.provider.(interface{ SupportsToolCalling() bool })
	return ok && provider.SupportsToolCalling()
}

func (a *Agent) SupportsToolCalling() bool {
	return a.supportsToolCalling()
}

func (a *Agent) ResolveThinkProfile(level think.Level) think.Profile {
	return think.Resolve(level, model.ProviderSupportsReasoning(a.provider))
}

func (a *Agent) ReplaceModel(provider model.Provider, modelName string) {
	a.provider = provider
	if modelName != "" {
		a.model = modelName
	}
}

func (a *Agent) ClearContext() {
	if a.context == nil {
		return
	}
	messages := a.context.Messages()
	systemPrompt := "You are Aetox, a concise and helpful terminal assistant."
	if len(messages) > 0 {
		systemPrompt = messages[0].Content
	}
	a.context.Reset(systemPrompt)
	a.lastUsage = model.Usage{}
}

// RestoreHistory appends prior conversation turns into the agent's context,
// so a reloaded chat session continues with its memory intact.
func (a *Agent) RestoreHistory(messages []model.Message) {
	if a == nil || a.context == nil {
		return
	}
	for _, m := range messages {
		a.context.AddMessage(m)
	}
}

func (a *Agent) ContextUsage() (messageCount int, usedChars int, maxChars int) {
	if a == nil || a.context == nil {
		return 0, 0, 0
	}
	return a.context.UsageStats()
}

// ContextMessages returns a copy of the conversation as currently held in
// memory (system prompt first) — for UI features like the context meter.
func (a *Agent) ContextMessages() []model.Message {
	if a == nil || a.context == nil {
		return nil
	}
	return a.context.Messages()
}

func (a *Agent) LastUsage() model.Usage {
	return a.lastUsage
}

func (a *Agent) buildRequest(messages []model.Message, maxTokens int, temperature float64, tools []model.ToolDefinition, toolChoice string, opts turn.TurnOptions) model.Request {
	req := model.Request{
		Model:       a.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		Tools:       tools,
		ToolChoice:  toolChoice,
	}
	profile := a.ResolveThinkProfile(opts.ThinkLevel)
	if effort := profile.ReasoningEffort(); effort != "" {
		req.Reasoning = &model.ReasoningConfig{Effort: effort}
	}
	if a.provider != nil && model.NormalizeProvider(a.provider.Name()) == "deepseek" {
		switch think.NormalizeLevel(string(opts.ThinkLevel)) {
		case think.LevelNoThinking:
			req.Thinking = &model.ThinkingConfig{Type: "disabled"}
		default:
			req.Thinking = &model.ThinkingConfig{Type: "enabled"}
		}
	}
	return req
}
