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

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
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

func (*browserSkill) Name() string { return "browser" }

func (*browserSkill) Description() string {
	return "ใช้งานเบราว์เซอร์ของ workbench — เปิดหน้า อ่านหน้า คลิก และกรอกข้อความ (ผู้ใช้เห็นทุกอย่างที่ทำ)"
}

func (s *browserSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	// Built from the permitted set so the description never advertises an
	// action this caller would be refused.
	lines := map[string]string{
		// `newTab` says WHEN, not just what — the same rule `capture` follows
		// below, and it was missing here for a day. A model told an extra tab
		// exists and not told when to want one does one of two bad things: never
		// opens a second and re-fetches the page it just left, or opens one per
		// site and buries the user. Neither is the condition that generalizes.
		// The one that does is whether the page you are on is still an INPUT to
		// the work — and that is a question the model can actually answer about
		// its own task, which "is this important" is not.
		"open":  "`open` (url, newTab?) — go to a page and wait for it to load, replacing the current one. newTab=true keeps this page and opens an extra one: use it only when you will come back here, such as a list of results you are working through or a page you are comparing against. Renders a local file too (.html/.svg/.pdf/image) by sandbox-relative path; source files are downloads, use read.",
		"read":  "`read` — the page's text plus its interactive elements, each tagged with a [n] ref.",
		"click": "`click` (ref) — press the element with that ref.",
		"type":  "`type` (ref, text, enter?) — fill an input/textarea/select/contenteditable. For a select, text must match an option read listed. enter=true submits.",
		// Says when, not just what. A model holding a camera and no rule for it
		// photographs every page it opens, and every picture costs more than the
		// read it duplicated — so the line leads with the condition and names
		// the shape of page that meets it rather than listing cases.
		"capture": "`capture` — a picture of the page, saved and shown to you. Only when `read` cannot answer because the answer was never in the text: a chart, canvas, map, rendered document, or a layout you suspect is wrong. Read first, photograph second.",
		// The other half of newTab's condition, and the reason it is safe to
		// give: a tab kept is a window on somebody's screen.
		"tabs": "`tabs` (act: list|select|close, id) — your own tabs. select switches which one the other actions work. Close what you are done with.",
		// wait is the only action that can tell "not yet" from "not there", and
		// the line has to say so, because the failure it prevents does not look
		// like a failure: a page whose content has not arrived reads
		// SUCCESSFULLY as an empty page.
		"wait": "`wait` (text, seconds?) — wait until that text appears on the page. Most pages fetch their real content after loading, so a read straight after open or click can succeed and come back empty; wait first whenever you expect something that is not there yet.",
		// back says what it costs to be wrong, which is the whole reason it
		// exists — `open` is destructive and re-opening a URL is not the same as
		// coming back.
		"back":   "`back` — return to the previous page in this tab, with whatever you had typed or scrolled still there. Re-opening a URL is not the same thing.",
		"dialog": "`dialog` (accept, text?) — how this page's next alert/confirm/prompt is answered. They never block, but they are answered CANCEL unless you say otherwise, so set accept=true before the click that raises one you mean to agree to.",
	}
	var b strings.Builder
	b.WriteString("Work the page open in the workbench browser — the user watches this happen. Actions:\n")
	for _, action := range allowed {
		b.WriteString(lines[action] + "\n")
	}
	// The facts that decide whether a sequence works, stated once here because
	// the model reads this and not this file: a ref belongs to one page, and
	// every action lands on one tab that belongs to this agent.
	//
	// That second sentence read "There is ONE tab" until the tools were fixed to
	// target the agent's own tab rather than whichever was showing, and "exactly
	// ONE tab of your own" until tabs became plural. The user half of it never
	// changed and never will: it is the rule that lets somebody click around
	// their own browser while the agent works.
	b.WriteString("\nrefs come from `read` and belong to one page: they go stale when it changes or you select another tab. Read, act, read again. " +
		"Every action works the tab you opened last or selected, so re-opening a URL to see what changed is never the answer. " +
		"The user's own tabs are theirs: switching between them never moves yours, and you cannot reach them. " +
		"Never type a password or API key into a page; ask the user to.")

	return toolDef("browser", b.String(), map[string]any{
		"type": "object",
		"properties": map[string]any{
			// Deliberately terse. The description above already explains every one
			// of these in full, and a schema that repeats its own prose pays twice
			// for one sentence — which is not affordable in a block that has
			// tens of tokens left, not hundreds.
			"action":  map[string]any{"type": "string", "enum": allowed, "description": "What to do"},
			"url":     map[string]any{"type": "string", "description": "action=open"},
			"ref":     map[string]any{"type": "integer", "description": "action=click/type"},
			"text":    map[string]any{"type": "string", "description": "action=type, wait, or dialog"},
			"enter":   map[string]any{"type": "boolean", "description": "action=type"},
			"newTab":  map[string]any{"type": "boolean", "description": "action=open"},
			"act":     map[string]any{"type": "string", "enum": []string{"list", "select", "close"}, "description": "action=tabs"},
			"id":      map[string]any{"type": "string", "description": "action=tabs, from list"},
			"seconds": map[string]any{"type": "integer", "description": "action=wait: how long, default 10, max 60"},
			"accept":  map[string]any{"type": "boolean", "description": "action=dialog: true answers OK, false Cancel"},
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
		return (&browserReadSkill{app: s.app}).Execute(ctx, skill.Input{})
	case "click":
		return (&browserClickSkill{app: s.app}).click(intArg(args["ref"]))
	case "type":
		return (&browserTypeSkill{app: s.app}).typeText(intArg(args["ref"]), str(args["text"]), boolArg(args["enter"]))
	case "capture":
		return (&browserCaptureSkill{app: s.app}).capture(ctx)
	case "tabs":
		return (&browserTabsSkill{app: s.app}).run(str(args["act"]), str(args["id"]))
	case "wait":
		return (&browserWaitSkill{app: s.app}).wait(ctx, str(args["text"]), intArg(args["seconds"]))
	case "back":
		return (&browserBackSkill{app: s.app}).back(ctx)
	case "dialog":
		return (&browserDialogSkill{app: s.app}).dialog(boolArg(args["accept"]), str(args["text"]))
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
