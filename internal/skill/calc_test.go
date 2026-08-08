package skill

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The claim calc makes is not "there is a JavaScript engine in here" — it is
// that a number in an answer was worked out rather than remembered, and that
// the user can see what worked it out. These tests pin both halves, plus the
// three ways a script written by a model goes wrong.

func run(t *testing.T, script string) Output {
	t.Helper()
	out, err := (&calcSkill{}).Execute(context.Background(), Input{"script": script})
	if err != nil {
		t.Fatalf("calc(%q) failed: %v", script, err)
	}
	return out
}

func TestCalcAnswersWithTheLastExpression(t *testing.T) {
	if got := run(t, "1234 * 5678").Content; got != "7006652" {
		t.Errorf("calc answered %q, want 7006652", got)
	}
	// The case this tool exists for: a percentage of a number nobody should be
	// asked to do in their head.
	if got := run(t, "Math.round(128400 * 0.037 * 100) / 100").Content; got != "4750.8" {
		t.Errorf("calc answered %q, want 4750.8", got)
	}
}

func TestCalcPrintsTheStepsAboveTheAnswer(t *testing.T) {
	out := run(t, `
		let total = 0
		for (const m of [1, 2, 3]) { total += m * 1000; print('month ' + m + ': ' + total) }
		total
	`)
	for _, want := range []string{"month 1: 1000", "month 3: 6000", "= 6000"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("calc output missing %q:\n%s", want, out.Content)
		}
	}
}

// Reaching for console.log is muscle memory, not a choice. If it is missing,
// the first attempt at a script dies on a ReferenceError and the round trip
// buys nothing but a house rule.
func TestCalcAcceptsConsoleLog(t *testing.T) {
	out := run(t, "console.log('subtotal', 1200); console.warn('rounded'); 1200 * 1.07")
	for _, want := range []string{"subtotal 1200", "rounded", "= 1284"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("calc output missing %q:\n%s", want, out.Content)
		}
	}
}

// console.table is the same muscle for tabular results — and a console that
// exists but lacks the member dies on a TypeError, a worse error than the
// ReferenceError console.log's absence gave ("Object has no member 'table'",
// seen in the wild 2026-08-07). The data prints as JSON; no column art.
func TestCalcAcceptsConsoleTable(t *testing.T) {
	out := run(t, `console.table([{year: 1, value: 9300}]); 9300`)
	for _, want := range []string{`"year":1`, `"value":9300`, "= 9300"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("calc output missing %q:\n%s", want, out.Content)
		}
	}
}

// `return` is what anyone writes when told to hand back an answer, and it is
// illegal at the top level of a script. It is also the way out of a real trap:
// a script whose last line is `({total: x})` is read as a call on the line
// above, and fails with a message about initialization that names nothing.
func TestCalcAcceptsReturn(t *testing.T) {
	if got := run(t, "const a = 21\nreturn a * 2").Content; got != "42" {
		t.Errorf("return answered %q, want 42", got)
	}
	if got := run(t, "const rate = 0.07\nreturn {rate, monthly: rate / 12}").Content; !strings.Contains(got, `"rate":0.07`) {
		t.Errorf("returning an object answered %q", got)
	}
	// The trap itself, written the way a model writes it.
	out, err := (&calcSkill{}).Execute(context.Background(), Input{
		"script": "const total = [1,2,3].reduce((a,b) => a+b, 0)\nreturn {total}",
	})
	if err != nil {
		t.Fatalf("the shape that avoids the trap failed: %v", err)
	}
	if !strings.Contains(out.Content, `"total":6`) {
		t.Errorf("answered %q", out.Content)
	}
}

// The trap where JavaScript's own message points at the wrong thing: the error
// blames a variable that is fine and never mentions the line break. Without the
// hint the model reads "Cannot access a variable before initialization", cannot
// see what is wrong with its script, and writes the same script again.
func TestCalcExplainsTheLineThatContinuesTheOneAboveIt(t *testing.T) {
	out, err := (&calcSkill{}).Execute(context.Background(), Input{
		"script": "const v = [1,2,3].map(n => n * 2)\nconst total = v.reduce((a,b) => a+b, 0)\n({v, total})",
	})
	if err == nil {
		t.Fatalf("the trap did not fire — this test is now blind: %q", out.Content)
	}
	for _, want := range []string{"line 3", "continuation", "return"} {
		if !strings.Contains(out.Stderr, want) {
			t.Errorf("the error does not explain the line break (%q missing):\n%s", want, out.Stderr)
		}
	}
}

// And the hint must stay quiet on a script that never had the problem, or it
// becomes noise attached to every unrelated error.
func TestCalcDoesNotBlameLineBreaksThatAreFine(t *testing.T) {
	out, _ := (&calcSkill{}).Execute(context.Background(), Input{"script": "const a = 1\nb + 1"})
	if strings.Contains(out.Stderr, "continuation") {
		t.Errorf("an ordinary error carries the line-break hint: %q", out.Stderr)
	}
}

// A calculation worth running a script for is bigger than one a person would do
// by hand. This is the size the limit is set against — a million rows of
// arithmetic must finish, or the tool only answers the questions that did not
// need it.
func TestCalcFinishesARealCalculation(t *testing.T) {
	if underRace {
		// The claim here is about how fast the shipped interpreter is, and -race
		// does not ship. Instrumented, three million iterations run past the
		// ceiling — which says nothing about the ceiling, only about the
		// instrumentation, so the test would be red for being right.
		t.Skip("timing claim about the uninstrumented interpreter; see race_off_test.go")
	}
	out := run(t, `let s = 0; for (let i = 1; i <= 3e6; i++) s += i; s`)
	if out.Content != "4500001500000" {
		t.Errorf("three million additions answered %q", out.Content)
	}
}

// "[object Object]" is the one rendering that says nothing, and it is goja's
// default for anything with a shape.
func TestCalcRendersAShapeAsJSON(t *testing.T) {
	if got := run(t, "({rate: 0.037, months: 12})").Content; !strings.Contains(got, `"rate":0.037`) {
		t.Errorf("calc rendered an object as %q", got)
	}
	if got := run(t, "[1, 2, 3].map(n => n * 2)").Content; got != "[2,4,6]" {
		t.Errorf("calc rendered an array as %q, want [2,4,6]", got)
	}
}

// The reason calc needs no permission: it is not that the dangerous things are
// blocked, it is that they were never in the room. If any of these ever answers
// something other than "undefined", calc has quietly become a second way to
// reach the machine — one the user never granted.
func TestCalcCannotReachTheMachine(t *testing.T) {
	for _, name := range []string{"require", "process", "fetch", "fs", "child_process", "XMLHttpRequest", "WebAssembly", "globalThis.eval === undefined ? 'x' : typeof require"} {
		script := "typeof " + name
		out, err := (&calcSkill{}).Execute(context.Background(), Input{"script": script})
		if err != nil {
			t.Fatalf("calc(%q) failed: %v", script, err)
		}
		if out.Content != "undefined" && !strings.Contains(out.Content, "undefined") {
			t.Errorf("%s is reachable from calc: %q", name, out.Content)
		}
	}
}

// A loop with no exit is a thing a model writes now and then. It must cost a
// second and an error, not the app.
func TestCalcStopsAScriptThatNeverFinishes(t *testing.T) {
	start := time.Now()
	out, err := (&calcSkill{}).Execute(context.Background(), Input{"script": "while (true) {}"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("an endless loop returned success: %q", out.Content)
	}
	if elapsed > calcTimeout+2*time.Second {
		t.Errorf("an endless loop ran for %s before it was stopped", elapsed)
	}
	if !strings.Contains(out.Stderr, "stopped") {
		t.Errorf("the error does not say the script was stopped: %q", out.Stderr)
	}
}

func TestCalcStopsAScriptThatEatsMemory(t *testing.T) {
	start := time.Now()
	_, err := (&calcSkill{}).Execute(context.Background(), Input{"script": "const rows = []; while (true) { rows.push('x'.repeat(1024)) }"})
	if err == nil {
		t.Fatal("a script allocating without end returned success")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("it ran for %s before it was stopped", elapsed)
	}
}

// A model that mistypes gets the engine's own message back, which names the
// line to fix — the most useful thing the tool can hand over.
func TestCalcHandsBackTheEngineError(t *testing.T) {
	out, err := (&calcSkill{}).Execute(context.Background(), Input{"script": "totl * 2"})
	if err == nil {
		t.Fatal("a reference to an undefined name returned success")
	}
	if !strings.Contains(out.Stderr, "totl") {
		t.Errorf("the error does not name what was wrong: %q", out.Stderr)
	}
}

func TestCalcSaysSoWhenAScriptProducedNothing(t *testing.T) {
	out := run(t, "const x = 2 + 2")
	if !strings.Contains(out.Content, "no value") {
		t.Errorf("a script with no result answered %q", out.Content)
	}
}

// The script is the audit trail. A number the user cannot check is the thing
// this tool was built to replace, so the line they read must be the arithmetic
// itself and not the word "calc".
func TestCalcShowsTheUserWhatItRan(t *testing.T) {
	const script = "128400 * 0.037"
	if got := run(t, script).Command; got != script {
		t.Errorf("the timeline would show %q instead of the script", got)
	}
}

func TestCalcRefusesAnEmptyScript(t *testing.T) {
	if _, err := (&calcSkill{}).ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("calc with no script returned success")
	}
}
