package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// anthropicAPIVersion is the required anthropic-version header value for
// the Messages API (https://api.anthropic.com/v1/messages).
const anthropicAPIVersion = "2023-06-01"

// defaultAnthropicMaxTokens is used when the caller does not set
// Request.MaxTokens — Anthropic's API requires max_tokens on every call.
// 8192 is safe for every model since Claude 3.5; a 4096 default truncated
// tool-call JSON mid-file (see cognitive.toolLoopMaxTokens).
const defaultAnthropicMaxTokens = 8192

type AnthropicConfig struct {
	// Provider is the logical provider name reported by Name(). Empty means
	// "anthropic". Set it when another provider speaks the Anthropic Messages
	// wire format (e.g. DeepSeek's /anthropic endpoint) so name-keyed logic —
	// toolLoopMaxTokens, status, reasoning normalization — stays correct.
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	Timeout  time.Duration
}

type AnthropicProvider struct {
	name       string
	model      string
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewAnthropicProvider(cfg AnthropicConfig) (*AnthropicProvider, error) {
	model := strings.TrimSpace(cfg.Model)
	apiKey := strings.TrimSpace(cfg.APIKey)
	baseURL := strings.TrimSpace(cfg.BaseURL)
	name := strings.TrimSpace(cfg.Provider)
	if name == "" {
		name = "anthropic"
	}
	if model == "" {
		return nil, ErrMissingModel
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if baseURL == "" {
		// Default to THIS provider's endpoint, not Anthropic's — this client
		// also serves Anthropic-wire-format providers like DeepSeek, and an
		// empty BaseURL (the persisted-preference normal case) used to send
		// their key to the real api.anthropic.com → 401 invalid x-api-key.
		baseURL = DefaultBaseURL(name)
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
		name:       name,
		model:      model,
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: newModelHTTPClient(timeout, baseURL),
	}, nil
}

func (p *AnthropicProvider) Name() string { return p.name }

func (p *AnthropicProvider) SupportsToolCalling() bool { return true }

func (p *AnthropicProvider) SupportsReasoning() bool { return true }

// ---------------------------------------------------------------------------
// Wire types (Anthropic Messages API — system is a top-level field, not a
// message; content is always an array of typed blocks).
// ---------------------------------------------------------------------------

type anthropicContentBlock struct {
	Type      string           `json:"type"`
	Text      string           `json:"text,omitempty"`
	Thinking  string           `json:"thinking,omitempty"`
	ID        string           `json:"id,omitempty"`
	Name      string           `json:"name,omitempty"`
	Input     json.RawMessage  `json:"input,omitempty"`
	ToolUseID string           `json:"tool_use_id,omitempty"`
	Content   string           `json:"content,omitempty"`
	IsError   bool             `json:"is_error,omitempty"`
	Source    *anthropicSource `json:"source,omitempty"`
	// CacheControl marks this block as the end of a cacheable prefix. Only
	// applyAnthropicCacheBreakpoints sets it, and only for Anthropic itself —
	// see that function for why.
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// anthropicCacheControl is a prompt-cache breakpoint. "ephemeral" is the only
// type the API has; the default 5-minute TTL refreshes on every hit, which an
// agent loop calling every few seconds never outlives.
type anthropicCacheControl struct {
	Type string `json:"type"`
}

// anthropicSource carries an image block's bytes. Anthropic takes base64 in a
// nested object with the media type beside it, rather than the single `data:`
// URL the OpenAI-compatible APIs take — same picture, different envelope, which
// is exactly why Message keeps images as raw bytes and lets each adapter wrap
// them.
type anthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	// CacheControl on the LAST tool caches every definition before it — the
	// registry's 30-odd schemas are the biggest stable block in the request.
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// anthropicSystem is the request's system field. Anthropic accepts either a
// plain string or an array of text blocks; the block form exists so a
// cache_control marker can ride on it. Providers that only borrow this wire
// format always get the string — same information, no assumption that they
// parse the array shape.
type anthropicSystem struct {
	Text   string
	Cached bool
}

func (s anthropicSystem) MarshalJSON() ([]byte, error) {
	if s.Cached {
		return json.Marshal([]anthropicContentBlock{{
			Type:         "text",
			Text:         s.Text,
			CacheControl: &anthropicCacheControl{Type: "ephemeral"},
		}})
	}
	return json.Marshal(s.Text)
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type anthropicThinking struct {
	Type string `json:"type"`
	// Display asks for the reasoning to come back readable. It defaults to
	// "omitted" on every current model — thinking blocks still stream, with an
	// empty text field — so without this Aetox's thinking panel sits blank on a
	// model that is visibly thinking, and the user sees a long pause instead.
	// Verified live: six effort levels all returned 0 characters of reasoning
	// until this was set.
	Display string `json:"display,omitempty"`
}

type anthropicRequest struct {
	Model       string               `json:"model"`
	System      anthropicSystem      `json:"system,omitzero"`
	Messages    []anthropicMessage   `json:"messages"`
	MaxTokens   int                  `json:"max_tokens"`
	Temperature float64              `json:"temperature,omitempty"`
	Tools       []anthropicTool      `json:"tools,omitempty"`
	ToolChoice  *anthropicToolChoice `json:"tool_choice,omitempty"`
	Thinking    *anthropicThinking   `json:"thinking,omitempty"`
	// OutputConfig carries the effort dial. This — not the long-deprecated
	// thinking.budget_tokens — is how thinking depth is actually chosen on
	// current models: budget_tokens is rejected with a 400 on Opus 5/4.8/4.7,
	// Sonnet 5 and Fable 5, so a runtime that maps a think level onto a token
	// budget is a runtime that cannot talk to any current Claude.
	OutputConfig *anthropicOutputConfig `json:"output_config,omitempty"`
	Stream       bool                   `json:"stream,omitempty"`
}

type anthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

// anthropicUsage counts input differently from the OpenAI-compatible format,
// and getting that wrong silently undercounts everything. Measured against
// DeepSeek's /anthropic endpoint on a well-cached call:
//
//	{"input_tokens":43,"cache_read_input_tokens":3968,"cache_creation_input_tokens":0}
//
// input_tokens is the freshly-evaluated part ALONE — 43 of a real 4011. The
// cached tokens are separate addends, not a subset, so the total input is the
// sum of all three. Reading input_tokens as "the prompt size" recorded that
// call as 43 tokens, and the better the cache worked the more it lost.
type anthropicUsage struct {
	InputTokens         int `json:"input_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
	OutputTokens        int `json:"output_tokens"`
}

// promptTokens is the whole input: fresh + written-to-cache + read-from-cache.
func (u anthropicUsage) promptTokens() int {
	return u.InputTokens + u.CacheCreationTokens + u.CacheReadTokens
}

// toUsage maps the Anthropic counters onto the shared shape. Cache creation
// counts as uncached: those tokens were evaluated this call, then stored.
func (u anthropicUsage) toUsage() Usage {
	return Usage{
		PromptTokens:       u.promptTokens(),
		CachedPromptTokens: u.CacheReadTokens,
		CacheReported:      true,
		CompletionTokens:   u.OutputTokens,
	}
}

type anthropicErrorPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type anthropicResponse struct {
	Model      string                  `json:"model"`
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
	Error      *anthropicErrorPayload  `json:"error"`
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
			if m.Content != "" || len(m.Images) == 0 {
				// The empty-text block is kept when there is no image, because
				// dropping it would silently delete a message the caller sent;
				// with an image the picture is the message and an empty text
				// block beside it is rejected by the API.
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
			}
			for _, img := range m.Images {
				mediaType := strings.TrimSpace(img.MediaType)
				if mediaType == "" {
					mediaType = "image/png"
				}
				blocks = append(blocks, anthropicContentBlock{
					Type: "image",
					Source: &anthropicSource{
						Type:      "base64",
						MediaType: mediaType,
						Data:      base64.StdEncoding.EncodeToString(img.Data),
					},
				})
			}
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
// convertThinkingToAnthropic decides the thinking block and the effort beside it.
//
// Current Claude models expose exactly two knobs: thinking is adaptive or off,
// and depth comes from output_config.effort (low → max). The older
// {"type":"enabled","budget_tokens":N} form is a 400 on every current model, so
// a think level maps to an effort string and never to a token count.
func convertThinkingToAnthropic(provider, model string, thinking *ThinkingConfig, reasoning *ReasoningConfig) (*anthropicThinking, *anthropicOutputConfig) {
	off := thinking != nil && strings.EqualFold(strings.TrimSpace(thinking.Type), "disabled")
	if thinking == nil && reasoning == nil {
		off = true
	}

	if off {
		// Fable-class models think unconditionally and reject an explicit
		// disable, so "off" there means sending no thinking field at all.
		if anthropicThinkingAlwaysOn(model) {
			return nil, nil
		}
		return &anthropicThinking{Type: "disabled"}, nil
	}

	block := &anthropicThinking{Type: "adaptive", Display: "summarized"}
	effort := anthropicEffort(provider, model, reasoning)
	if effort == "" {
		return block, nil
	}
	return block, &anthropicOutputConfig{Effort: effort}
}

// anthropicEffort asks the capability table what this provider and model call
// the requested level on the wire.
//
// It used to be a switch of its own, which is how DeepSeek — a borrower of this
// wire format, not Anthropic — ended up being sent effort names decided by
// Anthropic's ladder. The switch knew `medium` and `xhigh`, which DeepSeek's
// picker could never produce, and did not know `ultra`, which DeepSeek accepts;
// a level it did not recognise fell through to "" and quietly became the
// provider's own default, so asking for more thinking could get you less.
func anthropicEffort(provider, model string, reasoning *ReasoningConfig) string {
	if reasoning == nil {
		return ""
	}
	effort, ok := WireEffort(provider, model, reasoning.Effort)
	if !ok {
		return ""
	}
	return effort
}

// anthropicThinkingAlwaysOn reports the models that reject {"type":"disabled"}.
func anthropicThinkingAlwaysOn(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "fable") || strings.Contains(lower, "mythos")
}

func buildAnthropicRequest(provider, model string, req Request, stream bool) (anthropicRequest, error) {
	system, messages := convertMessagesToAnthropic(req.Messages)
	if len(messages) == 0 {
		return anthropicRequest{}, ErrNoMessages
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultAnthropicMaxTokens
	}

	thinking, outputConfig := convertThinkingToAnthropic(provider, model, req.Thinking, req.Reasoning)

	// Sampling parameters are gone from current Claude models: temperature,
	// top_p and top_k are rejected outright, and even where temperature is
	// still accepted it is pinned to 1 whenever thinking is on ("temperature
	// may only be set to 1 when thinking is enabled or in adaptive mode").
	// Aetox sets a temperature on every call (0.2 for the agent loop, 0.1 for
	// page digests), so sending it was a 400 that ended the turn.
	//
	// Providers that merely borrow this wire format are a different question —
	// DeepSeek's /anthropic endpoint still honors temperature — so the field is
	// dropped for Anthropic itself rather than for the runtime.
	temperature := req.Temperature
	if anthropicRejectsSampling(provider) {
		temperature = 0
	}

	request := anthropicRequest{
		Model:        model,
		System:       anthropicSystem{Text: system},
		Messages:     messages,
		MaxTokens:    maxTokens,
		Temperature:  temperature,
		Tools:        convertToolsToAnthropic(req.Tools),
		ToolChoice:   convertToolChoiceToAnthropic(req.ToolChoice),
		Thinking:     thinking,
		OutputConfig: outputConfig,
		Stream:       stream,
	}
	if anthropicNativeCaching(provider) {
		applyAnthropicCacheBreakpoints(&request)
	}
	return request, nil
}

// anthropicNativeCaching reports which providers honor cache_control markers.
// Anthropic requires them — without a marker nothing is cached and the whole
// prompt is re-evaluated at full price every turn. The wire-format borrowers
// are the opposite: DeepSeek caches automatically and ignores markers, so
// sending them buys nothing and assumes a field those APIs never documented.
// Same split, same reason, as anthropicRejectsSampling.
func anthropicNativeCaching(provider string) bool {
	return NormalizeProvider(provider) == "anthropic"
}

// applyAnthropicCacheBreakpoints marks the request's stable prefixes as
// cacheable: the tool definitions, the system prompt after them, and the
// conversation so far. Three of the four breakpoints the API allows.
//
// The first two never change within a session (the registry is fixed after
// bootstrap; prompt.Build runs once), so from the second call on they are
// pure 90%-discount reads. The last one moves forward every turn — Anthropic
// finds the previous turn's entry by checking earlier block boundaries, so
// the growing history re-reads from cache and only the newest turn is
// evaluated fresh. Prompts under the API's minimum (1024 tokens on most
// models) are silently not cached; the markers are still harmless there.
func applyAnthropicCacheBreakpoints(req *anthropicRequest) {
	if n := len(req.Tools); n > 0 {
		req.Tools[n-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}
	if req.System.Text != "" {
		req.System.Cached = true
	}
	if n := len(req.Messages); n > 0 {
		if blocks := req.Messages[n-1].Content; len(blocks) > 0 {
			blocks[len(blocks)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
		}
	}
}

// anthropicRejectsSampling reports which providers removed the sampling knobs.
// Anthropic did; the providers that only speak its wire format did not.
func anthropicRejectsSampling(provider string) bool {
	return NormalizeProvider(provider) == "anthropic"
}

func (p *AnthropicProvider) newHTTPRequest(ctx context.Context, body []byte) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	httpReq.Header.Set("x-api-key", p.apiKey)
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

	payload, err := buildAnthropicRequest(p.name, model, req, false)
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
	NoteQuotas(p.Name(), httpResp)

	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return Response{}, p.statusError(httpResp, responseBody)
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

	finishReason := strings.TrimSpace(parsed.StopReason)
	if finishReason == "max_tokens" {
		finishReason = FinishReasonLength
	}
	if err := errEmptyCompletion(p.Name(), finishReason, textOut, reasoningOut, len(toolCalls)); err != nil {
		return Response{}, err
	}

	return Response{
		Provider:         p.Name(),
		Model:            modelOr(parsed.Model, model),
		Text:             textOut,
		ReasoningContent: reasoningOut,
		ToolCalls:        toolCalls,
		FinishReason:     finishReason,
		Usage:            normalizeUsage(parsed.Usage.toUsage()),
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
		// StopReason rides on the message_delta event, not on the content
		// deltas — same field, different event, one struct.
		StopReason string `json:"stop_reason"`
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

	payload, err := buildAnthropicRequest(p.name, model, req, true)
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
	NoteQuotas(p.Name(), httpResp)

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			return Response{}, fmt.Errorf("anthropic request failed with status %d", httpResp.StatusCode)
		}
		return Response{}, p.statusError(httpResp, responseBody)
	}

	scanner := bufio.NewScanner(httpResp.Body)
	var text, reasoning strings.Builder
	toolBuilders := map[int]*anthropicStreamToolBuilder{}
	var toolOrder []int
	// Anthropic is the wire format Claude uses — and DeepSeek's default one, so
	// this is the path most tool calls actually take. Without it a model writing
	// a large file is silent from the end of its thinking until the call is
	// complete, which reads as a freeze.
	progress := newToolProgressTracker(req.OnToolCallProgress)
	respModel := model
	var usage Usage
	var finishReason string

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
				// message_start carries the whole input accounting; the
				// later message_delta only revises output.
				usage.PromptTokens = event.Message.Usage.promptTokens()
				usage.CachedPromptTokens = event.Message.Usage.CacheReadTokens
				usage.CacheReported = true
			}
		case "content_block_start":
			if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
				builder := &anthropicStreamToolBuilder{
					id:   event.ContentBlock.ID,
					name: event.ContentBlock.Name,
				}
				toolBuilders[event.Index] = builder
				toolOrder = append(toolOrder, event.Index)
				// Anthropic names the tool before sending a byte of its input,
				// so the row can appear immediately — no arguments needed.
				progress.report(event.Index, builder.id, builder.name, "")
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
					// Builder.String() is a view, not a copy — reading it every
					// delta costs nothing, and the tracker paces the scanning.
					progress.report(event.Index, b.id, b.name, b.argsBuf.String())
				}
			}
		case "message_delta":
			if event.Usage != nil {
				usage.CompletionTokens = event.Usage.OutputTokens
			}
			if event.Delta != nil && event.Delta.StopReason != "" {
				finishReason = event.Delta.StopReason
				if finishReason == "max_tokens" {
					finishReason = FinishReasonLength
				}
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
	if err := errEmptyCompletion(p.Name(), finishReason, textOut, reasoningOut, len(toolCalls)); err != nil {
		return Response{}, err
	}

	return Response{
		Provider:         p.Name(),
		Model:            respModel,
		Text:             textOut,
		ReasoningContent: reasoningOut,
		ToolCalls:        toolCalls,
		Usage:            normalizeUsage(usage),
		FinishReason:     finishReason,
	}, nil
}

// ---------------------------------------------------------------------------
// Model discovery
// ---------------------------------------------------------------------------

// DiscoverAnthropicModels asks the provider which models this account can use.
//
// The Messages API has no /models sibling in the same sense the
// OpenAI-compatible ones do, but Anthropic does serve GET /v1/models, and it is
// paginated. Asking beats shipping a list: hardcoded Claude names go stale
// every few months and a stale one is a 404 that reads like a bug in Aetox.
func DiscoverAnthropicModels(ctx context.Context, providerName, baseURL, apiKey string) ([]string, error) {
	endpoint := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if endpoint == "" {
		endpoint = strings.TrimSuffix(DefaultBaseURL(providerName), "/")
	}
	if endpoint == "" {
		return nil, fmt.Errorf("provider %q missing base URL", providerName)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var out []string
	afterID := ""

	// Bounded rather than "until has_more is false": a paging bug on either
	// side must not turn the model picker into an infinite loop.
	for page := 0; page < 5; page++ {
		url := endpoint + "/models?limit=100"
		if afterID != "" {
			url += "&after_id=" + afterID
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("anthropic-version", anthropicAPIVersion)
		if apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("%s model list failed with status %d: %s",
				providerName, resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var parsed struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("%s model list parse failed: %w", providerName, err)
		}
		for _, m := range parsed.Data {
			if m.ID != "" {
				out = append(out, m.ID)
			}
		}
		if !parsed.HasMore || parsed.LastID == "" {
			break
		}
		afterID = parsed.LastID
	}
	return out, nil
}

// statusError turns a rejected request into something the user can act on.
//
// The rate-limit case is the one that matters: Anthropic's 429 body is often
// literally {"message":"Error"}, so pasting it in front of the user says
// nothing. The retry window is in the headers, and by the time this runs the
// transport has already waited and retried — so what is left to say is when it
// is worth trying again.
//
// This format is not only Anthropic's: DeepSeek's default endpoint speaks it
// too, which is how an empty DeepSeek wallet arrived here as a 402 nobody had
// written a case for and went out as a raw JSON dump.
func (p *AnthropicProvider) statusError(resp *http.Response, body []byte) error {
	// The provider's own sentence where there is one, the raw payload only when
	// the body is shaped some other way. Both this API and the hosts borrowing
	// its format answer with {"error":{"message":…}}, and the JSON around that
	// sentence was never something a user should have to read past.
	detail := providerErrorMessage(body)
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if len(detail) > 500 {
		detail = detail[:500] + "…"
	}
	switch resp.StatusCode {
	case http.StatusPaymentRequired:
		// 402 is the whole statement — the status itself means "pay first".
		return outOfCreditsError(p.Name(), resp.StatusCode, detail)
	case http.StatusTooManyRequests, 529:
		// Same split the OpenAI-compatible path makes, for the same reason: a
		// host borrowing this format can spend its balance as easily as its
		// rate limit, and only one of the two is worth waiting out.
		if outOfCredits(body) {
			return outOfCreditsError(p.Name(), resp.StatusCode, detail)
		}
		if wait := rateLimitWindow(resp); wait > 0 {
			return fmt.Errorf("%s is rate limiting this key. Try again in %s.", p.Name(), humanizeDuration(wait))
		}
		return fmt.Errorf("%s is rate limiting this key. Try again shortly. (429)", p.Name())
	case http.StatusUnauthorized:
		return fmt.Errorf("%s rejected the credentials (401: %s)", p.Name(), detail)
	default:
		return fmt.Errorf("%s request failed with status %d: %s", p.Name(), resp.StatusCode, detail)
	}
}

// rateLimitWindow reads how long until the limit resets, preferring the plain
// Retry-After over Anthropic's own reset timestamps.
//
// The reset headers come in two dialects, seen live: API keys send the
// documented anthropic-ratelimit-*-reset headers as RFC3339, but a Pro/Max
// sign-in sends anthropic-ratelimit-unified-reset as Unix seconds — that one is
// the binding constraint (its Representative-Claim sibling says which window),
// so it is checked first, and both formats are accepted everywhere because the
// dialect is not ours to assume.
func rateLimitWindow(resp *http.Response) time.Duration {
	if wait, stated := providerRetryAfter(resp); stated && wait > 0 {
		return wait
	}
	for _, header := range []string{
		"anthropic-ratelimit-unified-reset",
		"anthropic-ratelimit-tokens-reset",
		"anthropic-ratelimit-requests-reset",
		"anthropic-ratelimit-input-tokens-reset",
	} {
		value := resp.Header.Get(header)
		if value == "" {
			continue
		}
		var when time.Time
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			when = parsed
		} else if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
			when = time.Unix(seconds, 0)
		} else {
			continue
		}
		if wait := time.Until(when); wait > 0 {
			return wait
		}
	}
	return 0
}
