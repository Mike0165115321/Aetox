package app

import "testing"

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
