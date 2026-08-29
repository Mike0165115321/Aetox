package skill

// The standard: a tool's block entry carries Existence and Signature. Judgment
// goes in Guidance() and is delivered once (guidance.go).
//
// This test is the standard's teeth, and it is a RATCHET rather than a line in
// the sand, deliberately. A plain ceiling would fail on the day it was written
// — thirty-odd tools are over it right now — and a test that starts red is a
// test somebody deletes. So:
//
//   - A tool not named below must fit the ceiling. That is every future tool,
//     and it is where the standard actually bites.
//   - A tool named below may not grow past the size it was on the day the
//     standard landed. It may shrink freely, and when it drops under the
//     ceiling its line here is deleted.
//
// The list is therefore a to-do list that empties itself, and the day it is
// empty this comment and the map go with it.

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

const (
	// A plain tool: one act, a handful of parameters. Eighty tokens is a
	// sentence of what it is plus its parameter names — measured against the
	// tools already at or under it (`time`, `calc`, `todo_write`), not picked
	// from the air.
	blockCeiling = 80
	// A packed tool signs several acts, so it gets an allowance per act on top
	// of a base.
	//
	// These two numbers were guessed at first (65 + 10) and re-derived from the
	// first real migration, which is the only way either of them could have been
	// right. Migrating `browser` landed it at 314 against a guessed ceiling of
	// 155, and the gap was not prose — it was structure. A nine-action tool pays
	// ~14 tokens for each signature line AND ~11 for that action's parameter in
	// the JSON schema, and ten typed properties with no descriptions at all
	// still cost 106 tokens. The base covers the opening line, the shared rules
	// and the envelope.
	//
	// So: 100 + 28 per act, which leaves `browser` about a tenth of headroom at
	// 314 of an allowed 352. Tight on purpose. The next action added to a packed
	// tool should have to be worth 28 tokens forever.
	packedBase   = 100
	packedPerAct = 28
)

// overweight is every tool in THIS package that exceeded the standard on
// 2026-08-18, with the size it was — measured, not estimated; the first draft
// of this list was guessed and every number in it was wrong, which the ratchet
// itself caught on the first run.
//
// `browser` (766) and `task` (1,568), the two largest in the whole block, are
// not here because they are registered by the desktop and this package cannot
// see them. They are covered by the same standard and pinned separately in
// desktop/tool_budget_test.go.
//
// The size it was. Shrinking is always allowed; growing is not. Delete a line when
// its tool comes under the ceiling — that is the migration, one tool at a time,
// with the diff as the progress bar.
var overweight = map[string]int{
	"shell":     498,
	"n8n":       624,
	"doc_write": 525,
	"windmill":  484,
	// `search` is what `grep`, `glob` and `list` became on 2026-08-29
	// (search_pack.go), and the number is why the pack was worth building: the
	// three of them were 320 + 206 + 77 = 603 tokens of every block that can
	// look at a file, and one entry does the same three acts for 474.
	//
	// It is over its ceiling (100 + 28 per act = 184) and will stay over it:
	// grep alone was 320 and exempted, because its options ARE its signature —
	// show, context, type, multiline and the paging pair are what you pass it,
	// not advice about when to reach for it. The prose already moved; what is
	// left is the shape of the call.
	//
	// The ratchet still bites the same way. This may shrink and may not grow,
	// and the two lines it replaced are gone rather than left to rot: a pin on
	// a tool that is no longer registered is never checked and never fails.
	//
	// grep was 392 until 2026-08-27 (mode prose to Guidance) and 326 until
	// 2026-08-29 (`type` and `multiline` added, paid for by more prose moving).
	"search":      474,
	"github":      390,
	// `change` is what `write`, `edit`, `edits` and `delete` became on
	// 2026-08-29 (change_pack.go), and the arithmetic is why it was worth
	// building: 141 + 234 + 243 + 159 = 777 tokens of every block that can
	// change a file, against 599 for one entry doing the same five acts.
	//
	// Over its ceiling (100 + 28 per act = 240) and staying there. What is left
	// after the prose moved is signature: the 300-line cap and the placement
	// rule ride in the block on purpose (write.go says why - both must be known
	// BEFORE the first call, and Guidance arrives after it), and `batch` carries
	// a nested array that cannot be described in fewer words than it has.
	//
	// The ratchet bites the same way: this may shrink and may not grow, and the
	// four lines it replaced are gone rather than left to rot - a pin on a tool
	// that is no longer registered is never checked and never fails.
	"change":      599,
	// `codebase` is diagnostics + symbol + repo_map (167 + 180 + 75 = 422 to
	// 263) and `media_read` is the three senses (100 + 143 + 136 = 379 to 231),
	// both on 2026-08-29. Over their 184-token ceilings because three acts with
	// three different shapes cannot introduce themselves in fewer words - and
	// under what the entries they replaced cost together, which is the number
	// that was worth having.
	"codebase":    263,
	"media_read":  231,
	// `pr` arrived packed (pr_pack.go) rather than as five tools, so there is
	// no before to compare it against - five standalone entries carrying these
	// same signatures would have been well past a thousand. Over the 240-token
	// ceiling because five acts with five different argument sets cannot say
	// what to pass them in fewer words; the judgment (push before you open,
	// a comment is public and cannot be edited from here) is in Guidance().
	"pr":          432,

	"sheet_write": 368,
	// Not registered since 2026-08-19 (defaults.go), so nothing reaches it
	// today. The pin stays because the file does: it is the size this tool
	// comes back at, and coming back over the standard should have to be
	// noticed rather than inherited.
	"notebook_edit":    317,
	"web_fetch":        215,
	"web_search":       198,
	"read":             185,
	"git":              176,
	"calc":             137,
	"skill_view":       122,
	"pdf_read":         96,
	"skills_list":      86,
}

func TestToolBlockEntriesCarryOnlyExistenceAndSignature(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	dispatcher := NewDispatcher(registry)

	var offenders []string
	for _, def := range dispatcher.ToolDefinitions() {
		name := def.Function.Name
		payload, err := json.Marshal(def)
		if err != nil {
			t.Fatalf("%s: definition does not marshal: %v", name, err)
		}
		tokens := len(payload) / 4 // the same rough rate desktop/tool_budget_test.go uses

		ceiling := blockCeiling
		if calls := PackedCalls(name); len(calls) > 0 {
			ceiling = packedBase + packedPerAct*len(calls)
		}
		if tokens <= ceiling {
			// Under the ceiling, so it must not still be claiming an exemption.
			if was, listed := overweight[name]; listed {
				t.Errorf("%s is now %d tokens, under its ceiling of %d — delete its line (%d) from overweight",
					name, tokens, ceiling, was)
			}
			continue
		}
		was, listed := overweight[name]
		if !listed {
			offenders = append(offenders, name)
			t.Errorf("%s is %d tokens, over the %d-token ceiling for its shape.\n"+
				"  A block entry carries what the tool IS and what to pass it. When to reach for it,\n"+
				"  what it costs and what to watch out for go in Guidance() and are sent once — see\n"+
				"  internal/skill/guidance.go.", name, tokens, ceiling)
			continue
		}
		if tokens > was {
			t.Errorf("%s grew from %d to %d tokens. It is already over the standard; it may shrink, not grow.\n"+
				"  Move the new prose into Guidance() instead.", name, was, tokens)
		}
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Logf("new tools over the standard: %v", offenders)
	}
}

// Guidance is delivered once per session and never again, because a session is
// the lifetime of exactly one Dispatcher. Twice would be a slow leak of the
// thing this whole design exists to stop sending.
func TestGuidanceIsSentOncePerSession(t *testing.T) {
	registry := NewRegistry()
	tool := &guidedProbe{}
	if err := registry.Register(tool, SourceBuiltin); err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher(registry)

	first, ok, err := d.ExecuteTool(t.Context(), "probe", nil)
	if !ok || err != nil {
		t.Fatalf("first call: ok=%v err=%v", ok, err)
	}
	const want = "when to reach for probe"
	if !strings.Contains(first.RawOutput, want) {
		t.Errorf("the model was never taught: %q", first.RawOutput)
	}
	if !strings.Contains(first.Content, want) {
		t.Errorf("the timeline never showed it: %q", first.Content)
	}
	if !strings.Contains(first.RawOutput, "did the thing") {
		t.Error("teaching replaced the result instead of preceding it")
	}

	second, _, _ := d.ExecuteTool(t.Context(), "probe", nil)
	if strings.Contains(second.RawOutput, "when to reach for probe") {
		t.Errorf("taught twice in one session: %q", second.RawOutput)
	}

	// A new session is a new Dispatcher, and it starts knowing nothing.
	fresh := NewDispatcher(registry)
	again, _, _ := fresh.ExecuteTool(t.Context(), "probe", nil)
	if !strings.Contains(again.RawOutput, "when to reach for probe") {
		t.Error("a new session did not get the guidance")
	}
}

// A failed first call is exactly when the judgment was missing, so it teaches
// too. This was worth pinning: the obvious implementation returns early on
// error and silently makes the failure case the one that never learns.
func TestAFailedFirstCallStillTeaches(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&guidedProbe{fail: true}, SourceBuiltin); err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher(registry)

	out, _, err := d.ExecuteTool(t.Context(), "probe", nil)
	if err == nil {
		t.Fatal("the probe was supposed to fail")
	}
	if !strings.Contains(out.RawOutput, "when to reach for probe") {
		t.Errorf("a call that failed for want of guidance was not given any: %q", out.RawOutput)
	}
}

// A tool with nothing to say once must not gain an empty banner.
func TestAToolWithNoGuidanceIsLeftAlone(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&guidedProbe{silent: true}, SourceBuiltin); err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher(registry)

	out, _, _ := d.ExecuteTool(t.Context(), "probe", nil)
	if out.RawOutput != "did the thing" {
		t.Errorf("output was decorated with an empty teaching: %q", out.RawOutput)
	}
}
