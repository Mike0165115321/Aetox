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
//   - **Which actions?** The old `browser_<action>` names. A profile that
//     names none of them gets every action; one that names some gets exactly
//     those. Nothing in the vocabulary changed, so a manifest written before
//     this file still means what it said.
//
// The description the model reads lists only the actions that caller may use,
// because a tool that advertises what it will refuse is a wasted turn — the
// same reasoning that keeps a connection's tools out of the block entirely
// until an account exists.
//
// For its first day this file answered the second question itself, by reading
// the open session's chair profile — it could, being the one packed tool that
// lives where chairProfile does. Then shell and github packed too, in a package
// that cannot see a profile, and skill/packed.go generalized the idea: the
// vocabulary moved to the one table every pack declares itself in, and the
// narrowing arrives from subagent.FilterRegistry like everyone else's. The
// private gate was retired the same day it would have become the second
// mechanism answering the same question.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/skill"
)

type browserSkill struct {
	app *App
	// actions this caller may use, nil for all of them. Set only by Narrow.
	actions []string
}

func (s *browserSkill) allowedActions() []string {
	if s == nil || len(s.actions) == 0 {
		out := make([]string, 0, len(skill.PackedCalls("browser")))
		for _, call := range skill.PackedCalls("browser") {
			out = append(out, call.Action)
		}
		return out
	}
	return s.actions
}

func (s *browserSkill) Actions() []string { return skill.PackedActions("browser") }

// Narrow hands back a browser offering only the named actions — a copy, for
// the same shared-registry reason as shell's.
func (s *browserSkill) Narrow(named []string) skill.Skill {
	narrowed := *s
	var actions []string
	want := map[string]bool{}
	for _, n := range named {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}
	for _, call := range skill.PackedCalls("browser") {
		if want[call.Permission] {
			actions = append(actions, call.Action)
		}
	}
	// Silence is the whole tool, not an empty one — the rule this file set.
	if len(actions) == 0 {
		return s
	}
	narrowed.actions = actions
	return &narrowed
}

// browserToolName is the one name this pack answers to, spelled once because
// the busy signal matches tool events against it (desktop/app.go
// recordToolAction) and a UI comparing against a literal is a UI that keeps
// working after the tool is renamed, while showing nothing.
const browserToolName = "browser"

func (*browserSkill) Name() string { return browserToolName }

func (*browserSkill) Description() string {
	return "ใช้งานเบราว์เซอร์ของ workbench — เปิดหน้า อ่านหน้า คลิก และกรอกข้อความ (ผู้ใช้เห็นทุกอย่างที่ทำ)"
}

func (s *browserSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	// Built from the permitted set so the description never advertises an
	// action this caller would be refused.
	// SIGNATURES ONLY. Every "when should I reach for this" sentence that used
	// to live here now lives in Guidance below and is sent once, on the first
	// call to that action — internal/skill/guidance.go. This entry went from
	// 766 tokens to a fifth of that without the model being told less; it is
	// told the same things at a different time.
	lines := map[string]string{
		"open":    "`open` (url, newTab?) — go to a page and wait for it to load.",
		"read":    "`read` (filter?) — the page's text, plus its interactive elements each tagged [n]; filter lists only the elements whose text contains it.",
		"click":   "`click` (ref) — press the element with that ref.",
		"type":    "`type` (ref, text, enter?) — fill an input/textarea/select/contenteditable.",
		"wait":    "`wait` (text, seconds?) — wait until that text appears.",
		"back":    "`back` — return to the previous page in this tab.",
		"scroll":  "`scroll` (to: down|up|top|bottom, screens) — move the page N screens; read again after.",
		"capture": "`capture` (full?) — a picture of the page; full=true photographs the whole document instead of the visible part.",
		"tabs":    "`tabs` (act: list|select|close, id) — your own tabs.",
		"dialog":  "`dialog` (accept, text?) — answer this page's next alert/confirm/prompt.",
		"console": "`console` — what this page logged, threw, or had blocked since it loaded.",
		"network": "`network` — the fetch/XHR calls this page's own code made since it loaded.",
	}
	var b strings.Builder
	b.WriteString("Work a web page in the workbench browser, where the user can watch. Actions:\n")
	for _, action := range allowed {
		b.WriteString(lines[action] + "\n")
	}
	// Two sentences survive the migration, for two different reasons.
	//
	// The first is signature rather than judgment: which tab an action lands on
	// is part of what calling it means, and it is one line.
	//
	// The second is a SAFETY rule, and safety does not go in Guidance however
	// much it reads like judgment. Guidance rides in the message stream, where a
	// conversation long enough to be summarised can lose it silently. That is a
	// fine price for "read before you photograph" and not for this one.
	//
	// What did move, and where: how a ref goes stale went to the guidance of the
	// actions that spend refs (read, click, type, tabs), and whose tabs are whose
	// went to `tabs`. A session that only opens a page hears neither and needs
	// neither, which is the whole point of keying per action.
	b.WriteString("\nEvery action works the tab you opened last or selected. " +
		"Never type a password or API key into a page; ask the user to.")

	return toolDef("browser", b.String(), map[string]any{
		"type": "object",
		// Types, not sentences. Every one of these used to describe itself —
		// "action=open: a URL, or a file path relative to the sandbox root" —
		// which is the signature line a few lines above, paid for a second time.
		// A parameter whose name and owning action are both already on screen
		// needs nothing here but its type.
		"properties": map[string]any{
			"action":  map[string]any{"type": "string", "enum": allowed},
			"url":     map[string]any{"type": "string"},
			"ref":     map[string]any{"type": "integer"},
			"filter":  map[string]any{"type": "string"},
			"text":    map[string]any{"type": "string"},
			"enter":   map[string]any{"type": "boolean"},
			"newTab":  map[string]any{"type": "boolean"},
			"to":      map[string]any{"type": "string"},
			"act":     map[string]any{"type": "string", "enum": []string{"list", "select", "close"}},
			"id":      map[string]any{"type": "string"},
			"seconds": map[string]any{"type": "integer"},
			"screens": map[string]any{"type": "integer"},
			"accept":  map[string]any{"type": "boolean"},
			"full":    map[string]any{"type": "boolean"},
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
		return skill.Output{Name: "browser"}, fmt.Errorf("action is required — one of %s", strings.Join(s.allowedActions(), ", "))
	}
	// Refused here as well as hidden from the description, because a
	// description is guidance and a gate is a gate.
	if !slices.Contains(s.allowedActions(), action) {
		return skill.Output{Name: "browser"}, fmt.Errorf("browser %s is not available here — this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}

	switch action {
	case "open":
		return (&browserOpenSkill{app: s.app}).open(ctx, str(args["url"]), boolArg(args["newTab"]))
	case "read":
		return (&browserReadSkill{app: s.app}).Execute(ctx, skill.Input{"filter": str(args["filter"])})
	case "click":
		return (&browserClickSkill{app: s.app}).click(intArg(args["ref"]))
	case "type":
		return (&browserTypeSkill{app: s.app}).typeText(intArg(args["ref"]), str(args["text"]), boolArg(args["enter"]))
	case "capture":
		return (&browserCaptureSkill{app: s.app}).capture(ctx, boolArg(args["full"]))
	case "tabs":
		return (&browserTabsSkill{app: s.app}).run(str(args["act"]), str(args["id"]))
	case "wait":
		return (&browserWaitSkill{app: s.app}).wait(ctx, str(args["text"]), intArg(args["seconds"]))
	case "back":
		return (&browserBackSkill{app: s.app}).back(ctx)
	case "scroll":
		return (&browserScrollSkill{app: s.app}).scroll(str(args["to"]), intArg(args["screens"]))
	case "dialog":
		return (&browserDialogSkill{app: s.app}).dialog(boolArg(args["accept"]), str(args["text"]))
	case "console", "network":
		return (&browserLogSkill{app: s.app, kind: action}).run(ctx)
	}
	return skill.Output{Name: "browser"}, fmt.Errorf("unknown browser action %q", action)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// Both of these used to be written out here, and both were a shape short: an
// int and a float64 but not a quoted number, a bool but not a quoted bool.
// Models send the quoted forms — `{"action":"click","ref":"1"}` is in this
// machine's tool_runs twelve times — and each one arrived as a zero-value that
// no longer resembled what was asked for. internal/skill has had the right rule
// since `read` needed it; this defers to it rather than agreeing with it.
func intArg(v any) int   { return skill.IntArg(v) }
func boolArg(v any) bool { return skill.BoolArg(v) }
