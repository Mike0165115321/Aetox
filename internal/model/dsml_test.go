package model

import (
	"strings"
	"testing"
)

func TestParseDSMLToolCallsLiftsCallsAndCleansText(t *testing.T) {
	text := "กำลังสร้างไฟล์ให้ครับ\n" +
		"<｜DSML｜tool_calls>\n" +
		"<｜DSML｜invoke name=\"write\">\n" +
		"<｜DSML｜parameter name=\"path\" string=\"true\">landing.html</｜DSML｜parameter>\n" +
		"<｜DSML｜parameter name=\"content\" string=\"true\"><!DOCTYPE html></｜DSML｜parameter>\n" +
		"</｜DSML｜invoke>\n" +
		"</｜DSML｜tool_calls>\n" +
		"<｜end▁of▁sentence｜>"

	cleaned, calls := parseDSMLToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "write" {
		t.Fatalf("unexpected call name %q", calls[0].Function.Name)
	}
	args, err := ParseToolArguments(calls[0].Function.Arguments)
	if err != nil {
		t.Fatalf("arguments not valid JSON: %v", err)
	}
	if args["path"] != "landing.html" || args["content"] != "<!DOCTYPE html>" {
		t.Fatalf("unexpected args: %#v", args)
	}
	if strings.Contains(cleaned, "DSML") || strings.Contains(cleaned, "end▁of▁sentence") {
		t.Fatalf("markup must be stripped, got %q", cleaned)
	}
	if cleaned != "กำลังสร้างไฟล์ให้ครับ" {
		t.Fatalf("surrounding text must survive, got %q", cleaned)
	}
}

func TestParseDSMLToolCallsJSONTypedParameter(t *testing.T) {
	text := `<|DSML|tool_calls><|DSML|invoke name="grep">` +
		`<|DSML|parameter name="pattern" string="true">TODO</|DSML|parameter>` +
		`<|DSML|parameter name="max" string="false">5</|DSML|parameter>` +
		`</|DSML|invoke></|DSML|tool_calls>`

	_, calls := parseDSMLToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Arguments != `{"max":5,"pattern":"TODO"}` {
		t.Fatalf("unexpected arguments: %s", calls[0].Function.Arguments)
	}
}

func TestParseDSMLToolCallsPassesPlainTextThrough(t *testing.T) {
	text := "just a normal answer mentioning DSML in prose"
	cleaned, calls := parseDSMLToolCalls(text)
	if calls != nil || cleaned != text {
		t.Fatalf("plain text must pass through untouched, got %q calls=%v", cleaned, calls)
	}
}
