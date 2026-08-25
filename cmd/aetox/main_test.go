package main

import (
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/model"
)

func TestPreparseGlobalFlagsIncludesThink(t *testing.T) {
	global, remaining, err := preparseGlobalFlags([]string{"--think", "high", "chat", "hello"})
	if err != nil {
		t.Fatalf("preparse failed: %v", err)
	}
	if len(global) != 2 || global[0] != "--think" || global[1] != "high" {
		t.Fatalf("unexpected global flags: %#v", global)
	}
	if len(remaining) != 2 || remaining[0] != "chat" || remaining[1] != "hello" {
		t.Fatalf("unexpected remaining args: %#v", remaining)
	}
}

func TestParseModelWithThinking(t *testing.T) {
	model, level, ok := parseModelWithThink("deepseek-r1(max)")
	if !ok {
		t.Fatalf("expected model suffix parse")
	}
	if model != "deepseek-r1" {
		t.Fatalf("expected model deepseek-r1 got %q", model)
	}
	if level != "max" {
		t.Fatalf("expected level max got %q", level)
	}
}

func TestParseModelWithThinkingRejectsInvalidLevel(t *testing.T) {
	model, level, ok := parseModelWithThink("deepseek-r1(foo_bar)")
	if ok {
		t.Fatalf("expected suffix parse to fail, got %q (%q)", model, level)
	}
}

func TestPreparseGlobalFlagsAcceptsOffThink(t *testing.T) {
	global, remaining, err := preparseGlobalFlags([]string{"--think", "off-think", "chat", "hello"})
	if err != nil {
		t.Fatalf("preparse failed: %v", err)
	}
	if len(global) != 2 || global[0] != "--think" || global[1] != "off-think" {
		t.Fatalf("unexpected global flags: %#v", global)
	}
	if len(remaining) != 2 || remaining[0] != "chat" || remaining[1] != "hello" {
		t.Fatalf("unexpected remaining args: %#v", remaining)
	}
}

// The built-in aetox provider has no thinking knob, so its status line carries
// no think suffix — it used to print one from whatever the config happened to
// hold, which is a level nothing would ever send.
func TestResolveModelStatusOmitsThinkWhenTheProviderHasNoDial(t *testing.T) {
	status := resolveModelStatus(config.Config{
		ModelProvider: "noop",
		ModelName:     "noop",
		ThinkLevel:    "high",
	}, model.BootstrapResult{
		Provider: model.NewNoopProvider("noop"),
	})
	// ModelProvider "noop" is a backward-compat alias — ResolveStatus
	// normalizes it to the current canonical name "aetox".
	want := "aetox/noop"
	if status != want {
		t.Fatalf("want %q got %q", want, status)
	}
}

func TestResolveModelStatusOmitsThinkEvenWhenTheConfigSaysOff(t *testing.T) {
	status := resolveModelStatus(config.Config{
		ModelProvider: "noop",
		ModelName:     "noop",
		ThinkLevel:    "off",
	}, model.BootstrapResult{
		Provider: model.NewNoopProvider("noop"),
	})
	want := "aetox/noop"
	if status != want {
		t.Fatalf("want %q got %q", want, status)
	}
}

func TestFormatModelModeLabelDefaultsToLow(t *testing.T) {
	label := formatModelModeLabel("deepseek", "deepseek-v4-flash", "")
	want := "deepseek/deepseek-v4-flash(high)"
	if label != want {
		t.Fatalf("want %q got %q", want, label)
	}
}

func TestDefaultThinkLevelNormalizesInput(t *testing.T) {
	if got := defaultThinkLevel("deepseek", "deepseek-v4-flash", "HIGH"); got != "high" {
		t.Fatalf("want high got %q", got)
	}
	if got := defaultThinkLevel("deepseek", "deepseek-v4-flash", ""); got != "high" {
		t.Fatalf("want high got %q", got)
	}
}
