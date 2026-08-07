package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/Mike0165115321/Aetox/internal/model"
)

// calc is arithmetic that actually happened.
//
// A number a model writes out of its own head is a guess wearing the clothes of
// an answer: 3.7% of 128,400 arrives with the same confidence whether it is
// right or not, and the user has no way to tell which one they got. Every other
// tool here exists because the model cannot see something; this one exists
// because it cannot count.
//
// The machinery to run code was already here — write a file, call shell — and
// it is the wrong machinery for a sum. It depends on what the user happens to
// have installed, it spends the shell's right to touch the machine on a
// multiplication, and it costs three round trips and a leftover file. So the
// interpreter is compiled in: goja is an ECMAScript engine in pure Go with no
// filesystem, no network and no process of its own to speak of. What makes that
// safe is not a check we wrote — it is that a fresh goja.Runtime has nothing to
// reach with. `require`, `process`, `fetch`, `fs` are not blocked here; they
// were never there.
//
// That is also why calc asks for no permission. The user's list of rights is
// the only place rights come from, and a tool that cannot see a file, open a
// socket or start a process is not asking for any — putting it behind the same
// gate as shell would teach the user that the gate means nothing.
//
// What it costs the model: the script it ran is what the user reads in the
// timeline. A wrong answer is now a wrong line of arithmetic somebody can point
// at, rather than a number nobody can question.
type calcSkill struct{}

// A calculation finishes in microseconds. The limit is here for the script that
// does not finish at all — a loop with no exit is a thing a model writes now and
// then, and it must cost a few seconds and an error message rather than the app.
//
// Five, not one. Measured on this interpreter: ~7M loop iterations a second, so
// a second buys a million rows of arithmetic and nothing beyond it — a 360-month
// amortization lands in 1ms, but summing ten million numbers, sorting a few
// hundred thousand, or running a Monte Carlo of any size all hit the wall. The
// ceiling should be where a real calculation stops, not where a small one does.
const (
	calcTimeout = 5 * time.Second
	// goja has no memory ceiling of its own, and an interrupt only lands between
	// statements — long enough for `while (true) rows.push(x)` to take hundreds
	// of megabytes before the clock runs out. Heap growth is watched instead, and
	// crossing this stops the script the same way the timeout does.
	calcHeapBudget = 256 << 20
	calcHeapPoll   = 100 * time.Millisecond
	// Whatever a calculation has to say fits in this. A script that prints more
	// is dumping data, and the answer is already lost in it.
	calcMaxLines = 200
)

func (*calcSkill) Name() string { return "calc" }

func (*calcSkill) Description() string {
	return "คำนวณด้วยการรันสคริปต์ JavaScript สั้นๆ แทนการคิดเลขในหัว"
}

func (*calcSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"script": map[string]any{
				"type":        "string",
				"description": "JavaScript. The answer is the last expression, or whatever you return. print(x) adds a line above it. Math, Date, JSON and BigInt only — no files, no network, no libraries.",
			},
		},
		"required": []string{"script"},
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "calc",
			// Capability, not occasions: when a number should be computed
			// rather than recalled is judgment, and it is taught in
			// internal/prompt where the weighing belongs. A list of occasions
			// here would outrank it (defaults_test.go:
			// TestToolDescriptionsStateCapabilityNotWordTriggers) and cost the
			// tool block on every request besides.
			Description: "Work a number out instead of recalling it. Runs a short JavaScript program inside this app — " +
				"nothing to install, no reach to files or the network — and the user sees the script beside the result.",
			Parameters: payload,
		},
	}
}

func (c *calcSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	script := strings.TrimSpace(stringArg(args["script"]))
	if script == "" {
		start := time.Now()
		err := errors.New("calc needs a script")
		return newToolOutput("calc", "calc", err.Error(), start, false, err), err
	}
	return c.Execute(ctx, Input{"script": script})
}

func (*calcSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	script := strings.TrimSpace(stringArg(input["script"]))
	if script == "" {
		err := errors.New("calc needs a script")
		return newToolOutput("calc", "calc", err.Error(), start, false, err), err
	}

	content, err := runCalc(ctx, script)
	truncated := false
	if err == nil {
		content, truncated = limitLines(content, calcMaxLines)
	}
	// The command line is the script itself: this is the whole reason a computed
	// number beats a remembered one, and it is only true if the user can see it.
	return newToolOutput("calc", script, content, start, truncated, err), err
}

func runCalc(ctx context.Context, script string) (string, error) {
	vm := goja.New()

	var printed strings.Builder
	// The only thing put into the sandbox. A script's answer is its last
	// expression, which is one value — print is how a calculation that produces
	// a table, a schedule, or a set of steps says all of it.
	write := func(call goja.FunctionCall) goja.Value {
		parts := make([]string, 0, len(call.Arguments))
		for _, arg := range call.Arguments {
			parts = append(parts, display(vm, arg))
		}
		printed.WriteString(strings.Join(parts, " "))
		printed.WriteByte('\n')
		return goja.Undefined()
	}
	if err := vm.Set("print", write); err != nil {
		return "", err
	}
	// console.log is muscle memory: a model writing JavaScript reaches for it
	// without deciding to, and a bare goja has no console at all. Left out, the
	// first attempt at half the scripts ever written here dies on a
	// ReferenceError and costs a round trip to learn a house rule that buys
	// nothing. It writes where print writes.
	console := vm.NewObject()
	for _, name := range []string{"log", "info", "warn", "error", "debug"} {
		if err := console.Set(name, write); err != nil {
			return "", err
		}
	}
	if err := vm.Set("console", console); err != nil {
		return "", err
	}

	stop := watchCalc(ctx, vm)
	value, err := vm.RunString(script)
	if isIllegalReturn(err) {
		// `return` at the top level is a syntax error in a script and the most
		// natural thing to write when you are told to hand back an answer. It
		// is also the way out of a real trap: a script whose last line is
		// `({total: x})` reads as a call on the line above it, because the line
		// above ends in `)` and no semicolon is inserted between them — which
		// fails as "Cannot access a variable before initialization", an error
		// that says nothing about the missing semicolon. Wrapping in a function
		// makes `return {…}` legal, so the idiom that avoids the trap is the
		// one that works.
		value, err = vm.RunString("(function(){\n" + script + "\n})()")
	}
	stop()

	if err != nil {
		var interrupt *goja.InterruptedError
		if errors.As(err, &interrupt) {
			return "", fmt.Errorf("%v — the script was stopped", interrupt.Value())
		}
		// A goja error already reads like a JS engine's ("ReferenceError: x is
		// not defined at <eval>:1:1"), which is the most useful thing to hand
		// back: it names the line to fix. Except for one shape, where it names
		// the wrong thing entirely — see continuationHint.
		if hint := continuationHint(script); hint != "" {
			return "", fmt.Errorf("%w%s", err, hint)
		}
		return "", err
	}

	out := printed.String()
	if value != nil && !goja.IsUndefined(value) {
		if out != "" {
			out += "= "
		}
		out += display(vm, value)
	}
	if strings.TrimSpace(out) == "" {
		// A script that computed nothing and printed nothing ran fine and said
		// nothing, which the model must be told rather than left to assume.
		return "(the script produced no value — end it with the expression you want, or call print)", nil
	}
	return strings.TrimRight(out, "\n"), nil
}

// continuationHint names the trap that JavaScript's own error message hides.
//
// A script that ends with `({total: x})` — the ordinary way to hand back an
// object as the last expression — is not a new statement. The line above it
// ends in `)`, no semicolon is inserted between the two, and the parentheses
// become a call on whatever that line evaluated to. What comes back is
// "Cannot access a variable before initialization", which points at a variable
// that is fine and says nothing about the line break that caused it. Two of the
// first fifteen scripts written against this tool hit it.
//
// So the shape is detected in the source and described in words, rather than
// repaired behind the author's back: a script silently rewritten is a script
// nobody can reason about, and the fix — a semicolon, or `return` — belongs to
// whoever wrote it.
func continuationHint(script string) string {
	lines := strings.Split(script, "\n")
	previous := ""
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if previous != "" && (strings.HasPrefix(line, "(") || strings.HasPrefix(line, "[")) &&
			!strings.HasSuffix(previous, ";") && !strings.HasSuffix(previous, "{") &&
			!strings.HasSuffix(previous, "}") && !strings.HasSuffix(previous, ",") {
			return fmt.Sprintf(" — line %d starts with %q, which JavaScript reads as a continuation of the line above it, not as a new statement. End the line above with ';', or hand the value back with return.", i+1, line[:1])
		}
		previous = line
	}
	return ""
}

// isIllegalReturn recognises the one syntax error worth a second attempt: the
// script used `return`, which a bare script may not and a function body may.
func isIllegalReturn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Illegal return statement")
}

// watchCalc stops the script when it runs too long, eats too much, or the turn
// itself is cancelled. Returns the function that takes the watch back off.
func watchCalc(ctx context.Context, vm *goja.Runtime) func() {
	done := make(chan struct{})
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	go func() {
		deadline := time.NewTimer(calcTimeout)
		defer deadline.Stop()
		poll := time.NewTicker(calcHeapPoll)
		defer poll.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				vm.Interrupt("the turn was cancelled")
				return
			case <-deadline.C:
				vm.Interrupt(fmt.Sprintf("still running after %s", calcTimeout))
				return
			case <-poll.C:
				var now runtime.MemStats
				runtime.ReadMemStats(&now)
				if now.HeapAlloc > baseline.HeapAlloc+calcHeapBudget {
					vm.Interrupt("the script asked for too much memory")
					return
				}
			}
		}
	}()

	return func() {
		close(done)
		vm.ClearInterrupt()
	}
}

// display renders a value the way the answer should be read: a number as a
// number, anything with a shape as JSON. goja's own String() on an object gives
// "[object Object]", which is the one rendering that says nothing.
func display(vm *goja.Runtime, value goja.Value) string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return value.String()
	}
	switch exported := value.Export().(type) {
	case string:
		return exported
	case map[string]any, []any:
		if encoded, err := json.Marshal(exported); err == nil {
			return string(encoded)
		}
	}
	return value.String()
}
