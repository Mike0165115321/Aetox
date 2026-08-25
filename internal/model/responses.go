package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/debuglog"
)

// The Responses runtime — the wire format a ChatGPT subscription speaks.
//
// It is a fourth wire format rather than a variation on the OpenAI-compatible
// one, because chatgpt.com/backend-api/codex serves only /responses and that
// endpoint disagrees with /chat/completions on nearly everything that matters:
//
//	chat/completions            responses
//	messages[]                  instructions + input[]
//	role/content per message    typed items (message, function_call, ...)
//	tools[].function.name       tools[].name
//	tool role message           function_call_output item
//	choices[].delta             a dozen named SSE event types
//
// It also has no non-streaming mode worth using: the backend answers
// text/event-stream either way, so Complete below runs the stream and collects
// it rather than maintaining a second parser that would drift.

type ResponsesConfig struct {
	Provider    string
	Model       string
	BaseURL     string
	Timeout     time.Duration
	APIKey      string
	TokenSource func(context.Context) (string, error)
	Headers     map[string]string
}

type ResponsesProvider struct {
	provider    string
	model       string
	baseURL     string
	apiKey      string
	tokenSource func(context.Context) (string, error)
	headers     map[string]string
	httpClient  *http.Client
}

func NewResponsesProvider(cfg ResponsesConfig) (*ResponsesProvider, error) {
	provider := strings.TrimSpace(strings.ToLower(cfg.Provider))
	if provider == "" {
		provider = "chatgpt"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, ErrMissingModel
	}
	if strings.TrimSpace(cfg.APIKey) == "" && cfg.TokenSource == nil {
		return nil, ErrMissingAPIKey
	}
	baseURL := strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimSuffix(DefaultBaseURL(provider), "/")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	return &ResponsesProvider{
		provider:    provider,
		model:       model,
		baseURL:     baseURL,
		apiKey:      strings.TrimSpace(cfg.APIKey),
		tokenSource: cfg.TokenSource,
		headers:     cfg.Headers,
		httpClient:  newModelHTTPClient(timeout, baseURL),
	}, nil
}

func (p *ResponsesProvider) Name() string { return p.provider }

func (p *ResponsesProvider) SupportsToolCalling() bool { return true }

func (p *ResponsesProvider) SupportsReasoning() bool { return true }

// ---------------------------------------------------------------------------
// Request
// ---------------------------------------------------------------------------

type responsesRequest struct {
	Model string `json:"model"`
	// Instructions is where the system prompt goes. There is no system item:
	// a system message sent as input is accepted and ignored.
	Instructions string           `json:"instructions,omitempty"`
	Input        []responsesItem  `json:"input"`
	Tools        []responsesTool  `json:"tools,omitempty"`
	ToolChoice   string           `json:"tool_choice,omitempty"`
	Reasoning    *responsesReason `json:"reasoning,omitempty"`
	Text         *responsesText   `json:"text,omitempty"`
	// No max_output_tokens and no temperature. The ChatGPT backend rejects both
	// outright ("Unsupported parameter: max_output_tokens") — this endpoint
	// serves a subscription, and the knobs a per-token API exposes are not part
	// of the deal. Aetox's MaxTokens/Temperature are simply not expressible
	// here, which is a truthful answer rather than a silently ignored one.
	Include []string `json:"include,omitempty"`
	Stream  bool     `json:"stream"`
	// Store must be false. The ChatGPT backend refuses to persist turns for a
	// third-party client, and a stored turn would also mean the user's code
	// sitting in someone else's conversation history.
	Store    bool `json:"store"`
	Parallel bool `json:"parallel_tool_calls"`
}

type responsesItem struct {
	Type string `json:"type"`
	Role string `json:"role,omitempty"`
	// Content is used by message items only.
	Content []responsesContent `json:"content,omitempty"`
	// The function_call / function_call_output fields.
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Output    string `json:"output,omitempty"`
}

type responsesContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	// The input_file pair. FileData is a data: URL, not bare base64 — the
	// backend answers a bare payload with "Invalid 'input[N].content[M]
	// .file_data'", which names the field but not the reason. Filename is sent
	// alongside because the model is expected to refer to the document by it.
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data,omitempty"`
}

// responsesTool is flatter than the chat/completions shape: name and parameters
// sit on the tool, not inside a nested "function" object. Sending the nested
// form is a 400 that names no field.
type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict"`
}

type responsesReason struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type responsesText struct {
	Verbosity string `json:"verbosity,omitempty"`
}

func buildResponsesRequest(provider, model string, req Request) (responsesRequest, error) {
	instructions, input := convertMessagesToResponses(req.Messages)
	if len(input) == 0 {
		return responsesRequest{}, ErrNoMessages
	}

	out := responsesRequest{
		Model:        model,
		Instructions: instructions,
		Input:        input,
		Tools:        convertToolsToResponses(req.Tools),
		Stream:       true,
		Store:        false,
		Parallel:     false,
	}
	if len(out.Tools) > 0 {
		choice := strings.TrimSpace(req.ToolChoice)
		if choice == "" {
			choice = "auto"
		}
		out.ToolChoice = choice
	}
	if effort := responsesEffort(provider, model, req.Reasoning); effort != "" {
		// The summary field is what makes the model's thinking visible at all
		// on this endpoint — raw chain-of-thought is never exposed, and without
		// asking, the reasoning happens and streams nothing, leaving Aetox's
		// thinking panel empty on a model that is thinking.
		//
		// "auto", which is the endpoint's own default, and back to it after two
		// weeks on "detailed".
		//
		// The trade this was meant to win: on 2026-08-07 auto produced one
		// headline in one burst at the end — a panel reading "คิดเป็นเวลา 1
		// วินาที" over a bold line, on a turn the model billed far more thinking
		// for. "detailed" was supposed to write sections as it goes so the panel
		// streams while the thinking happens, and it was changed on that single
		// observation, against the public API's documented behaviour rather than
		// against this backend.
		//
		// What followed: 1,266 turns ran with thinking on across gpt-5.6-luna,
		// terra and 5.4-mini between 2026-08-08 and 2026-08-20, and the owner
		// reports the panel never showed anything at all in that whole stretch.
		// One headline was little; nothing is worse, and a value the endpoint
		// documents as its default is the safer place to sit while the question
		// is open.
		//
		// **Provisional.** What settles it is TestLiveCodexReasoningReachesThe
		// ThinkingPanel, which asks the backend directly and records which event
		// names arrive. It could not run on 2026-08-20: the request was accepted
		// and answered 429 plan_type=free, resets_in ~25 days. Run it when the
		// plan resets — or on a paid one — before either value is called correct.
		out.Reasoning = &responsesReason{Effort: effort, Summary: "auto"}
		// With store:false the reasoning is not kept server-side, so it has to
		// come back to us to be echoed into the next turn.
		out.Include = []string{"reasoning.encrypted_content"}
	}
	return out, nil
}

// convertMessagesToResponses turns Aetox's flat message list into the typed
// item list this endpoint wants. The two shapes that have no counterpart
// elsewhere are the pair that carry a tool round trip: the assistant's call
// becomes a function_call item of its own (not a field on a message), and the
// result comes back as function_call_output keyed by the same call_id.
func convertMessagesToResponses(msgs []Message) (instructions string, input []responsesItem) {
	var systemParts []string

	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			if s := strings.TrimSpace(m.Content); s != "" {
				systemParts = append(systemParts, s)
			}

		case RoleTool:
			input = append(input, responsesItem{
				Type:   "function_call_output",
				CallID: m.ToolCallID,
				Output: m.Content,
			})

		case RoleAssistant:
			if text := strings.TrimSpace(m.Content); text != "" {
				input = append(input, responsesItem{
					Type: "message",
					Role: "assistant",
					Content: []responsesContent{
						{Type: "output_text", Text: text},
					},
				})
			}
			for _, call := range m.ToolCalls {
				args := call.Function.Arguments
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				input = append(input, responsesItem{
					Type:      "function_call",
					Name:      call.Function.Name,
					Arguments: args,
					CallID:    call.ID,
				})
			}

		default:
			content := make([]responsesContent, 0, len(m.Images)+len(m.Documents)+1)
			if text := m.Content; text != "" {
				content = append(content, responsesContent{Type: "input_text", Text: text})
			}
			for _, img := range m.Images {
				content = append(content, responsesContent{
					Type:     "input_image",
					ImageURL: dataURL(img),
				})
			}
			for _, doc := range m.Documents {
				content = append(content, responsesContent{
					Type:     "input_file",
					Filename: documentFilename(doc),
					FileData: documentDataURL(doc),
				})
			}
			if len(content) == 0 {
				continue
			}
			input = append(input, responsesItem{Type: "message", Role: "user", Content: content})
		}
	}

	return strings.Join(systemParts, "\n\n"), input
}

func convertToolsToResponses(tools []ToolDefinition) []responsesTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]responsesTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, responsesTool{
			Type:        "function",
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
			// strict:false on purpose. Strict mode rejects any schema without
			// additionalProperties:false on every object, and Aetox's tool
			// schemas are written for four providers, not for this one.
			Strict: false,
		})
	}
	return out
}

func (p *ResponsesProvider) newHTTPRequest(ctx context.Context, body []byte) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	// responses=experimental is required by the ChatGPT backend; the public
	// API ignores it, so it is safe to send on both.
	httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	for name, value := range p.headers {
		httpReq.Header.Set(name, value)
	}
	if p.tokenSource != nil {
		token, err := p.tokenSource(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s sign-in: %w", p.provider, err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
		return httpReq, nil
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	return httpReq, nil
}

// ---------------------------------------------------------------------------
// Streaming
// ---------------------------------------------------------------------------

// responsesEvent is the union of every event this runtime acts on. Unknown
// types fall through untouched: the endpoint adds event types over time and a
// client that errors on one it has not seen breaks on someone else's release.
type responsesEvent struct {
	Type   string `json:"type"`
	Delta  string `json:"delta"`
	ItemID string `json:"item_id"`
	Item   *struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
	Response *struct {
		Model  string          `json:"model"`
		Status string          `json:"status"`
		Usage  *responsesUsage `json:"usage"`
		Error  *struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
	Message string `json:"message"`
}

type responsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

func (u responsesUsage) toUsage() Usage {
	return Usage{
		// input_tokens already includes the cached part, matching Usage's
		// documented meaning — no addition needed here, unlike the Anthropic
		// adapter.
		PromptTokens:       u.InputTokens,
		CachedPromptTokens: u.InputTokensDetails.CachedTokens,
		CacheReported:      true,
		CompletionTokens:   u.OutputTokens,
		TotalTokens:        u.TotalTokens,
	}
}

type responsesToolBuilder struct {
	callID  string
	name    string
	argsBuf strings.Builder
	order   int
}

func (p *ResponsesProvider) Complete(ctx context.Context, req Request) (Response, error) {
	// One parser, not two: this endpoint streams whatever you ask it for.
	return p.StreamComplete(ctx, req, nil, nil)
}

func (p *ResponsesProvider) StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler, onReasoningChunk StreamChunkHandler) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	payload, err := buildResponsesRequest(p.provider, model, req)
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
	// Before the status check: a 429 states the same rate-limit headers as a
	// 200, and is precisely the moment the remaining quota is worth knowing.
	NoteQuotas(p.Name(), httpResp)

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return Response{}, p.statusError(httpResp)
	}

	var text, reasoning strings.Builder
	builders := map[string]*responsesToolBuilder{}
	var order []string
	// Event types this switch does not handle, logged once each per stream.
	//
	// The switch below has no default, so an event shape the backend changed or
	// never sent in the first place is indistinguishable here from one that
	// simply did not occur — and the symptom of that is a UI panel that is
	// empty for a reason nothing on the machine records. The thinking panel
	// went quiet on Codex at some point between 2026-08-08 and 2026-08-20 and
	// there was no way to tell whether the summary events stopped arriving,
	// arrived under other names, or were never requested; every answer to that
	// was a guess, because this loop threw the evidence away.
	//
	// Names only, never payloads: the point is to learn which shapes arrive,
	// and the content of a reasoning summary is the user's conversation.
	unknownEvents := map[string]bool{}
	progress := newToolProgressTracker(req.OnToolCallProgress)
	respModel := model
	var usage Usage
	var streamErr error

	err = scanSSE(httpResp.Body, func(data string) (bool, error) {
		var event responsesEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			// A malformed frame is not worth killing a turn over — the ones
			// that matter are all well-formed, and the endpoint interleaves
			// keep-alive shapes this struct does not model.
			return false, nil
		}

		switch event.Type {
		case "response.output_text.delta":
			if event.Delta == "" {
				return false, nil
			}
			text.WriteString(event.Delta)
			if onChunk != nil {
				if err := onChunk(event.Delta); err != nil {
					return true, err
				}
			}

		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			if event.Delta == "" {
				return false, nil
			}
			reasoning.WriteString(event.Delta)
			if onReasoningChunk != nil {
				if err := onReasoningChunk(event.Delta); err != nil {
					return true, err
				}
			}

		case "response.output_item.added":
			if event.Item == nil || event.Item.Type != "function_call" {
				return false, nil
			}
			key := event.Item.ID
			if key == "" {
				key = event.Item.CallID
			}
			builder := &responsesToolBuilder{
				callID: event.Item.CallID,
				name:   event.Item.Name,
				order:  len(order),
			}
			builder.argsBuf.WriteString(event.Item.Arguments)
			builders[key] = builder
			order = append(order, key)
			// The name arrives before any argument, so the row can be drawn
			// immediately — the same reason the Anthropic path reports here.
			progress.report(builder.order, builder.callID, builder.name, builder.argsBuf.String())

		case "response.function_call_arguments.delta":
			builder, ok := builders[event.ItemID]
			if !ok {
				return false, nil
			}
			builder.argsBuf.WriteString(event.Delta)
			progress.report(builder.order, builder.callID, builder.name, builder.argsBuf.String())

		case "response.output_item.done":
			if event.Item == nil || event.Item.Type != "function_call" {
				return false, nil
			}
			key := event.Item.ID
			if key == "" {
				key = event.Item.CallID
			}
			builder, ok := builders[key]
			if !ok {
				builder = &responsesToolBuilder{order: len(order)}
				builders[key] = builder
				order = append(order, key)
			}
			// The done event carries the complete arguments. Trusting it over
			// the accumulated deltas is deliberate: a dropped delta produces
			// invalid JSON that fails at tool-dispatch time, far from here.
			if event.Item.Arguments != "" {
				builder.argsBuf.Reset()
				builder.argsBuf.WriteString(event.Item.Arguments)
			}
			if event.Item.CallID != "" {
				builder.callID = event.Item.CallID
			}
			if event.Item.Name != "" {
				builder.name = event.Item.Name
			}

		case "response.completed":
			if event.Response != nil {
				if event.Response.Model != "" {
					respModel = event.Response.Model
				}
				if event.Response.Usage != nil {
					usage = event.Response.Usage.toUsage()
				}
			}
			return true, nil

		case "response.failed", "response.incomplete":
			streamErr = fmt.Errorf("%s: %s", p.provider, responsesErrorText(event))
			return true, nil

		case "error":
			streamErr = fmt.Errorf("%s: %s", p.provider, responsesErrorText(event))
			return true, nil

		default:
			if t := strings.TrimSpace(event.Type); t != "" && !unknownEvents[t] {
				unknownEvents[t] = true
				debuglog.Msg("responses: unhandled event %q from %s", t, p.provider)
			}
		}
		return false, nil
	})
	if err != nil {
		return Response{}, err
	}
	if streamErr != nil {
		return Response{}, streamErr
	}

	toolCalls := make([]ToolCall, 0, len(order))
	for _, key := range order {
		builder := builders[key]
		if builder.name == "" {
			continue
		}
		args := strings.TrimSpace(builder.argsBuf.String())
		if args == "" {
			args = "{}"
		}
		callID := builder.callID
		if callID == "" {
			callID = key
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:       callID,
			Type:     "function",
			Function: FunctionCall{Name: builder.name, Arguments: args},
		})
	}

	textOut := strings.TrimSpace(text.String())
	reasoningOut := strings.TrimSpace(reasoning.String())
	// No finish reason to state: this endpoint reports a truncation as its own
	// "response.incomplete" event, which has already returned above.
	if err := errEmptyCompletion(p.provider, "", textOut, reasoningOut, len(toolCalls), "responses stream"); err != nil {
		return Response{}, err
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

func responsesErrorText(event responsesEvent) string {
	if event.Error != nil && event.Error.Message != "" {
		return event.Error.Message
	}
	if event.Response != nil && event.Response.Error != nil && event.Response.Error.Message != "" {
		return event.Response.Error.Message
	}
	if event.Message != "" {
		return event.Message
	}
	return "the provider ended the response without saying why"
}

// statusError turns a rejected request into something the user can act on.
// The three that actually happen here mean three different things and a bare
// status code sends people to the wrong fix.
func (p *ResponsesProvider) statusError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	detail := strings.TrimSpace(string(body))
	if len(detail) > 500 {
		detail = detail[:500] + "…"
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s rejected the sign-in. Sign in again. (401: %s)", p.provider, detail)
	case http.StatusForbidden:
		return fmt.Errorf("%s refused this account. The plan may not include it, or this client is not accepted. (403: %s)", p.provider, detail)
	case http.StatusTooManyRequests:
		// This backend says exactly when the plan resets, and "try again in 4
		// days" is a different decision for the user than "try again shortly".
		var limit struct {
			Error struct {
				Type            string `json:"type"`
				PlanType        string `json:"plan_type"`
				ResetsInSeconds int64  `json:"resets_in_seconds"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &limit) == nil && limit.Error.ResetsInSeconds > 0 {
			plan := limit.Error.PlanType
			if plan == "" {
				plan = "this"
			}
			return fmt.Errorf("%s: the %s plan's limit is used up. It resets in %s.",
				p.provider, plan, humanizeDuration(time.Duration(limit.Error.ResetsInSeconds)*time.Second))
		}
		return fmt.Errorf("%s plan limit reached. It resets on its own schedule. (429: %s)", p.provider, detail)
	default:
		return fmt.Errorf("%s request failed with status %d: %s", p.provider, resp.StatusCode, detail)
	}
}

// ---------------------------------------------------------------------------
// Model discovery
// ---------------------------------------------------------------------------

// codexClientVersion is sent as the required client_version query parameter on
// the model list. The endpoint rejects the request outright without it, and
// uses it to decide which models this client is allowed to see.
const codexClientVersion = "0.145.0"

// DiscoverResponsesModels asks the backend which models this account may use.
//
// Worth the extra call rather than shipping a list: the set is per-account and
// per-plan, it changed under this code during development, and a stale
// hardcoded name is a 400 that reads like a bug in Aetox ("gpt-5.1-codex is not
// supported when using Codex with a ChatGPT account").
func DiscoverResponsesModels(ctx context.Context, providerName, baseURL string, headers map[string]string, token string) ([]string, error) {
	endpoint := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if endpoint == "" {
		endpoint = strings.TrimSuffix(DefaultBaseURL(providerName), "/")
	}
	if endpoint == "" {
		return nil, fmt.Errorf("provider %q missing base URL", providerName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint+"/models?client_version="+codexClientVersion, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s model list failed with status %d: %s",
			providerName, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Models []struct {
			// "slug", not "id" — this is not the public /v1/models shape.
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%s model list parse failed: %w", providerName, err)
	}

	out := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		if m.Slug != "" {
			out = append(out, m.Slug)
		}
	}
	return out, nil
}

// responsesEffort reads the same capability table every other wire format
// reads, so this endpoint's ladder cannot drift from the one the picker offers.
func responsesEffort(provider, model string, reasoning *ReasoningConfig) string {
	if reasoning == nil {
		return ""
	}
	effort, ok := WireEffort(provider, model, reasoning.Effort)
	if !ok {
		return ""
	}
	return effort
}
