package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleConfig struct {
	Provider      string
	Model         string
	APIKey        string
	BaseURL       string
	Timeout       time.Duration
	RequireAPIKey *bool
}

type OpenAICompatibleProvider struct {
	provider   string
	model      string
	apiKey     string
	baseURL    string
	reasoning  bool
	httpClient *http.Client
}

func NewOpenAICompatibleProvider(cfg OpenAICompatibleConfig) (*OpenAICompatibleProvider, error) {
	provider := strings.TrimSpace(strings.ToLower(cfg.Provider))
	if provider == "" {
		provider = "openai-compatible"
	}
	model := strings.TrimSpace(cfg.Model)
	apiKey := strings.TrimSpace(cfg.APIKey)
	baseURL := strings.TrimSpace(cfg.BaseURL)
	requireAPIKey := true
	if cfg.RequireAPIKey != nil {
		requireAPIKey = *cfg.RequireAPIKey
	}

	if model == "" {
		return nil, ErrMissingModel
	}
	if requireAPIKey && apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if baseURL == "" {
		baseURL = defaultOpenAICompatibleBaseURL(provider)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	return &OpenAICompatibleProvider{
		provider:   provider,
		model:      model,
		apiKey:     apiKey,
		baseURL:    baseURL,
		reasoning:  supportsNativeReasoning(provider),
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func defaultOpenAICompatibleBaseURL(provider string) string {
	baseURL := DefaultBaseURL(provider)
	if baseURL != "" {
		return baseURL
	}
	return DefaultBaseURL("openai")
}

func (p *OpenAICompatibleProvider) Name() string {
	return p.provider
}

func (p *OpenAICompatibleProvider) SupportsToolCalling() bool {
	return true
}

func (p *OpenAICompatibleProvider) SupportsReasoning() bool {
	return p.reasoning
}

func (p *OpenAICompatibleProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	payload := struct {
		Model           string           `json:"model"`
		Messages        []Message        `json:"messages"`
		Temperature     float64          `json:"temperature,omitempty"`
		MaxTokens       int              `json:"max_tokens,omitempty"`
		Tools           []ToolDefinition `json:"tools,omitempty"`
		ToolChoice      string           `json:"tool_choice,omitempty"`
		Reasoning       *ReasoningConfig `json:"reasoning,omitempty"`
		Thinking        *ThinkingConfig  `json:"thinking,omitempty"`
		ReasoningEffort string           `json:"reasoning_effort,omitempty"`
		IncludeReasoning *bool           `json:"include_reasoning,omitempty"`
	}{
		Model:       model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}
	if p.usesDeepSeekThinking() {
		payload.Thinking = normalizeDeepSeekThinking(req.Thinking, req.Reasoning)
		payload.ReasoningEffort = normalizeDeepSeekReasoningEffort(req.Reasoning)
	} else if p.usesGroqReasoning() {
		payload.ReasoningEffort = normalizeStandardReasoningEffort(req.Reasoning)
		payload.IncludeReasoning = boolPtr(false)
	} else if p.usesOpenAIReasoningEffort() || p.usesGeminiReasoningEffort() {
		payload.ReasoningEffort = normalizeStandardReasoningEffort(req.Reasoning)
	} else if p.SupportsReasoning() {
		payload.Reasoning = req.Reasoning
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	requestURL := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()

	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return Response{}, fmt.Errorf(
			"%s request failed with status %d: %s",
			p.provider,
			httpResp.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Message
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Model string `json:"model"`
		Usage Usage  `json:"usage"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return Response{}, fmt.Errorf("%s response parse failed: %w", p.provider, err)
	}
	if len(parsed.Choices) == 0 {
		return Response{}, fmt.Errorf("%s response has no choices", p.provider)
	}

	rawMessage := parsed.Choices[0].Message.Message
	rawMessage.ToolCalls = append(rawMessage.ToolCalls, parsed.Choices[0].Message.ToolCalls...)
	text := strings.TrimSpace(rawMessage.Content)
	reasoning := strings.TrimSpace(rawMessage.ReasoningContent)
	if text == "" && reasoning == "" && len(rawMessage.ToolCalls) == 0 {
		return Response{}, fmt.Errorf("%s response has empty text", p.provider)
	}

	return Response{
		Provider:         p.Name(),
		Model:            modelOr(parsed.Model, model),
		Text:             text,
		ReasoningContent: reasoning,
		ToolCalls:        rawMessage.ToolCalls,
		Usage:            normalizeUsage(parsed.Usage),
	}, nil
}

func (p *OpenAICompatibleProvider) StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}
	payload := struct {
		Model           string           `json:"model"`
		Messages        []Message        `json:"messages"`
		Temperature     float64          `json:"temperature,omitempty"`
		MaxTokens       int              `json:"max_tokens,omitempty"`
		Tools           []ToolDefinition `json:"tools,omitempty"`
		ToolChoice      string           `json:"tool_choice,omitempty"`
		Reasoning       *ReasoningConfig `json:"reasoning,omitempty"`
		Thinking        *ThinkingConfig  `json:"thinking,omitempty"`
		ReasoningEffort string           `json:"reasoning_effort,omitempty"`
		IncludeReasoning *bool           `json:"include_reasoning,omitempty"`
		Stream          bool             `json:"stream"`
	}{
		Model:       model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
		Stream:      true,
	}
	if p.usesDeepSeekThinking() {
		payload.Thinking = normalizeDeepSeekThinking(req.Thinking, req.Reasoning)
		payload.ReasoningEffort = normalizeDeepSeekReasoningEffort(req.Reasoning)
	} else if p.usesGroqReasoning() {
		payload.ReasoningEffort = normalizeStandardReasoningEffort(req.Reasoning)
		payload.IncludeReasoning = boolPtr(false)
	} else if p.usesOpenAIReasoningEffort() || p.usesGeminiReasoningEffort() {
		payload.ReasoningEffort = normalizeStandardReasoningEffort(req.Reasoning)
	} else if p.SupportsReasoning() {
		payload.Reasoning = req.Reasoning
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	requestURL := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			return Response{}, fmt.Errorf("%s request failed with status %d", p.provider, httpResp.StatusCode)
		}
		return Response{}, fmt.Errorf(
			"%s request failed with status %d: %s",
			p.provider,
			httpResp.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	scanner := bufio.NewScanner(httpResp.Body)
	var builder strings.Builder
	var reasoningBuilder strings.Builder
	var lastUsage *Usage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var parsed struct {
			Choices []struct {
				Delta struct {
					Message
					ToolCalls []ToolCall `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Model string `json:"model"`
			Usage Usage  `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			return Response{}, fmt.Errorf("%s stream parse failed: %w", p.provider, err)
		}
		if len(parsed.Choices) == 0 {
			if parsed.Usage.TotalTokenCount() > 0 {
				lastUsage = normalizeUsage(parsed.Usage)
			}
			continue
		}
		delta := parsed.Choices[0].Delta
		if chunk := delta.Content; chunk != "" {
			builder.WriteString(chunk)
			if onChunk != nil {
				if err := onChunk(chunk); err != nil {
					return Response{}, err
				}
			}
		}
		if reasoningChunk := delta.ReasoningContent; reasoningChunk != "" {
			reasoningBuilder.WriteString(reasoningChunk)
		}
		if parsed.Usage.TotalTokenCount() > 0 {
			lastUsage = normalizeUsage(parsed.Usage)
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, err
	}

	reply := builder.String()
	reply = strings.TrimSpace(reply)
	reasoning := strings.TrimSpace(reasoningBuilder.String())
	if reply == "" && reasoning == "" {
		return Response{}, fmt.Errorf("%s stream response has empty text", p.provider)
	}
	return Response{
		Provider:         p.Name(),
		Model:            model,
		Text:             reply,
		ReasoningContent: reasoning,
		Usage:            lastUsage,
	}, nil
}

func supportsNativeReasoning(provider string) bool {
	switch NormalizeProvider(provider) {
	case "openrouter", "deepseek", "openai", "groq", "gemini":
		return true
	default:
		return false
	}
}

func (p *OpenAICompatibleProvider) usesDeepSeekThinking() bool {
	return NormalizeProvider(p.provider) == "deepseek"
}

func (p *OpenAICompatibleProvider) usesOpenAIReasoningEffort() bool {
	return NormalizeProvider(p.provider) == "openai"
}

func (p *OpenAICompatibleProvider) usesGroqReasoning() bool {
	return NormalizeProvider(p.provider) == "groq"
}

func (p *OpenAICompatibleProvider) usesGeminiReasoningEffort() bool {
	return NormalizeProvider(p.provider) == "gemini"
}

func normalizeDeepSeekThinking(thinking *ThinkingConfig, reasoning *ReasoningConfig) *ThinkingConfig {
	if thinking != nil {
		switch strings.ToLower(strings.TrimSpace(thinking.Type)) {
		case "disabled":
			return &ThinkingConfig{Type: "disabled"}
		case "enabled":
			return &ThinkingConfig{Type: "enabled"}
		}
	}
	if reasoning != nil {
		return &ThinkingConfig{Type: "enabled"}
	}
	return nil
}

func normalizeDeepSeekReasoningEffort(reasoning *ReasoningConfig) string {
	if reasoning == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(reasoning.Effort)) {
	case "xhigh", "max":
		return "max"
	case "low", "medium", "high":
		return "high"
	default:
		return ""
	}
}

func normalizeStandardReasoningEffort(reasoning *ReasoningConfig) string {
	if reasoning == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(reasoning.Effort)) {
	case "none", "default", "minimal", "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(reasoning.Effort))
	default:
		return ""
	}
}

func boolPtr(value bool) *bool {
	return &value
}
