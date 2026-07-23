package model

// DeepSeek V4's native tool-call markup ("DSML") sometimes leaks into message
// content as plain text instead of arriving as structured tool_calls — a known
// ecosystem issue (thinking-mode turns especially; see e.g. CherryHQ/
// cherry-studio#14714). parseDSMLToolCalls is the backstop, not a gate
// (ARCHITECTURE.md §17): if a response carries no structured calls but its
// text contains a well-formed DSML block, the calls are lifted out and the
// block stripped from the text; anything else passes through untouched.
//
// Grammar (DeepSeek-V4-Flash encoding/README.md; fullwidth ｜ in the real
// tokens, ASCII | tolerated for robustness):
//
//	<｜DSML｜tool_calls>
//	<｜DSML｜invoke name="function_name">
//	<｜DSML｜parameter name="param" string="true">raw string</｜DSML｜parameter>
//	<｜DSML｜parameter name="count" string="false">5</｜DSML｜parameter>
//	</｜DSML｜invoke>
//	</｜DSML｜tool_calls>
//
// string="true" → value is a raw string; string="false" → value is JSON.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	dsmlBlockRe  = regexp.MustCompile(`(?s)<[|｜]DSML[|｜]tool_calls>(.*?)</[|｜]DSML[|｜]tool_calls>`)
	dsmlInvokeRe = regexp.MustCompile(`(?s)<[|｜]DSML[|｜]invoke name="([^"]+)">(.*?)</[|｜]DSML[|｜]invoke>`)
	dsmlParamRe  = regexp.MustCompile(`(?s)<[|｜]DSML[|｜]parameter name="([^"]+)"(?:\s+string="(true|false)")?>(.*?)</[|｜]DSML[|｜]parameter>`)
	dsmlEOSRe    = regexp.MustCompile(`<[|｜]end▁of▁sentence[|｜]>`)
)

// parseDSMLToolCalls extracts DSML tool calls from text. Returns the text with
// the DSML blocks (and any trailing EOS token) removed, plus the parsed calls.
// Returns the input unchanged and nil when no complete block is present.
func parseDSMLToolCalls(text string) (string, []ToolCall) {
	if !dsmlBlockRe.MatchString(text) {
		return text, nil
	}
	var calls []ToolCall
	for _, block := range dsmlBlockRe.FindAllStringSubmatch(text, -1) {
		for _, inv := range dsmlInvokeRe.FindAllStringSubmatch(block[1], -1) {
			args := map[string]json.RawMessage{}
			for _, p := range dsmlParamRe.FindAllStringSubmatch(inv[2], -1) {
				name, isString, value := p[1], p[2], strings.TrimSpace(p[3])
				if isString == "false" && json.Valid([]byte(value)) {
					args[name] = json.RawMessage(value)
				} else {
					encoded, err := json.Marshal(value)
					if err != nil {
						continue
					}
					args[name] = json.RawMessage(encoded)
				}
			}
			encodedArgs, err := json.Marshal(args)
			if err != nil {
				continue
			}
			calls = append(calls, ToolCall{
				ID:       fmt.Sprintf("dsml-%d", len(calls)+1),
				Type:     "function",
				Function: FunctionCall{Name: inv[1], Arguments: string(encodedArgs)},
			})
		}
	}
	if len(calls) == 0 {
		return text, nil
	}
	cleaned := dsmlBlockRe.ReplaceAllString(text, "")
	cleaned = dsmlEOSRe.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned), calls
}
