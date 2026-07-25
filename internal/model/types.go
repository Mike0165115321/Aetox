package model

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
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
	// OnToolCallProgress, if set, tracks a tool call while it is still being
	// written: first the moment the tool name is known, then again each time
	// another line of `content` arrives, and once more when the subject (the
	// argument worth reading) finally shows up. Writing an 800-line file means
	// the model spends a minute streaming that content, and the call is only
	// dispatched once the JSON is complete — so without this the UI has nothing
	// to show for the longest part of the job.
	//
	// id is the provider's tool-call id, which the UI uses to recognize the row
	// across updates; subject may be empty on the early ones, because argument
	// order is the model's choice and "content" can arrive long before "path".
	// Not serialized; providers that don't stream simply never call it.
	OnToolCallProgress func(id, name, subject string, lines int) `json:"-"`
}

type Response struct {
	Provider         string
	Model            string
	Text             string
	ReasoningContent string
	Usage            *Usage
	ToolCalls        []ToolCall
	// FinishReason is the provider's normalized stop reason. Only
	// FinishReasonLength is meaningful to callers: the output hit
	// MaxTokens and anything in it (tool-call JSON especially) may be
	// cut off mid-way. Empty when the provider didn't report one.
	FinishReason string
}

// FinishReasonLength marks a response truncated by the max-token limit
// (OpenAI-compatible "length", Anthropic "max_tokens").
const FinishReasonLength = "length"

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

// ArgSubjectKeys is the priority order for "which argument names this call" —
// the path a write touches, the URL a fetch opens. Shared with the turn
// executor so the label shown while a call streams in is byte-identical to the
// one shown when it runs; the UI matches on that label to avoid drawing the
// same call twice.
var ArgSubjectKeys = []string{"path", "file_path", "url", "command", "pattern", "query", "name"}

// partialArgRe matches one complete "key": "value" pair. The closing quote is
// required, so a value still arriving cannot match and be reported truncated.
var partialArgRe = regexp.MustCompile(`"(path|file_path|url|command|pattern|query|name)"\s*:\s*"((?:[^"\\]|\\.)*)"`)

// SubjectFromPartialArgs pulls the first readable argument out of a tool call
// whose JSON is still streaming in, so the UI can name the call before its
// arguments finish. Reports ok=false while nothing is complete yet.
//
// ponytail: a scan, not a parser — a `"url": "..."` sitting inside an earlier
// string value would be taken at face value. Models emit arguments in schema
// order (path before content), so the first match is the real one in practice;
// swap in a streaming JSON tokenizer if that ever stops holding.
func SubjectFromPartialArgs(args string) (string, bool) {
	matches := partialArgRe.FindAllStringSubmatch(args, -1)
	if len(matches) == 0 {
		return "", false
	}
	firstByKey := map[string]string{}
	for _, m := range matches {
		if _, seen := firstByKey[m[1]]; !seen {
			firstByKey[m[1]] = m[2]
		}
	}
	for _, key := range ArgSubjectKeys {
		raw, ok := firstByKey[key]
		if !ok {
			continue
		}
		value := raw
		if unquoted, err := strconv.Unquote(`"` + raw + `"`); err == nil {
			value = unquoted
		}
		if value = strings.TrimSpace(value); value != "" {
			return capSubject(value), true
		}
	}
	return "", false
}

// SubjectFromArgs is the same choice made over arguments that have finished
// arriving and parsed cleanly.
//
// Both paths live here, and both end in capSubject, because the UI recognizes
// a call by its label: if the streaming path and the completed path disagree
// by so much as a truncation, one tool call draws two rows.
func SubjectFromArgs(args map[string]any) string {
	for _, key := range ArgSubjectKeys {
		if v, ok := args[key].(string); ok {
			if v = strings.TrimSpace(v); v != "" {
				return capSubject(v)
			}
		}
	}
	return ""
}

// subjectCap keeps a label to one readable line. Cut on a rune boundary, so a
// Thai or otherwise multi-byte path is never sliced into mojibake.
const subjectCap = 60

func capSubject(s string) string {
	runes := []rune(s)
	if len(runes) <= subjectCap {
		return s
	}
	return string(runes[:subjectCap]) + "…"
}

// ContentLinesSoFar counts the lines of a `content` argument that has only
// partly arrived. JSON escapes a newline as the two characters \ and n, so the
// count is of those, not of real newlines — the raw stream has none.
func ContentLinesSoFar(args string) int {
	i := strings.Index(args, `"content"`)
	if i < 0 {
		return 0
	}
	return strings.Count(args[i:], `\n`) + 1
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

// modelOr returns value, falling back to fallback when the provider's
// response omitted the model name.
func modelOr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
