package main

// One tool for one thing: the page open in the workbench browser.
//
// It was four — `browser_open`, `browser_read`, `browser_click`, `browser_type`
// — and they were four tools describing one object, which is the shape the
// owner rejected on 2026-08-10: "browser จริง ๆ มันควรจะแพ็ครวมกัน... หากเพิ่ม
// จะได้ไม่ต้องเสียเวลามาไล่เปิดทีละตัว ๆ". Every capability added to the browser
// used to cost another entry in the tool block of every request that carries
// it; now it costs an action.
//
// The objection worth recording, because it is what the design answers: four
// names are four *rights*, and `tools:` in a profile grants by name — collapsing
// them would make "may look at a page" and "may act on one" the same permission,
// which is not a distinction to lose in a product whose rule is that rights come
// only from a list the user can see. The owner's answer was "รวมแล้วแยกย่อย
// ข้างใน", and that is exactly what this file does: one tool on the outside, the
// four original names still the vocabulary of permission on the inside.
//
// So the gate has two levels and they answer different questions:
//
//   - **Does this caller get the browser at all?** The registry filter, on the
//     name `browser` — desk categories, and a profile's `tools:` list.
//   - **Which actions?** allowedBrowserActions, on the old `browser_<action>`
//     names. A profile that names none of them gets every action; one that
//     names some gets exactly those. Nothing in the vocabulary changed, so a
//     manifest written before this file still means what it said.
//
// The description the model reads lists only the actions that caller may use,
// because a tool that advertises what it will refuse is a wasted turn — the
// same reasoning that keeps a connection's tools out of the block entirely
// until an account exists.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// browserActions are the four, in the order a session uses them.
var browserActions = []string{"open", "read", "click", "type"}

// legacyBrowserTool is the per-action permission name — the one this codebase
// has used since 2026-07-22 and that manifests, categories and docs still name.
func legacyBrowserTool(action string) string { return "browser_" + action }

// allowedBrowserActions reports which actions this session's caller may use.
//
// Only a chair (an agent chatted with directly, or dispatched) can narrow:
// a desk session is judged by the registry filter above this, which either
// handed over `browser` or did not. Reading `tools:` here rather than asking
// the filter is deliberate — the filter answers about the name `browser`, and
// the question here is about the four names inside it.
func (a *App) allowedBrowserActions() []string {
	p := a.chairProfile()
	if p == nil {
		return browserActions
	}
	var named []string
	for _, action := range browserActions {
		if slices.Contains(p.Tools, legacyBrowserTool(action)) {
			named = append(named, action)
		}
	}
	// A profile that names no action at all is not asking for a narrower
	// browser — it asked for `browser` and got it whole. Treating silence as
	// "nothing allowed" would hand an agent a tool that refuses every call.
	if len(named) == 0 {
		return browserActions
	}
	return named
}

type browserSkill struct{ app *App }

func (*browserSkill) Name() string { return "browser" }

func (*browserSkill) Description() string {
	return "ใช้งานเบราว์เซอร์ของ workbench — เปิดหน้า อ่านหน้า คลิก และกรอกข้อความ (ผู้ใช้เห็นทุกอย่างที่ทำ)"
}

func (s *browserSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.app.allowedBrowserActions()

	// Built from the permitted set so the description never advertises an
	// action this caller would be refused.
	lines := map[string]string{
		"open":  "`open` (url) — go to a page and wait for it to load. Also opens a local file the browser can render (.html, .svg, .pdf, an image) by its sandbox-relative path; source files are downloads, not pages — use read for those.",
		"read":  "`read` — the current page's text, plus its interactive elements each tagged with a [n] ref.",
		"click": "`click` (ref) — press the element with that ref.",
		"type":  "`type` (ref, text, enter?) — fill an input/textarea/select/contenteditable. For a select, text must match an option read listed. enter=true submits.",
	}
	var b strings.Builder
	b.WriteString("Work the page open in the workbench browser — the user watches this happen. Actions:\n")
	for _, action := range allowed {
		b.WriteString(lines[action] + "\n")
	}
	// The two facts that decide whether a sequence works, stated once here
	// because the model reads this and not this file: refs come from a read,
	// and there is only ever one tab.
	b.WriteString("\nrefs come from `read` and go stale as soon as the page changes: read, act, read again. " +
		"There is ONE tab — every action works the page `open` last went to, so re-opening a URL to see what changed is never the answer. " +
		"Never type a password or an API key into a page; ask the user to type it.")

	return toolDef("browser", b.String(), map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": allowed, "description": "What to do"},
			"url":    map[string]any{"type": "string", "description": "action=open: a URL, or a file path relative to the sandbox root"},
			"ref":    map[string]any{"type": "integer", "description": "action=click/type: element ref from a read"},
			"text":   map[string]any{"type": "string", "description": "action=type: what to type, or the option to choose"},
			"enter":  map[string]any{"type": "boolean", "description": "action=type: press Enter afterwards"},
		},
		"required": []string{"action"},
	})
}

func (s *browserSkill) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	return s.run(ctx, args)
}

func (s *browserSkill) Execute(ctx context.Context, input skill.Input) (skill.Output, error) {
	return s.run(ctx, map[string]any(input))
}

func (s *browserSkill) run(ctx context.Context, args map[string]any) (skill.Output, error) {
	action := strings.ToLower(strings.TrimSpace(str(args["action"])))
	if action == "" {
		return skill.Output{Name: "browser"}, fmt.Errorf("action is required — one of %s", strings.Join(s.app.allowedBrowserActions(), ", "))
	}
	// Refused here as well as hidden from the description, because a
	// description is guidance and a gate is a gate.
	if !slices.Contains(s.app.allowedBrowserActions(), action) {
		return skill.Output{Name: "browser"}, fmt.Errorf("browser %s is not available here — this session may use: %s",
			action, strings.Join(s.app.allowedBrowserActions(), ", "))
	}

	switch action {
	case "open":
		return (&browserOpenSkill{app: s.app}).open(ctx, str(args["url"]))
	case "read":
		return (&browserReadSkill{app: s.app}).Execute(ctx, skill.Input{})
	case "click":
		return (&browserClickSkill{app: s.app}).click(intArg(args["ref"]))
	case "type":
		return (&browserTypeSkill{app: s.app}).typeText(intArg(args["ref"]), str(args["text"]), boolArg(args["enter"]))
	}
	return skill.Output{Name: "browser"}, fmt.Errorf("unknown browser action %q", action)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func intArg(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func boolArg(v any) bool {
	b, _ := v.(bool)
	return b
}
