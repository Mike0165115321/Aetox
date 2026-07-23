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

// anthropicAPIVersion is the required anthropic-version header value for
// the Messages API (https://api.anthropic.com/v1/messages).
const anthropicAPIVersion = "2023-06-01"

// defaultAnthropicMaxTokens is used when the caller does not set
// Request.MaxTokens — Anthropic's API requires max_tokens on every call.
const defaultAnthropicMaxTokens = 4096

type AnthropicConfig struct {
	Model   string
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

type AnthropicProvider struct {
	model      string
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewAnthropicProvider(cfg AnthropicConfig) (*AnthropicProvider, error) {
	model := strings.TrimSpace(cfg.Model)
	apiKey := strings.TrimSpace(cfg.APIKey)
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if model == "" {
		return nil, ErrMissingModel
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL("anthropic")
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	return &AnthropicProvider{
		model:      model,
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) SupportsToolCalling() bool { return true }

func (p *AnthropicProvider) SupportsReasoning() bool { return true }

// ---------------------------------------------------------------------------
// Wire types (Anthropic Messages API — system is a top-level field, not a
// message; content is always an array of typed blocks).
// ---------------------------------------------------------------------------

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type anthropicThinking struct {
	Type string `json:"type"`
}

type anthropicRequest struct {
	Model       string               `json:"model"`
	System      string               `json:"system,omitempty"`
	Messages    []anthropicMessage   `json:"messages"`
	MaxTokens   int                  `json:"max_tokens"`
	Temperature float64              `json:"temperature,omitempty"`
	Tools       []anthropicTool      `json:"tools,omitempty"`
	ToolChoice  *anthropicToolChoice `json:"tool_choice,omitempty"`
	Thinking    *anthropicThinking   `json:"thinking,omitempty"`
	Stream      bool                 `json:"stream,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicErrorPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type anthropicResponse struct {
	Model   string                  `json:"model"`
	Content []anthropicContentBlock `json:"content"`
	Usage   anthropicUsage          `json:"usage"`
	Error   *anthropicErrorPayload  `json:"error"`
}

// ---------------------------------------------------------------------------
// Request conversion (internal Request/Message -> Anthropic wire shape)
// ---------------------------------------------------------------------------

// convertMessagesToAnthropic splits system messages into the top-level
// system prompt and converts the rest into Anthropic's user/assistant
// turns. Anthropic requires strictly alternating roles, so consecutive
// messages that map to the same role (e.g. several tool results in a row)
// are merged into a single turn.
func convertMessagesToAnthropic(msgs []Message) (string, []anthropicMessage) {
	var systemParts []string
	var out []anthropicMessage

	for _, m := range msgs {
		if m.Role == RoleSystem {
			if s := strings.TrimSpace(m.Content); s != "" {
				systemParts = append(systemParts, s)
			}
			continue
		}

		var role string
		var blocks []anthropicContentBlock
		switch m.Role {
		case RoleAssistant:
			role = "assistant"
			if text := strings.TrimSpace(m.Content); text != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: text})
			}
			for _, tc := range m.ToolCalls {
				args := strings.TrimSpace(tc.Function.Arguments)
				if args == "" || !json.Valid([]byte(args)) {
					args = "{}"
				}
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: json.RawMessage(args),
				})
			}
		case RoleTool:
			role = "user"
			blocks = append(blocks, anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			})
		default: // RoleUser and anything unrecognized
			role = "user"
			blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
		}
		if len(blocks) == 0 {
			continue
		}

		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content = append(out[n-1].Content, blocks...)
			continue
		}
		out = append(out, anthropicMessage{Role: role, Content: blocks})
	}

	return strings.Join(systemParts, "\n\n"), out
}

func convertToolsToAnthropic(tools []ToolDefinition) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		schema := t.Function.Parameters
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: schema,
		})
	}
	return out
}

func convertToolChoiceToAnthropic(choice string) *anthropicToolChoice {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "", "auto":
		return nil
	case "none":
		return &anthropicToolChoice{Type: "none"}
	case "required", "any":
		return &anthropicToolChoice{Type: "any"}
	default:
		return &anthropicToolChoice{Type: "tool", Name: choice}
	}
}

// convertThinkingToAnthropic maps the internal thinking/reasoning knobs onto
// Anthropic's adaptive thinking mode. Adaptive is used (rather than a fixed
// budget_tokens) so this works unmodified across current Claude models.
func convertThinkingToAnthropic(thinking *ThinkingConfig, reasoning *ReasoningConfig) *anthropicThinking {
	if thinking != nil && strings.EqualFold(strings.TrimSpace(thinking.Type), "disabled") {
		return nil
	}
	if thinking != nil || reasoning != nil {
		return &anthropicThinking{Type: "adaptive"}
	}
	return nil
}

func buildAnthropicRequest(model string, req Request, stream bool) (anthropicRequest, error) {
	system, messages := convertMessagesToAnthropic(req.Messages)
	if len(messages) == 0 {
		return anthropicRequest{}, ErrNoMessages
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultAnthropicMaxTokens
	}

	return anthropicRequest{
		Model:       model,
		System:      system,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Tools:       convertToolsToAnthropic(req.Tools),
		ToolChoice:  convertToolChoiceToAnthropic(req.ToolChoice),
		Thinking:    convertThinkingToAnthropic(req.Thinking, req.Reasoning),
		Stream:      stream,
	}, nil
}

func (p *AnthropicProvider) newHTTPRequest(ctx context.Context, body []byte) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	return httpReq, nil
}

// ---------------------------------------------------------------------------
// Complete
// ---------------------------------------------------------------------------

func (p *AnthropicProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	payload, err := buildAnthropicRequest(model, req, false)
	if err != nil {
		return Response{}, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	httpReq, err := p.newHTTPRequest(ctx, body)
	if err != nil {
		return Response{}, err
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
		return Response{}, fmt.Errorf("anthropic request failed with status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return Response{}, fmt.Errorf("anthropic response parse failed: %w", err)
	}
	if parsed.Error != nil {
		return Response{}, fmt.Errorf("anthropic error: %s", parsed.Error.Message)
	}

	var text, reasoning strings.Builder
	var toolCalls []ToolCall
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "thinking":
			reasoning.WriteString(block.Thinking)
		case "tool_use":
			args := "{}"
			if len(block.Input) > 0 {
				args = string(block.Input)
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:       block.ID,
				Type:     "function",
				Function: FunctionCall{Name: block.Name, Arguments: args},
			})
		}
	}

	textOut := strings.TrimSpace(text.String())
	reasoningOut := strings.TrimSpace(reasoning.String())
	if textOut == "" && reasoningOut == "" && len(toolCalls) == 0 {
		return Response{}, fmt.Errorf("anthropic response has empty text")
	}

	return Response{
		Provider:         p.Name(),
		Model:            modelOr(parsed.Model, model),
		Text:             textOut,
		ReasoningContent: reasoningOut,
		ToolCalls:        toolCalls,
		Usage: normalizeUsage(Usage{
			PromptTokens:     parsed.Usage.InputTokens,
			CompletionTokens: parsed.Usage.OutputTokens,
		}),
	}, nil
}

// ---------------------------------------------------------------------------
// StreamComplete (SSE)
// ---------------------------------------------------------------------------

type anthropicStreamEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message *struct {
		Model string         `json:"model"`
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
	ContentBlock *anthropicContentBlock `json:"content_block"`
	Delta        *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	Usage *anthropicUsage        `json:"usage"`
	Error *anthropicErrorPayload `json:"error"`
}

type anthropicStreamToolBuilder struct {
	id      string
	name    string
	argsBuf strings.Builder
}

func (p *AnthropicProvider) StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler, onReasoningChunk StreamChunkHandler) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	payload, err := buildAnthropicRequest(model, req, true)
	if err != nil {
		return Response{}, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	httpReq, err := p.newHTTPRequest(ctx, body)
	if err != nil {
		return Response{}, err
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			return Response{}, fmt.Errorf("anthropic request failed with status %d", httpResp.StatusCode)
		}
		return Response{}, fmt.Errorf("anthropic request failed with status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	scanner := bufio.NewScanner(httpResp.Body)
	var text, reasoning strings.Builder
	toolBuilders := map[int]*anthropicStreamToolBuilder{}
	var toolOrder []int
	respModel := model
	var usage Usage

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return Response{}, fmt.Errorf("anthropic stream parse failed: %w", err)
		}

		switch event.Type {
		case "message_start":
			if event.Message != nil {
				if event.Message.Model != "" {
					respModel = event.Message.Model
				}
				usage.PromptTokens = event.Message.Usage.InputTokens
			}
		case "content_block_start":
			if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
				toolBuilders[event.Index] = &anthropicStreamToolBuilder{
					id:   event.ContentBlock.ID,
					name: event.ContentBlock.Name,
				}
				toolOrder = append(toolOrder, event.Index)
			}
		case "content_block_delta":
			if event.Delta == nil {
				continue
			}
			switch event.Delta.Type {
			case "text_delta":
				if event.Delta.Text != "" {
					text.WriteString(event.Delta.Text)
					if onChunk != nil {
						if err := onChunk(event.Delta.Text); err != nil {
							return Response{}, err
						}
					}
				}
			case "thinking_delta":
				reasoning.WriteString(event.Delta.Thinking)
				if onReasoningChunk != nil && event.Delta.Thinking != "" {
					if err := onReasoningChunk(event.Delta.Thinking); err != nil {
						return Response{}, err
					}
				}
			case "input_json_delta":
				if b, ok := toolBuilders[event.Index]; ok {
					b.argsBuf.WriteString(event.Delta.PartialJSON)
				}
			}
		case "message_delta":
			if event.Usage != nil {
				usage.CompletionTokens = event.Usage.OutputTokens
			}
		case "error":
			if event.Error != nil {
				return Response{}, fmt.Errorf("anthropic error: %s", event.Error.Message)
			}
			return Response{}, fmt.Errorf("anthropic stream error")
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, err
	}

	var toolCalls []ToolCall
	for _, idx := range toolOrder {
		b := toolBuilders[idx]
		args := strings.TrimSpace(b.argsBuf.String())
		if args == "" {
			args = "{}"
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:       b.id,
			Type:     "function",
			Function: FunctionCall{Name: b.name, Arguments: args},
		})
	}

	textOut := strings.TrimSpace(text.String())
	reasoningOut := strings.TrimSpace(reasoning.String())
	if textOut == "" && reasoningOut == "" && len(toolCalls) == 0 {
		return Response{}, fmt.Errorf("anthropic stream response has empty text")
	}

	return Response{
		Provider:         p.Name(),
		Model:            respModel,
		Text:             textOut,
		ReasoningContent: reasoningOut,
		ToolCalls:        toolCalls,
		Usage:            normalizeUsage(usage),
	}, nil
}
