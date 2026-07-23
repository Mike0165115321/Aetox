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

	"github.com/Mike0165115321/Aetox/internal/debuglog"
)

type OllamaConfig struct {
	Model   string
	BaseURL string
	Timeout time.Duration
}

type OllamaProvider struct {
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewOllamaProvider(cfg OllamaConfig) (*OllamaProvider, error) {
	model := strings.TrimSpace(cfg.Model)
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if model == "" {
		return nil, ErrMissingModel
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	return &OllamaProvider{
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) SupportsToolCalling() bool {
	return true
}

func (p *OllamaProvider) SupportsReasoning() bool {
	return false
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Tools    json.RawMessage     `json:"tools,omitempty"`
	Options  map[string]any      `json:"options,omitempty"`
}

// ollamaOptions maps Request.MaxTokens to Ollama's num_predict so the output
// cap is explicit here too — Ollama's own default can be as low as 128,
// which would truncate tool-call generation without us ever asking for it.
func ollamaOptions(req Request) map[string]any {
	if req.MaxTokens <= 0 {
		return nil
	}
	return map[string]any{"num_predict": req.MaxTokens}
}

type ollamaChatMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunctionCall `json:"function"`
}

func convertMessagesToOllama(msgs []Message) []ollamaChatMessage {
	out := make([]ollamaChatMessage, 0, len(msgs))
	for _, m := range msgs {
		ocm := ollamaChatMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			ocm.ToolCalls = make([]ollamaToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				args := strings.TrimSpace(tc.Function.Arguments)
				var rawArgs json.RawMessage
				if args != "" && args != "{}" {
					if json.Valid([]byte(args)) {
						rawArgs = json.RawMessage(args)
					}
				}
				if rawArgs == nil {
					rawArgs = json.RawMessage("{}")
				}
				ocm.ToolCalls = append(ocm.ToolCalls, ollamaToolCall{
					Function: ollamaFunctionCall{
						Name:      tc.Function.Name,
						Arguments: rawArgs,
					},
				})
			}
		}
		out = append(out, ocm)
	}
	return out
}

type ollamaFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaResponse struct {
	Model            string        `json:"model"`
	Message          ollamaMessage `json:"message"`
	Response         string        `json:"response"`
	Done             bool          `json:"done"`
	DoneReason       string        `json:"done_reason"`
	Error            string        `json:"error"`
	PromptTokens     int           `json:"prompt_eval_count"`
	CompletionTokens int           `json:"eval_count"`
}

type ollamaMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content"`
	ToolCalls        []ollamaToolCall `json:"tool_calls"`
}

func (p *OllamaProvider) Complete(ctx context.Context, req Request) (Response, error) {
	defer debuglog.Block(fmt.Sprintf("Ollama.Complete model=%s msgs=%d tools=%d", req.Model, len(req.Messages), len(req.Tools)))()

	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	var toolsJSON json.RawMessage
	if len(req.Tools) > 0 {
		encoded, err := json.Marshal(req.Tools)
		if err != nil {
			return Response{}, err
		}
		toolsJSON = encoded
		debuglog.Info("tools sent", fmt.Sprintf("%d definitions", len(req.Tools)))
	} else {
		debuglog.Msg("no tools in request (chat-only mode)")
	}

	payload := ollamaChatRequest{
		Model:    model,
		Messages: convertMessagesToOllama(req.Messages),
		Stream:   false,
		Tools:    toolsJSON,
		Options:  ollamaOptions(req),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	requestURL := p.baseURL + "/api/chat"
	debuglog.Msg("HTTP %s body(%d): %s", requestURL, len(body), truncOllama(string(body), 500))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

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
		debuglog.Msg("HTTP %d: %s", httpResp.StatusCode, truncOllama(string(responseBody), 200))
		// Backstop, not a gate: tools are always offered to every model
		// (ARCHITECTURE.md §17). If Ollama itself rejects them for this model,
		// retry the same request as plain chat so the turn still succeeds —
		// the model answers in text instead of the turn erroring out.
		if len(req.Tools) > 0 && strings.Contains(strings.ToLower(string(responseBody)), "does not support tools") {
			debuglog.Msg("model rejected tools — retrying without tools (chat-only)")
			retry := req
			retry.Tools = nil
			retry.ToolChoice = ""
			return p.Complete(ctx, retry)
		}
		return Response{}, fmt.Errorf("ollama request failed with status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var parsed ollamaResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return Response{}, fmt.Errorf("ollama response parse failed: %w", err)
	}
	if parsed.Error != "" {
		debuglog.Msg("ollama error: %s", parsed.Error)
		return Response{}, fmt.Errorf("ollama error: %s", parsed.Error)
	}

	reply := strings.TrimSpace(parsed.Message.Content)
	if reply == "" {
		reply = strings.TrimSpace(parsed.Response)
	}
	if !parsed.Done && reply == "" && len(parsed.Message.ToolCalls) == 0 {
		return Response{}, fmt.Errorf("ollama streaming mode is unsupported in this adapter")
	}
	if reply == "" && len(parsed.Message.ToolCalls) == 0 {
		return Response{}, fmt.Errorf("ollama response has empty text")
	}

	toolCalls := convertOllamaToolCalls(parsed.Message.ToolCalls)
	debuglog.Info("content", truncOllama(reply, 150))
	debuglog.Info("toolCalls", fmt.Sprintf("%d parsed", len(toolCalls)))
	for i, tc := range toolCalls {
		debuglog.Msg("toolCall[%d] %s(%s)", i, tc.Function.Name, truncOllama(tc.Function.Arguments, 80))
	}

	return Response{
		Provider:         p.Name(),
		Model:            modelOr(parsed.Model, model),
		Text:             reply,
		ReasoningContent: strings.TrimSpace(parsed.Message.ReasoningContent),
		ToolCalls:        toolCalls,
		FinishReason:     strings.TrimSpace(parsed.DoneReason), // Ollama already uses "length" for the num_predict cap
		Usage:            normalizeUsage(Usage{PromptTokens: parsed.PromptTokens, CompletionTokens: parsed.CompletionTokens}),
	}, nil
}

func (p *OllamaProvider) StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler, onReasoningChunk StreamChunkHandler) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	var toolsJSON json.RawMessage
	if len(req.Tools) > 0 {
		encoded, err := json.Marshal(req.Tools)
		if err != nil {
			return Response{}, err
		}
		toolsJSON = encoded
	}

	payload := ollamaChatRequest{
		Model:    model,
		Messages: convertMessagesToOllama(req.Messages),
		Stream:   true,
		Tools:    toolsJSON,
		Options:  ollamaOptions(req),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	requestURL := p.baseURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			return Response{}, fmt.Errorf("ollama request failed with status %d", httpResp.StatusCode)
		}
		return Response{}, fmt.Errorf("ollama request failed with status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	scanner := bufio.NewScanner(httpResp.Body)
	var builder strings.Builder
	var reasonBuilder strings.Builder
	var toolCallBuilders []*streamToolCallBuilder
	var lastUsage *Usage
	var doneReason string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var parsed ollamaResponse
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			return Response{}, fmt.Errorf("ollama stream parse failed: %w", err)
		}
		if parsed.Error != "" {
			return Response{}, fmt.Errorf("ollama error: %s", parsed.Error)
		}
		chunk := strings.TrimSpace(parsed.Message.Content)
		if chunk == "" {
			chunk = strings.TrimSpace(parsed.Response)
		}
		if chunk != "" {
			builder.WriteString(chunk)
			if onChunk != nil {
				if err := onChunk(chunk); err != nil {
					return Response{}, err
				}
			}
		}
		if reasonChunk := strings.TrimSpace(parsed.Message.ReasoningContent); reasonChunk != "" {
			reasonBuilder.WriteString(reasonChunk)
			if onReasoningChunk != nil {
				if err := onReasoningChunk(reasonChunk); err != nil {
					return Response{}, err
				}
			}
		}

		toolCallBuilders = mergeStreamToolCalls(toolCallBuilders, parsed.Message.ToolCalls)

		lastPromptTokens := maxInt(0, parsed.PromptTokens)
		lastCompletionTokens := maxInt(0, parsed.CompletionTokens)
		if parsed.PromptTokens > 0 || parsed.CompletionTokens > 0 {
			lastUsage = &Usage{
				PromptTokens:     lastPromptTokens,
				CompletionTokens: lastCompletionTokens,
				TotalTokens:      lastPromptTokens + lastCompletionTokens,
			}
		}
		if parsed.Done {
			doneReason = strings.TrimSpace(parsed.DoneReason)
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, err
	}

	reply := strings.TrimSpace(builder.String())
	toolCalls := finalizeStreamToolCalls(toolCallBuilders)
	if reply == "" && len(toolCalls) == 0 {
		return Response{}, fmt.Errorf("ollama stream response has empty text")
	}

	return Response{
		Provider:         p.Name(),
		Model:            model,
		Text:             reply,
		ReasoningContent: strings.TrimSpace(reasonBuilder.String()),
		ToolCalls:        toolCalls,
		FinishReason:     doneReason,
		Usage:            lastUsage,
	}, nil
}

func convertOllamaToolCalls(raw []ollamaToolCall) []ToolCall {
	if len(raw) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(raw))
	for i, tc := range raw {
		args := strings.TrimSpace(string(tc.Function.Arguments))
		if args == "" {
			args = "{}"
		}
		id := fmt.Sprintf("call_%d", i)
		out = append(out, ToolCall{
			ID:   id,
			Type: "function",
			Function: FunctionCall{
				Name:      strings.TrimSpace(tc.Function.Name),
				Arguments: args,
			},
		})
	}
	return out
}

type streamToolCallBuilder struct {
	index   int
	name    string
	argsBuf strings.Builder
}

func mergeStreamToolCalls(existing []*streamToolCallBuilder, incoming []ollamaToolCall) []*streamToolCallBuilder {
	for _, tc := range incoming {
		idx := len(existing)
		existing = append(existing, &streamToolCallBuilder{
			index:   idx,
			name:    tc.Function.Name,
			argsBuf: strings.Builder{},
		})
		existing[idx].argsBuf.WriteString(strings.TrimSpace(string(tc.Function.Arguments)))
	}
	return existing
}

func finalizeStreamToolCalls(builders []*streamToolCallBuilder) []ToolCall {
	if len(builders) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(builders))
	for _, b := range builders {
		args := strings.TrimSpace(b.argsBuf.String())
		if args == "" {
			args = "{}"
		}
		out = append(out, ToolCall{
			ID:   fmt.Sprintf("call_%d", b.index),
			Type: "function",
			Function: FunctionCall{
				Name:      strings.TrimSpace(b.name),
				Arguments: args,
			},
		})
	}
	return out
}

func truncOllama(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
