package model

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	RoleTool      MessageRole = "tool"
)

type Message struct {
	Role             MessageRole `json:"role"`
	Content          string      `json:"content"`
	ReasoningContent string      `json:"reasoning_content,omitempty"`
	Name             string      `json:"name,omitempty"`
	// ToolCallID is used when returning tool outputs to providers that implement
	// function/tool calling APIs.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// ToolCalls follows the OpenAI-compatible function-call field.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type Request struct {
	Model       string           `json:"-"`
	Messages    []Message        `json:"messages"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
	Reasoning   *ReasoningConfig `json:"reasoning,omitempty"`
	Thinking    *ThinkingConfig  `json:"thinking,omitempty"`
}

type Response struct {
	Provider         string
	Model            string
	Text             string
	ReasoningContent string
	Usage            *Usage
	ToolCalls        []ToolCall
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ReasoningConfig struct {
	Effort string `json:"effort,omitempty"`
}

type ThinkingConfig struct {
	Type string `json:"type,omitempty"`
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

func ParseToolArguments(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
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

// StreamingProvider streams the visible reply via onChunk. onReasoningChunk is
// a separate, optional callback for a provider's own reasoning/thinking
// tokens (DeepSeek reasoning_content, Anthropic thinking_delta, ...) as they
// arrive — nil-safe, so callers that don't care about reasoning pass nil and
// nothing changes for them. Kept as its own callback rather than tagging
// StreamChunkHandler's single stream, since a reasoning chunk isn't part of
// the reply text and must never be concatenated into it.
type StreamingProvider interface {
	StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler, onReasoningChunk StreamChunkHandler) (Response, error)
}

type ReasoningProvider interface {
	SupportsReasoning() bool
}

func ProviderSupportsReasoning(provider Provider) bool {
	if provider == nil {
		return false
	}
	reasoningProvider, ok := provider.(ReasoningProvider)
	return ok && reasoningProvider.SupportsReasoning()
}
