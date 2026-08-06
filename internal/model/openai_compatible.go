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
	"strings"
	"time"
)

// openAIMessage mirrors Message field for field, tag for tag and — this is the
// part that matters — in the same order, differing only in that `content` can
// be a string or an array of parts. Go emits JSON object keys in struct field
// order, so keeping the order means a message with no images serializes to
// exactly the bytes it did before this type existed. That is not tidiness: the
// conversation prefix is what providers cache, and a reordered key would miss
// the cache on every turn for every user (Registry.Names carries the same note
// for the same reason).
type openAIRequestMessage struct {
	Role             MessageRole `json:"role"`
	Content          any         `json:"content"`
	ReasoningContent string      `json:"reasoning_content,omitempty"`
	Name             string      `json:"name,omitempty"`
	ToolCallID       string      `json:"tool_call_id,omitempty"`
	ToolCalls        []ToolCall  `json:"tool_calls,omitempty"`
}

type openAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

// openAIImageURL is a struct rather than a bare string because the field is an
// object in the spec, and providers that reject a plain string do so with a
// generic schema error that is miserable to trace back to this line.
type openAIImageURL struct {
	URL string `json:"url"`
}

// convertMessagesToOpenAI is the identity function for every message without an
// image, and the parts form for the few that carry one.
func convertMessagesToOpenAI(msgs []Message) []openAIRequestMessage {
	out := make([]openAIRequestMessage, 0, len(msgs))
	for _, m := range msgs {
		converted := openAIRequestMessage{
			Role:             m.Role,
			Content:          m.Content,
			ReasoningContent: m.ReasoningContent,
			Name:             m.Name,
			ToolCallID:       m.ToolCallID,
			ToolCalls:        m.ToolCalls,
		}
		if len(m.Images) > 0 {
			parts := make([]openAIContentPart, 0, len(m.Images)+1)
			// Text first: the question about the picture reads as a caption
			// under it otherwise, and several providers weight the last part
			// most heavily.
			if m.Content != "" {
				parts = append(parts, openAIContentPart{Type: "text", Text: m.Content})
			}
			for _, img := range m.Images {
				parts = append(parts, openAIContentPart{
					Type:     "image_url",
					ImageURL: &openAIImageURL{URL: dataURL(img)},
				})
			}
			converted.Content = parts
		}
		out = append(out, converted)
	}
	return out
}

// dataURL wraps an image the way the OpenAI-compatible APIs take one inline —
// the same `data:` form a browser understands, which is also what the desktop
// already builds for the composer's thumbnail.
func dataURL(img Image) string {
	mediaType := strings.TrimSpace(img.MediaType)
	if mediaType == "" {
		mediaType = "image/png"
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
}

// documentDataURL is dataURL for a file. Split rather than made generic over
// both: the default media type differs, and a document silently defaulting to
// image/png would be a very confusing 400.
func documentDataURL(doc Document) string {
	mediaType := strings.TrimSpace(doc.MediaType)
	if mediaType == "" {
		mediaType = "application/pdf"
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(doc.Data)
}

// documentFilename is what the model refers to the document by. Never empty:
// at least one backend rejects the part without it, and "document.pdf" is a
// better failure than a 400.
func documentFilename(doc Document) string {
	if name := strings.TrimSpace(doc.Name); name != "" {
		return name
	}
	return "document.pdf"
}

type OpenAICompatibleConfig struct {
	Provider      string
	Model         string
	APIKey        string
	BaseURL       string
	Timeout       time.Duration
	RequireAPIKey *bool
	// TokenSource, when set, replaces APIKey and is consulted per request
	// rather than once at construction — an OAuth access token expires, and a
	// provider built at startup would otherwise be holding a dead one an hour
	// into the session. Nil is the API-key path, unchanged.
	TokenSource func(context.Context) (string, error)
	// Headers are extra headers this provider's credentials require (Copilot
	// refuses requests that do not identify an editor client).
	Headers map[string]string
}

type OpenAICompatibleProvider struct {
	provider    string
	model       string
	apiKey      string
	baseURL     string
	reasoning   bool
	tokenSource func(context.Context) (string, error)
	headers     map[string]string
	httpClient  *http.Client
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
	if requireAPIKey && apiKey == "" && cfg.TokenSource == nil {
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
		provider:    provider,
		model:       model,
		apiKey:      apiKey,
		baseURL:     baseURL,
		reasoning:   supportsNativeReasoning(provider),
		tokenSource: cfg.TokenSource,
		headers:     cfg.Headers,
		httpClient:  newModelHTTPClient(timeout),
	}, nil
}

// applyAuth sets every header that depends on credentials. It runs per
// request, which is the whole point: a TokenSource refreshes an expiring token
// here, where a failure can still be reported as a request error.
func (p *OpenAICompatibleProvider) applyAuth(ctx context.Context, req *http.Request) error {
	req.Header.Set("Content-Type", "application/json")
	for name, value := range p.headers {
		req.Header.Set(name, value)
	}
	if p.tokenSource != nil {
		token, err := p.tokenSource(ctx)
		if err != nil {
			return fmt.Errorf("%s sign-in: %w", p.provider, err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	return nil
}

// openAIMessage is the response-side message shape: the shared Message plus
// the other spelling of the reasoning field. Message carries DeepSeek's
// "reasoning_content"; Ollama's OpenAI-compatible endpoint and llama.cpp
// (LM Studio's runtime) send plain "reasoning" instead, which was dropped —
// so a thinking model that had spent its budget reasoning came back with an
// empty content field and was reported as "response has empty text" against a
// server that had answered perfectly. Same bug ollamaMessage.reasoning already
// fixes for the native Ollama client.
type openAIMessage struct {
	Message
	Reasoning string `json:"reasoning"`
}

// reasoningText returns whichever field the provider populated. Not trimmed:
// streaming deltas split mid-sentence, and trimming each chunk eats the spaces
// between them.
func (m openAIMessage) reasoningText() string {
	if m.ReasoningContent != "" {
		return m.ReasoningContent
	}
	return m.Reasoning
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

// streamOptions asks for the usage object on a streamed response. The spec
// makes it opt-in, so a server that follows it reports nothing without this.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
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
		Model            string                 `json:"model"`
		Messages         []openAIRequestMessage `json:"messages"`
		Temperature      float64                `json:"temperature,omitempty"`
		MaxTokens        int                    `json:"max_tokens,omitempty"`
		Tools            []ToolDefinition       `json:"tools,omitempty"`
		ToolChoice       string                 `json:"tool_choice,omitempty"`
		Reasoning        *ReasoningConfig       `json:"reasoning,omitempty"`
		Thinking         *ThinkingConfig        `json:"thinking,omitempty"`
		ReasoningEffort  string                 `json:"reasoning_effort,omitempty"`
		IncludeReasoning *bool                  `json:"include_reasoning,omitempty"`
		ReasoningSplit   *bool                  `json:"reasoning_split,omitempty"`
	}{
		Model:       model,
		Messages:    convertMessagesToOpenAI(req.Messages),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}
	if p.usesDeepSeekThinking() {
		payload.Thinking = normalizeDeepSeekThinking(req.Thinking, req.Reasoning)
		payload.ReasoningEffort = p.wireEffort(model, req.Reasoning)
	} else if p.usesMiniMaxThinking() {
		// The whole dial is the thinking block; there is no effort field to
		// send, so none is sent. reasoning_split is not optional in practice:
		// left off, the thinking comes back inside content wrapped in
		// <think>...</think> and lands in the user's answer instead of the
		// thinking panel.
		payload.Thinking = req.Thinking
		payload.ReasoningSplit = boolPtr(true)
	} else if p.usesGroqReasoning() {
		payload.ReasoningEffort = p.wireEffort(model, req.Reasoning)
		payload.IncludeReasoning = boolPtr(false)
	} else if p.usesOpenAIReasoningEffort() || p.usesGeminiReasoningEffort() || p.usesPlainReasoningEffort() {
		payload.ReasoningEffort = p.wireEffort(model, req.Reasoning)
		if payload.ReasoningEffort != "" && p.usesOpenAIReasoningEffort() {
			// OpenAI's reasoning models reject temperature outright ("Unsupported
			// value: 'temperature'"), the same way Anthropic rejects it alongside
			// extended thinking. Aetox sets one on every call, so a reasoning
			// effort and a temperature together is a 400 that ends the turn.
			payload.Temperature = 0
		}
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
	if err := p.applyAuth(ctx, httpReq); err != nil {
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
				openAIMessage
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

	rawMessage := parsed.Choices[0].Message.openAIMessage.Message
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
	reasoning := strings.TrimSpace(parsed.Choices[0].Message.reasoningText())
	if err := errEmptyCompletion(p.provider, strings.TrimSpace(parsed.Choices[0].FinishReason), text, reasoning, len(rawMessage.ToolCalls)); err != nil {
		return Response{}, err
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
		Model            string                 `json:"model"`
		Messages         []openAIRequestMessage `json:"messages"`
		Temperature      float64                `json:"temperature,omitempty"`
		MaxTokens        int                    `json:"max_tokens,omitempty"`
		Tools            []ToolDefinition       `json:"tools,omitempty"`
		ToolChoice       string                 `json:"tool_choice,omitempty"`
		Reasoning        *ReasoningConfig       `json:"reasoning,omitempty"`
		Thinking         *ThinkingConfig        `json:"thinking,omitempty"`
		ReasoningEffort  string                 `json:"reasoning_effort,omitempty"`
		IncludeReasoning *bool                  `json:"include_reasoning,omitempty"`
		ReasoningSplit   *bool                  `json:"reasoning_split,omitempty"`
		Stream           bool                   `json:"stream"`
		// Without this the spec says a streamed response carries no usage at
		// all, and a server that follows it sends none: LM Studio recorded 0
		// tokens for every streamed turn, which is every desktop turn. DeepSeek
		// happens to send usage unasked, which is why this went unnoticed.
		StreamOptions *streamOptions `json:"stream_options,omitempty"`
	}{
		Model:         model,
		Messages:      convertMessagesToOpenAI(req.Messages),
		Temperature:   req.Temperature,
		MaxTokens:     req.MaxTokens,
		Tools:         req.Tools,
		ToolChoice:    req.ToolChoice,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
	if p.usesDeepSeekThinking() {
		payload.Thinking = normalizeDeepSeekThinking(req.Thinking, req.Reasoning)
		payload.ReasoningEffort = p.wireEffort(model, req.Reasoning)
	} else if p.usesMiniMaxThinking() {
		// The whole dial is the thinking block; there is no effort field to
		// send, so none is sent. reasoning_split is not optional in practice:
		// left off, the thinking comes back inside content wrapped in
		// <think>...</think> and lands in the user's answer instead of the
		// thinking panel.
		payload.Thinking = req.Thinking
		payload.ReasoningSplit = boolPtr(true)
	} else if p.usesGroqReasoning() {
		payload.ReasoningEffort = p.wireEffort(model, req.Reasoning)
		payload.IncludeReasoning = boolPtr(false)
	} else if p.usesOpenAIReasoningEffort() || p.usesGeminiReasoningEffort() || p.usesPlainReasoningEffort() {
		payload.ReasoningEffort = p.wireEffort(model, req.Reasoning)
		if payload.ReasoningEffort != "" && p.usesOpenAIReasoningEffort() {
			// OpenAI's reasoning models reject temperature outright ("Unsupported
			// value: 'temperature'"), the same way Anthropic rejects it alongside
			// extended thinking. Aetox sets one on every call, so a reasoning
			// effort and a temperature together is a 400 that ends the turn.
			payload.Temperature = 0
		}
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
	if err := p.applyAuth(ctx, httpReq); err != nil {
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
					openAIMessage
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
		if reasoningChunk := delta.reasoningText(); reasoningChunk != "" {
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
	if err := errEmptyCompletion(p.provider, strings.TrimSpace(finishReason), reply, reasoning, len(toolCalls)); err != nil {
		return Response{}, err
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

// supportsNativeReasoning asks the provider catalog, which is where this fact
// already lived. The hardcoded list this replaces was a second copy of it, and
// the two had to be edited together every time a provider was added — the same
// arrangement that let the thinking tables drift apart.
func supportsNativeReasoning(provider string) bool {
	_, canReason := providerReasoningCapability(NormalizeProvider(provider))
	return canReason
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

// usesPlainReasoningEffort is the top-level reasoning_effort field with none of
// its neighbours' quirks: Kimi keeps temperature (OpenAI rejects it alongside
// reasoning) and wants the reasoning back (Groq is asked to suppress it).
//
// Named for the shape rather than for Kimi, because the next provider to speak
// this dialect should join the list instead of growing a fourth branch — this
// chain of provider tests is the same drift the effort tables just came out of.
func (p *OpenAICompatibleProvider) usesPlainReasoningEffort() bool {
	return NormalizeProvider(p.provider) == "kimi"
}

func (p *OpenAICompatibleProvider) usesMiniMaxThinking() bool {
	return NormalizeProvider(p.provider) == "minimax"
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

// wireEffort is what reasoning_effort should say for this provider and model.
//
// It replaces two switches — one for DeepSeek, one for everybody else — that
// between them were the third and fourth places encoding which levels a
// provider has. The DeepSeek one is the reason this consolidation happened: it
// folded low and medium onto "high" and xhigh onto "max", so on this wire
// format three of DeepSeek's six real levels could not be reached at all.
func (p *OpenAICompatibleProvider) wireEffort(model string, reasoning *ReasoningConfig) string {
	if reasoning == nil {
		return ""
	}
	effort, ok := WireEffort(p.provider, model, reasoning.Effort)
	if !ok {
		return ""
	}
	return effort
}

func boolPtr(value bool) *bool {
	return &value
}
