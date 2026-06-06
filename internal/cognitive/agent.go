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
		context:  memory.NewContext(systemPrompt, cfg.MaxTurns, cfg.MaxChars),
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

func (a *Agent) RespondStream(ctx context.Context, userMessage string, onChunk func(string) error) (string, bool, error) {
	if a.provider == nil {
		return "", false, errors.New("agent provider is not initialized")
	}
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
	a.context.Add(model.RoleAssistant, reply)
	return reply, false, nil
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
}
