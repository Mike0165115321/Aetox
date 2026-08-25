package model

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/oauth"
)

// Live round trips against real provider backends, skipped unless AETOX_LIVE=1.
//
// Everything else in this package proves Aetox builds the request it meant to.
// Only these prove the request is one the provider accepts — which is why the
// ChatGPT check below runs against the machine's own Codex session rather than
// a fixture: the Responses backend is the one surface no unit test can stand in
// for.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveSubscription -v -count=1
func TestLiveSubscriptionAsksAndAnswers(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against real subscription backends")
	}

	t.Run("codex", func(t *testing.T) {
		signInCodexLive(t)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		p, err := NewProvider(ProviderOptions{
			Provider: "codex",
			Model:    "gpt-5.5",
			Timeout:  90 * time.Second,
		})
		if err != nil {
			t.Fatalf("NewProvider: %v", err)
		}
		assertAnswersLive(ctx, t, p)
	})
}

// signInCodexLive points this test at a real ChatGPT credential, whichever way
// the machine has one. Two ways onto this backend and it takes either —
// otherwise it skips on the machine of anyone who signed in the way the product
// documents. The import is preferred when both exist because it leaves the
// developer's own store untouched.
func signInCodexLive(t *testing.T) {
	t.Helper()
	if oauth.CodexCLIAvailable() {
		t.Setenv("AETOX_DATA_ROOT", t.TempDir())
		if err := oauth.ImportCodexCLI(); err != nil {
			t.Fatalf("ImportCodexCLI: %v", err)
		}
		return
	}
	root := realDataRoot()
	if root == "" {
		t.Skip("cannot locate this machine's credential store")
	}
	// Writes back here on a refresh, which is the point: this is the credential
	// the app itself would use, exercised as the app uses it.
	t.Setenv("AETOX_DATA_ROOT", root)
	if !oauth.Has("codex") {
		t.Skip("no ChatGPT sign-in — run `aetox login codex`, or sign into the Codex CLI")
	}
}

// The picker, end to end. §64 removed this provider partly because a signed-in
// provider whose model list cannot be fetched is a provider the user cannot
// actually select a model for — so "the list is not empty" is a shipping
// condition, not a nicety. DiscoverResponsesModels is also the one restored
// function with no unit coverage: it has nothing to assert against but a live
// account, because the list is per-plan.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveCodexModelPicker -v -count=1
func TestLiveCodexModelPicker(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against the real ChatGPT backend")
	}
	signInCodexLive(t)

	// The call the desktop makes, not the layer below it: an empty base URL and
	// no key is exactly what Settings passes for a signed-in provider, and the
	// resolution of both is the part that broke before.
	models, err := ModelChoicesWithEndpointAndAPIKey("codex", "", "")
	if err != nil {
		t.Fatalf("ModelChoicesWithEndpointAndAPIKey: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("the model picker is empty for a signed-in ChatGPT account")
	}
	t.Logf("%d models: %s", len(models), strings.Join(models, ", "))
}

// A picture, for real. TestResponsesMessagesCarryImages proves Aetox builds the
// input_image part; only this proves the backend accepts it and the model
// looked. Three coloured bars, because no OCR fallback could answer this and no
// caption in the prompt names a colour — a right answer can only come from the
// pixels.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveCodexSeesAnImage -v -count=1
func TestLiveCodexSeesAnImage(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against the real ChatGPT backend")
	}
	signInCodexLive(t)

	const modelName = "gpt-5.5"
	if !ResolveVision("codex", modelName) {
		t.Fatalf("ResolveVision says %q is blind; this test has nothing to prove", modelName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	p, err := NewProvider(ProviderOptions{Provider: "codex", Model: modelName, Timeout: 90 * time.Second})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	resp, err := retryPastRateLimit(t, func() (Response, error) {
		return p.Complete(ctx, Request{
			Messages: []Message{{
				Role:    RoleUser,
				Content: "Name the three colours in this image, left to right. Answer with just the three words.",
				Images:  []Image{{MediaType: "image/png", Data: threeBarPNG(t)}},
			}},
			MaxTokens: 2000,
		})
	})
	skipIfOutOfQuota(t, err)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got := strings.ToLower(resp.Text)
	t.Logf("answer: %q", strings.TrimSpace(resp.Text))
	for _, want := range []string{"red", "green", "blue"} {
		if !strings.Contains(got, want) {
			t.Fatalf("answer %q is missing %q — the picture did not reach the model, or it did not look", resp.Text, want)
		}
	}
}

// A whole PDF, for real. The unit test proves Aetox builds the input_file part;
// only this proves the backend takes it and the model read what is inside.
//
// The number is drawn by the PDF's own text operator and appears nowhere in the
// prompt, so a correct answer cannot come from anywhere else — and pdf_read is
// not in this path at all, which is the point of the whole change.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveCodexReadsAPDF -v -count=1
func TestLiveCodexReadsAPDF(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against the real ChatGPT backend")
	}
	signInCodexLive(t)

	const modelName = "gpt-5.5"
	if !ResolveDocuments("codex", modelName) {
		t.Fatalf("ResolveDocuments says %q takes no document; this test has nothing to prove", modelName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	p, err := NewProvider(ProviderOptions{Provider: "codex", Model: modelName, Timeout: 90 * time.Second})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	resp, err := retryPastRateLimit(t, func() (Response, error) {
		return p.Complete(ctx, Request{
			Messages: []Message{{
				Role:    RoleUser,
				Content: "What number appears in this document? Reply with just the number.",
				Documents: []Document{{
					Name:      "probe.pdf",
					MediaType: "application/pdf",
					Data:      []byte(livePDFFixture),
				}},
			}},
			MaxTokens: 2000,
		})
	})
	skipIfOutOfQuota(t, err)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	t.Logf("answer: %q", strings.TrimSpace(resp.Text))
	if !strings.Contains(resp.Text, "4271") {
		t.Fatalf("answer %q does not contain 4271 — the document did not reach the model, or it did not read it", resp.Text)
	}
}

// A hand-written minimal PDF drawing one number, mirroring the fixture in
// internal/skill/pdf_read_test.go. Inline so the thing being read sits next to
// the assertion looking for it.
const livePDFFixture = `%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj
4 0 obj<</Length 52>>stream
BT /F1 18 Tf 20 100 Td (MANGOSTEEN 4271) Tj ET
endstream
endobj
5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj
trailer<</Root 1 0 R>>
`

// The thinking panel, for real. responses.go sends reasoning.effort plus
// summary:auto, and the summary is the only reason the panel has anything to
// show — the raw chain never crosses the wire. A unit test can only prove Aetox
// asks; whether the backend honours the ask on this plan is an account
// question, and a silent no is indistinguishable from a model that thought
// briefly.
//
//	AETOX_LIVE=1 go test ./internal/model/ -run TestLiveCodexStreamsItsThinking -v -count=1
func TestLiveCodexStreamsItsThinking(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against the real ChatGPT backend")
	}
	signInCodexLive(t)

	caps := ResolveThinkingCapabilities("codex", "gpt-5.5")
	if !caps.Supported {
		t.Fatal("codex reports no thinking support; this test has nothing to prove")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	p, err := NewProvider(ProviderOptions{Provider: "codex", Model: "gpt-5.5", Timeout: 90 * time.Second})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	streamer, ok := p.(StreamingProvider)
	if !ok {
		t.Fatal("provider does not stream")
	}

	// summary:"auto" leaves the decision with the backend, and on a question it
	// can answer without a step it legitimately summarises nothing — the first
	// draft of this test asked for 3×4×7 and went red once in four runs on
	// exactly that. The fix is a question that cannot be answered in one hop,
	// plus the highest effort the ladder offers, plus attempts: a summary that
	// never arrives across all three is the plumbing, not the model's mood.
	effort := caps.Levels[len(caps.Levels)-1]
	const question = "Three switches outside a windowless room control three bulbs inside. " +
		"You may flip switches freely but may enter the room only once. How do you tell which switch controls which bulb?"

	var thought, answer strings.Builder
	var resp Response
	for attempt := 1; attempt <= 3; attempt++ {
		thought.Reset()
		answer.Reset()
		resp, err = retryPastRateLimit(t, func() (Response, error) {
			thought.Reset()
			answer.Reset()
			return streamer.StreamComplete(ctx, Request{
				Messages:  []Message{{Role: RoleUser, Content: question}},
				Reasoning: &ReasoningConfig{Effort: effort},
				MaxTokens: 4000,
			}, func(s string) error { answer.WriteString(s); return nil },
				func(s string) error { thought.WriteString(s); return nil })
		})
		skipIfOutOfQuota(t, err)
		if err != nil {
			t.Fatalf("StreamComplete (attempt %d): %v", attempt, err)
		}
		if strings.TrimSpace(thought.String()) != "" {
			break
		}
		t.Logf("attempt %d summarised nothing; retrying", attempt)
	}

	// The answer is a sanity check that the turn ran at all, not the subject:
	// heat is the trick, and any correct solution says so somewhere.
	if full := strings.ToLower(answer.String() + resp.Text); !strings.Contains(full, "heat") && !strings.Contains(full, "warm") {
		t.Errorf("answer does not look like a real solution: %q", firstLine(answer.String()+resp.Text))
	}
	// The callback is what fills the panel while it happens; ReasoningContent is
	// what the transcript keeps. Aetox writes both from the same deltas, so one
	// without the other means the runtime dropped it between the two.
	streamed, final := strings.TrimSpace(thought.String()), strings.TrimSpace(resp.ReasoningContent)
	if streamed == "" || final == "" {
		t.Fatalf("no reasoning summary after 3 attempts at effort %q: streamed %q, final %q — "+
			"the thinking panel would sit blank", effort, streamed, final)
	}
	t.Logf("thinking (%d chars): %s", len(streamed), firstLine(streamed))
	t.Logf("answer: %s", firstLine(answer.String()+resp.Text))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// threeBarPNG draws red | green | blue. Deliberately shapes rather than text:
// text would let an OCR path answer correctly and prove nothing about vision.
func threeBarPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 240, 120))
	bars := []color.RGBA{{220, 40, 40, 255}, {40, 180, 60, 255}, {50, 90, 230, 255}}
	for x := range 240 {
		for y := range 120 {
			img.Set(x, y, bars[x/80])
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return buf.Bytes()
}

// realDataRoot is the store an `aetox login` on this machine actually wrote to.
//
// TestMain points the whole package at a throwaway directory so a unit test
// never depends on the developer's logins. The live tests are the one place
// that wants the real one back, because the credential under test *is* the
// machine's sign-in. It mirrors oauth.Root() with the env override skipped —
// reading AETOX_DATA_ROOT here would just find TestMain's temp dir.
// AETOX_REAL_DATA_ROOT covers a non-standard install.
func realDataRoot() string {
	if root := strings.TrimSpace(os.Getenv("AETOX_REAL_DATA_ROOT")); root != "" {
		return root
	}
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "aetox")
}

// assertAnswersLive runs the two things a provider has to do for Aetox to be
// usable on it: answer a question, and call a tool when told to.
func assertAnswersLive(ctx context.Context, t *testing.T, p Provider) {
	t.Helper()

	resp, err := retryPastRateLimit(t, func() (Response, error) {
		return p.Complete(ctx, Request{
			Messages: []Message{
				{Role: RoleSystem, Content: "Answer in one short sentence."},
				{Role: RoleUser, Content: "What is the capital of Japan?"},
			},
			MaxTokens: 2000,
		})
	})
	skipIfOutOfQuota(t, err)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(strings.ToLower(resp.Text), "tokyo") {
		t.Fatalf("answer did not contain the fact asked for: %q", resp.Text)
	}
	t.Logf("answer: %q", strings.TrimSpace(resp.Text))
	if resp.Usage != nil {
		t.Logf("usage: prompt=%d (cached %d) completion=%d",
			resp.Usage.PromptTokens, resp.Usage.CachedPromptTokens, resp.Usage.CompletionTokens)
	} else {
		t.Error("no usage reported — the token counter will read zero for this provider")
	}

	// A provider that answers but cannot call a tool is not usable as an agent,
	// and the tool shape is the part most likely to be wrong per-provider.
	streamer, ok := p.(StreamingProvider)
	if !ok {
		t.Fatal("provider does not stream; the desktop needs streaming")
	}
	var streamed strings.Builder
	weatherTool := []ToolDefinition{{
		Type: "function",
		Function: ToolFunction{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			Parameters:  []byte(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}}
	toolResp, err := retryPastRateLimit(t, func() (Response, error) {
		streamed.Reset()
		return streamer.StreamComplete(ctx, Request{
			Messages: []Message{
				{Role: RoleSystem, Content: "Use the get_weather tool when asked about weather. Do not answer from memory."},
				{Role: RoleUser, Content: "What is the weather in Bangkok right now?"},
			},
			Tools:     weatherTool,
			MaxTokens: 2000,
		}, func(chunk string) error { streamed.WriteString(chunk); return nil }, nil)
	})
	skipIfOutOfQuota(t, err)
	if err != nil {
		t.Fatalf("StreamComplete with tools: %v", err)
	}
	if len(toolResp.ToolCalls) == 0 {
		t.Fatalf("no tool call; the model answered %q instead", toolResp.Text)
	}
	call := toolResp.ToolCalls[0]
	t.Logf("tool call: %s(%s) id=%s", call.Function.Name, call.Function.Arguments, call.ID)
	if call.Function.Name != "get_weather" {
		t.Fatalf("called %q; want get_weather", call.Function.Name)
	}
	if !strings.Contains(strings.ToLower(call.Function.Arguments), "bangkok") {
		t.Fatalf("arguments = %q; want the city from the question", call.Function.Arguments)
	}
	if call.ID == "" {
		t.Fatal("tool call has no id; the result cannot be routed back to it")
	}

	// And the other half of the round trip: hand the result back and check the
	// provider accepts the shape this runtime sends for it.
	final, err := retryPastRateLimit(t, func() (Response, error) {
		return p.Complete(ctx, Request{
			Messages: []Message{
				{Role: RoleSystem, Content: "Use the get_weather tool when asked about weather."},
				{Role: RoleUser, Content: "What is the weather in Bangkok right now?"},
				{Role: RoleAssistant, ToolCalls: toolResp.ToolCalls},
				{Role: RoleTool, ToolCallID: call.ID, Name: call.Function.Name, Content: `{"temp_c":34,"conditions":"hazy sunshine"}`},
			},
			Tools:     weatherTool,
			MaxTokens: 2000,
		})
	})
	skipIfOutOfQuota(t, err)
	if err != nil {
		t.Fatalf("second turn (tool result round trip): %v", err)
	}
	if !strings.Contains(final.Text, "34") {
		t.Fatalf("final answer did not use the tool result: %q", final.Text)
	}
	t.Logf("after tool result: %q", strings.TrimSpace(final.Text))
}

// retryPastRateLimit runs call, and if the provider says "too fast" rather than
// "you are out", waits and tries once more.
//
// The Gemini free tier allows roughly one request a minute per model, so a test
// that makes three calls in a row hits the limiter on the second every time.
// That is pacing, not a defect, and it must not read as one.
func retryPastRateLimit(t *testing.T, call func() (Response, error)) (Response, error) {
	t.Helper()
	resp, err := call()
	if err == nil || !isPerMinuteLimit(err) {
		return resp, err
	}
	t.Logf("rate limited; waiting out the window before retrying: %v", err)
	time.Sleep(65 * time.Second)
	return call()
}

// isPerMinuteLimit distinguishes a short throttling window (wait a minute) from
// a spent plan (wait days). Only the first is worth retrying inside a test.
func isPerMinuteLimit(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "RESOURCE_EXHAUSTED") && strings.Contains(msg, "reset after")
}

// skipIfOutOfQuota separates "this account has nothing left this cycle" from
// "the request was wrong".
//
// A plan-limit 429 is itself evidence the runtime works: the backend only
// reaches quota enforcement after it has accepted the credentials, the headers,
// the model id and the body. Failing the test on it would mean a green suite
// depends on how much of someone's monthly plan is left.
func skipIfOutOfQuota(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "limit is used up"),
		strings.Contains(msg, "usage_limit_reached"),
		strings.Contains(msg, "RESOURCE_EXHAUSTED"),
		strings.Contains(msg, "limit reached"):
		t.Skipf("request accepted but the plan is out of quota — the wire format is proven up to metering: %v", err)
	}
}

// Live check of the Claude thinking contract, skipped unless AETOX_LIVE=1.
//
//	AETOX_LIVE=1 ANTHROPIC_API_KEY=... go test ./internal/model/ -run TestLiveAnthropicEffort -v -count=1
//
// The whole point: thinking depth is output_config.effort, and the older
// thinking.budget_tokens is a 400. Which of those is true cannot be settled by
// reading — the request either is accepted or it is not.
func TestLiveAnthropicEffortLadder(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run against the real Anthropic API")
	}

	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		t.Skip("no Claude credentials: set ANTHROPIC_API_KEY")
	}

	model := strings.TrimSpace(os.Getenv("AETOX_LIVE_MODEL"))
	if model == "" {
		model = "claude-sonnet-5"
	}

	for _, level := range []string{"off", "low", "medium", "high", "xhigh", "max"} {
		t.Run(level, func(t *testing.T) {
			// Through the factory, so a signed-in plan is resolved exactly as
			// it is for a real turn rather than by a test-only shortcut.
			p, err := NewProvider(ProviderOptions{
				Provider: "anthropic", Model: model, APIKey: apiKey,
				Timeout: 90 * time.Second,
			})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}

			req := Request{
				Messages: []Message{
					{Role: RoleSystem, Content: "You are Aetox, a helpful assistant."},
					{Role: RoleUser, Content: "Reply with the single word: ok"},
				},
				// The temperature Aetox sets on every real call. If the runtime
				// forwarded it, this request would 400 — so every subtest here
				// is also a regression test for that.
				Temperature: 0.2,
				MaxTokens:   2000,
			}
			if level == "off" {
				req.Thinking = &ThinkingConfig{Type: "disabled"}
			} else {
				req.Reasoning = &ReasoningConfig{Effort: level}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			resp, err := retryPastRateLimit(t, func() (Response, error) { return p.Complete(ctx, req) })
			skipIfOutOfQuota(t, err)
			if err != nil {
				t.Fatalf("effort %q rejected: %v", level, err)
			}
			t.Logf("effort %-6s → %q (thinking %d chars)", level,
				strings.TrimSpace(resp.Text), len(resp.ReasoningContent))
		})
	}
}
