package model

import (
	"context"
	"fmt"
	"strings"
	"time"
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

func (p *NoopProvider) SupportsToolCalling() bool {
	return false
}

func (p *NoopProvider) SupportsReasoning() bool {
	return false
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

// StreamComplete simulates real-model streaming by trickling the noop
// response out word by word, so UI code paths that expect a
// StreamingProvider (typing indicators, incremental render) can be
// exercised without a live API key.
func (p *NoopProvider) StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler) (Response, error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return Response{}, err
	}

	words := strings.Fields(resp.Text)
	for i, word := range words {
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		default:
		}
		chunk := word
		if i > 0 {
			chunk = " " + word
		}
		if onChunk != nil {
			if err := onChunk(chunk); err != nil {
				return Response{}, err
			}
		}
		time.Sleep(40 * time.Millisecond)
	}

	return resp, nil
}
