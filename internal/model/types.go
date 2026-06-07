package model

import (
	"context"
	"errors"
)

var (
	ErrNoMessages    = errors.New("model request missing messages")
	ErrMissingModel  = errors.New("model name is required")
	ErrMissingAPIKey = errors.New("missing model API key")
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

type Message struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

type Request struct {
	Model       string    `json:"-"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type Response struct {
	Provider string
	Model    string
	Text     string
	Usage    *Usage
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u Usage) TotalTokenCount() int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	prompt := u.PromptTokens
	completion := u.CompletionTokens
	if prompt < 0 {
		prompt = 0
	}
	if completion < 0 {
		completion = 0
	}
	return prompt + completion
}

func normalizeUsage(usage Usage) *Usage {
	if usage.TotalTokenCount() <= 0 && usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 {
		return nil
	}
	normalized := Usage{
		PromptTokens:     maxInt(0, usage.PromptTokens),
		CompletionTokens: maxInt(0, usage.CompletionTokens),
		TotalTokens:      usage.TotalTokenCount(),
	}
	return &normalized
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
}

type StreamChunkHandler func(chunk string) error

type StreamingProvider interface {
	StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler) (Response, error)
}
