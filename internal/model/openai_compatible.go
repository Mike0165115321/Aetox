package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Mikedev115/Aetox/internal/debuglog"
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
	File     *openAIFilePart `json:"file,omitempty"`
}

// openAIFilePart is how this wire format takes a document inline. The shape is
// not inferred from the image part beside it: it is what the AI SDK builds in
// packages/openai-compatible/src/chat/convert-to-openai-compatible-chat-messages.ts,
// which is the code most of this ecosystem actually talks to these endpoints
// with — `{type: "file", file: {filename, file_data}}`, file_data being a data:
// URL rather than bare base64.
//
// documentDataURL and documentFilename already produced exactly those two
// values. They were written for this and then never wired to anything: the
// only provider that has ever sent a document is the Responses one.
type openAIFilePart struct {
	Filename string `json:"filename"`
	FileData string `json:"file_data"`
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
		if len(m.Images) > 0 || len(m.Documents) > 0 {
			parts := make([]openAIContentPart, 0, len(m.Images)+len(m.Documents)+1)
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
			for _, doc := range m.Documents {
				parts = append(parts, openAIContentPart{
					Type: "file",
					File: &openAIFilePart{
						Filename: documentFilename(doc),
						FileData: documentDataURL(doc),
					},
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
		httpClient:  newModelHTTPClient(timeout, baseURL),
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

// statusError turns a rejected request into something the user can act on.
//
// The 429 split is the one that matters: this wire format uses the same status
// for "slow down" and "the account has no credit left", and only one of them
// fixes itself by waiting. Dumping the raw JSON body treated both as the user's
// problem to decode.
//
// Which of the two it is comes from outOfCredits, shared with the transport, so
// the sentence shown and the decision to stop retrying can never disagree.
func (p *OpenAICompatibleProvider) statusError(resp *http.Response, body []byte) error {
	// The provider's own sentence wherever it has one, and the raw payload only
	// when the body is shaped some other way. Each host words the instruction
	// differently ("check your plan and billing details", "no resource package,
	// please recharge") and that difference is usually the actionable part —
	// but the JSON around it never is, and it used to be what the user got.
	detail := providerErrorMessage(body)
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if len(detail) > 500 {
		detail = detail[:500] + "…"
	}
	switch resp.StatusCode {
	case http.StatusPaymentRequired:
		// 402 is the whole statement — the status itself means "pay first", so
		// there is no body to inspect and no dialect to keep up with.
		return outOfCreditsError(p.provider, resp.StatusCode, detail)
	case http.StatusTooManyRequests:
		if outOfCredits(body) {
			return outOfCreditsError(p.provider, resp.StatusCode, detail)
		}
		if wait, stated := providerRetryAfter(resp); stated && wait > 0 {
			return fmt.Errorf("%s is rate limiting this key. Try again in %s.", p.provider, humanizeDuration(wait))
		}
		return fmt.Errorf("%s is rate limiting this key. Try again shortly. (429)", p.provider)
	case http.StatusUnauthorized:
		// The provider's sentence first, then whatever it left out. A 401 that
		// says only "Incorrect API key provided" is complete on a provider with
		// one endpoint and misleading on one with six — see credentialHint.
		if hint := credentialHint(body); hint != "" {
			return fmt.Errorf("%s rejected the credentials (401: %s). %s", p.provider, detail, hint)
		}
		return fmt.Errorf("%s rejected the credentials (401: %s)", p.provider, detail)
	default:
		// A 404 is usually a model id nobody serves, and that message is
		// already clear. It is also how at least one provider says "the key is
		// good, the account is not entitled" — a different problem with a
		// different fix, and unreadable as the raw body. Asked before the
		// generic sentence, answered only when the provider's own words say so.
		if err := accountAccessError(p.provider, resp.StatusCode, body, detail); err != nil {
			return err
		}
		return fmt.Errorf("%s request failed with status %d: %s", p.provider, resp.StatusCode, detail)
	}
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

// SupportsToolCalling narrows a provider-wide yes to the one model in use.
//
// `return true` was this method's whole body, and tool_support.go had already
// written down what that costs: "OpenAICompatibleProvider claims tool support
// for every row it serves, so a model that turns out not to have it has nothing
// else to catch it." Measured against the catalog on 2026-08-23, the rows this
// client serves carry 117 models that cannot call a tool and were being sent
// tool definitions anyway — 69 of OpenRouter's 360, 39 of NVIDIA's 102, and 9
// of Groq's 15.
//
// This is the opposite error from the vision one. There Aetox called sighted
// models blind; here it calls toolless models capable. Both came from answering
// a per-model question at a level that cannot see the model.
//
// Only ever narrows. A catalog that has never heard of the model leaves the
// provider's answer alone, which is what keeps every local runtime and every id
// no table describes working exactly as before. The asymmetry is deliberate:
// wrongly withholding tools turns a coding agent into a chat window, so that
// answer is given only where the catalog names the model outright.
//
// It does not replace IsToolBlockRejection. That catches the model the catalog
// was wrong about, at the cost of one failed call; this stops the call from
// being made at all for the ones it has right.
func (p *OpenAICompatibleProvider) SupportsToolCalling() bool {
	return resolveModalities(p.provider, p.model).Tools
}

// modelToolCalling is the shared rule, written once so that a second provider
// adopting it cannot answer the same question differently.
func modelToolCalling(provider, modelName string, providerAnswer bool) bool {
	if !providerAnswer {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "" {
		return providerAnswer
	}
	installedCatalogMu.RLock()
	c := installedCatalog
	installedCatalogMu.RUnlock()
	facts, known := c.For(provider, name)
	if !known {
		return providerAnswer
	}
	if !facts.ToolCall {
		// Worth a line in the log: from the outside this looks like the agent
		// choosing not to use its tools, and the reason is two files away.
		debuglog.Msg("tools: %s/%s is listed as not tool-calling; sending none", provider, name)
	}
	return facts.ToolCall
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
		Model               string                 `json:"model"`
		Messages            []openAIRequestMessage `json:"messages"`
		Temperature         float64                `json:"temperature,omitempty"`
		MaxTokens           int                    `json:"max_tokens,omitempty"`
		MaxCompletionTokens int                    `json:"max_completion_tokens,omitempty"`
		Tools               []ToolDefinition       `json:"tools,omitempty"`
		ToolChoice          string                 `json:"tool_choice,omitempty"`
		Reasoning           *ReasoningConfig       `json:"reasoning,omitempty"`
		Thinking            *ThinkingConfig        `json:"thinking,omitempty"`
		ReasoningEffort     string                 `json:"reasoning_effort,omitempty"`
		IncludeReasoning    *bool                  `json:"include_reasoning,omitempty"`
		ReasoningSplit      *bool                  `json:"reasoning_split,omitempty"`
	}{
		Model:       model,
		Messages:    convertMessagesToOpenAI(req.Messages),
		Temperature: req.Temperature,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}
	p.setOutputCap(&payload.MaxTokens, &payload.MaxCompletionTokens, req.MaxTokens)
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
		// Asked of the MODEL, not of Groq. include_reasoning is a field only
		// Groq's reasoning models accept, and llama-3.3-70b-versatile — the
		// catalog's own fallback for this provider — answers it with
		// "400: `include_reasoning` is not supported with this model", which
		// ends the turn before a single token is generated.
		//
		// The table two files over already knew: resolveGroqThinkingCapabilities
		// only claims gpt-oss and qwen3, and everything else falls to the
		// conservative "no thinking" row. This branch was reached by provider
		// name alone and never asked it. reasoning_effort was already safe by
		// accident (wireEffort returns "" for an unsupporting model and
		// omitempty drops it); the bool pointer is what always got sent.
		if ResolveThinkingCapabilities(p.provider, model).Supported {
			payload.IncludeReasoning = boolPtr(false)
		}
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
	// Whether this request is carrying a document at all. The replay below is
	// gated on it so that a 400 mentioning "file" for some unrelated reason
	// cannot cost a second call.
	sentDocuments := false
	for _, m := range req.Messages {
		if len(m.Documents) > 0 {
			sentDocuments = true
			break
		}
	}

	if p.dropTemperature(model) {
		payload.Temperature = 0
	}

	send := func() (*http.Response, error) {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if err := p.applyAuth(ctx, httpReq); err != nil {
			return nil, err
		}
		httpResp, err := p.httpClient.Do(httpReq)
		if err != nil {
			return nil, err
		}
		NoteQuotas(p.Name(), httpResp)
		return httpResp, nil
	}
	httpResp, err := send()
	if err != nil {
		return Response{}, err
	}
	responseBody, err := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	if err != nil {
		return Response{}, err
	}

	// The flag is not cleared here: this is the only place that reads it, the
	// retry happens once, and the temperature check below never asks. Clearing
	// it was a write nothing could observe, which reads as state being kept.
	if documentPartRefused(httpResp.StatusCode, responseBody, sentDocuments) {
		payload.Messages = convertMessagesToOpenAI(stripDocuments(req.Messages))
		if httpResp, err = send(); err != nil {
			return Response{}, err
		}
		responseBody, err = io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if err != nil {
			return Response{}, err
		}
	}

	if temperatureRefused(httpResp.StatusCode, responseBody) && payload.Temperature != 0 {
		p.rememberTemperatureRefusal(model)
		payload.Temperature = 0
		if httpResp, err = send(); err != nil {
			return Response{}, err
		}
		responseBody, err = io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if err != nil {
			return Response{}, err
		}
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return Response{}, p.statusError(httpResp, responseBody)
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
		Model               string                 `json:"model"`
		Messages            []openAIRequestMessage `json:"messages"`
		Temperature         float64                `json:"temperature,omitempty"`
		MaxTokens           int                    `json:"max_tokens,omitempty"`
		MaxCompletionTokens int                    `json:"max_completion_tokens,omitempty"`
		Tools               []ToolDefinition       `json:"tools,omitempty"`
		ToolChoice          string                 `json:"tool_choice,omitempty"`
		Reasoning           *ReasoningConfig       `json:"reasoning,omitempty"`
		Thinking            *ThinkingConfig        `json:"thinking,omitempty"`
		ReasoningEffort     string                 `json:"reasoning_effort,omitempty"`
		IncludeReasoning    *bool                  `json:"include_reasoning,omitempty"`
		ReasoningSplit      *bool                  `json:"reasoning_split,omitempty"`
		Stream              bool                   `json:"stream"`
		// Without this the spec says a streamed response carries no usage at
		// all, and a server that follows it sends none: LM Studio recorded 0
		// tokens for every streamed turn, which is every desktop turn. DeepSeek
		// happens to send usage unasked, which is why this went unnoticed.
		StreamOptions *streamOptions `json:"stream_options,omitempty"`
	}{
		Model:         model,
		Messages:      convertMessagesToOpenAI(req.Messages),
		Temperature:   req.Temperature,
		Tools:         req.Tools,
		ToolChoice:    req.ToolChoice,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
	p.setOutputCap(&payload.MaxTokens, &payload.MaxCompletionTokens, req.MaxTokens)
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
		// Asked of the MODEL, not of Groq. include_reasoning is a field only
		// Groq's reasoning models accept, and llama-3.3-70b-versatile — the
		// catalog's own fallback for this provider — answers it with
		// "400: `include_reasoning` is not supported with this model", which
		// ends the turn before a single token is generated.
		//
		// The table two files over already knew: resolveGroqThinkingCapabilities
		// only claims gpt-oss and qwen3, and everything else falls to the
		// conservative "no thinking" row. This branch was reached by provider
		// name alone and never asked it. reasoning_effort was already safe by
		// accident (wireEffort returns "" for an unsupporting model and
		// omitempty drops it); the bool pointer is what always got sent.
		if ResolveThinkingCapabilities(p.provider, model).Supported {
			payload.IncludeReasoning = boolPtr(false)
		}
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

	// Whether this request is carrying a document at all. The replay below is
	// gated on it so that a 400 mentioning "file" for some unrelated reason
	// cannot cost a second call.
	sentDocuments := false
	for _, m := range req.Messages {
		if len(m.Documents) > 0 {
			sentDocuments = true
			break
		}
	}

	if p.dropTemperature(model) {
		payload.Temperature = 0
	}

	send := func() (*http.Response, error) {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if err := p.applyAuth(ctx, httpReq); err != nil {
			return nil, err
		}
		httpResp, err := p.httpClient.Do(httpReq)
		if err != nil {
			return nil, err
		}
		NoteQuotas(p.Name(), httpResp)
		return httpResp, nil
	}
	httpResp, err := send()
	if err != nil {
		return Response{}, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if !documentPartRefused(httpResp.StatusCode, responseBody, sentDocuments) &&
			(!temperatureRefused(httpResp.StatusCode, responseBody) || payload.Temperature == 0) {
			return Response{}, p.statusError(httpResp, responseBody)
		}
		// Safe to replay: the refusal arrives before the first SSE frame, so
		// nothing has been streamed to the user to take back.
		// Not cleared, for the reason the non-streaming path gives: one retry,
		// and nothing below asks the flag again.
		if documentPartRefused(httpResp.StatusCode, responseBody, sentDocuments) {
			payload.Messages = convertMessagesToOpenAI(stripDocuments(req.Messages))
		} else {
			p.rememberTemperatureRefusal(model)
			payload.Temperature = 0
		}
		if httpResp, err = send(); err != nil {
			return Response{}, err
		}
		if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
			retryBody, _ := io.ReadAll(httpResp.Body)
			httpResp.Body.Close()
			return Response{}, p.statusError(httpResp, retryBody)
		}
	}
	defer httpResp.Body.Close()

	scanner := newStreamScanner(httpResp.Body)
	var builder strings.Builder
	var reasoningBuilder strings.Builder
	var lastUsage *Usage
	var finishReason string
	// Counted only to be said out loud if this stream turns out to be empty.
	// "0 frames" is a gateway that opened a stream and closed it; a stream with
	// frames that still says nothing is the model. The error used to name
	// neither, which is why an empty answer from a gateway was unreadable.
	dataFrames := 0
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
		dataFrames++
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
	if err := errEmptyCompletion(p.provider, strings.TrimSpace(finishReason), reply, reasoning, len(toolCalls),
		fmt.Sprintf("%d stream frames", dataFrames)); err != nil {
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
	// Carried for the same reason ToolCall.ExtraContent exists: a signature
	// dropped here is a 400 on the next turn. Unlike arguments it arrives whole
	// rather than in pieces, so the accumulator takes the last non-empty one
	// instead of concatenating.
	ExtraContent json.RawMessage `json:"extra_content,omitempty"`
}

// streamToolAccumulator stitches streamed tool-call fragments back into whole
// ToolCalls, preserving first-seen order.
//
// Two wire shapes have to come out the same way, because both call themselves
// OpenAI-compatible:
//
//   - OpenAI's own: one call per index, opened by a fragment carrying id and
//     index, continued by fragments carrying index and a slice of the arguments.
//   - Gemini's compat layer: each call arrives WHOLE in its own chunk, with its
//     own id and NO index field at all — which decodes to 0 for every one of
//     them. Keying on the index alone folded a parallel pair into one call whose
//     arguments were the two JSON objects glued together
//     ({"city":"X"}{"city":"X"}), and sending that back cost the next turn a
//     flat "400 Request contains an invalid argument" with nothing named.
//     Measured against gemini-3.7-flash, 21 Aug 2026.
//
// So the id decides when there is one, and the index only fills in for the
// fragments that carry no id. A slot is the position in `calls`, which is also
// what the progress rows are keyed by — two calls that share a wire index must
// not share a row either.
type streamToolAccumulator struct {
	calls   []*ToolCall
	byIndex map[int]int
	byID    map[string]int
	// progress reports a call as it is written — see Request.OnToolCallProgress.
	progress *toolProgressTracker
}

func newStreamToolAccumulator(onProgress func(id, name, subject string, lines int)) *streamToolAccumulator {
	return &streamToolAccumulator{
		byIndex:  map[int]int{},
		byID:     map[string]int{},
		progress: newToolProgressTracker(onProgress),
	}
}

func (a *streamToolAccumulator) add(deltas []streamToolCallDelta) {
	for _, d := range deltas {
		slot := a.slotFor(d)
		call := a.calls[slot]
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
		if len(d.ExtraContent) > 0 {
			call.ExtraContent = d.ExtraContent
		}
		a.progress.report(slot, call.ID, call.Function.Name, call.Function.Arguments)
	}
}

// slotFor answers which call this fragment belongs to.
func (a *streamToolAccumulator) slotFor(d streamToolCallDelta) int {
	if d.ID == "" {
		// No id to go on: the index is all there is, and a fragment with no id
		// is a continuation by definition — OpenAI sends the id once, on the
		// fragment that opens the call.
		if slot, ok := a.byIndex[d.Index]; ok {
			return slot
		}
		return a.open(d.Index)
	}
	if slot, ok := a.byID[d.ID]; ok {
		return slot
	}
	// An id nobody has claimed. If the call sitting at this index has no id yet
	// it is the same call — a provider that opens with the index and names it a
	// fragment later. Otherwise this is a new call that happens to reuse the
	// index, which is every call after the first on Gemini.
	if slot, ok := a.byIndex[d.Index]; ok && a.calls[slot].ID == "" {
		a.byID[d.ID] = slot
		return slot
	}
	slot := a.open(d.Index)
	a.byID[d.ID] = slot
	return slot
}

// open starts a new call and gives it the index, so the fragments that follow
// with no id of their own reach the newest call rather than the oldest.
func (a *streamToolAccumulator) open(index int) int {
	a.calls = append(a.calls, &ToolCall{Type: "function"})
	slot := len(a.calls) - 1
	a.byIndex[index] = slot
	return slot
}

func (a *streamToolAccumulator) finalize() []ToolCall {
	if len(a.calls) == 0 {
		return nil
	}
	calls := make([]ToolCall, 0, len(a.calls))
	for slot, call := range a.calls {
		// The arguments are complete now, so this is the last chance for the row
		// to learn its subject — and the chance pacing would otherwise eat when
		// the naming argument came last. See toolProgressTracker.flush.
		a.progress.flush(slot, call.ID, call.Function.Name, call.Function.Arguments)
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

// temperatureRefused reports the one 400 that is a statement about the request
// rather than about the account, and the only one worth replaying.
//
// Aetox puts a temperature on every call (0.2, from the agent). A growing set
// of models accept exactly one value and reject the field outright:
//
//	OpenAI:   "Unsupported value: 'temperature' does not support 0.2 with this model"
//	Zen:      "[invalid_request_error] invalid temperature: only 1 is allowed for this model"
//
// The `openai` row already handles its own case by provider name, and that
// approach cannot be extended here: a gateway serves nine vendors down one
// base URL, so the rule is per MODEL and changes whenever any of those nine
// ships. A table of model ids kept in this file would be a copy of nine
// release schedules and wrong the week one of them moved.
//
// So it is read off what the server said instead. A 400 that names the
// temperature is the endpoint refusing the request shape, which is exactly the
// case where sending it again without the field is both safe and correct —
// dropping it (omitempty) leaves the model on its own default, which is the
// value it just said was the only one allowed.
// documentPartRefused reports the 400 that is a refusal of the FILE part
// carried with the request rather than of anything else about it.
//
// It exists because document_capabilities.go had written down its own blocker:
// "an unverified wire shape here is a 400 on a turn that works fine today, and
// the fallback it would replace is not broken." That was the reason only one
// provider had ever been allowed to send a document, and why the note beside it
// said each of the others should be added "as it is proven, not as it is read
// about."
//
// This is what makes the rest of them addable. The `file` part is read from a
// real implementation (the AI SDK's openai-compatible chat converter, which is
// what most of this ecosystem actually talks to these endpoints with) but it is
// not verified against every gateway that speaks this dialect, and there are
// fourteen. So a refusal is caught, the documents are dropped, and the turn is
// replayed without them — which lands the user exactly where they were before
// this path existed, on pdf_read, having spent one extra call rather than a
// whole turn.
//
// Narrower than temperatureRefused on purpose: it only fires when documents
// were actually sent, so a 400 that merely happens to contain the word "file"
// cannot cost anybody a second request.
func documentPartRefused(status int, body []byte, sentDocuments bool) bool {
	if !sentDocuments || status != http.StatusBadRequest {
		return false
	}
	said := strings.ToLower(string(body))
	for _, phrase := range []string{
		"file", "document", "input_file", "file_data", "unsupported content",
	} {
		if strings.Contains(said, phrase) {
			return true
		}
	}
	return false
}

// stripDocuments returns the messages without their document parts, for the
// replay above. The text and images are untouched: only the part that was
// refused is withdrawn.
func stripDocuments(msgs []Message) []Message {
	out := make([]Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		out[i].Documents = nil
	}
	return out
}

func temperatureRefused(status int, body []byte) bool {
	return status == http.StatusBadRequest &&
		strings.Contains(strings.ToLower(string(body)), "temperature")
}

// refusedTemperature remembers the models that have answered that way, so the
// replay costs one extra round trip per model per run instead of one per turn.
// Deliberately not persisted: it is a fact about a deployment, and a provider
// that widens what it accepts should not have to wait for a file to be cleared.
var refusedTemperature sync.Map

// dropTemperature answers, before the first attempt, whether this model will
// refuse one — so the replay below is a safety net rather than a toll.
//
// Two sources, in that order of authority: what this process has already been
// told to its face, then what the fetched catalog claims. The measured one is
// first because it cannot be stale about this deployment, and the catalog is a
// third party the `nvidia` row already proves can be wrong about an endpoint.
func (p *OpenAICompatibleProvider) dropTemperature(modelID string) bool {
	if _, seen := refusedTemperature.Load(p.provider + "/" + modelID); seen {
		return true
	}
	return catalogRefusesTemperature(p.provider, modelID)
}

func (p *OpenAICompatibleProvider) rememberTemperatureRefusal(modelID string) {
	refusedTemperature.Store(p.provider+"/"+modelID, struct{}{})
}

func (p *OpenAICompatibleProvider) usesDeepSeekThinking() bool {
	return NormalizeProvider(p.provider) == "deepseek"
}

// setOutputCap writes the output ceiling into whichever field this host calls
// it, because the two spellings are not interchangeable and picking the wrong
// one ends the turn before a token is generated.
//
// OpenAI retired max_tokens on its newer models and answers them with
// "400: Unsupported parameter: 'max_tokens' is not supported with this model.
// Use 'max_completion_tokens' instead" — measured on gpt-5.6-sol, 21 Aug 2026,
// where it made every chat on the openai provider fail on the first turn.
// max_completion_tokens is accepted by the older chat models too, so this is
// per-provider rather than per-model: a model list kept here would be a second
// copy of OpenAI's release schedule, and it would be wrong the week they ship.
//
// Everybody else keeps max_tokens. The name is not OpenAI's to change for
// hosts that only speak its wire format — llama.cpp, LM Studio, Ollama's compat
// endpoint and most of the catalog reject a field they have never heard of.
func (p *OpenAICompatibleProvider) setOutputCap(maxTokens, maxCompletionTokens *int, want int) {
	if want <= 0 {
		return
	}
	if p.usesMaxCompletionTokens() {
		*maxCompletionTokens = want
		return
	}
	*maxTokens = want
}

func (p *OpenAICompatibleProvider) usesMaxCompletionTokens() bool {
	return NormalizeProvider(p.provider) == "openai"
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
	switch NormalizeProvider(p.provider) {
	case "kimi", "opencode", "opencode-go":
		// The OpenCode rows joined rather than growing a branch, which is what
		// the note above asked the next provider to do. Not inferred from a
		// successful call either: parseOpenAiVariant in their own repo
		// (routes/zen/util/variant.ts) reads
		// `body.reasoningEffort ?? body.reasoning_effort ?? body.reasoning?.effort`,
		// so the top-level field is the first spelling it looks for.
		//
		// The gateway does nothing else with it — handler.ts passes the value
		// to logger.metric and forwards the body untouched — so whether an
		// effort is honoured is the upstream model's business, exactly as if
		// that vendor had been called directly. Which models have a dial is
		// therefore a per-model question, answered by the fetched catalog in
		// ResolveThinkingCapabilities rather than by a list kept here.
		return true
	}
	return false
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
