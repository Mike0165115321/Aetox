package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// Images ride along with Content on a user message, for models that can
	// actually look at one. Aetox's four "senses" tools exist because most
	// models had no vision when they were written (ARCHITECTURE.md §22/§31);
	// image_ocr in particular turns a screenshot into the letters inside it and
	// throws the picture away, which is the right answer for a blind model and
	// a loss for every model shipped since.
	//
	// `json:"-"` on purpose. Each provider spells an image differently — a
	// content-part array here, a sibling `images` field there — so the wire
	// shape belongs in each adapter, next to the rest of that API's quirks, and
	// never in this struct. A caller that forgets to handle Images therefore
	// sends the text alone rather than an invalid body.
	//
	// Only ever set when the model can see: ResolveVision decides, and the
	// image_ocr path stays exactly as it was for everything that cannot.
	Images []Image `json:"-"`
	// Documents are whole files handed to the model to read itself — a PDF with
	// its tables and figures intact, rather than the flattened text layer
	// pdf_read extracts. Same contract as Images in every respect: `json:"-"`
	// because each wire format spells a file differently, set only when
	// ResolveDocuments says this model accepts one, and a caller that ignores
	// the field sends the text alone rather than an invalid body.
	//
	// The trade against pdf_read is the opposite of the usual one and is worth
	// stating: this costs more tokens, not fewer — the model ingests the whole
	// document instead of 220 extracted lines. It buys fidelity (layout, tables,
	// charts, scanned pages) and completeness. desktop/app.go caps the size at
	// which that trade stops being worth making.
	Documents []Document `json:"-"`
}

// Image is one picture attached to a message, already decoded from whatever the
// user dropped in. MediaType is an IANA type ("image/png"); Data is raw bytes,
// base64-encoded per provider rather than stored that way, because two of the
// three wire formats want it wrapped differently and holding the encoded form
// would mean decoding it back to re-wrap it.
type Image struct {
	MediaType string
	Data      []byte
}

// Document is one file attached for the model to read. Name is carried because
// unlike an image these arrive with a filename the model is expected to refer
// to — and at least one backend requires the field.
type Document struct {
	Name      string
	MediaType string
	Data      []byte
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
	// PromptTokens is the TOTAL input for the call — tokens served from the
	// provider's prompt cache included. The two wire formats disagree about
	// this and each adapter normalizes to this meaning (measured against
	// DeepSeek, which serves both):
	//
	//	openai-compatible: prompt_tokens 4011 = hit 3968 + miss 43   (total already)
	//	anthropic:         input_tokens 43, cache_read 3968 separate (miss only)
	//
	// The Anthropic reading used to be stored as-is, so a well-cached call was
	// recorded as 43 input tokens instead of 4011.
	PromptTokens int `json:"prompt_tokens"`

	// CachedPromptTokens is the part of PromptTokens the provider served from
	// its prompt cache. The rest (PromptTokens - CachedPromptTokens) was
	// evaluated fresh, tokens written into the cache included — those are
	// processed, not reused.
	CachedPromptTokens int `json:"-"`

	// CacheReported separates "this provider does not do cache accounting"
	// (Ollama, LM Studio) from "it does, and nothing hit this time". Both are
	// zero, and a UI that cannot tell them apart shows a local model a
	// confident 0% hit rate it never claimed.
	CacheReported bool `json:"-"`

	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// UncachedPromptTokens is the input the provider actually had to evaluate.
func (u Usage) UncachedPromptTokens() int {
	if n := u.PromptTokens - u.CachedPromptTokens; n > 0 {
		return n
	}
	return 0
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
		PromptTokens:       maxInt(0, usage.PromptTokens),
		CachedPromptTokens: maxInt(0, usage.CachedPromptTokens),
		CacheReported:      usage.CacheReported,
		CompletionTokens:   maxInt(0, usage.CompletionTokens),
		TotalTokens:        usage.TotalTokenCount(),
	}
	// A cached count above the total is a parsing mistake, not a discount.
	if normalized.CachedPromptTokens > normalized.PromptTokens {
		normalized.CachedPromptTokens = normalized.PromptTokens
	}
	return &normalized
}

// ArgSubjectKeys is the priority order for "which argument names this call" —
// the path a write touches, the URL a fetch opens. Shared with the turn
// executor so the label shown while a call streams in is byte-identical to the
// one shown when it runs; the UI matches on that label to avoid drawing the
// same call twice.
//
// `description` sits below the real subjects — a path, a URL — because a tool
// that touches a named thing should be labelled with that thing. It sits above
// `command` because a shell call's own "few words naming the job" is what the
// user wants on that row; the command line itself is one click away in the
// detail view and is usually too long to read there anyway. `task` has only a
// description, so its label is unaffected by where the two sit relative to
// each other.
var ArgSubjectKeys = []string{"path", "file_path", "url", "description", "command", "pattern", "query", "name"}

// partialArgRe matches one complete "key": "value" pair. The closing quote is
// required, so a value still arriving cannot match and be reported truncated.
var partialArgRe = regexp.MustCompile(`"(path|file_path|url|command|pattern|query|name|description)"\s*:\s*"((?:[^"\\]|\\.)*)"`)

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

// errEmptyCompletion is the error a runtime returns when a provider answered
// with nothing at all — no text, no reasoning, no tool call.
//
// Except when the provider stopped because it hit the token limit. That case is
// a truncation, and it is a normal thing to ask for: the desktop's "test
// connection" button pings with a tiny max_tokens precisely because it does not
// want an answer, only proof that the endpoint is alive. Reporting that as
// "response has empty text" told users their working provider was broken.
//
// Returns nil when there is nothing to complain about, so call sites read:
//
//	if err := errEmptyCompletion(name, finishReason, text, reasoning, calls); err != nil {
func errEmptyCompletion(provider, finishReason, text, reasoning string, toolCalls int) error {
	if text != "" || reasoning != "" || toolCalls > 0 {
		return nil
	}
	if finishReason == FinishReasonLength {
		return nil
	}
	return fmt.Errorf("%s response has empty text", provider)
}
