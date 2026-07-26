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
		httpClient: newModelHTTPClient(timeout),
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

// openAIUsage is the usage object as this wire format sends it. Cache
// accounting has two spellings in the wild and both are read: OpenAI's own
// prompt_tokens_details.cached_tokens, and DeepSeek's flat
// prompt_cache_hit_tokens (it sends both, in agreement — measured).
//
// Pointers, so a provider that does no cache accounting at all leaves the
// fields nil and is reported as "unknown" rather than as a real zero.
// prompt_tokens is already the full input here, cached part included, so
// nothing needs recomputing — unlike the Anthropic format.
type openAIUsage struct {
	Usage
	CacheHitTokens *int `json:"prompt_cache_hit_tokens"`
	Details        *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u openAIUsage) toUsage() Usage {
	out := u.Usage
	if u.Details != nil && u.Details.CachedTokens != nil {
		out.CachedPromptTokens = *u.Details.CachedTokens
		out.CacheReported = true
	}
	if u.CacheHitTokens != nil {
		out.CachedPromptTokens = *u.CacheHitTokens
		out.CacheReported = true
	}
	return out
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
		Model            string           `json:"model"`
		Messages         []Message        `json:"messages"`
		Temperature      float64          `json:"temperature,omitempty"`
		MaxTokens        int              `json:"max_tokens,omitempty"`
		Tools            []ToolDefinition `json:"tools,omitempty"`
		ToolChoice       string           `json:"tool_choice,omitempty"`
		Reasoning        *ReasoningConfig `json:"reasoning,omitempty"`
		Thinking         *ThinkingConfig  `json:"thinking,omitempty"`
		ReasoningEffort  string           `json:"reasoning_effort,omitempty"`
		IncludeReasoning *bool            `json:"include_reasoning,omitempty"`
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
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Model string      `json:"model"`
		Usage openAIUsage `json:"usage"`
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
	// Backstop for DeepSeek's DSML leak: tool-call markup arriving as plain
	// text with no structured calls — lift the calls out, strip the markup.
	if len(rawMessage.ToolCalls) == 0 {
		if cleaned, calls := parseDSMLToolCalls(text); len(calls) > 0 {
			text = cleaned
			rawMessage.ToolCalls = calls
		}
	}
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
		Usage:            normalizeUsage(parsed.Usage.toUsage()),
		FinishReason:     strings.TrimSpace(parsed.Choices[0].FinishReason),
	}, nil
}

func (p *OpenAICompatibleProvider) StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler, onReasoningChunk StreamChunkHandler) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}
	payload := struct {
		Model            string           `json:"model"`
		Messages         []Message        `json:"messages"`
		Temperature      float64          `json:"temperature,omitempty"`
		MaxTokens        int              `json:"max_tokens,omitempty"`
		Tools            []ToolDefinition `json:"tools,omitempty"`
		ToolChoice       string           `json:"tool_choice,omitempty"`
		Reasoning        *ReasoningConfig `json:"reasoning,omitempty"`
		Thinking         *ThinkingConfig  `json:"thinking,omitempty"`
		ReasoningEffort  string           `json:"reasoning_effort,omitempty"`
		IncludeReasoning *bool            `json:"include_reasoning,omitempty"`
		Stream           bool             `json:"stream"`
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
	var finishReason string
	toolAcc := newStreamToolAccumulator(req.OnToolCallProgress)
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
		// OpenAI-style streaming delivers tool_calls in fragments keyed by
		// index (arguments arrive char-by-char across deltas); toolAcc stitches
		// them back together. DeepSeek instead leaks DSML markup into content —
		// handled by the backstop after the loop when no structured calls came.
		var parsed struct {
			Choices []struct {
				Delta struct {
					Message
					ToolCalls []streamToolCallDelta `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Model string      `json:"model"`
			Usage openAIUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			return Response{}, fmt.Errorf("%s stream parse failed: %w", p.provider, err)
		}
		if len(parsed.Choices) == 0 {
			if parsed.Usage.TotalTokenCount() > 0 {
				lastUsage = normalizeUsage(parsed.Usage.toUsage())
			}
			continue
		}
		if fr := strings.TrimSpace(parsed.Choices[0].FinishReason); fr != "" {
			finishReason = fr
		}
		delta := parsed.Choices[0].Delta
		toolAcc.add(delta.ToolCalls)
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
			if onReasoningChunk != nil {
				if err := onReasoningChunk(reasoningChunk); err != nil {
					return Response{}, err
				}
			}
		}
		if parsed.Usage.TotalTokenCount() > 0 {
			lastUsage = normalizeUsage(parsed.Usage.toUsage())
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, err
	}

	reply := strings.TrimSpace(builder.String())
	reasoning := strings.TrimSpace(reasoningBuilder.String())
	toolCalls := toolAcc.finalize()
	// DSML backstop, same as Complete: only when the model sent no structured
	// calls but leaked the markup into content instead.
	if len(toolCalls) == 0 {
		if cleaned, calls := parseDSMLToolCalls(reply); len(calls) > 0 {
			reply = cleaned
			toolCalls = calls
		}
	}
	if reply == "" && reasoning == "" && len(toolCalls) == 0 {
		return Response{}, fmt.Errorf("%s stream response has empty text", p.provider)
	}
	return Response{
		Provider:         p.Name(),
		Model:            model,
		Text:             reply,
		ReasoningContent: reasoning,
		ToolCalls:        toolCalls,
		Usage:            lastUsage,
		FinishReason:     finishReason,
	}, nil
}

// streamToolCallDelta is one fragment of an OpenAI-style streamed tool call.
// arguments arrives in pieces across deltas; index ties the pieces together.
type streamToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// streamToolAccumulator stitches streamed tool-call fragments back into whole
// ToolCalls, preserving first-seen order.
type streamToolAccumulator struct {
	byIndex map[int]*ToolCall
	order   []int
	// progress reports a call as it is written — see Request.OnToolCallProgress.
	progress *toolProgressTracker
}

func newStreamToolAccumulator(onProgress func(id, name, subject string, lines int)) *streamToolAccumulator {
	return &streamToolAccumulator{
		byIndex:  map[int]*ToolCall{},
		progress: newToolProgressTracker(onProgress),
	}
}

func (a *streamToolAccumulator) add(deltas []streamToolCallDelta) {
	for _, d := range deltas {
		call := a.byIndex[d.Index]
		if call == nil {
			call = &ToolCall{Type: "function"}
			a.byIndex[d.Index] = call
			a.order = append(a.order, d.Index)
		}
		if d.ID != "" {
			call.ID = d.ID
		}
		if d.Type != "" {
			call.Type = d.Type
		}
		if d.Function.Name != "" {
			call.Function.Name = d.Function.Name
		}
		call.Function.Arguments += d.Function.Arguments
		a.progress.report(d.Index, call.ID, call.Function.Name, call.Function.Arguments)
	}
}

func (a *streamToolAccumulator) finalize() []ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	calls := make([]ToolCall, 0, len(a.order))
	for _, idx := range a.order {
		call := a.byIndex[idx]
		// The arguments are complete now, so this is the last chance for the row
		// to learn its subject — and the chance pacing would otherwise eat when
		// the naming argument came last. See toolProgressTracker.flush.
		a.progress.flush(idx, call.ID, call.Function.Name, call.Function.Arguments)
		calls = append(calls, *call)
	}
	return calls
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
