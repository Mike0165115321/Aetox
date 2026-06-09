package app

import (
	"strings"
	"testing"
)

type testConsole struct {
	lines []string
}

func (c *testConsole) Print(msg any) {}

func (c *testConsole) Printf(format string, args ...any) {}

func (c *testConsole) Println(msg ...any) {
	c.lines = append(c.lines, strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(strings.Join(stringify(msg), " ")), "\n"), "\r")))
}

func (c *testConsole) Errorf(format string, args ...any) {}

func (c *testConsole) ReadLine() (string, error) { return "", nil }

func stringify(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.TrimSpace(toString(value)))
	}
	return out
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func TestRenderHeaderStatusLineIncludesModelModeLabel(t *testing.T) {
	app := &App{
		title:       "Aetox CLI",
		modelStatus: "deepseek/deepseek-v4-flash(low)",
	}

	line := app.renderHeaderStatusLine()
	if line == "" {
		t.Fatalf("expected header status line output")
	}
	if line != renderAlignedStatusLine("Aetox CLI", "deepseek/deepseek-v4-flash(low)") {
		t.Fatalf("unexpected header line %q", line)
	}
}

func TestRenderPromptStatusLineIncludesContextOnPromptRow(t *testing.T) {
	app := &App{
		title:              "Aetox CLI",
		modelStatus:        "deepseek/deepseek-v4-flash(off)",
		modelContextTokens: 320,
	}

	line := app.renderPromptStatusLine()
	if line == "" {
		t.Fatalf("expected prompt status line output")
	}
	if line != renderAlignedStatusLine(">", "context 0/320 tokens") {
		t.Fatalf("unexpected prompt line %q", line)
	}
}
