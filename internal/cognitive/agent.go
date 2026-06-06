package cognitive

import (
	"context"
	"errors"
	"strings"

	"aetox-cli/internal/memory"
	"aetox-cli/internal/model"
)

type Agent struct {
	provider model.Provider
	model    string
	context  *memory.Context
}

type AgentConfig struct {
	Provider     model.Provider
	Model        string
	SystemPrompt string
	MaxTurns     int
	MaxChars     int
}

func NewAgent(cfg AgentConfig) *Agent {
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = "You are Aetox, a concise and helpful terminal assistant."
	}
	return &Agent{
		provider: cfg.Provider,
		model:    cfg.Model,
		context: memory.NewContext(systemPrompt, cfg.MaxTurns, cfg.MaxChars),
	}
}

func (a *Agent) Respond(ctx context.Context, userMessage string) (string, error) {
	if a.provider == nil {
		return "", errors.New("agent provider is not initialized")
	}
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

	a.context.Add(model.RoleAssistant, reply)
	return reply, nil
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
}
