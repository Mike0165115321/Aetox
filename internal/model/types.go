package model

import (
	"context"
	"errors"
)

var (
	ErrNoMessages   = errors.New("model request missing messages")
	ErrMissingModel = errors.New("model name is required")
	ErrMissingAPIKey = errors.New("missing model API key")
)

type MessageRole string

const (
	RoleSystem MessageRole = "system"
	RoleUser   MessageRole = "user"
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
}

type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
}

