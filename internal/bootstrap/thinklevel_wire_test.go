package bootstrap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/safety"
)

// captureProvider is a stand-in for a provider's HTTP endpoint that records the
// request body and answers with the smallest valid reply. It speaks the
// Anthropic wire format because that is what DeepSeek — the provider with a
// real effort dial — is bootstrapped onto by default.
type captureProvider struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
}

func newCaptureProvider(t *testing.T, reply string) *captureProvider {
	t.Helper()
	c := &captureProvider{}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err == nil {
			c.mu.Lock()
			c.bodies = append(c.bodies, parsed)
			c.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(c.server.Close)
	return c
}

const anthropicShapedReply = `{"id":"msg_test","type":"message","role":"assistant",` +
	`"model":"deepseek-v4-flash","content":[{"type":"text","text":"ok"}],` +
	`"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`

const openAIShapedReply = `{"id":"cmpl_test","model":"kimi-k3","choices":[{"index":0,` +
	`"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],` +
	`"usage":{"prompt_tokens":1,"completion_tokens":1}}`

func (c *captureProvider) first(t *testing.T) map[string]any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.bodies) == 0 {
		t.Fatal("no request reached the provider")
	}
	return c.bodies[0]
}

// The think level the user picked has to survive the whole way to the wire.
//
// It did not. bootstrap built the sub-agent tools with cfg.ThinkLevel and then
// built the app without it, so app.thinkLevel resolved an empty string —
// think.NormalizeLevel reads that as "medium" — and every main-agent turn on the
// desktop went out at a depth nobody selected, "off" included. The unit tests
// under cognitive and think all passed throughout: each link was correct, and
// the break was in the wiring between them.
//
// So this test asserts against the payload rather than against any function's
// return value. It is the only place that can tell "the user picked off" from
// "the request said off", and those were different things for the entire life of
// the picker.
func TestEngineSendsTheConfiguredThinkLevelOnTheWire(t *testing.T) {
	for _, tc := range []struct {
		level        string
		wantThinking string // thinking.type
		wantEffort   string // output_config.effort, "" for absent
	}{
		{"off", "disabled", ""},
		{"high", "adaptive", "high"},
		{"max", "adaptive", "max"},
		// The levels DeepSeek has always accepted and the picker never offered.
		// Here to prove the addition is real all the way out, not just a longer
		// menu: a level that cannot reach the wire is not a level.
		{"low", "adaptive", "low"},
		// Not offered in the picker — the service folds them itself — but a
		// stored config carrying one must still land on a real depth.
		{"medium", "adaptive", "high"},
		{"xhigh", "adaptive", "high"},
		{"ultra", "adaptive", "max"},
	} {
		t.Run(tc.level, func(t *testing.T) {
			capture := newCaptureProvider(t, anthropicShapedReply)
			t.Setenv("AETOX_DATA_ROOT", t.TempDir())

			res, err := Engine(config.Config{
				ModelProvider: "deepseek",
				ModelName:     "deepseek-v4-flash",
				ModelBaseURL:  capture.server.URL,
				ModelAPIKey:   "test-key",
				ThinkLevel:    tc.level,
				SandboxRoot:   t.TempDir(),
				ApprovalMode:  string(safety.ApprovalAsk),
			}, Options{Console: DiscardConsole(), Approve: approveNothing})
			if err != nil {
				t.Fatalf("Engine: %v (%s)", err, res.Status)
			}
			if _, err := res.App.RunOnce(context.Background(), "say ok"); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}

			body := capture.first(t)
			// Only the two fields under test are reported on failure. The body
			// also carries the system prompt, and printing it buries the one
			// line that says what went wrong under thirty kilobytes.
			sent, _ := json.Marshal(map[string]any{"thinking": body["thinking"], "output_config": body["output_config"]})

			thinking, _ := body["thinking"].(map[string]any)
			if got, _ := thinking["type"].(string); got != tc.wantThinking {
				t.Errorf("thinking.type = %q, want %q; sent %s", got, tc.wantThinking, sent)
			}
			outputConfig, present := body["output_config"].(map[string]any)
			if tc.wantEffort == "" {
				if present {
					t.Errorf("output_config present at level %q, want it absent; sent %s", tc.level, sent)
				}
				return
			}
			if !present {
				t.Fatalf("output_config missing at level %q; sent %s", tc.level, sent)
			}
			if got, _ := outputConfig["effort"].(string); got != tc.wantEffort {
				t.Errorf("output_config.effort = %q, want %q", got, tc.wantEffort)
			}
		})
	}
}

// The same question for a provider whose dial rides a different wire format,
// and for the newest one in the catalog: a provider can be added, appear in the
// picker, answer questions, and still send no effort at all — the wiring that
// decides is a chain of provider-name tests in openai_compatible.go, and a name
// missing from it fails silently and looks exactly like working.
//
// Kimi is also the first provider with no "off": K3 always thinks. So the third
// case here is not "off is sent", it is "off is not offered, and a level that
// arrives anyway lands on the least thinking the model has" — a picker entry
// that cannot be honoured is the failure this whole exercise started with.
func TestEngineSendsReasoningEffortOnTheOpenAICompatibleWire(t *testing.T) {
	for _, tc := range []struct{ level, wantEffort string }{
		{"low", "low"},
		{"high", "high"},
		{"max", "max"},
		{"off", "low"}, // not offered by Kimi; folds onto its floor
	} {
		t.Run(tc.level, func(t *testing.T) {
			capture := newCaptureProvider(t, openAIShapedReply)
			t.Setenv("AETOX_DATA_ROOT", t.TempDir())

			res, err := Engine(config.Config{
				ModelProvider: "kimi",
				ModelName:     "kimi-k3",
				ModelBaseURL:  capture.server.URL,
				ModelAPIKey:   "test-key",
				ThinkLevel:    tc.level,
				SandboxRoot:   t.TempDir(),
				ApprovalMode:  string(safety.ApprovalAsk),
			}, Options{Console: DiscardConsole(), Approve: approveNothing})
			if err != nil {
				t.Fatalf("Engine: %v (%s)", err, res.Status)
			}
			if _, err := res.App.RunOnce(context.Background(), "say ok"); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}

			body := capture.first(t)
			got, _ := body["reasoning_effort"].(string)
			if got != tc.wantEffort {
				t.Errorf("reasoning_effort = %q, want %q (level %q)", got, tc.wantEffort, tc.level)
			}
			// The reasoning object is OpenRouter's shape, not this one. Sending
			// both is how a provider ends up ignoring the field it does read.
			if _, present := body["reasoning"]; present {
				t.Errorf("sent a reasoning object as well as reasoning_effort: %v", body["reasoning"])
			}
		})
	}
}

// A dial that is a switch, not a ladder.
//
// MiniMax has a `thinking` block and no effort field at all, so the two things
// worth pinning are that the switch reaches the request and that no invented
// effort rides along with it. The third case is the one the docs warn about:
// without reasoning_split the model returns its thinking inside content wrapped
// in <think> tags, which would land in the user's answer rather than the
// thinking panel.
func TestEngineSendsMiniMaxThinkingAsASwitch(t *testing.T) {
	for _, tc := range []struct {
		level    string
		wantType string
	}{
		{"on", "adaptive"},
		{"off", "disabled"},
		{"high", "adaptive"}, // not a level MiniMax has; folds onto on
	} {
		t.Run(tc.level, func(t *testing.T) {
			capture := newCaptureProvider(t, openAIShapedReply)
			t.Setenv("AETOX_DATA_ROOT", t.TempDir())

			res, err := Engine(config.Config{
				ModelProvider: "minimax",
				ModelName:     "MiniMax-M3",
				ModelBaseURL:  capture.server.URL,
				ModelAPIKey:   "test-key",
				ThinkLevel:    tc.level,
				SandboxRoot:   t.TempDir(),
				ApprovalMode:  string(safety.ApprovalAsk),
			}, Options{Console: DiscardConsole(), Approve: approveNothing})
			if err != nil {
				t.Fatalf("Engine: %v (%s)", err, res.Status)
			}
			if _, err := res.App.RunOnce(context.Background(), "say ok"); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}

			body := capture.first(t)
			thinking, _ := body["thinking"].(map[string]any)
			if got, _ := thinking["type"].(string); got != tc.wantType {
				t.Errorf("thinking.type = %q, want %q", got, tc.wantType)
			}
			if got, present := body["reasoning_effort"]; present {
				t.Errorf("sent reasoning_effort=%v; MiniMax has no effort field", got)
			}
			if split, _ := body["reasoning_split"].(bool); !split {
				t.Error("reasoning_split not set: the thinking would come back inside content as <think> tags")
			}
		})
	}
}
