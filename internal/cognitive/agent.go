package cognitive

import (
	"context"
	"errors"
	"strings"

	"aetox-cli/internal/memory"
	"aetox-cli/internal/model"
)

const (
	defaultMaxToolCalls = 4
)

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
	MaxTurns     int
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
		context:      memory.NewContext(systemPrompt, cfg.MaxTurns, cfg.MaxChars),
	}
}

func (a *Agent) RespondWithTools(
	ctx context.Context,
	modelTools []model.ToolDefinition,
	userMessage string,
	execTool func(context.Context, model.ToolCall) (string, error),
) (string, error) {
	if len(modelTools) == 0 || execTool == nil || !a.supportsToolCalling() {
		return a.Respond(ctx, userMessage)
	}
	if a.provider == nil {
		return "", errors.New("agent provider is not initialized")
	}
	a.lastUsage = model.Usage{}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", errors.New("input is empty")
	}
	a.context.Add(model.RoleUser, msg)

	maxToolCalls := a.maxToolCalls
	if maxToolCalls <= 0 {
		maxToolCalls = defaultMaxToolCalls
	}
	for i := 0; i < maxToolCalls; i++ {
		response, err := a.provider.Complete(ctx, model.Request{
			Model:       a.model,
			Messages:    a.context.Messages(),
			MaxTokens:   768,
			Temperature: 0.2,
			Tools:       modelTools,
			ToolChoice:  "auto",
		})
		if err != nil {
			if i == 0 {
				return a.Respond(ctx, msg)
			}
			return "", err
		}
		if response.Usage != nil {
			a.lastUsage = *response.Usage
		}

		content := strings.TrimSpace(response.Text)
		if len(response.ToolCalls) == 0 {
			if content == "" {
				content = "(empty response)"
			}
			a.context.Add(model.RoleAssistant, content)
			return content, nil
		}

		for _, toolCall := range response.ToolCalls {
			callOutput, toolErr := a.executeToolCall(ctx, toolCall, execTool)
			a.context.AddMessage(model.Message{
				Role:       model.RoleTool,
				Name:       toolCall.Function.Name,
				ToolCallID: toolCall.ID,
				Content:    callOutput,
			})
			if toolErr != nil && content == "" {
				// keep moving; model can decide whether to recover from this tool failure.
				content = callOutput
			}
		}
		if content != "" {
			a.context.Add(model.RoleAssistant, content)
		}
	}

	return "agent tool loop reached maximum iterations", nil
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

func (a *Agent) Respond(ctx context.Context, userMessage string) (string, error) {
	if a.provider == nil {
		return "", errors.New("agent provider is not initialized")
	}
	a.lastUsage = model.Usage{}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", errors.New("input is empty")
	}

	a.context.Add(model.RoleUser, msg)

	response, err := a.provider.Complete(ctx, model.Request{
		Model:       a.model,
		Messages:    a.context.Messages(),
		MaxTokens:   768,
		Temperature: 0.2,
	})
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

	a.context.Add(model.RoleAssistant, reply)
	return reply, nil
}

func (a *Agent) RespondStream(ctx context.Context, userMessage string, onChunk func(string) error) (string, bool, error) {
	if a.provider == nil {
		return "", false, errors.New("agent provider is not initialized")
	}
	a.lastUsage = model.Usage{}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", false, errors.New("input is empty")
	}

	a.context.Add(model.RoleUser, msg)

	req := model.Request{
		Model:       a.model,
		Messages:    a.context.Messages(),
		MaxTokens:   768,
		Temperature: 0.2,
	}

	if streamer, ok := a.provider.(model.StreamingProvider); ok {
		response, err := streamer.StreamComplete(ctx, req, onChunk)
		if err == nil {
			reply := strings.TrimSpace(response.Text)
			if reply == "" {
				reply = "(empty response)"
			}
			a.lastUsage = model.Usage{}
			if response.Usage != nil {
				a.lastUsage = *response.Usage
			}
			a.context.Add(model.RoleAssistant, reply)
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
	a.context.Add(model.RoleAssistant, reply)
	return reply, false, nil
}

func (a *Agent) supportsToolCalling() bool {
	provider, ok := a.provider.(interface{ SupportsToolCalling() bool })
	return ok && provider.SupportsToolCalling()
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

func (a *Agent) ContextUsage() (messageCount int, usedChars int, maxChars int) {
	if a == nil || a.context == nil {
		return 0, 0, 0
	}
	return a.context.UsageStats()
}

func (a *Agent) LastUsage() model.Usage {
	return a.lastUsage
}
