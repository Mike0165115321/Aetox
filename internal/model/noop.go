package model

import (
	"context"
	"fmt"
	"strings"
)

type NoopProvider struct {
	DefaultModel string
}

func NewNoopProvider(model string) *NoopProvider {
	return &NoopProvider{DefaultModel: model}
}

func (p *NoopProvider) Name() string {
	return "noop"
}

func (p *NoopProvider) Complete(_ context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.DefaultModel
	}
	if model == "" {
		model = "noop"
	}

	lastMessage := req.Messages[len(req.Messages)-1]
	text := strings.TrimSpace(lastMessage.Content)
	if text == "" {
		text = "(empty prompt)"
	}

	return Response{
		Provider: p.Name(),
		Model:    model,
		Text:     fmt.Sprintf("[noop:%s] %s", model, text),
	}, nil
}

