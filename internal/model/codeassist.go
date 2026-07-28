package model

import (
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

// The Code Assist runtime — a personal Google account's free Gemini tier.
//
// It is the Gemini generateContent body, wrapped. Google's private Code Assist
// surface takes the standard request as a nested `request` field and adds a
// `project` beside it, so nothing from the OpenAI-compatible path applies:
// contents/parts instead of messages, functionCall parts instead of tool_calls,
// and every streamed chunk arriving one level down under `response`.
//
// The project id is not optional despite being typed that way in Google's own
// client — without it this endpoint answers 500.

type CodeAssistConfig struct {
	Provider    string
	Model       string
	BaseURL     string
	Project     string
	Timeout     time.Duration
	TokenSource func(context.Context) (string, error)
}

type CodeAssistProvider struct {
	provider    string
	model       string
	baseURL     string
	project     string
	tokenSource func(context.Context) (string, error)
	httpClient  *http.Client
}

func NewCodeAssistProvider(cfg CodeAssistConfig) (*CodeAssistProvider, error) {
	provider := strings.TrimSpace(strings.ToLower(cfg.Provider))
	if provider == "" {
		provider = "code-assist"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, ErrMissingModel
	}
	if cfg.TokenSource == nil {
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

	return &CodeAssistProvider{
		provider:    provider,
		model:       model,
		baseURL:     baseURL,
		project:     strings.TrimSpace(cfg.Project),
		tokenSource: cfg.TokenSource,
		httpClient:  newModelHTTPClient(timeout),
	}, nil
}

func (p *CodeAssistProvider) Name() string { return p.provider }

func (p *CodeAssistProvider) SupportsToolCalling() bool { return true }

func (p *CodeAssistProvider) SupportsReasoning() bool { return true }

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

// caRequest is the outer envelope. The key casing really is mixed — `model` and
// `project` camel, `user_prompt_id` snake — because it mirrors an internal
// proto rather than the public API.
type caRequest struct {
	Model   string      `json:"model"`
	Project string      `json:"project,omitempty"`
	Request geminiInner `json:"request"`
}

type geminiInner struct {
	Contents          []geminiContent  `json:"contents"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
	Tools             []geminiTools    `json:"tools,omitempty"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart is a union: exactly one of the fields below is set. Thought marks
// a part as the model's reasoning rather than its answer — the same text field
// carries both, and treating a thought as the reply is how a model's private
// notes end up shown as its answer.
type geminiPart struct {
	Text             string              `json:"text,omitempty"`
	Thought          bool                `json:"thought,omitempty"`
	ThoughtSignature string              `json:"thoughtSignature,omitempty"`
	InlineData       *geminiInlineData   `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResp `json:"functionResponse,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFunctionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiFunctionResp struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTools struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiGenConfig struct {
	Temperature     float64            `json:"temperature,omitempty"`
	MaxOutputTokens int                `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *geminiThinkConfig `json:"thinkingConfig,omitempty"`
}

type geminiThinkConfig struct {
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

// caResponse is one streamed chunk. Everything worth reading is one level down
// under `response` — the outer object only adds credit accounting.
type caResponse struct {
	Response struct {
		Candidates []struct {
			Content      geminiContent `json:"content"`
			FinishReason string        `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount        int `json:"promptTokenCount"`
			CandidatesTokenCount    int `json:"candidatesTokenCount"`
			TotalTokenCount         int `json:"totalTokenCount"`
			CachedContentTokenCount int `json:"cachedContentTokenCount"`
			ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
		} `json:"usageMetadata"`
		ModelVersion string `json:"modelVersion"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// ---------------------------------------------------------------------------
// Request building
// ---------------------------------------------------------------------------

func buildCodeAssistRequest(model, project string, req Request) (caRequest, error) {
	system, contents := convertMessagesToGemini(req.Messages)
	if len(contents) == 0 {
		return caRequest{}, ErrNoMessages
	}

	inner := geminiInner{
		Contents: contents,
		Tools:    convertToolsToGemini(req.Tools),
	}
	if system != "" {
		inner.SystemInstruction = &geminiContent{
			// "user", not "system": this API has no system role, and the
			// instruction is carried by the field name instead.
			Role:  "user",
			Parts: []geminiPart{{Text: system}},
		}
	}

	cfg := geminiGenConfig{Temperature: req.Temperature, MaxOutputTokens: req.MaxTokens}
	if req.Reasoning != nil || req.Thinking != nil {
		// Without includeThoughts the model still thinks and simply never says
		// so, leaving Aetox's thinking panel empty on a reasoning model.
		cfg.ThinkingConfig = &geminiThinkConfig{IncludeThoughts: true}
	}
	if cfg != (geminiGenConfig{}) {
		inner.GenerationConfig = &cfg
	}

	return caRequest{Model: model, Project: project, Request: inner}, nil
}

// convertMessagesToGemini flattens Aetox's messages into Gemini's alternating
// contents. Two mappings are not obvious: an assistant tool call becomes a
// functionCall part on a "model" turn, and its result becomes a
// functionResponse part on a "user" turn — this API has no tool role, and the
// result is matched back by function *name*, not by an id.
func convertMessagesToGemini(msgs []Message) (system string, contents []geminiContent) {
	var systemParts []string
	// callNames remembers which name a call id belongs to, because the tool
	// result arriving later carries only the id Aetox assigned.
	callNames := map[string]string{}

	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			if s := strings.TrimSpace(m.Content); s != "" {
				systemParts = append(systemParts, s)
			}

		case RoleAssistant:
			var parts []geminiPart
			if text := strings.TrimSpace(m.Content); text != "" {
				parts = append(parts, geminiPart{Text: text})
			}
			for _, call := range m.ToolCalls {
				args := json.RawMessage(call.Function.Arguments)
				if len(strings.TrimSpace(call.Function.Arguments)) == 0 {
					args = json.RawMessage("{}")
				}
				callNames[call.ID] = call.Function.Name
				parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{
					Name: call.Function.Name,
					Args: args,
				}})
			}
			if len(parts) == 0 {
				continue
			}
			contents = append(contents, geminiContent{Role: "model", Parts: parts})

		case RoleTool:
			name := callNames[m.ToolCallID]
			if name == "" {
				name = m.Name
			}
			contents = append(contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{{FunctionResponse: &geminiFunctionResp{
					Name: name,
					// The value must be an object; a bare string is rejected.
					Response: map[string]any{"output": m.Content},
				}}},
			})

		default:
			parts := make([]geminiPart, 0, len(m.Images)+1)
			if m.Content != "" {
				parts = append(parts, geminiPart{Text: m.Content})
			}
			for _, img := range m.Images {
				mediaType := strings.TrimSpace(img.MediaType)
				if mediaType == "" {
					mediaType = "image/png"
				}
				parts = append(parts, geminiPart{InlineData: &geminiInlineData{
					MimeType: mediaType,
					Data:     base64.StdEncoding.EncodeToString(img.Data),
				}})
			}
			if len(parts) == 0 {
				continue
			}
			contents = append(contents, geminiContent{Role: "user", Parts: parts})
		}
	}
	return strings.Join(systemParts, "\n\n"), contents
}

func convertToolsToGemini(tools []ToolDefinition) []geminiTools {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]geminiFunctionDecl, 0, len(tools))
	for _, tool := range tools {
		decls = append(decls, geminiFunctionDecl{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}
	// One tool object holding every declaration, not one object per tool —
	// the plural spelling is a schema error here.
	return []geminiTools{{FunctionDeclarations: decls}}
}

// ---------------------------------------------------------------------------
// Streaming
// ---------------------------------------------------------------------------

func (p *CodeAssistProvider) Complete(ctx context.Context, req Request) (Response, error) {
	return p.StreamComplete(ctx, req, nil, nil)
}

func (p *CodeAssistProvider) StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler, onReasoningChunk StreamChunkHandler) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	payload, err := buildCodeAssistRequest(model, p.project, req)
	if err != nil {
		return Response{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	token, err := p.tokenSource(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("%s sign-in: %w", p.provider, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+":streamGenerateContent?alt=sse", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return Response{}, p.statusError(httpResp)
	}

	var text, reasoning strings.Builder
	var toolCalls []ToolCall
	progress := newToolProgressTracker(req.OnToolCallProgress)
	respModel := model
	var usage Usage
	var finishReason string
	var streamErr error

	err = scanSSE(httpResp.Body, func(data string) (bool, error) {
		var chunk caResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return false, nil
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			streamErr = fmt.Errorf("%s: %s", p.provider, chunk.Error.Message)
			return true, nil
		}
		if chunk.Response.ModelVersion != "" {
			respModel = chunk.Response.ModelVersion
		}
		if u := chunk.Response.UsageMetadata; u != nil {
			usage = Usage{
				PromptTokens:       u.PromptTokenCount,
				CachedPromptTokens: u.CachedContentTokenCount,
				CacheReported:      true,
				// Thinking tokens are billed and counted separately from the
				// visible answer; dropping them under-reports every reasoning
				// turn.
				CompletionTokens: u.CandidatesTokenCount + u.ThoughtsTokenCount,
				TotalTokens:      u.TotalTokenCount,
			}
		}

		for _, candidate := range chunk.Response.Candidates {
			if candidate.FinishReason != "" {
				finishReason = candidate.FinishReason
			}
			for _, part := range candidate.Content.Parts {
				switch {
				case part.FunctionCall != nil:
					args := strings.TrimSpace(string(part.FunctionCall.Args))
					if args == "" {
						args = "{}"
					}
					id := part.FunctionCall.ID
					if id == "" {
						// This API does not always issue call ids, but every
						// caller in Aetox keys a tool result by one. Synthesize
						// a stable one; the wire round trip matches by name.
						id = "call_" + strconv.Itoa(len(toolCalls)+1)
					}
					toolCalls = append(toolCalls, ToolCall{
						ID:       id,
						Type:     "function",
						Function: FunctionCall{Name: part.FunctionCall.Name, Arguments: args},
					})
					// Gemini sends a call complete rather than in fragments, so
					// the row is drawn once with everything already known.
					progress.report(len(toolCalls)-1, id, part.FunctionCall.Name, args)

				case part.Thought:
					if part.Text == "" {
						continue
					}
					reasoning.WriteString(part.Text)
					if onReasoningChunk != nil {
						if err := onReasoningChunk(part.Text); err != nil {
							return true, err
						}
					}

				case part.Text != "":
					text.WriteString(part.Text)
					if onChunk != nil {
						if err := onChunk(part.Text); err != nil {
							return true, err
						}
					}
				}
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

	textOut := strings.TrimSpace(text.String())
	reasoningOut := strings.TrimSpace(reasoning.String())
	if err := errEmptyCompletion(p.provider, normalizeGeminiFinishReason(finishReason), textOut, reasoningOut, len(toolCalls)); err != nil {
		return Response{}, err
	}

	return Response{
		Provider:         p.Name(),
		Model:            respModel,
		Text:             textOut,
		ReasoningContent: reasoningOut,
		ToolCalls:        toolCalls,
		Usage:            normalizeUsage(usage),
		FinishReason:     normalizeGeminiFinishReason(finishReason),
	}, nil
}

// normalizeGeminiFinishReason maps this API's spelling onto the one callers
// already act on. Only truncation matters to them.
func normalizeGeminiFinishReason(reason string) string {
	if strings.EqualFold(reason, "MAX_TOKENS") {
		return FinishReasonLength
	}
	return ""
}

func (p *CodeAssistProvider) statusError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	detail := strings.TrimSpace(string(body))
	if len(detail) > 500 {
		detail = detail[:500] + "…"
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s rejected the sign-in — sign in again (401: %s)", p.provider, detail)
	case http.StatusForbidden:
		return fmt.Errorf("%s refused this account — Code Assist may not be enabled for it (403: %s)", p.provider, detail)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%s free-tier limit reached — it resets on its own schedule (429: %s)", p.provider, detail)
	case http.StatusInternalServerError:
		// The signature failure of this endpoint: a request without a project
		// id is a 500, not a 400, and the body says nothing useful.
		if p.project == "" {
			return fmt.Errorf("%s has no project id — sign out and sign in again to have one assigned (500)", p.provider)
		}
		return fmt.Errorf("%s request failed with status 500: %s", p.provider, detail)
	default:
		return fmt.Errorf("%s request failed with status %d: %s", p.provider, resp.StatusCode, detail)
	}
}
